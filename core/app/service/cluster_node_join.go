package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/ssh"
	"github.com/pkg/sftp"
)

// "Join cluster" of a Terminal → Hosts entry.
//
// The master reaches the host over SSH with the credentials saved in
// Terminal → Hosts and:
//  1. installs the panel when it is missing: the master downloads and
//     verifies the release package (the same cache as the master-distributed
//     node upgrade), uploads it over SFTP and runs install.sh non-interactively;
//  2. runs `1panel cluster-join` on the host, which creates a dedicated API
//     key restricted to the master IP and prints it with the panel port /
//     SSL / entrance;
//  3. registers (or, when the host is already a node, re-keys) the panel
//     node and tests the connection.
//
// Progress is kept in memory per terminal host and polled by the UI. The
// API key only travels inside the SSH session and is stored encrypted like
// a key pasted by hand; it is never logged nor returned.

const (
	joinStageConnect  = "connect"
	joinStageDownload = "download"
	joinStageUpload   = "upload"
	joinStageInstall  = "install"
	joinStageKey      = "key"
	joinStageRegister = "register"
	joinStageDone     = "done"
	joinStageFailed   = "failed"

	joinTimeout      = 45 * time.Minute
	joinLogLines     = 400
	joinResultMarker = "CLUSTER_JOIN_RESULT "
)

type clusterJoinState struct {
	Stage     string
	Running   bool
	Logs      []string
	Error     string
	Done      int64
	Total     int64
	Installed bool
	NodeID    uint
	NodeName  string
	EntryURL  string
	Username  string
	Password  string
	StartedAt time.Time
}

var clusterJoins = struct {
	sync.Mutex
	hosts map[uint]*clusterJoinState
}{hosts: map[uint]*clusterJoinState{}}

func joinUpdate(hostID uint, mut func(s *clusterJoinState)) {
	clusterJoins.Lock()
	defer clusterJoins.Unlock()
	if st, ok := clusterJoins.hosts[hostID]; ok {
		mut(st)
	}
}

func joinLog(hostID uint, line string) {
	line = strings.TrimRight(joinANSI.ReplaceAllString(line, ""), " \t")
	if strings.TrimSpace(line) == "" {
		return
	}
	joinUpdate(hostID, func(s *clusterJoinState) {
		if s.Password != "" {
			line = strings.ReplaceAll(line, s.Password, "********")
		}
		s.Logs = append(s.Logs, line)
		if len(s.Logs) > joinLogLines {
			s.Logs = s.Logs[len(s.Logs)-joinLogLines:]
		}
	})
}

var joinANSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func (u *ClusterNodeService) JoinCluster(req dto.ClusterNodeJoinReq) error {
	host, err := computeTerminalHostByID(req.HostID)
	if err != nil {
		return err
	}
	if req.MasterIP != "" {
		for _, item := range strings.Split(req.MasterIP, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if net.ParseIP(item) == nil {
				if _, _, err := net.ParseCIDR(item); err != nil {
					return fmt.Errorf("主节点 IP 格式错误: %s", item)
				}
			}
		}
	}
	clusterJoins.Lock()
	if st, ok := clusterJoins.hosts[host.ID]; ok && st.Running {
		clusterJoins.Unlock()
		return fmt.Errorf("主机 %s 正在加入集群，请等待完成", host.Name)
	}
	clusterJoins.hosts[host.ID] = &clusterJoinState{Stage: joinStageConnect, Running: true, StartedAt: time.Now()}
	clusterJoins.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), joinTimeout)
		defer cancel()
		err := u.runJoin(ctx, host, req)
		joinUpdate(host.ID, func(s *clusterJoinState) {
			s.Running = false
			if err != nil {
				s.Stage, s.Error = joinStageFailed, truncateErr(err.Error())
			} else {
				s.Stage = joinStageDone
			}
		})
		if err != nil {
			joinLog(host.ID, "✗ "+err.Error())
			global.LOG.Warnf("join cluster for terminal host %s failed: %v", host.Name, err)
		}
	}()
	return nil
}

func (u *ClusterNodeService) JoinClusterStatus(hostID uint) dto.ClusterNodeJoinStatus {
	clusterJoins.Lock()
	defer clusterJoins.Unlock()
	st, ok := clusterJoins.hosts[hostID]
	if !ok {
		return dto.ClusterNodeJoinStatus{HostID: hostID}
	}
	res := dto.ClusterNodeJoinStatus{
		HostID: hostID, Stage: st.Stage, Running: st.Running, Error: st.Error,
		Done: st.Done, Total: st.Total, Installed: st.Installed,
		NodeID: st.NodeID, NodeName: st.NodeName, EntryURL: st.EntryURL,
		Logs: append([]string{}, st.Logs...),
	}
	// The initial login of a panel installed by this join is shown once
	// the join has finished; it is never written to the log.
	if !st.Running && st.Installed {
		res.Username, res.Password = st.Username, st.Password
	}
	return res
}

