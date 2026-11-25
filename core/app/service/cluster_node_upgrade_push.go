package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
)

// "Master distribution" of a node panel upgrade.
//
// The master downloads the release package for the node's arch once into
// a local cache (concurrent requests for the same file share one download),
// verifies it against the release's .sha256, uploads it into the node's
// push directory with the resumable chunk upload used by cluster sync, and
// finally calls the node's /core/settings/upgrade with PackagePath +
// PackageSha256, so the node installs that file without touching the
// internet. Progress of the master side is kept in memory per node and
// reported by NodeUpgradeStatus until the node takes over.

const (
	clusterUpgradeModeDirect = "direct"
	clusterUpgradeModeMaster = "master"

	pushStagePreparing = "preparing" // master downloads / verifies the package
	pushStagePushing   = "pushing"   // master uploads it to the node
	pushStageDone      = "done"      // the node runs the upgrade now
	pushStageFailed    = "failed"

	// Uploads running at the same time (the master uplink is shared).
	pushUpgradeParallel = 4
	pushUpgradeTimeout  = 90 * time.Minute
)

type pushUpgradeState struct {
	Stage   string
	Version string
	Done    int64
	Total   int64
	Error   string
}

var pushUpgrades = struct {
	sync.Mutex
	nodes map[uint]*pushUpgradeState
}{nodes: map[uint]*pushUpgradeState{}}

var pushUpgradeSlots = make(chan struct{}, pushUpgradeParallel)

func loadPushState(nodeID uint) (pushUpgradeState, bool) {
	pushUpgrades.Lock()
	defer pushUpgrades.Unlock()
	st, ok := pushUpgrades.nodes[nodeID]
	if !ok {
		return pushUpgradeState{}, false
	}
	return *st, true
}

func setPushState(nodeID uint, mut func(s *pushUpgradeState)) {
	pushUpgrades.Lock()
	defer pushUpgrades.Unlock()
	st, ok := pushUpgrades.nodes[nodeID]
	if !ok {
		st = &pushUpgradeState{}
		pushUpgrades.nodes[nodeID] = st
	}
	mut(st)
}

// claimPushUpgrade marks a node as being upgraded by the master; it fails
// when a push to that node is already running.
func claimPushUpgrade(nodeID uint, version string) error {
	pushUpgrades.Lock()
	defer pushUpgrades.Unlock()
	if st, ok := pushUpgrades.nodes[nodeID]; ok && (st.Stage == pushStagePreparing || st.Stage == pushStagePushing) {
		return fmt.Errorf("节点正在更新到 %s", st.Version)
	}
	pushUpgrades.nodes[nodeID] = &pushUpgradeState{Stage: pushStagePreparing, Version: version}
	return nil
}

// clearPushState forgets the master-side state of a node (direct mode).
func clearPushState(nodeID uint) error {
	pushUpgrades.Lock()
	defer pushUpgrades.Unlock()
	if st, ok := pushUpgrades.nodes[nodeID]; ok && (st.Stage == pushStagePreparing || st.Stage == pushStagePushing) {
		return fmt.Errorf("主节点正在向该节点下发 %s，请稍后", st.Version)
	}
	delete(pushUpgrades.nodes, nodeID)
	return nil
}

func failPushUpgrade(nodeID uint, err error) {
	if global.LOG != nil {
		global.LOG.Errorf("push upgrade to node %d failed: %v", nodeID, err)
	}
	setPushState(nodeID, func(s *pushUpgradeState) {
		s.Stage = pushStageFailed
		s.Error = truncateErr(err.Error())
	})
}

// remoteNodeArch maps the node's kernel arch to the release asset arch.
func (u *ClusterNodeService) remoteNodeArch(node model.ClusterNode) (string, error) {
	var info struct {
		KernelArch string `json:"kernelArch"`
	}
	if err := u.callRemoteJSON(node, http.MethodGet, "/api/v2/dashboard/base/os", nil, &info, 20*time.Second); err != nil {
		return "", err
	}
	return normalizeReleaseArch(info.KernelArch)
}

func normalizeReleaseArch(kernelArch string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kernelArch)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	case "armv7l", "armv7":
		return "armv7", nil
	case "ppc64le":
		return "ppc64le", nil
	case "s390x":
		return "s390x", nil
	case "riscv64":
		return "riscv64", nil
	}
	return "", fmt.Errorf("unsupported node arch %q", kernelArch)
}

// remotePackageDir returns the node's push directory; "" means the node
// panel predates master distribution.
func (u *ClusterNodeService) remotePackageDir(node model.ClusterNode) (string, error) {
	var status struct {
		PackageDir string `json:"packageDir"`
	}
	if err := u.callRemoteJSON(node, http.MethodGet, "/api/v2/core/settings/upgrade/status", nil, &status, 15*time.Second); err != nil {
		return "", err
	}
	return strings.TrimSpace(status.PackageDir), nil
}

// startPushUpgrade checks what can be checked quickly, then prepares and
// pushes the package in the background.
func (u *ClusterNodeService) startPushUpgrade(node model.ClusterNode, target string) error {
	arch, err := u.remoteNodeArch(node)
	if err != nil {
		return fmt.Errorf("读取节点架构失败: %v", err)
	}
	pkgDir, err := u.remotePackageDir(node)
	if err != nil {
		return fmt.Errorf("读取节点更新状态失败: %v", err)
	}
	if pkgDir == "" {
		return fmt.Errorf("节点面板版本过旧，不支持主节点下发，请选择「节点直连下载」完成本次更新")
	}
	if _, _, err := forkPackageURL(target, arch); err != nil {
		return err
	}
	if err := claimPushUpgrade(node.ID, target); err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pushUpgradeTimeout)
		defer cancel()
		if err := u.runPushUpgrade(ctx, node, target, arch, pkgDir); err != nil {
			failPushUpgrade(node.ID, err)
		}
	}()
	return nil
}

