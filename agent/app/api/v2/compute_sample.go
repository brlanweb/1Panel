package v2

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/gin-gonic/gin"
)

// Read-only sampling for the master panel's compute scheduler. The scripts
// are fixed here (no caller-supplied code), only read /proc, /sys,
// nvidia-smi / npu-smi and docker ps, and the endpoint is polled every few
// seconds, so it is deliberately not operation-logged.
//
// Keep both scripts byte-identical to core/app/service/compute_collect.go:
// the master parses the output with the same parsers it uses over SSH.

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

type computeSampleReq struct {
	Tier string `json:"tier" validate:"required,oneof=fast slow all"`
}

// @Tags AI
// @Summary Read-only host sample for compute scheduling (used by the master panel)
// @Accept json
// @Param request body computeSampleReq true "request"
// @Success 200 {object} computeExecRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/compute/sample [post]
func (b *BaseApi) ComputeSample(c *gin.Context) {
	var req computeSampleReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	script, timeout := computeFastScript, 20*time.Second
	switch req.Tier {
	case "slow":
		script, timeout = computeSlowScript, 30*time.Second
	case "all":
		script, timeout = computeSlowScript+computeFastScript, 30*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	cmd.Stdin = strings.NewReader(script)
	out := &computeLimitedBuffer{max: computeExecMaxOutput}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	res := computeExecRes{Output: out.buf.String()}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut, res.ExitCode = true, -1
	} else if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			helper.InternalServer(c, err)
			return
		}
		res.ExitCode = exitErr.ExitCode()
	}
	helper.SuccessWithData(c, res)
}
