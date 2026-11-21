package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/common"
	"github.com/1Panel-dev/1Panel/agent/utils/compose"
	"github.com/1Panel-dev/1Panel/agent/utils/docker"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

// Proxy nodes (Settings → Proxy node).
//
// The user pastes a Clash-format subscription URL. The panel downloads it,
// keeps only its `proxies`, and runs them in a mihomo (Clash Meta) container
// with one "select" group, so a single node can be chosen from a dropdown.
// The container exposes an authenticated mixed (HTTP + SOCKS5) proxy:
//
//   - on the host at 127.0.0.1:<port> (for host processes);
//   - inside the docker network 1panel-network as
//     1panel-proxy-node:7890 (for containers such as vLLM).
//
// After every save all nodes are tested in the background — latency first
// (mihomo /proxies/{name}/delay), then download speed through a per-node
// listener — without blocking the UI; the page polls the status.
const (
	proxyNodeSettingURL      = "ProxyNodeSubscriptionURL"
	proxyNodeSettingImage    = "ProxyNodeImage"
	proxyNodeSettingPort     = "ProxyNodePort"
	proxyNodeSettingSelected = "ProxyNodeSelected"
	proxyNodeSettingSecret   = "ProxyNodeSecret"
	proxyNodeSettingUser     = "ProxyNodeUser"
	proxyNodeSettingPass     = "ProxyNodePassword"
	proxyNodeSettingResults  = "ProxyNodeTestResults"
	proxyNodeSettingUpdated  = "ProxyNodeUpdatedAt"

	proxyNodeDefaultImage = "metacubex/mihomo:latest"
	proxyNodeDefaultPort  = 7897
	proxyNodeContainer    = "1panel-proxy-node"
	proxyNodeInnerPort    = 7890
	proxyNodeCtrlPort     = 9090
	proxyNodeGroup        = "PROXY"
	proxyNodeTestBase     = 20000
	proxyNodeDelayURL     = "https://www.gstatic.com/generate_204"
	proxyNodeSpeedURL     = "https://speed.cloudflare.com/__down?bytes=10000000"
	proxyNodeMaxNodes     = 2000
)

type ProxyNodeService struct{}

type IProxyNodeService interface {
	Status() dto.ProxyNodeStatus
	Save(req dto.ProxyNodeSave) error
	Refresh() error
	Select(name string) error
	Test() error
	Operate(operate string) error
	ProxyURLForContainer() (string, error)
}

func NewIProxyNodeService() IProxyNodeService {
	return &ProxyNodeService{}
}

type proxyNodeRuntime struct {
	mu        sync.Mutex
	nodes     []dto.ProxyNodeItem
	testing   bool
	testDone  int
	testTotal int
	testedAt  string
	message   string
	loaded    bool
}

var proxyNodeState = &proxyNodeRuntime{}

func proxyNodeDir() string {
	return path.Join(global.Dir.DataDir, "proxy-node")
}

func proxyNodeSetting(key, def string) string {
	if v, err := settingRepo.GetValueByKey(key); err == nil && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func proxyNodePort() int {
	port, err := strconv.Atoi(proxyNodeSetting(proxyNodeSettingPort, ""))
	if err != nil || port <= 0 || port > 65534 {
		return proxyNodeDefaultPort
	}
	return port
}

// proxyNodeCredentials returns (creating on first use) the random
// controller secret and proxy user/password.
func proxyNodeCredentials() (secret, user, pass string) {
	get := func(key string, n int) string {
		v := proxyNodeSetting(key, "")
		if v == "" {
			v = common.RandStrAndNum(n)
			_ = settingRepo.UpdateOrCreate(key, v)
		}
		return v
	}
	return get(proxyNodeSettingSecret, 24), get(proxyNodeSettingUser, 8), get(proxyNodeSettingPass, 16)
}

func (p *proxyNodeRuntime) ensureLoaded() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return
	}
	p.loaded = true
	raw := proxyNodeSetting(proxyNodeSettingResults, "")
	if raw == "" {
		return
	}
	var saved struct {
		Nodes    []dto.ProxyNodeItem `json:"nodes"`
		TestedAt string              `json:"testedAt"`
	}
	if json.Unmarshal([]byte(raw), &saved) == nil {
		p.nodes, p.testedAt = saved.Nodes, saved.TestedAt
	}
}

func (p *proxyNodeRuntime) persist() {
	p.mu.Lock()
	data, _ := json.Marshal(map[string]interface{}{"nodes": p.nodes, "testedAt": p.testedAt})
	p.mu.Unlock()
	_ = settingRepo.UpdateOrCreate(proxyNodeSettingResults, string(data))
}