type joinProbe struct {
	arch      string
	installed bool
	root      bool
	sshClient string
}

func (u *ClusterNodeService) runJoin(ctx context.Context, host computeTerminalHost, req dto.ClusterNodeJoinReq) error {
	logf := func(format string, args ...interface{}) { joinLog(host.ID, fmt.Sprintf(format, args...)) }
	stage := func(s string) {
		joinUpdate(host.ID, func(st *clusterJoinState) { st.Stage, st.Done, st.Total = s, 0, 0 })
	}

	logf("连接 %s@%s:%d ...", host.User, host.Addr, host.Port)
	info, err := computeTerminalConnInfo(host)
	if err != nil {
		return err
	}
	info.DialTimeOut = 15 * time.Second
	client, err := ssh.NewClient(info)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %v", err)
	}
	defer client.Close()

	probe, err := joinProbeHost(ctx, client)
	if err != nil {
		return err
	}
	if !probe.root && client.SudoItem == "" {
		return fmt.Errorf("登录用户 %s 不是 root 且没有免密 sudo，无法安装面板或创建 API Key", host.User)
	}
	sudo := !probe.root
	logf("架构 %s，面板%s", probe.arch, map[bool]string{true: "已安装", false: "未安装"}[probe.installed])

	masterIP := strings.TrimSpace(req.MasterIP)
	if masterIP == "" {
		masterIP = probe.sshClient
	}
	if masterIP == "" {
		return errors.New("无法识别主节点访问该主机时的出口 IP，请手动填写主节点 IP")
	}
	logf("API Key 白名单: %s", masterIP)

	if !probe.installed {
		if err := u.joinInstall(ctx, host, client, probe, sudo, req, stage, logf); err != nil {
			return err
		}
	}

	stage(joinStageKey)
	logf("创建主节点专用 API Key ...")
	script := "PATH=$PATH:/usr/local/bin:/usr/bin\n1panel cluster-join --master-ip " + computeShellQuote(masterIP) + " 2>&1\n"
	var res struct {
		APIKey   string `json:"apiKey"`
		KeyHint  string `json:"keyHint"`
		Port     string `json:"port"`
		SSL      bool   `json:"ssl"`
		Entrance string `json:"entrance"`
		Version  string `json:"version"`
	}
	out, runErr := joinRun(ctx, client, script, sudo, func(line string) {
		if !strings.HasPrefix(line, joinResultMarker) {
			logf("%s", line)
		}
	})
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, joinResultMarker); idx >= 0 {
			_ = json.Unmarshal([]byte(strings.TrimSpace(line[idx+len(joinResultMarker):])), &res)
		}
	}
	if res.APIKey == "" {
		if strings.Contains(out, "unknown command") {
			return errors.New("该主机的面板版本过旧，不支持自动加入集群；请先在该主机上把面板升级到与主节点相同的版本后重试")
		}
		if runErr != nil {
			return fmt.Errorf("创建 API Key 失败: %v", runErr)
		}
		return errors.New("创建 API Key 失败：未返回结果")
	}
	logf("API Key 已创建 (%s)，面板版本 %s", res.KeyHint, res.Version)

	stage(joinStageRegister)
	entryHost := strings.TrimSpace(req.EntryHost)
	if entryHost == "" {
		entryHost = host.Addr
	}
	scheme := "http"
	if res.SSL {
		scheme = "https"
	}
	entryURL := scheme + "://" + net.JoinHostPort(entryHost, res.Port)
	if e := strings.Trim(res.Entrance, "/ "); e != "" {
		entryURL += "/" + e
	}
	node, err := u.joinRegister(host, req, entryURL, res.APIKey)
	if err != nil {
		return err
	}
	joinUpdate(host.ID, func(s *clusterJoinState) { s.NodeID, s.NodeName, s.EntryURL = node.ID, node.Name, entryURL })
	logf("已登记为集群节点 %s (%s)", node.Name, entryURL)

	// A panel that has just been installed may still be starting.
	var info2 dto.ClusterNodeInfo
	for i := 0; i < 10; i++ {
		info2 = u.checkAndPersist(node)
		if info2.Status == model.ClusterNodeStatusOnline || ctx.Err() != nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if info2.Status != model.ClusterNodeStatusOnline {
		logf("节点已登记，但连接测试未通过: %s", info2.LastError)
		logf("请确认主节点能访问 %s（防火墙/安全组放行面板端口），然后在集群节点列表里重新测试", entryURL)
		return nil
	}
	logf("✓ 连接测试通过（%d ms），已纳入主节点统一管理", info2.LatencyMs)
	return nil
}

