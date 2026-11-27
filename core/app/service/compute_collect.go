package service

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

// Agentless collection scripts and their parsers.
//
// Each tier is a single POSIX sh script executed in one SSH session; the
// output is split into "@@SECTION" blocks. Only /proc, /sys and
// nvidia-smi are read, nothing is written to the host. The fast tier runs
// every 15s (5s while somebody watches the page), the slow tier every
// 5 minutes.

const computeFastScript = `export LC_ALL=C
echo '@@STAT'; head -1 /proc/stat; cat /proc/loadavg; nproc 2>/dev/null
echo '@@MEM'; grep -E '^(MemTotal|MemAvailable|SwapTotal|SwapFree):' /proc/meminfo
echo '@@GPU'
if command -v nvidia-smi >/dev/null 2>&1; then timeout 8 nvidia-smi --query-gpu=index,name,utilization.gpu,temperature.gpu,power.draw,memory.used,memory.total --format=csv,noheader,nounits 2>/dev/null; fi
echo '@@NPU'
S=$(command -v npu-smi 2>/dev/null || ls /usr/local/sbin/npu-smi /usr/local/bin/npu-smi 2>/dev/null | head -1)
if [ -n "$S" ]; then timeout 8 "$S" info 2>/dev/null; fi
echo '@@NET'
for d in /sys/class/net/*; do n=${d##*/}
case $n in lo|veth*|docker*|br-*|virbr*|cali*|flannel*|cni*|tun*|tap*|vxlan*|kube*) continue;; esac
echo "$n $(cat $d/operstate 2>/dev/null || echo unknown) $(cat $d/speed 2>/dev/null || echo -1) $(cat $d/statistics/rx_bytes 2>/dev/null || echo 0) $(cat $d/statistics/tx_bytes 2>/dev/null || echo 0)"
done
echo '@@RDMA'
for p in /sys/class/infiniband/*/ports/*; do [ -d "$p" ] || continue
dev=$(echo "$p" | cut -d/ -f5); port=${p##*/}; c=$p/counters
nd=$(ls /sys/class/infiniband/$dev/device/net 2>/dev/null | head -1)
st=$(awk '{print $2}' $p/state 2>/dev/null); rt=$(awk '{print $1}' $p/rate 2>/dev/null)
v=""; for k in port_rcv_data port_xmit_data port_rcv_errors symbol_error link_downed port_xmit_discards link_error_recovery; do v="$v $(cat $c/$k 2>/dev/null || echo 0)"; done
echo "$dev $port ${st:-UNKNOWN} ${rt:-0} ${nd:--}$v"
done
echo '@@DISK'; df -P / 2>/dev/null | tail -1
`

const computeSlowScript = `export LC_ALL=C
echo '@@META'; hostname; uname -m; (. /etc/os-release 2>/dev/null; echo "${PRETTY_NAME:-unknown}"); (head -1 /proc/driver/nvidia/version 2>/dev/null || grep -m1 '^Version=' /usr/local/Ascend/driver/version.info 2>/dev/null || echo -); nproc 2>/dev/null || echo 0
echo '@@ADDR'; ip -o -4 addr show 2>/dev/null | awk '{print $2, $4}'
echo '@@ROUTE'; ip -o -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<NF;i++) if($i=="src") print $(i+1)}'
echo '@@ARP'; tail -n +2 /proc/net/arp 2>/dev/null | awk '$3=="0x2"{print $1, $4, $6}'
echo '@@CTR'; docker ps --format '{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}' 2>/dev/null
echo '@@PROC'; ps -eo pid=,args= 2>/dev/null | grep -E '[v]llm|[s]glang|[t]ensorfold|[l]lama-server|[o]llama serve|[l]mdeploy|[t]rtllm-serve|[t]ext-generation-launcher|[t]ritonserver|[f]unasr' | grep -v -E 'grep|sh -s' | cut -c1-800
`

func computeSections(out string) map[string][]string {
	res := map[string][]string{}
	cur := ""
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(line, "@@") {
			cur = strings.TrimSpace(strings.TrimPrefix(line, "@@"))
			if _, ok := res[cur]; !ok {
				res[cur] = nil
			}
			continue
		}
		if cur == "" || strings.TrimSpace(line) == "" {
			continue
		}
		res[cur] = append(res[cur], line)
	}
	return res
}