func (u *ProxyNodeService) Status() dto.ProxyNodeStatus {
	proxyNodeState.ensureLoaded()
	_, user, pass := proxyNodeCredentials()
	port := proxyNodePort()
	status := dto.ProxyNodeStatus{
		SubscriptionURL: proxyNodeSetting(proxyNodeSettingURL, ""),
		Image:           proxyNodeSetting(proxyNodeSettingImage, proxyNodeDefaultImage),
		Port:            port,
		Selected:        proxyNodeSetting(proxyNodeSettingSelected, ""),
		ContainerName:   proxyNodeContainer,
		Running:         proxyNodeRunning(),
		UpdatedAt:       proxyNodeSetting(proxyNodeSettingUpdated, ""),
	}
	if status.SubscriptionURL != "" {
		status.ProxyURL = fmt.Sprintf("http://%s:%s@127.0.0.1:%d", user, pass, port)
	}
	proxyNodeState.mu.Lock()
	status.Nodes = append([]dto.ProxyNodeItem{}, proxyNodeState.nodes...)
	status.Testing = proxyNodeState.testing
	status.TestDone, status.TestTotal = proxyNodeState.testDone, proxyNodeState.testTotal
	status.TestedAt = proxyNodeState.testedAt
	status.Message = proxyNodeState.message
	proxyNodeState.mu.Unlock()
	return status
}

// ProxyURLForContainer is the proxy address other containers (attached to
// 1panel-network) use, e.g. vLLM model downloads and deployments.
func (u *ProxyNodeService) ProxyURLForContainer() (string, error) {
	if proxyNodeSetting(proxyNodeSettingURL, "") == "" {
		return "", buserr.WithDetail("ErrInvalidParams", "no proxy subscription configured (Settings → Proxy node)", nil)
	}
	if !proxyNodeRunning() {
		return "", buserr.WithDetail("ErrInvalidParams", "the proxy node container is not running (Settings → Proxy node)", nil)
	}
	_, user, pass := proxyNodeCredentials()
	return fmt.Sprintf("http://%s:%s@%s:%d", user, pass, proxyNodeContainer, proxyNodeInnerPort), nil
}

func (u *ProxyNodeService) Save(req dto.ProxyNodeSave) error {
	subURL := strings.TrimSpace(req.SubscriptionURL)
	if parsed, err := url.Parse(subURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return buserr.WithDetail("ErrInvalidParams", "subscription URL must be an http(s) URL", nil)
	}
	if req.Port != 0 && (req.Port < 1024 || req.Port > 65534) {
		return buserr.WithDetail("ErrInvalidParams", "port must be between 1024 and 65534", nil)
	}
	image := strings.TrimSpace(req.Image)
	if image == "" {
		image = proxyNodeDefaultImage
	}
	pairs := map[string]string{proxyNodeSettingURL: subURL, proxyNodeSettingImage: image}
	if req.Port != 0 {
		pairs[proxyNodeSettingPort] = strconv.Itoa(req.Port)
	}
	if req.Selected != "" {
		pairs[proxyNodeSettingSelected] = req.Selected
	}
	for k, v := range pairs {
		if err := settingRepo.UpdateOrCreate(k, v); err != nil {
			return err
		}
	}
	return u.Refresh()
}

