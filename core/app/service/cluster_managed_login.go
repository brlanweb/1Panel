package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"golang.org/x/sync/errgroup"
)

// Managed login lock, master side.
//
// The master switches the web login of a node panel on/off through
// /api/v2/core/cluster/managed/login on that node. The node accepts the
// call only with API-key authentication, so a user logged into the node
// itself can never unlock it from the browser.

const managedLoginPath = "/api/v2/core/cluster/managed/login"

// managedLoginMasterName is what a locked node shows on its login page.
func managedLoginMasterName() string {
	settingRepo := repo.NewISettingRepo()
	if name, _ := settingRepo.GetValueByKey("PanelName"); strings.TrimSpace(name) != "" {
		if ip, _ := settingRepo.GetValueByKey("SystemIP"); strings.TrimSpace(ip) != "" {
			return fmt.Sprintf("%s (%s)", strings.TrimSpace(name), strings.TrimSpace(ip))
		}
		return strings.TrimSpace(name)
	}
	ip, _ := settingRepo.GetValueByKey("SystemIP")
	return strings.TrimSpace(ip)
}

// readNodeManagedLogin returns the lock state of a node; supported=false
// when the node is too old to have the endpoint.
func (u *ClusterNodeService) readNodeManagedLogin(node model.ClusterNode) (auth.ManagedLoginState, error) {
	var state auth.ManagedLoginState
	if err := u.callRemoteJSON(node, http.MethodGet, managedLoginPath, nil, &state, 5*time.Second); err != nil {
		if isMissingRouteErr(err) {
			return auth.ManagedLoginState{Supported: false}, nil
		}
		return state, err
	}
	state.Supported = true
	return state, nil
}

func (u *ClusterNodeService) writeNodeManagedLogin(node model.ClusterNode, req dto.ClusterManagedLoginReq) (auth.ManagedLoginState, error) {
	var state auth.ManagedLoginState
	if err := u.callRemoteJSON(node, http.MethodPost, managedLoginPath, req, &state, 10*time.Second); err != nil {
		if isMissingRouteErr(err) {
			return auth.ManagedLoginState{Supported: false}, fmt.Errorf("节点面板版本过低，不支持登录锁定，请先更新节点面板")
		}
		return state, err
	}
	state.Supported = true
	return state, nil
}

func persistNodeManagedLogin(nodeID uint, state auth.ManagedLoginState) {
	if !state.Supported {
		_ = clusterNodeRepo.Update(nodeID, map[string]interface{}{"login_locked": nil, "login_unlock_until": 0})
		return
	}
	_ = clusterNodeRepo.Update(nodeID, map[string]interface{}{
		"login_locked":       state.Locked,
		"login_unlock_until": state.UnlockUntil,
	})
}

// SetNodesLoginLock applies the lock change to every requested node and
// reports a per-node result; one failing node never aborts the others.
func (u *ClusterNodeService) SetNodesLoginLock(req dto.ClusterNodeLoginLockReq) ([]dto.ClusterNodeLoginLockResult, error) {
	results := make([]dto.ClusterNodeLoginLockResult, len(req.NodeIDs))
	master := managedLoginMasterName()
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(5)
	for i, id := range req.NodeIDs {
		i, id := i, id
		group.Go(func() error {
			item := dto.ClusterNodeLoginLockResult{NodeID: id}
			node, _ := clusterNodeRepo.Get(repo.WithByID(id))
			if node.ID == 0 {
				item.Message = "节点不存在"
				results[i] = item
				return nil
			}
			item.Name = node.Name
			state, err := u.writeNodeManagedLogin(node, dto.ClusterManagedLoginReq{
				Locked:        req.Locked,
				UnlockMinutes: req.UnlockMinutes,
				Master:        master,
			})
			item.Supported = state.Supported
			if err != nil {
				item.Message = truncateErr(err.Error())
				results[i] = item
				return nil
			}
			persistNodeManagedLogin(node.ID, state)
			item.Success = true
			item.Locked = state.Locked
			item.UnlockUntil = state.UnlockUntil
			results[i] = item
			return nil
		})
	}
	_ = group.Wait()
	return results, nil
}

// refreshNodeManagedLogin is called after a successful health check.
func (u *ClusterNodeService) refreshNodeManagedLogin(node model.ClusterNode) {
	state, err := u.readNodeManagedLogin(node)
	if err != nil {
		return
	}
	persistNodeManagedLogin(node.ID, state)
}

// releaseNodeManagedLogin removes the lock before a node is deleted from
// the cluster. It returns a hint when the node stays locked.
func (u *ClusterNodeService) releaseNodeManagedLogin(node model.ClusterNode) error {
	if node.LoginLocked == nil || (!*node.LoginLocked && node.LoginUnlockUntil == 0) {
		// Never locked as far as we know; still try, but ignore failures.
		_, _ = u.writeNodeManagedLogin(node, dto.ClusterManagedLoginReq{Locked: false})
		return nil
	}
	if _, err := u.writeNodeManagedLogin(node, dto.ClusterManagedLoginReq{Locked: false}); err != nil {
		return err
	}
	return nil
}