func joinProbeHost(ctx context.Context, client *ssh.SSHClient) (joinProbe, error) {
	script := `echo "ARCH=$(uname -m)"
echo "UID=$(id -u)"
echo "SSHCONN=$SSH_CONNECTION"
if command -v 1panel >/dev/null 2>&1 || [ -x /usr/local/bin/1panel-core ] || [ -x /usr/bin/1panel ]; then echo INSTALLED=1; fi
`
	out, err := joinRun(ctx, client, script, false, nil)
	if err != nil {
		return joinProbe{}, fmt.Errorf("探测主机失败: %v", err)
	}
	var p joinProbe
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ARCH="):
			p.arch = strings.TrimPrefix(line, "ARCH=")
		case strings.HasPrefix(line, "UID="):
			p.root = strings.TrimPrefix(line, "UID=") == "0"
		case strings.HasPrefix(line, "SSHCONN="):
			if fields := strings.Fields(strings.TrimPrefix(line, "SSHCONN=")); len(fields) > 0 && net.ParseIP(fields[0]) != nil {
				p.sshClient = fields[0]
			}
		case line == "INSTALLED=1":
			p.installed = true
		}
	}
	return p, nil
}

func (u *ClusterNodeService) joinInstall(ctx context.Context, host computeTerminalHost, client *ssh.SSHClient, probe joinProbe, sudo bool, req dto.ClusterNodeJoinReq, stage func(string), logf func(string, ...interface{})) error {
	arch, err := normalizeReleaseArch(probe.arch)
	if err != nil {
		return err
	}
	tag := joinReleaseTag()
	if tag == "" {
		return errors.New("无法获取面板发布版本，请检查主节点能否访问发布渠道后重试")
	}

	stage(joinStageDownload)
	logf("主节点准备安装包 %s (%s) ...", tag, arch)
	pkgPath, _, err := cachedUpgradePackage(tag, arch, func(done, total int64) {
		joinUpdate(host.ID, func(s *clusterJoinState) { s.Done, s.Total = done, total })
	})
	if err != nil {
		return fmt.Errorf("主节点下载安装包失败: %v", err)
	}

	stage(joinStageUpload)
	remoteDir := "/tmp/1panel-join-" + joinRandom(6)
	remotePkg := path.Join(remoteDir, path.Base(pkgPath))
	logf("上传安装包到 %s ...", remoteDir)
	if err := joinUpload(ctx, client, pkgPath, remoteDir, remotePkg, func(done, total int64) {
		joinUpdate(host.ID, func(s *clusterJoinState) { s.Done, s.Total = done, total })
	}); err != nil {
		return fmt.Errorf("上传安装包失败: %v", err)
	}

	stage(joinStageInstall)
	username, password := "admin_"+joinRandom(3), joinRandom(8)
	joinUpdate(host.ID, func(s *clusterJoinState) { s.Username, s.Password = username, password })
	lang := "en"
	if l, _ := settingRepo.GetValueByKey("Language"); strings.HasPrefix(l, "zh") {
		lang = "zh"
	}
	installDocker := "n"
	if req.InstallDocker {
		installDocker = "y"
	}
	pkgDir := strings.TrimSuffix(path.Base(pkgPath), ".tar.gz")
	// Credentials go through the script on stdin (not argv), install.sh
	// gets /dev/null as stdin so a prompt can never eat the script.
	script := fmt.Sprintf(`set -e
cd %[1]s
tar -xzf %[2]s
cd %[3]s
export PANEL_USERNAME=%[4]s PANEL_PASSWORD=%[5]s PANEL_LANG=%[6]s
bash install.sh -y --install-docker %[7]s </dev/null 2>&1
cd /
rm -rf %[1]s
`, computeShellQuote(remoteDir), computeShellQuote(path.Base(remotePkg)), computeShellQuote(pkgDir),
		computeShellQuote(username), computeShellQuote(password), lang, installDocker)
	logf("执行 install.sh（非交互，安装 Docker: %s）...", installDocker)
	if _, err := joinRun(ctx, client, script, sudo, func(line string) { logf("%s", line) }); err != nil {
		_, _ = joinRun(context.Background(), client, "rm -rf "+computeShellQuote(remoteDir)+"\n", sudo, nil)
		return fmt.Errorf("安装面板失败: %v", err)
	}
	joinUpdate(host.ID, func(s *clusterJoinState) { s.Installed = true })
	logf("面板安装完成")
	return nil
}