// Refresh downloads the subscription, rewrites the mihomo config, (re)starts
// the container and starts a background test of all nodes.
func (u *ProxyNodeService) Refresh() error {
	subURL := proxyNodeSetting(proxyNodeSettingURL, "")
	if subURL == "" {
		return buserr.WithDetail("ErrInvalidParams", "subscription URL is empty", nil)
	}
	proxies, err := downloadClashProxies(subURL)
	if err != nil {
		return buserr.WithDetail("ErrInvalidParams", err.Error(), nil)
	}
	names := make([]string, 0, len(proxies))
	types := map[string]string{}
	for _, item := range proxies {
		name, _ := item["name"].(string)
		typ, _ := item["type"].(string)
		names = append(names, name)
		types[name] = typ
	}
	selected := proxyNodeSetting(proxyNodeSettingSelected, "")
	if selected != "" && types[selected] == "" && selected != "DIRECT" {
		selected = ""
		_ = settingRepo.UpdateOrCreate(proxyNodeSettingSelected, "")
	}
	if err := writeProxyNodeFiles(proxies, names, selected); err != nil {
		return err
	}
	if err := startProxyNodeContainer(true); err != nil {
		return err
	}
	_ = settingRepo.UpdateOrCreate(proxyNodeSettingUpdated, time.Now().Format(time.DateTime))

	// Keep previous results for nodes that still exist.
	proxyNodeState.ensureLoaded()
	proxyNodeState.mu.Lock()
	previous := map[string]dto.ProxyNodeItem{}
	for _, item := range proxyNodeState.nodes {
		previous[item.Name] = item
	}
	nodes := make([]dto.ProxyNodeItem, 0, len(names))
	for _, name := range names {
		item := dto.ProxyNodeItem{Name: name, Type: types[name]}
		if prev, ok := previous[name]; ok {
			item.Delay, item.Speed, item.Tested = prev.Delay, prev.Speed, prev.Tested
		}
		nodes = append(nodes, item)
	}
	proxyNodeState.nodes = nodes
	proxyNodeState.message = ""
	proxyNodeState.mu.Unlock()
	proxyNodeState.persist()

	if selected != "" {
		go func() {
			if waitProxyNodeController(20*time.Second) == nil {
				_ = selectProxyNode(selected)
			}
		}()
	}
	return u.Test()
}

func (u *ProxyNodeService) Select(name string) error {
	proxyNodeState.ensureLoaded()
	proxyNodeState.mu.Lock()
	found := name == "DIRECT"
	for _, item := range proxyNodeState.nodes {
		if item.Name == name {
			found = true
			break
		}
	}
	proxyNodeState.mu.Unlock()
	if !found {
		return buserr.WithDetail("ErrInvalidParams", "unknown node: "+name, nil)
	}
	if err := settingRepo.UpdateOrCreate(proxyNodeSettingSelected, name); err != nil {
		return err
	}
	if proxyNodeRunning() {
		if err := selectProxyNode(name); err != nil {
			return err
		}
	}
	return nil
}

func (u *ProxyNodeService) Operate(operate string) error {
	switch operate {
	case "start":
		if proxyNodeSetting(proxyNodeSettingURL, "") == "" {
			return buserr.WithDetail("ErrInvalidParams", "subscription URL is empty", nil)
		}
		return startProxyNodeContainer(false)
	case "stop":
		out, err := compose.Down(path.Join(proxyNodeDir(), "docker-compose.yml"))
		if err != nil {
			return fmt.Errorf("%s %v", strings.TrimSpace(out), err)
		}
		return nil
	}
	return buserr.New("ErrInvalidParams")
}

// Test starts a background latency + speed test of every node. A test that
// is already running is left alone (the UI just keeps polling).
func (u *ProxyNodeService) Test() error {
	proxyNodeState.ensureLoaded()
	proxyNodeState.mu.Lock()
	if proxyNodeState.testing {
		proxyNodeState.mu.Unlock()
		return nil
	}
	if len(proxyNodeState.nodes) == 0 {
		proxyNodeState.mu.Unlock()
		return buserr.WithDetail("ErrInvalidParams", "no nodes to test", nil)
	}
	proxyNodeState.testing = true
	proxyNodeState.testDone = 0
	proxyNodeState.testTotal = len(proxyNodeState.nodes) * 2
	proxyNodeState.message = ""
	proxyNodeState.mu.Unlock()
	go runProxyNodeTests()
	return nil
}

// --------------------------------------------------------------- helpers

func downloadClashProxies(subURL string) ([]map[string]interface{}, error) {
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
	req, err := http.NewRequest(http.MethodGet, subURL, nil)
	if err != nil {
		return nil, err
	}
	// Subscription services return the Clash YAML format for Clash UAs.
	req.Header.Set("User-Agent", "clash.meta")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download subscription failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download subscription failed: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return nil, fmt.Errorf("read subscription failed: %v", err)
	}
	return parseClashProxies(raw)
}

