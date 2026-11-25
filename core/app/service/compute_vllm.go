package service

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/google/uuid"
)

// Runtime backend "vllm": a single-host instance on a panel host (cluster
// node or this panel) is created as a deployment of that host's own
// AI → vLLM page. The compute scheduler only decides where and when; the
// container is the same compose project the vLLM page manages, so it shows
// up (and can be inspected / logged) there. Multi-host Ray and agentless
// (terminal) hosts keep using the script backend.

const computeVllmWait = time.Hour // image pull + compose up

var computeVllmNameRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// computeVllmName maps an instance name onto the vLLM deployment name rule
// ^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$.
func computeVllmName(inst string) string {
	n := "xpanel-" + strings.Trim(computeVllmNameRe.ReplaceAllString(inst, "-"), "-.")
	if len(n) > 63 {
		n = n[:63]
	}
	return strings.TrimRight(n, "-.")
}

type computeVllmModel struct {
	ID     uint   `json:"id"`
	RepoID string `json:"repoId"`
	Kind   string `json:"kind"`
	Dir    string `json:"dir"`
	Status string `json:"status"`
}

type computeVllmDeployment struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	ContainerName string `json:"containerName"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type computeVllmPage[T any] struct {
	Total int64 `json:"total"`
	Items []T   `json:"items"`
}

// computeVllmModelID finds the model registered at modelPath on the host,
// registering the directory in AI → vLLM → Models when it is missing.
func computeVllmModelID(h model.ComputeHost, modelPath string) (uint, error) {
	want := filepath.Clean(modelPath)
	find := func() (uint, error) {
		var page computeVllmPage[computeVllmModel]
		body := map[string]interface{}{"page": 1, "pageSize": 1000, "kind": "model"}
		if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/models/search", body, &page, 20*time.Second); err != nil {
			return 0, err
		}
		for _, m := range page.Items {
			if filepath.Clean(m.Dir) == want {
				if m.Status != "Success" {
					return 0, fmt.Errorf("model %s on %s is %s", m.RepoID, h.Name, m.Status)
				}
				return m.ID, nil
			}
		}
		return 0, nil
	}
	id, err := find()
	if err != nil || id > 0 {
		return id, err
	}
	body := map[string]interface{}{"repoId": path.Base(want), "kind": "model", "dir": want}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/models/import", body, nil, 2*time.Minute); err != nil {
		return 0, fmt.Errorf("register %s in AI → vLLM on %s: %w", want, h.Name, err)
	}
	id, err = find()
	if err == nil && id == 0 {
		err = fmt.Errorf("model %s not registered on %s", want, h.Name)
	}
	return id, err
}

func computeVllmFind(h model.ComputeHost, id uint, name string) (computeVllmDeployment, bool) {
	var page computeVllmPage[computeVllmDeployment]
	body := map[string]interface{}{"page": 1, "pageSize": 1000, "info": name}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/deployments/search", body, &page, 20*time.Second); err != nil {
		return computeVllmDeployment{}, false
	}
	for _, d := range page.Items {
		if (id > 0 && d.ID == id) || d.Name == name {
			return d, true
		}
	}
	return computeVllmDeployment{}, false
}

func computeVllmGPUType(rt model.ComputeRuntime) string {
	if rt.Device == computeKindNPU {
		return "ascend"
	}
	return "nvidia"
}

// computeVllmLaunch creates / updates the deployment and waits until the
// vLLM page reports it running. It returns the deployment id and the
// container name.
func computeVllmLaunch(in model.ComputeInstance, prof model.ComputeProfile, rt model.ComputeRuntime,
	h model.ComputeHost, key string) (uint, string, string, error) {
	if !h.IsPanel() {
		return 0, "", "", fmt.Errorf("runtime %s needs a panel host (cluster node or local), %s is reached over SSH", rt.Name, h.Name)
	}
	modelID, err := computeVllmModelID(h, prof.ModelPath)
	if err != nil {
		return 0, "", "", err
	}
	name := computeVllmName(in.Name)
	existing, ok := computeVllmFind(h, in.VllmDeploymentID, name)
	req := map[string]interface{}{
		"name": name, "external": false, "apiKey": key, "image": rt.Image, "port": prof.Port,
		"modelId": modelID, "servedName": prof.ServedName, "gpuType": computeVllmGPUType(rt), "gpuDevices": "all",
		"args": prof.ExtraArgs, "env": prof.Env, "taskID": uuid.New().String(),
	}
	if ok {
		req["id"] = existing.ID
	}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/deployments", req, nil, time.Minute); err != nil {
		return 0, "", "", fmt.Errorf("create AI → vLLM deployment %s on %s: %w", name, h.Name, err)
	}
	dep, ok := computeVllmFind(h, existing.ID, name)
	if !ok {
		return 0, "", "", fmt.Errorf("deployment %s not found on %s after create", name, h.Name)
	}
	deadline := time.Now().Add(computeVllmWait)
	for time.Now().Before(deadline) {
		cur, ok := computeVllmFind(h, dep.ID, name)
		if ok {
			switch cur.Status {
			case "Running":
				return cur.ID, cur.ContainerName, fmt.Sprintf("AI → vLLM deployment %s (#%d) running", name, cur.ID), nil
			case "Error":
				return cur.ID, cur.ContainerName, "", fmt.Errorf("AI → vLLM deployment %s failed: %s", name, cur.Message)
			}
		}
		time.Sleep(computeHealthPoll)
	}
	return dep.ID, dep.ContainerName, "", fmt.Errorf("AI → vLLM deployment %s not running after %s", name, computeVllmWait)
}

// computeVllmOperate stops (keeps the record) or removes the deployment.
func computeVllmOperate(in model.ComputeInstance, h model.ComputeHost, remove bool) (string, error) {
	dep, ok := computeVllmFind(h, in.VllmDeploymentID, computeVllmName(in.Name))
	if !ok {
		return "deployment not found, nothing to stop", nil
	}
	if remove {
		err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/deployments/del", map[string]uint{"id": dep.ID}, nil, 3*time.Minute)
		return "removed AI → vLLM deployment " + dep.Name, err
	}
	if dep.Status == "Stopped" {
		return "already stopped", nil
	}
	err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/deployments/operate",
		map[string]interface{}{"id": dep.ID, "operate": "stop"}, nil, 3*time.Minute)
	if err != nil {
		global.LOG.Debugf("compute vllm stop %s: %v", dep.Name, err)
	}
	return "stopped AI → vLLM deployment " + dep.Name, err
}