func cAtoF(s string) float64 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func cAtoU(s string) uint64 {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// ---- fast tier -----------------------------------------------------------

type computeCPUStat struct {
	Total uint64
	Idle  uint64
}

type computeNicRaw struct {
	Name      string
	State     string
	SpeedMbps int64
	Rx, Tx    uint64
}

type computeRdmaRaw struct {
	Dev, Port, State, NetDev string
	RateGbps                 int
	RcvData, XmitData        uint64 // in 4-byte words (IB spec)
	RcvErrors, SymbolErrors  uint64
	LinkDowned, XmitDiscards uint64
	LinkErrRecovery          uint64
}

// computeGPURaw is one accelerator: an NVIDIA GPU or an Ascend NPU chip.
type computeGPURaw struct {
	Kind     string // gpu / npu
	Index    int
	Card     int // NPU card id (npu-smi "NPU" column); GPU index otherwise
	Chip     int // NPU chip id on the card
	Name     string
	Health   string // NPU health (OK / Warning / Alarm / Critical)
	Util     float64
	Temp     float64
	Power    float64
	MemUsed  float64 // MiB; 0 when N/A (unified memory)
	MemTotal float64
}

type computeFastRaw struct {
	CPU          computeCPUStat
	Load1        float64
	Cores        int
	MemTotal     uint64 // bytes
	MemAvailable uint64
	SwapTotal    uint64
	SwapFree     uint64
	GPUs         []computeGPURaw
	Nics         []computeNicRaw
	Rdma         []computeRdmaRaw
	DiskTotal    uint64
	DiskUsed     uint64
}

func parseComputeFast(out string) computeFastRaw {
	sec := computeSections(out)
	var r computeFastRaw
	for i, line := range sec["STAT"] {
		f := strings.Fields(line)
		switch {
		case i == 0 && len(f) > 4 && f[0] == "cpu":
			var total uint64
			for _, v := range f[1:] {
				total += cAtoU(v)
			}
			idle := cAtoU(f[4])
			if len(f) > 5 {
				idle += cAtoU(f[5]) // iowait
			}
			r.CPU = computeCPUStat{Total: total, Idle: idle}
		case i == 1 && len(f) > 0:
			r.Load1 = cAtoF(f[0])
		case i == 2 && len(f) == 1:
			r.Cores = int(cAtoU(f[0]))
		}
	}
	for _, line := range sec["MEM"] {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v := cAtoU(f[1]) * 1024
		switch strings.TrimSuffix(f[0], ":") {
		case "MemTotal":
			r.MemTotal = v
		case "MemAvailable":
			r.MemAvailable = v
		case "SwapTotal":
			r.SwapTotal = v
		case "SwapFree":
			r.SwapFree = v
		}
	}
	for _, line := range sec["GPU"] {
		f := strings.Split(line, ",")
		if len(f) < 7 {
			continue
		}
		idx := int(cAtoU(f[0]))
		r.GPUs = append(r.GPUs, computeGPURaw{
			Kind:     "gpu",
			Index:    idx,
			Card:     idx,
			Name:     strings.TrimSpace(f[1]),
			Util:     cAtoF(f[2]),
			Temp:     cAtoF(f[3]),
			Power:    cAtoF(f[4]),
			MemUsed:  cAtoF(f[5]),
			MemTotal: cAtoF(f[6]),
		})
	}
	r.GPUs = append(r.GPUs, parseComputeNPU(sec["NPU"])...)
	for _, line := range sec["NET"] {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		speed, _ := strconv.ParseInt(f[2], 10, 64)
		r.Nics = append(r.Nics, computeNicRaw{Name: f[0], State: f[1], SpeedMbps: speed, Rx: cAtoU(f[3]), Tx: cAtoU(f[4])})
	}
	for _, line := range sec["RDMA"] {
		f := strings.Fields(line)
		if len(f) < 12 {
			continue
		}
		nd := f[4]
		if nd == "-" {
			nd = ""
		}
		r.Rdma = append(r.Rdma, computeRdmaRaw{
			Dev: f[0], Port: f[1], State: f[2], RateGbps: int(cAtoF(f[3])), NetDev: nd,
			RcvData: cAtoU(f[5]), XmitData: cAtoU(f[6]), RcvErrors: cAtoU(f[7]), SymbolErrors: cAtoU(f[8]),
			LinkDowned: cAtoU(f[9]), XmitDiscards: cAtoU(f[10]), LinkErrRecovery: cAtoU(f[11]),
		})
	}
	if lines := sec["DISK"]; len(lines) > 0 {
		f := strings.Fields(lines[0])
		if len(f) >= 4 {
			r.DiskTotal = cAtoU(f[1]) * 1024
			r.DiskUsed = cAtoU(f[2]) * 1024
		}
	}
	return r
}

// ---- slow tier -----------------------------------------------------------

type computeProcRaw struct {
	Pid        int    `json:"pid"`
	Engine     string `json:"engine"`
	Port       int    `json:"port"`
	Model      string `json:"model"`
	ServedName string `json:"servedName"`
	Cmd        string `json:"cmd"`
	// Worker is a non-head rank of a multi-host group (vLLM --headless or
	// --node-rank > 0, SGLang --node-rank > 0). It carries the same --port
	// as the head but serves no API, so it is never an instance by itself.
	Worker bool `json:"worker"`
}

type computeNeighbour struct {
	IP    string `json:"ip"`
	Mac   string `json:"mac"`
	Iface string `json:"iface"`
}

type computeContainer struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

type computeSlowRaw struct {
	Hostname   string
	Arch       string
	OS         string
	Driver     string
	Cores      int
	DefaultIP  string            // source address of the default route
	Addrs      map[string]string // iface -> ipv4
	Neighbours []computeNeighbour
	Containers []computeContainer
	Procs      []computeProcRaw
}

// NVIDIA: "... 580.159.03 ..."; Ascend version.info: "Version=24.1.rc2".
var computeDriverRe = regexp.MustCompile(`\b(\d{3}\.\d+(?:\.\d+)?)\b|Version=(\S+)`)

func parseComputeSlow(out string) computeSlowRaw {
	sec := computeSections(out)
	r := computeSlowRaw{Addrs: map[string]string{}}
	meta := sec["META"]
	get := func(i int) string {
		if i < len(meta) {
			return strings.TrimSpace(meta[i])
		}
		return ""
	}
	r.Hostname, r.Arch, r.OS = get(0), get(1), get(2)
	if m := computeDriverRe.FindStringSubmatch(get(3)); len(m) > 2 {
		r.Driver = m[1]
		if r.Driver == "" {
			r.Driver = m[2]
		}
	}
	r.Cores = int(cAtoU(get(4)))
	for _, line := range sec["ADDR"] {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		ip := strings.SplitN(f[1], "/", 2)[0]
		if _, ok := r.Addrs[f[0]]; !ok {
			r.Addrs[f[0]] = ip
		}
	}
	for _, line := range sec["ROUTE"] {
		if ip := strings.TrimSpace(line); ip != "" && r.DefaultIP == "" {
			r.DefaultIP = ip
		}
	}
	for _, line := range sec["ARP"] {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		r.Neighbours = append(r.Neighbours, computeNeighbour{IP: f[0], Mac: f[1], Iface: f[2]})
	}
	for _, line := range sec["CTR"] {
		f := strings.SplitN(line, "|", 4)
		if len(f) < 3 {
			continue
		}
		c := computeContainer{Name: f[0], Image: f[1], Status: f[2]}
		if len(f) > 3 {
			c.Ports = strings.TrimSpace(f[3])
		}
		r.Containers = append(r.Containers, c)
	}
	seen := map[int]bool{}
	for _, line := range sec["PROC"] {
		if p, ok := parseComputeProc(line); ok {
			// Keep one entry per serving port (vLLM forks workers with the
			// same command line).
			if p.Port > 0 && seen[p.Port] {
				continue
			}
			if p.Port > 0 {
				seen[p.Port] = true
			}
			r.Procs = append(r.Procs, p)
		}
	}
	// Engines whose command line carries no engine name (FunASR runs as
	// "python /app/server.py") are recognised by their container instead,
	// one entry per published host port.
	for _, c := range r.Containers {
		for _, p := range computeContainerProcs(c) {
			if p.Port > 0 && seen[p.Port] {
				continue
			}
			if p.Port > 0 {
				seen[p.Port] = true
			}
			r.Procs = append(r.Procs, p)
		}
	}
	return r
}

// computeContainerEngines maps a keyword of a container name / image to an
// engine whose process cannot be recognised from ps.
var computeContainerEngines = []struct{ Key, Engine string }{
	{"funasr", "funasr"},
	{"sensevoice", "funasr"},
	{"paraformer", "funasr"},
	{"text-embeddings-inference", "tei"},
	{"xinference", "xinference"},
}

// "10.15.9.123:8006->8000/tcp, :::8006->8000/tcp"
var computePortMapRe = regexp.MustCompile(`:(\d+)->(\d+)/tcp`)

func computeContainerProcs(c computeContainer) []computeProcRaw {
	if !strings.HasPrefix(strings.ToLower(c.Status), "up") {
		return nil
	}
	hay := strings.ToLower(c.Name + " " + c.Image)
	engine := ""
	for _, e := range computeContainerEngines {
		if strings.Contains(hay, e.Key) {
			engine = e.Engine
			break
		}
	}
	if engine == "" {
		return nil
	}
	base := computeProcRaw{Engine: engine, Model: c.Image, ServedName: c.Name, Cmd: "docker: " + c.Name + " (" + c.Image + ")"}
	var res []computeProcRaw
	seen := map[int]bool{}
	for _, m := range computePortMapRe.FindAllStringSubmatch(c.Ports, -1) {
		port := int(cAtoU(m[1]))
		if port == 0 || seen[port] {
			continue
		}
		seen[port] = true
		p := base
		p.Port = port
		res = append(res, p)
	}
	if len(res) == 0 {
		// host network or nothing published: still show the engine.
		res = append(res, base)
	}
	return res
}

func parseComputeProc(line string) (computeProcRaw, bool) {
	f := strings.Fields(line)
	if len(f) < 2 {
		return computeProcRaw{}, false
	}
	p := computeProcRaw{Pid: int(cAtoU(f[0])), Cmd: strings.Join(f[1:], " ")}
	args := f[1:]
	isServe := false
	// Engines other than vLLM / SGLang are recognised only by the program
	// itself (never by a path or argument), so wrapper scripts such as
	// "bash -c cd ~/funasr-replicas && ./bench.sh" are not taken for one.
	prog := computeProgram(args)
	for _, e := range computeProgEngines {
		if strings.Contains(prog, e.Key) {
			p.Engine = e.Engine
			isServe = isServe || e.AlwaysServe
			break
		}
	}
	for i, a := range args {
		if p.Engine == "" || p.Engine == "vllm" || p.Engine == "sglang" {
			switch {
			case strings.Contains(a, "sglang"):
				p.Engine = "sglang"
			case strings.HasSuffix(a, "vllm") || strings.Contains(a, "vllm.entrypoints"):
				p.Engine = "vllm"
			}
		}
		if a == "serve" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			isServe = true
			if args[i+1] != "api_server" {
				p.Model = args[i+1]
			} else if i+2 < len(args) && !strings.HasPrefix(args[i+2], "-") {
				p.Model = args[i+2] // lmdeploy serve api_server <model>
			}
		}
		if a == "serve" {
			isServe = true // "ollama serve", "tensorfold-native serve --model ..."
		}
		if strings.Contains(a, "api_server") || strings.Contains(a, "launch_server") {
			isServe = true
		}
		key, val := a, ""
		if k, v, ok := strings.Cut(a, "="); ok && strings.HasPrefix(a, "-") {
			key, val = k, v
		} else if i+1 < len(args) {
			val = args[i+1]
		}
		switch key {
		case "--port", "--server-port", "--http-port":
			p.Port = int(cAtoU(val))
		case "--model", "--model-path", "--model-id":
			if !strings.HasPrefix(val, "-") {
				p.Model = val
			}
		case "-m":
			// llama-server -m model.gguf (for python "-m" names a module)
			if p.Engine == "llama.cpp" {
				p.Model = val
			}
		case "--served-model-name", "--alias", "--name", "--model-name":
			p.ServedName = val
		case "--headless":
			p.Worker = true
		case "--node-rank":
			if rank := cAtoU(val); rank > 0 {
				p.Worker = true
			}
		}
	}
	if p.Engine == "" || !isServe {
		return p, false
	}
	if p.Port == 0 {
		p.Port = computeEngineDefaultPort(p.Engine)
	}
	if p.ServedName == "" {
		p.ServedName = p.Model
	}
	return p, true
}