func parseClashProxies(raw []byte) ([]map[string]interface{}, error) {
	var doc struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil || len(doc.Proxies) == 0 {
		return nil, fmt.Errorf("the subscription is not a Clash format config (no proxies found)")
	}
	seen := map[string]bool{"DIRECT": true, "REJECT": true, proxyNodeGroup: true}
	result := make([]map[string]interface{}, 0, len(doc.Proxies))
	for _, item := range doc.Proxies {
		name, _ := item["name"].(string)
		typ, _ := item["type"].(string)
		name = strings.TrimSpace(name)
		if name == "" || typ == "" || seen[name] {
			continue
		}
		seen[name] = true
		item["name"] = name
		result = append(result, item)
		if len(result) >= proxyNodeMaxNodes {
			break
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("the subscription contains no usable proxies")
	}
	return result, nil
}

func buildProxyNodeConfig(proxies []map[string]interface{}, names []string, selected string) ([]byte, error) {
	secret, user, pass := proxyNodeCredentials()
	auth := user + ":" + pass
	groupProxies := append([]string{}, names...)
	if selected != "" {
		// mihomo selects the first member of a select group on start.
		ordered := []string{selected}
		for _, name := range names {
			if name != selected {
				ordered = append(ordered, name)
			}
		}
		groupProxies = ordered
	}
	groupProxies = append(groupProxies, "DIRECT")
	users := []map[string]string{{"username": user, "password": pass}}
	listeners := make([]map[string]interface{}, 0, len(names))
	for i, name := range names {
		// One listener per node, bound to that node, used by the speed
		// test. Not published on the host.
		listeners = append(listeners, map[string]interface{}{
			"name": fmt.Sprintf("speedtest-%d", i), "type": "mixed", "listen": "0.0.0.0",
			"port": proxyNodeTestBase + i, "proxy": name, "users": users,
		})
	}
	config := map[string]interface{}{
		"mixed-port":          proxyNodeInnerPort,
		"allow-lan":           true,
		"bind-address":        "*",
		"mode":                "rule",
		"log-level":           "warning",
		"ipv6":                false,
		"external-controller": fmt.Sprintf("0.0.0.0:%d", proxyNodeCtrlPort),
		"secret":              secret,
		"authentication":      []string{auth},
		"profile":             map[string]bool{"store-selected": false},
		"proxies":             proxies,
		"proxy-groups": []map[string]interface{}{
			{"name": proxyNodeGroup, "type": "select", "proxies": groupProxies},
		},
		"listeners": listeners,
		"rules":     []string{"MATCH," + proxyNodeGroup},
	}
	return yaml.Marshal(config)
}

func writeProxyNodeFiles(proxies []map[string]interface{}, names []string, selected string) error {
	dir := proxyNodeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	config, err := buildProxyNodeConfig(proxies, names, selected)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path.Join(dir, "config.yaml"), config, 0o600); err != nil {
		return err
	}
	port := proxyNodePort()
	service := map[string]interface{}{
		"image":          proxyNodeSetting(proxyNodeSettingImage, proxyNodeDefaultImage),
		"container_name": proxyNodeContainer,
		"restart":        "unless-stopped",
		"ports": []string{
			fmt.Sprintf("127.0.0.1:%d:%d", port, proxyNodeInnerPort),
			fmt.Sprintf("127.0.0.1:%d:%d", port+1, proxyNodeCtrlPort),
		},
		"volumes":  []string{path.Join(dir, "config.yaml") + ":/root/.config/mihomo/config.yaml:ro"},
		"networks": []string{"1panel-network"},
		"labels":   map[string]string{"createdBy": "1Panel"},
	}
	project := map[string]interface{}{
		"services": map[string]interface{}{"mihomo": service},
		"networks": map[string]interface{}{"1panel-network": map[string]bool{"external": true}},
	}
	content, err := yaml.Marshal(project)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(dir, "docker-compose.yml"), content, 0o644)
}

func startProxyNodeContainer(restart bool) error {
	_ = docker.CreateDefaultDockerNetwork()
	composePath := path.Join(proxyNodeDir(), "docker-compose.yml")
	if _, err := os.Stat(composePath); err != nil {
		return buserr.WithDetail("ErrInvalidParams", "proxy node is not configured yet", nil)
	}
	running := proxyNodeRunning()
	out, err := compose.Up(composePath)
	if err != nil {
		return fmt.Errorf("start proxy node failed: %s %v", strings.TrimSpace(out), err)
	}
	if restart && running {
		// The compose file did not change, so `up` kept the old container;
		// restart it to load the rewritten config.
		if out, err := compose.Restart(composePath); err != nil {
			return fmt.Errorf("restart proxy node failed: %s %v", strings.TrimSpace(out), err)
		}
	}
	return nil
}

func proxyNodeRunning() bool {
	out, err := cmd.NewCommandMgr(cmd.WithTimeout(5*time.Second)).RunWithStdout("docker", "inspect", "-f", "{{.State.Running}}", proxyNodeContainer)
	return err == nil && strings.TrimSpace(out) == "true"
}