func (u *ClusterNodeService) runPushUpgrade(ctx context.Context, node model.ClusterNode, target, arch, pkgDir string) error {
	progress := func(done, total int64) {
		setPushState(node.ID, func(s *pushUpgradeState) { s.Done, s.Total = done, total })
	}
	pkgPath, sum, err := cachedUpgradePackage(target, arch, progress)
	if err != nil {
		return fmt.Errorf("主节点下载更新包失败: %v", err)
	}

	select {
	case pushUpgradeSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-pushUpgradeSlots }()

	setPushState(node.ID, func(s *pushUpgradeState) { s.Stage, s.Done, s.Total = pushStagePushing, 0, 0 })
	fileName := path.Base(pkgPath)
	syncSvc := &ClusterSyncService{nodes: u}
	if err := syncSvc.uploadRemoteFile(ctx, node, pkgDir, fileName, pkgPath, true, progress); err != nil {
		return fmt.Errorf("下发更新包到节点失败: %v", err)
	}
	body := map[string]string{
		"version":       target,
		"packagePath":   path.Join(pkgDir, fileName),
		"packageSha256": sum,
	}
	if err := u.callRemoteJSON(node, http.MethodPost, "/api/v2/core/settings/upgrade", body, nil, 60*time.Second); err != nil {
		return fmt.Errorf("节点启动更新失败: %v", err)
	}
	setPushState(node.ID, func(s *pushUpgradeState) { s.Stage = pushStageDone })
	return nil
}

// ---- Master-side package cache ----------------------------------------

type cachedPackage struct {
	once sync.Once
	path string
	sum  string
	err  error
	// progress listeners of every node waiting for this download
	mu        sync.Mutex
	listeners []func(done, total int64)
}

var upgradePackageCache = struct {
	sync.Mutex
	items map[string]*cachedPackage
}{items: map[string]*cachedPackage{}}

func upgradeCacheDir() string {
	return path.Join(global.CONF.Base.InstallDir, "1panel/tmp/upgrade/cache")
}

// cachedUpgradePackage returns the verified package for tag / arch and its
// sha256, downloading it at most once at a time. A failed download is not
// cached, so the next request retries.
func cachedUpgradePackage(tag, arch string, progress func(done, total int64)) (string, string, error) {
	fileName := forkPackageFileName(tag, arch)
	key := tag + "/" + fileName

	upgradePackageCache.Lock()
	item, ok := upgradePackageCache.items[key]
	if !ok {
		item = &cachedPackage{}
		upgradePackageCache.items[key] = item
	}
	upgradePackageCache.Unlock()

	if progress != nil {
		item.mu.Lock()
		item.listeners = append(item.listeners, progress)
		item.mu.Unlock()
	}
	item.once.Do(func() {
		item.path, item.sum, item.err = prepareUpgradePackage(tag, arch, fileName, func(done, total int64) {
			item.mu.Lock()
			ls := append([]func(int64, int64){}, item.listeners...)
			item.mu.Unlock()
			for _, l := range ls {
				l(done, total)
			}
		})
		// The entry only joins concurrent requests; later ones reuse the
		// verified file on disk (or download again after a failure).
		upgradePackageCache.Lock()
		if upgradePackageCache.items[key] == item {
			delete(upgradePackageCache.items, key)
		}
		upgradePackageCache.Unlock()
	})
	return item.path, item.sum, item.err
}

func prepareUpgradePackage(tag, arch, fileName string, progress func(done, total int64)) (string, string, error) {
	dir := path.Join(upgradeCacheDir(), tag)
	pkgPath := path.Join(dir, fileName)
	sumPath := pkgPath + ".sha256"

	// Reuse a package verified by an earlier run (survives a restart).
	if raw, err := os.ReadFile(sumPath); err == nil {
		want := strings.TrimSpace(string(raw))
		if got, err := fileSha256(pkgPath); err == nil && got == want {
			if info, err := os.Stat(pkgPath); err == nil && progress != nil {
				progress(info.Size(), info.Size())
			}
			return pkgPath, got, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	_ = os.Remove(sumPath)
	url, _, err := forkPackageURL(tag, arch)
	if err != nil {
		return "", "", err
	}
	if err := downloadForkPackage(url, pkgPath, nil, progress); err != nil {
		_ = os.Remove(pkgPath)
		return "", "", err
	}
	if err := verifyForkPackageChecksum(tag, fileName, pkgPath, nil); err != nil {
		_ = os.Remove(pkgPath)
		return "", "", err
	}
	sum, err := fileSha256(pkgPath)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(sumPath, []byte(sum+"\n"), 0o600); err != nil {
		return "", "", err
	}
	pruneUpgradeCache(tag)
	return pkgPath, sum, nil
}

// pruneUpgradeCache drops cached packages of versions older than keep.
func pruneUpgradeCache(keep string) {
	entries, err := os.ReadDir(upgradeCacheDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && compareForkVersion(e.Name(), keep) < 0 {
			_ = os.RemoveAll(path.Join(upgradeCacheDir(), e.Name()))
		}
	}
}