// joinReleaseTag installs the master's own version when it is a published
// release (the node then has the same features, incl. cluster-join), else
// the newest stable release.
func joinReleaseTag() string {
	if v := strings.TrimSpace(global.CONF.Base.Version); v != "" {
		if _, err := findForkRelease(v); err == nil {
			return v
		}
	}
	return latestForkVersion()
}

func joinRandom(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func joinUpload(ctx context.Context, client *ssh.SSHClient, local, remoteDir, remote string, progress func(done, total int64)) error {
	sc, err := sftp.NewClient(client.Client, sftp.UseConcurrentWrites(true))
	if err != nil {
		return err
	}
	defer sc.Close()
	if err := sc.MkdirAll(remoteDir); err != nil {
		return err
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	stat, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := sc.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	stop := context.AfterFunc(ctx, func() { _ = sc.Close() })
	defer stop()
	pw := &joinProgressWriter{total: stat.Size(), progress: progress}
	if _, err := dst.ReadFrom(io.TeeReader(src, pw)); err != nil {
		return err
	}
	return nil
}

type joinProgressWriter struct {
	done, total int64
	last        time.Time
	progress    func(done, total int64)
}

func (w *joinProgressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if time.Since(w.last) > 500*time.Millisecond || w.done == w.total {
		w.last = time.Now()
		w.progress(w.done, w.total)
	}
	return len(p), nil
}

// joinRun runs a script through `sh -s` (fed on stdin), under `sudo -n`
// when asked. onLine receives every output line as it arrives; the whole
// output (capped) is returned as well.
func joinRun(ctx context.Context, client *ssh.SSHClient, script string, sudo bool, onLine func(string)) (string, error) {
	session, err := client.Client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	w := &joinLineWriter{onLine: onLine, all: &computeLimitedBuffer{max: computeSSHMaxOutput}}
	session.Stdout, session.Stderr = w, w
	session.Stdin = strings.NewReader(script)
	cmd := "sh -s"
	if sudo {
		cmd = "sudo -n sh -s"
	}
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = session.Signal("KILL")
		_ = session.Close()
		err = errors.New("执行超时")
	}
	w.flush()
	return w.all.buf.String(), err
}

type joinLineWriter struct {
	mu      sync.Mutex
	partial []byte
	onLine  func(string)
	all     *computeLimitedBuffer
}

func (w *joinLineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.all.Write(p)
	if w.onLine == nil {
		return len(p), nil
	}
	w.partial = append(w.partial, p...)
	for {
		idx := -1
		for i, b := range w.partial {
			if b == '\n' || b == '\r' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(w.partial[:idx])
		w.partial = w.partial[idx+1:]
		w.onLine(line)
	}
	return len(p), nil
}

func (w *joinLineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.onLine != nil && len(w.partial) > 0 {
		w.onLine(string(w.partial))
	}
	w.partial = nil
}

// joinRegister creates the node, or re-keys the node that already points
// at the same host:port (joining twice rotates the key instead of adding
// a duplicate).
func (u *ClusterNodeService) joinRegister(host computeTerminalHost, req dto.ClusterNodeJoinReq, entryURL, apiKey string) (model.ClusterNode, error) {
	target, _ := url.Parse(entryURL)
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return model.ClusterNode{}, err
	}
	for _, n := range nodes {
		if p, err := url.Parse(n.EntryURL); err == nil && p.Hostname() == target.Hostname() && joinURLPort(p) == joinURLPort(target) {
			upd := dto.ClusterNodeUpdate{ID: n.ID, Name: n.Name, EntryURL: entryURL, Remark: n.Remark, GroupID: n.GroupID, APIKey: apiKey}
			if req.GroupID != 0 {
				upd.GroupID = req.GroupID
			}
			if err := u.Update(upd); err != nil {
				return model.ClusterNode{}, err
			}
			return u.getClusterNode(n.ID)
		}
	}
	base := strings.TrimSpace(req.Name)
	if base == "" {
		base = strings.TrimSpace(host.Name)
	}
	if base == "" {
		base = host.Addr
	}
	name := base
	for i := 2; ; i++ {
		if existing, _ := clusterNodeRepo.Get(repo.WithByName(name)); existing.ID == 0 {
			break
		}
		name = base + "-" + strconv.Itoa(i)
	}
	remark := fmt.Sprintf("由终端主机 %s 加入集群", host.Name)
	if err := u.Create(dto.ClusterNodeCreate{Name: name, EntryURL: entryURL, Remark: remark, GroupID: req.GroupID, APIKey: apiKey}); err != nil {
		return model.ClusterNode{}, err
	}
	node, _ := clusterNodeRepo.Get(repo.WithByName(name))
	if node.ID == 0 {
		return node, errors.New("登记集群节点失败")
	}
	return node, nil
}

func joinURLPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