func proxyNodeContainerIP() string {
	out, err := cmd.NewCommandMgr(cmd.WithTimeout(5*time.Second)).RunWithStdout("docker", "inspect", "-f",
		`{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}`, proxyNodeContainer)
	if err != nil {
		return ""
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func proxyNodeController(method, apiPath string, body io.Reader, timeout time.Duration) (*http.Response, error) {
	secret, _, _ := proxyNodeCredentials()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%d%s", proxyNodePort()+1, apiPath), body)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func waitProxyNodeController(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := proxyNodeController(http.MethodGet, "/version", nil, 2*time.Second)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("the proxy node controller did not become ready")
}

func selectProxyNode(name string) error {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := proxyNodeController(http.MethodPut, "/proxies/"+url.PathEscape(proxyNodeGroup), strings.NewReader(string(body)), 5*time.Second)
	if err != nil {
		return fmt.Errorf("switch node failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("switch node failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func proxyNodeDelay(name string) int {
	query := url.Values{"url": {proxyNodeDelayURL}, "timeout": {"5000"}}
	resp, err := proxyNodeController(http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+query.Encode(), nil, 8*time.Second)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	var result struct {
		Delay int `json:"delay"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&result) != nil || result.Delay <= 0 {
		return -1
	}
	return result.Delay
}

// proxyNodeSpeed downloads through the node's dedicated listener for at
// most 8 seconds and returns bytes per second (-1 on failure).
func proxyNodeSpeed(containerIP string, index int) float64 {
	if containerIP == "" {
		return -1
	}
	_, user, pass := proxyNodeCredentials()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s:%s@%s:%d", url.QueryEscape(user), url.QueryEscape(pass), containerIP, proxyNodeTestBase+index))
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyNodeSpeedURL, nil)
	if err != nil {
		return -1
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return -1
	}
	start := time.Now()
	readCtx, readCancel := context.WithTimeout(ctx, 8*time.Second)
	defer readCancel()
	buf := make([]byte, 64*1024)
	var total int64
	for readCtx.Err() == nil {
		n, err := resp.Body.Read(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}
	elapsed := time.Since(start).Seconds()
	if total == 0 || elapsed <= 0 {
		return -1
	}
	return float64(total) / elapsed
}

func runProxyNodeTests() {
	defer func() {
		proxyNodeState.mu.Lock()
		proxyNodeState.testing = false
		proxyNodeState.testedAt = time.Now().Format(time.DateTime)
		proxyNodeState.mu.Unlock()
		proxyNodeState.persist()
	}()
	if err := waitProxyNodeController(30 * time.Second); err != nil {
		proxyNodeState.mu.Lock()
		proxyNodeState.message = err.Error()
		proxyNodeState.mu.Unlock()
		return
	}
	proxyNodeState.mu.Lock()
	names := make([]string, len(proxyNodeState.nodes))
	for i, item := range proxyNodeState.nodes {
		names[i] = item.Name
	}
	proxyNodeState.mu.Unlock()

	update := func(name string, apply func(item *dto.ProxyNodeItem)) {
		proxyNodeState.mu.Lock()
		defer proxyNodeState.mu.Unlock()
		for i := range proxyNodeState.nodes {
			if proxyNodeState.nodes[i].Name == name {
				apply(&proxyNodeState.nodes[i])
				break
			}
		}
		proxyNodeState.testDone++
	}

	// Phase 1: latency of every node (cheap, high concurrency).
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(16)
	delays := make([]int, len(names))
	for i, name := range names {
		i, name := i, name
		group.Go(func() error {
			delays[i] = proxyNodeDelay(name)
			update(name, func(item *dto.ProxyNodeItem) {
				item.Delay = delays[i]
				item.Tested = true
				if delays[i] < 0 {
					item.Speed = -1
				}
			})
			return nil
		})
	}
	_ = group.Wait()

	// Phase 2: download speed of the reachable nodes, fastest first, with
	// low concurrency so the nodes do not compete for bandwidth too much.
	containerIP := proxyNodeContainerIP()
	order := make([]int, 0, len(names))
	for i := range names {
		if delays[i] > 0 {
			order = append(order, i)
		} else {
			update(names[i], func(item *dto.ProxyNodeItem) {})
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return delays[order[a]] < delays[order[b]] })
	group, _ = errgroup.WithContext(context.Background())
	group.SetLimit(3)
	for _, idx := range order {
		idx := idx
		group.Go(func() error {
			speed := proxyNodeSpeed(containerIP, idx)
			update(names[idx], func(item *dto.ProxyNodeItem) { item.Speed = speed })
			return nil
		})
	}
	_ = group.Wait()
}
