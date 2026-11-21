package service

import (
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"gopkg.in/yaml.v3"
)

func TestParseVllmArgs(t *testing.T) {
	got, err := parseVllmArgs("# comment\n--max-num-seqs 64\n\n--enable-prefix-caching\n--foo=bar")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--max-num-seqs", "64", "--enable-prefix-caching", "--foo=bar"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := parseVllmArgs("rm -rf /"); err == nil {
		t.Fatal("expected error for non-flag line")
	}
}

func TestParseVllmEnv(t *testing.T) {
	env, err := parseVllmEnv("A=1\n# c\nB = two words\n")
	if err != nil || env["A"] != "1" || env["B"] != "two words" {
		t.Fatalf("unexpected %v %v", env, err)
	}
	if _, err := parseVllmEnv("1BAD=x"); err == nil {
		t.Fatal("expected error for bad key")
	}
	if _, err := parseVllmEnv("NOEQUALS"); err == nil {
		t.Fatal("expected error for missing =")
	}
}

func TestBuildVllmCompose(t *testing.T) {
	d := &model.VllmDeployment{
		Name: "qwen", Image: "vllm/vllm-openai:v0.11.0", ContainerName: "1panel-vllm-qwen",
		HostIP: "127.0.0.1", Port: 8001, ModelPath: "/opt/1panel/ai/vllm/models/Qwen/Qwen3-8B",
		ServedName: "Qwen3-8B", GPUType: "nvidia", GPUDevices: "0,1", TensorParallel: 2,
		MaxModelLen: 8192, GPUMemoryUtil: "0.85", APIKey: "sk-test", Args: "--enable-prefix-caching",
		Env: "VLLM_LOGGING_LEVEL=INFO",
	}
	out, err := buildVllmCompose(d)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			Image   string            `yaml:"image"`
			Ports   []string          `yaml:"ports"`
			Volumes []string          `yaml:"volumes"`
			Command []string          `yaml:"command"`
			Env     map[string]string `yaml:"environment"`
			Deploy  struct {
				Resources struct {
					Reservations struct {
						Devices []struct {
							Driver    string   `yaml:"driver"`
							DeviceIDs []string `yaml:"device_ids"`
						} `yaml:"devices"`
					} `yaml:"reservations"`
				} `yaml:"resources"`
			} `yaml:"deploy"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	svc, ok := parsed.Services["qwen"]
	if !ok {
		t.Fatalf("service missing:\n%s", out)
	}
	if svc.Ports[0] != "127.0.0.1:8001:8000" {
		t.Fatalf("port %v", svc.Ports)
	}
	if !strings.HasSuffix(svc.Volumes[0], ":/models/model:ro") {
		t.Fatalf("volume %v", svc.Volumes)
	}
	cmd := strings.Join(svc.Command, " ")
	for _, want := range []string{"--tensor-parallel-size 2", "--max-model-len 8192", "--gpu-memory-utilization 0.85", "--api-key sk-test", "--served-model-name Qwen3-8B", "--enable-prefix-caching"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("command %q missing %q", cmd, want)
		}
	}
	if svc.Env["VLLM_LOGGING_LEVEL"] != "INFO" {
		t.Fatalf("env %v", svc.Env)
	}
	devs := svc.Deploy.Resources.Reservations.Devices
	if len(devs) != 1 || devs[0].Driver != "nvidia" || strings.Join(devs[0].DeviceIDs, ",") != "0,1" {
		t.Fatalf("devices %+v", devs)
	}

	d.GPUType = "cpu"
	out, _ = buildVllmCompose(d)
	if strings.Contains(out, "nvidia") {
		t.Fatal("cpu deployment must not reserve GPUs")
	}
}

func TestVllmEndpointAndMask(t *testing.T) {
	if e := vllmEndpoint(&model.VllmDeployment{Port: 8000, HostIP: "0.0.0.0"}); e != "http://127.0.0.1:8000/v1" {
		t.Fatal(e)
	}
	if e := vllmEndpoint(&model.VllmDeployment{External: true, BaseURL: "http://x/v1"}); e != "http://x/v1" {
		t.Fatal(e)
	}
	if m := maskSecret("sk-1234567890"); m != "sk-1****7890" {
		t.Fatal(m)
	}
}
