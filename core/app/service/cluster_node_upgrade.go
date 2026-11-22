package service

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
)

// Remote panel version / disk / upgrade for the cluster overview.
//
// Every fork build exposes the same upgrade API as the master
// (/api/v2/core/settings/upgrade and .../upgrade/status), reachable with the
// node's API key, so the master only has to forward the request: the node
// downloads the package from the fork release itself, verifies it and
// restarts. Nothing is pushed from the master.

// remoteNodeVersion reads the panel version of a node (SystemVersion of its
// agent settings, the same field the node's own footer shows).
func (u *ClusterNodeService) remoteNodeVersion(node model.ClusterNode) (string, error) {
	var setting struct {
		SystemVersion string `json:"systemVersion"`
	}
	if err := u.callRemoteJSON(node, http.MethodPost, "/api/v2/settings/search", map[string]interface{}{}, &setting, 10*time.Second); err != nil {
		return "", err
	}
	return strings.TrimSpace(setting.SystemVersion), nil
}

type remoteDisk struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"usedPercent"`
}

// remoteNodeDisks reads the disk list of a node from the full dashboard
// endpoint (the lightweight /dashboard/current/node has no disk data).
// io/net options are "all"; their counters are ignored here.
func (u *ClusterNodeService) remoteNodeDisks(node model.ClusterNode) ([]dto.ClusterDiskInfo, error) {
	var current struct {
		DiskData []remoteDisk `json:"diskData"`
	}
	if err := u.callRemoteJSON(node, http.MethodGet, "/api/v2/dashboard/current/all/all", nil, &current, 15*time.Second); err != nil {
		return nil, err
	}
	return normalizeDisks(current.DiskData), nil
}

func normalizeDisks(list []remoteDisk) []dto.ClusterDiskInfo {
	disks := make([]dto.ClusterDiskInfo, 0, len(list))
	for _, item := range list {
		if item.Total == 0 {
			continue
		}
		disks = append(disks, dto.ClusterDiskInfo{Path: item.Path, Total: item.Total, Used: item.Used, UsedPercent: item.UsedPercent})
	}
	// "/" first, then the larger mounts.
	sort.SliceStable(disks, func(i, j int) bool {
		if (disks[i].Path == "/") != (disks[j].Path == "/") {
			return disks[i].Path == "/"
		}
		return disks[i].Total > disks[j].Total
	})
	return disks
}

// primaryDisk picks the summary disk: the root mount, or the largest one.
func primaryDisk(disks []dto.ClusterDiskInfo) (dto.ClusterDiskInfo, bool) {
	if len(disks) == 0 {
		return dto.ClusterDiskInfo{}, false
	}
	return disks[0], true
}

// fillNodeExtras adds version and disk data to a snapshot. Both are best
// effort: a failure leaves the stored values and never marks the node as
// failed (older nodes or a slow disk scan must not hide CPU/memory).
func (u *ClusterNodeService) fillNodeExtras(node model.ClusterNode, snap *dto.ClusterNodeSnapshot) map[string]interface{} {
	upMap := map[string]interface{}{}
	snap.Version = node.Version
	snap.DiskUsedPercent, snap.DiskTotal, snap.DiskUsed = node.DiskUsedPercent, node.DiskTotal, node.DiskUsed
	var (
		version    string
		versionErr error
		disks      []dto.ClusterDiskInfo
		disksErr   error
		wg         sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); version, versionErr = u.remoteNodeVersion(node) }()
	go func() { defer wg.Done(); disks, disksErr = u.remoteNodeDisks(node) }()
	wg.Wait()
	if versionErr == nil && version != "" {
		snap.Version = version
		upMap["version"] = version
	}
	if disksErr == nil {
		snap.Disks = disks
		if disk, ok := primaryDisk(disks); ok {
			snap.DiskUsedPercent, snap.DiskTotal, snap.DiskUsed = disk.UsedPercent, disk.Total, disk.Used
			upMap["disk_used_percent"] = disk.UsedPercent
			upMap["disk_total"] = disk.Total
			upMap["disk_used"] = disk.Used
		}
	}
	return upMap
}

// latestForkVersion is the newest published stable fork release, "" when
// the release channel is unreachable (cached, see loadForkReleases).
func latestForkVersion() string {
	releases, err := loadForkReleases()
	if err != nil {
		logForkReleaseError(err)
		return ""
	}
	for _, item := range releases {
		if !item.Prerelease {
			return item.TagName
		}
	}
	return ""
}

func (u *ClusterNodeService) getClusterNode(id uint) (model.ClusterNode, error) {
	node, _ := clusterNodeRepo.Get(repo.WithByID(id))
	if node.ID == 0 {
		return node, buserr.New("ErrRecordNotFound")
	}
	return node, nil
}

// UpgradeNode asks a node to upgrade its panel to the given (or newest)
// fork release. The node runs the same upgrade as its own footer button.
func (u *ClusterNodeService) UpgradeNode(req dto.ClusterNodeUpgradeReq) error {
	node, err := u.getClusterNode(req.ID)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(req.Version)
	if target == "" {
		resetForkReleaseCache()
		target = latestForkVersion()
		if target == "" {
			return fmt.Errorf("无法获取最新版本，请稍后重试")
		}
	}
	current, err := u.remoteNodeVersion(node)
	if err != nil {
		return fmt.Errorf("读取节点版本失败: %v", err)
	}
	if current != "" && compareForkVersion(target, current) <= 0 {
		return fmt.Errorf("节点已是最新版本 %s", current)
	}
	if err := u.callRemoteJSON(node, http.MethodPost, "/api/v2/core/settings/upgrade", map[string]string{"version": target}, nil, 30*time.Second); err != nil {
		return fmt.Errorf("节点启动更新失败: %v", err)
	}
	return nil
}

// NodeUpgradeStatus polls the node's upgrade progress. While the node
// restarts after installing it is unreachable; that is reported as
// Reachable=false instead of an error so the page keeps polling.
func (u *ClusterNodeService) NodeUpgradeStatus(id uint) (dto.ClusterNodeUpgradeStatus, error) {
	node, err := u.getClusterNode(id)
	if err != nil {
		return dto.ClusterNodeUpgradeStatus{}, err
	}
	result := dto.ClusterNodeUpgradeStatus{ID: id}
	var status dto.UpgradeStatus
	if err := u.callRemoteJSON(node, http.MethodGet, "/api/v2/core/settings/upgrade/status", nil, &status, 10*time.Second); err != nil {
		result.Error = truncateErr(err.Error())
		return result, nil
	}
	result.Reachable = true
	result.Stage = status.Stage
	result.Version = status.Version
	result.CurrentVersion = status.CurrentVersion
	result.Done, result.Total = status.Done, status.Total
	result.Error = status.Error
	if status.CurrentVersion != "" && status.CurrentVersion != node.Version {
		_ = clusterNodeRepo.Update(node.ID, map[string]interface{}{"version": status.CurrentVersion})
	}
	return result, nil
}