// computeProgEngines: keyword of the program name → engine. AlwaysServe
// marks programs that are servers by themselves (no "serve" sub-command).
var computeProgEngines = []struct {
	Key, Engine string
	AlwaysServe bool
}{
	{"tensorfold", "tensorfold", false},
	{"llama-server", "llama.cpp", true},
	{"ollama", "ollama", false},
	{"lmdeploy", "lmdeploy", false},
	{"trtllm-serve", "trtllm", true},
	{"text-generation-launcher", "tgi", true},
	{"tritonserver", "triton", true},
	{"funasr", "funasr", true},
}

func computeEngineDefaultPort(engine string) int {
	switch engine {
	case "sglang":
		return 30000
	case "llama.cpp":
		return 8080
	case "ollama":
		return 11434
	case "lmdeploy":
		return 23333
	case "tgi":
		return 80
	case "funasr":
		return 10095
	}
	return 8000
}

// computeProgram returns the base name of the program a command line runs:
// the executable itself, or for an interpreter the module (-m) / script.
func computeProgram(args []string) string {
	if len(args) == 0 {
		return ""
	}
	base := func(s string) string {
		if i := strings.LastIndex(s, "/"); i >= 0 {
			s = s[i+1:]
		}
		return strings.ToLower(s)
	}
	exe := base(args[0])
	if !strings.HasPrefix(exe, "python") {
		return exe
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "-m" && i+1 < len(args) {
			return base(args[i+1])
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		return base(a)
	}
	return exe
}
