package v2

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/gin-gonic/gin"
)

// Compute scheduling executor.
//
// The master panel's compute scheduler runs its collection / launch / stop
// scripts on panel-managed hosts through this endpoint instead of SSH, so a
// node that already runs the panel needs no extra credentials. It is a
// normal (API-key signed, operation-logged) agent API: the same trust level
// as the terminal and the cronjob shell scripts of the panel.

type computeExecReq struct {
	Script     string `json:"script" validate:"required"`
	TimeoutSec int    `json:"timeoutSec"`
}

type computeExecRes struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
	TimedOut bool   `json:"timedOut"`
}

const (
	computeExecMaxScript  = 256 << 10
	computeExecMaxOutput  = 4 << 20
	computeExecMaxTimeout = 30 * time.Minute
)

type computeLimitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *computeLimitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

// @Tags AI
// @Summary Run a compute scheduling script on this host (used by the master panel)
// @Accept json
// @Param request body computeExecReq true "request"
// @Success 200 {object} computeExecRes
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/compute/exec [post]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"算力调度执行脚本","formatEN":"compute scheduling runs a script"}
func (b *BaseApi) ComputeExec(c *gin.Context) {
	var req computeExecReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if len(req.Script) > computeExecMaxScript {
		helper.BadRequest(c, errors.New("script too large"))
		return
	}
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = time.Minute
	}
	if timeout > computeExecMaxTimeout {
		timeout = computeExecMaxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	cmd.Stdin = strings.NewReader(req.Script)
	out := &computeLimitedBuffer{max: computeExecMaxOutput}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	res := computeExecRes{Output: out.buf.String()}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
	} else if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			helper.InternalServer(c, err)
			return
		}
	}
	helper.SuccessWithData(c, res)
}
