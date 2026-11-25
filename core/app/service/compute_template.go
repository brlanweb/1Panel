package service

import (
	"bytes"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/1Panel-dev/1Panel/core/app/model"
)

// Runtime templates are Go text/template shell scripts rendered once per
// host of an instance. Variables:
//
//   .Name .ContainerName .ServedName .ModelPath .ModelDir .Port .Image
//   .ExtraArgs (raw, appended verbatim) .APIKey .Env (map)
//   .NodeCount .Rank .IsHead .HostAddr .HeadAddr
//   .FabricIface .FabricIP .HeadFabricIP .RdmaDev   (empty without fabric)
//
// Function q shell-quotes a value; env renders "-e K=V" pairs.

type computeTplVars struct {
	Name, ContainerName, ServedName, ModelPath, ModelDir string
	Port                                                 int
	Image, ExtraArgs, APIKey                             string
	Env                                                  map[string]string
	NodeCount, Rank                                      int
	IsHead                                               bool
	HostAddr, HeadAddr                                   string
	FabricIface, FabricIP, HeadFabricIP, RdmaDev         string
}

var computeTplFuncs = template.FuncMap{
	"q": computeShellQuote,
	"env": func(m map[string]string) string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			b.WriteString("-e ")
			b.WriteString(computeShellQuote(k + "=" + m[k]))
			b.WriteByte(' ')
		}
		return b.String()
	},
}

var computeSafeWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func computeShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if computeSafeWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func computeRender(name, tpl string, v computeTplVars) (string, error) {
	t, err := template.New(name).Funcs(computeTplFuncs).Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func computeParseEnv(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) != "" {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

func computeContainerName(inst string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)
	return "xpanel-ai-" + strings.Trim(re.ReplaceAllString(inst, "-"), "-")
}

// computeBuildVars renders the per-host variables of an instance.
// hostIDs[0] is the head. fabric maps hostID -> (iface, ip, rdma) on the
// link towards the other members (first match).
func computeBuildVars(inst model.ComputeInstance, prof model.ComputeProfile, rt model.ComputeRuntime, hostIDs []uint,
	hosts map[uint]model.ComputeHost, apiKey string) ([]computeTplVars, error) {
	if len(hostIDs) == 0 {
		return nil, fmt.Errorf("no hosts")
	}
	fabric := computeFabricOf(hostIDs)
	head := hosts[hostIDs[0]]
	env := computeParseEnv(prof.Env)
	res := make([]computeTplVars, 0, len(hostIDs))
	for rank, id := range hostIDs {
		h, ok := hosts[id]
		if !ok {
			return nil, fmt.Errorf("host #%d not found", id)
		}
		v := computeTplVars{
			Name: inst.Name, ContainerName: inst.ContainerName, ServedName: prof.ServedName,
			ModelPath: prof.ModelPath, ModelDir: path.Dir(strings.TrimRight(prof.ModelPath, "/")),
			Port: inst.Port, Image: rt.Image, ExtraArgs: prof.ExtraArgs, APIKey: apiKey, Env: env,
			NodeCount: len(hostIDs), Rank: rank, IsHead: rank == 0, HostAddr: h.Addr, HeadAddr: head.Addr,
		}
		if f, ok := fabric[id]; ok {
			v.FabricIface, v.FabricIP, v.RdmaDev = f.iface, f.ip, f.rdma
		}
		if f, ok := fabric[hostIDs[0]]; ok {
			v.HeadFabricIP = f.ip
		}
		if v.HeadFabricIP == "" {
			v.HeadFabricIP = head.Addr
		}
		if v.FabricIP == "" {
			v.FabricIP = h.Addr
		}
		res = append(res, v)
	}
	return res, nil
}

type computeFabricEnd struct{ iface, ip, rdma string }

func computeFabricOf(hostIDs []uint) map[uint]computeFabricEnd {
	member := map[uint]bool{}
	for _, id := range hostIDs {
		member[id] = true
	}
	links, _ := computeLinks()
	res := map[uint]computeFabricEnd{}
	for _, l := range links {
		if !member[l.HostAID] || !member[l.HostBID] || l.State != "ACTIVE" {
			continue
		}
		if _, ok := res[l.HostAID]; !ok {
			res[l.HostAID] = computeFabricEnd{l.IfaceA, l.IPA, l.RdmaA}
		}
		if _, ok := res[l.HostBID]; !ok {
			res[l.HostBID] = computeFabricEnd{l.IfaceB, l.IPB, l.RdmaB}
		}
	}
	return res
}

// ---- built-in runtimes ----------------------------------------------------

const computeStopDocker = `docker rm -f {{q .ContainerName}} >/dev/null 2>&1 || true
`

const computeVllmSingle = `set -e
docker rm -f {{q .ContainerName}} >/dev/null 2>&1 || true
docker run -d --name {{q .ContainerName}} --restart unless-stopped \
  --gpus all --ipc host --network host --shm-size 16g \
  -v {{q .ModelDir}}:{{q .ModelDir}}:ro {{env .Env}}\
  {{q .Image}} \
  vllm serve {{q .ModelPath}} --served-model-name {{q .ServedName}} \
  --host 0.0.0.0 --port {{.Port}}{{if .APIKey}} --api-key {{q .APIKey}}{{end}} {{.ExtraArgs}}
`

const computeVllmRay = `set -e
docker rm -f {{q .ContainerName}} >/dev/null 2>&1 || true
{{- if .IsHead}}
docker run -d --name {{q .ContainerName}} --restart unless-stopped \
  --gpus all --ipc host --network host --shm-size 16g --privileged \
  -v {{q .ModelDir}}:{{q .ModelDir}}:ro -v /dev/infiniband:/dev/infiniband \
  -e VLLM_HOST_IP={{q .FabricIP}}{{if .FabricIface}} -e NCCL_SOCKET_IFNAME={{q .FabricIface}} -e GLOO_SOCKET_IFNAME={{q .FabricIface}}{{end}}{{if .RdmaDev}} -e NCCL_IB_HCA={{q .RdmaDev}}{{end}} {{env .Env}}\
  --entrypoint bash {{q .Image}} -c {{q (printf "ray start --head --port=6379 --node-ip-address=%s && python3 -c 'import ray,time\nray.init(address=\"auto\")\nfor _ in range(720):\n    if len([n for n in ray.nodes() if n[\"Alive\"]]) >= %d: break\n    time.sleep(5)' && exec vllm serve %s --served-model-name %s --host 0.0.0.0 --port %d --tensor-parallel-size %d --distributed-executor-backend ray %s %s" .FabricIP .NodeCount (q .ModelPath) (q .ServedName) .Port .NodeCount (or (and .APIKey (printf "--api-key %s" (q .APIKey))) "") .ExtraArgs)}}
{{- else}}
docker run -d --name {{q .ContainerName}} --restart unless-stopped \
  --gpus all --ipc host --network host --shm-size 16g --privileged \
  -v {{q .ModelDir}}:{{q .ModelDir}}:ro -v /dev/infiniband:/dev/infiniband \
  -e VLLM_HOST_IP={{q .FabricIP}}{{if .FabricIface}} -e NCCL_SOCKET_IFNAME={{q .FabricIface}} -e GLOO_SOCKET_IFNAME={{q .FabricIface}}{{end}}{{if .RdmaDev}} -e NCCL_IB_HCA={{q .RdmaDev}}{{end}} {{env .Env}}\
  --entrypoint bash {{q .Image}} -c {{q (printf "ray start --block --address=%s:6379 --node-ip-address=%s" .HeadFabricIP .FabricIP)}}
{{- end}}
`

func computeBuiltinRuntimes() []model.ComputeRuntime {
	return []model.ComputeRuntime{
		{Name: "vllm-single", Engine: "vllm", Image: "nvcr.io/nvidia/vllm:25.09-py3", MultiNode: false,
			LaunchTemplate: computeVllmSingle, StopTemplate: computeStopDocker, HealthPath: "/health", MetricsPath: "/metrics",
			Builtin: true, Remark: "vLLM single host (docker)"},
		{Name: "vllm-ray-multinode", Engine: "vllm", Image: "nvcr.io/nvidia/vllm:25.09-py3", MultiNode: true,
			LaunchTemplate: computeVllmRay, StopTemplate: computeStopDocker, HealthPath: "/health", MetricsPath: "/metrics",
			Builtin: true, Remark: "vLLM over Ray across a fabric domain (NCCL over RoCE)"},
	}
}
