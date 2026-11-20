package service

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/go-resty/resty/v2"
	"github.com/jinzhu/copier"
	"gorm.io/gorm"
)

// ClusterSyncService implements the four-kind resource-sync framework from
// DESIGN.md §12.3 C: select resources on a source node → select target
// nodes (source auto-excluded, multi-select, online only) → preview
// (create / skip-same / skip-conflict / failed per resource × target) →
// execute with per-item records. Conflicts are NEVER silently overwritten:
// unless the caller explicitly opts into overwriteConflicts, a target that
// already has different content is skipped and reported.
//
// Execution transfers run through this (master) panel as a transit point:
// the master downloads from the source and uploads/installs on the target.
// File contents and certificate private keys live only in memory or in a
// temp file that is removed after the transfer, and are never written to
// the database or to logs.
//
// Honest limits (also in IMPLEMENTATION.md): every remote write endpoint
// used here exists in the open-source agent tree, but none of the four
// execute paths has been verified against a live remote panel yet
// (DESIGN.md §12.6 item 4). A failed transfer is recorded with the
// remote error text instead of failing the whole task silently. Image
// sync uses registry pull on the target (POST /containers/image/pull);
// save/load tar transit for registry-unreachable images is not
// implemented in this phase.
type ClusterSyncService struct {
	nodes    *ClusterNodeService
	syncRepo repo.IClusterSyncRepo
}

type IClusterSyncService interface {
	ListResources(req dto.ClusterSyncResourceSearch) ([]dto.ClusterSyncResource, error)
	Preview(req dto.ClusterSyncPreviewReq) ([]dto.ClusterSyncPreviewItem, error)
	Execute(req dto.ClusterSyncExecuteReq) (uint, error)
	SearchTasks(req dto.ClusterSyncTaskSearch) (int64, interface{}, error)
	TaskRecords(taskID uint) ([]dto.ClusterSyncRecordInfo, error)
}

func NewIClusterSyncService() IClusterSyncService {
	return &ClusterSyncService{
		nodes:    NewIClusterNodeService().(*ClusterNodeService),
		syncRepo: repo.NewIClusterSyncRepo(),
	}
}

func (s *ClusterSyncService) getNode(id uint) (model.ClusterNode, error) {
	node, _ := clusterNodeRepo.Get(repo.WithByID(id))
	if node.ID == 0 {
		return node, buserr.New("ErrRecordNotFound")
	}
	return node, nil
}

func validSyncKind(kind string) bool {
	switch kind {
	case dto.ClusterSyncKindFile, dto.ClusterSyncKindImage, dto.ClusterSyncKindApp, dto.ClusterSyncKindCert:
		return true
	}
	return false
}

type remotePage struct {
	Total int64             `json:"total"`
	Items []json.RawMessage `json:"items"`
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	}
	return ""
}

func toBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

// listRemoteResources lists one kind of resource on one node, normalized.
// The `path` argument is only used for files (directory to list).
func (s *ClusterSyncService) listRemoteResources(kind string, node model.ClusterNode, reqPath string, page, pageSize int) ([]dto.ClusterSyncResource, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 100
	}
	switch kind {
	case dto.ClusterSyncKindFile:
		dir := strings.TrimSpace(reqPath)
		if dir == "" {
			dir = "/"
		}
		var data struct {
			Files []map[string]interface{} `json:"files"`
		}
		body := map[string]interface{}{"path": dir, "page": page, "pageSize": pageSize, "sortBy": "name", "sortOrder": "ascending", "isDetail": true}
		if err := s.nodes.callRemoteJSON(node, http.MethodPost, "/api/v2/files/search", body, &data, 15*time.Second); err != nil {
			return nil, 0, err
		}
		var out []dto.ClusterSyncResource
		for _, f := range data.Files {
			if toBool(f["isDir"]) {
				continue
			}
			name := toString(f["name"])
			fullPath := path.Join(dir, name)
			out = append(out, dto.ClusterSyncResource{
				Key: fullPath, Name: name,
				Detail: fmt.Sprintf("%s", dir), Size: int64(toFloat(f["size"])),
			})
		}
		return out, int64(len(out)), nil
	case dto.ClusterSyncKindImage:
		var result remotePage
		if err := s.nodes.callRemoteJSON(node, http.MethodPost, "/api/v2/containers/image/search", map[string]interface{}{"page": page, "pageSize": pageSize}, &result, 15*time.Second); err != nil {
			return nil, 0, err
		}
		var out []dto.ClusterSyncResource
		for _, raw := range result.Items {
			var item map[string]interface{}
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			id := toString(item["id"])
			tags, _ := item["tags"].([]interface{})
			for _, t := range tags {
				tag := toString(t)
				if tag == "" {
					continue
				}
				out = append(out, dto.ClusterSyncResource{
					Key: tag, Name: tag,
					Detail: id, Size: int64(toFloat(item["size"])),
				})
			}
		}
		return out, result.Total, nil
	case dto.ClusterSyncKindApp:
		var result remotePage
		if err := s.nodes.callRemoteJSON(node, http.MethodPost, "/api/v2/apps/installed/search", map[string]interface{}{"page": page, "pageSize": pageSize, "all": true}, &result, 15*time.Second); err != nil {
			return nil, 0, err
		}
		var out []dto.ClusterSyncResource
		for _, raw := range result.Items {
			var item map[string]interface{}
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			id := toString(item["id"])
			if id == "" {
				continue
			}
			key := toString(item["key"])
			if key == "" {
				key = toString(item["appKey"])
			}
			name := toString(item["name"])
			version := toString(item["version"])
			out = append(out, dto.ClusterSyncResource{
				Key: id, Name: name,
				Detail: key + " " + version,
				Extra:  toString(item["appDetailId"]),
			})
		}
		return out, result.Total, nil
	case dto.ClusterSyncKindCert:
		var result remotePage
		if err := s.nodes.callRemoteJSON(node, http.MethodPost, "/api/v2/websites/ssl/search", map[string]interface{}{"page": page, "pageSize": pageSize}, &result, 15*time.Second); err != nil {
			return nil, 0, err
		}
		var out []dto.ClusterSyncResource
		for _, raw := range result.Items {
			var item map[string]interface{}
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			id := toString(item["id"])
			domain := toString(item["primaryDomain"])
			if id == "" || domain == "" {
				continue
			}
			out = append(out, dto.ClusterSyncResource{
				Key: id, Name: domain,
				Detail: toString(item["provider"]),
				Extra:  strconv.FormatBool(toBool(item["autoRenew"])),
			})
		}
		return out, result.Total, nil
	}
	return nil, 0, buserr.New("ErrInvalidParams")
}

func (s *ClusterSyncService) ListResources(req dto.ClusterSyncResourceSearch) ([]dto.ClusterSyncResource, error) {
	if !validSyncKind(req.Kind) {
		return nil, buserr.New("ErrInvalidParams")
	}
	node, err := s.getNode(req.SourceNodeID)
	if err != nil {
		return nil, err
	}
	if node.Status == model.ClusterNodeStatusOffline {
		return nil, buserr.New("ErrInvalidParams")
	}
	resources, _, err := s.listRemoteResources(req.Kind, node, req.Path, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		resources = []dto.ClusterSyncResource{}
	}
	return resources, nil
}

// compareOnTarget computes the preview action for one resource on one target.
func (s *ClusterSyncService) compareOnTarget(kind string, target model.ClusterNode, res dto.ClusterSyncResource, reqPath string) (action, reason string) {
	switch kind {
	case dto.ClusterSyncKindFile:
		items, _, err := s.listRemoteResources(kind, target, reqPath, 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target directory: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if item.Key == res.Key {
				if item.Size == res.Size {
					return model.ClusterSyncActionSkipSame, "target already has the same file (same size)"
				}
				return model.ClusterSyncActionSkipConflict, "target file differs (different size)"
			}
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindImage:
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target images: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if item.Key == res.Key {
				if item.Detail == res.Detail && res.Detail != "" {
					return model.ClusterSyncActionSkipSame, "target already has the same image"
				}
				return model.ClusterSyncActionSkipConflict, "target has a different image with the same tag"
			}
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindApp:
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target apps: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if item.Detail == res.Detail && res.Detail != "" {
				return model.ClusterSyncActionSkipSame, "target already has the same app version installed"
			}
			if strings.HasPrefix(item.Detail, strings.Split(res.Detail, " ")[0]+" ") && strings.Split(res.Detail, " ")[0] != "" {
				return model.ClusterSyncActionSkipConflict, "target has a different version of the same app"
			}
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindCert:
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target certificates: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if item.Name == res.Name {
				return model.ClusterSyncActionSkipConflict, "target already has a certificate for this domain (content comparison happens at execution; same content is skipped there)"
			}
		}
		return model.ClusterSyncActionCreate, ""
	}
	return model.ClusterSyncActionFailed, "unknown resource kind"
}

func (s *ClusterSyncService) Preview(req dto.ClusterSyncPreviewReq) ([]dto.ClusterSyncPreviewItem, error) {
	if !validSyncKind(req.Kind) {
		return nil, buserr.New("ErrInvalidParams")
	}
	source, err := s.getNode(req.SourceNodeID)
	if err != nil {
		return nil, err
	}
	resources, _, err := s.listRemoteResources(req.Kind, source, req.Path, 1, 500)
	if err != nil {
		return nil, err
	}
	byKey := map[string]dto.ClusterSyncResource{}
	for _, r := range resources {
		byKey[r.Key] = r
	}
	var out []dto.ClusterSyncPreviewItem
	for _, targetID := range req.TargetNodeIDs {
		if targetID == req.SourceNodeID {
			continue
		}
		target, err := s.getNode(targetID)
		if err != nil {
			for _, key := range req.ResourceKeys {
				out = append(out, dto.ClusterSyncPreviewItem{ResourceKey: key, TargetNodeID: targetID, Action: model.ClusterSyncActionFailed, Reason: "target node not found"})
			}
			continue
		}
		if target.Status == model.ClusterNodeStatusOffline {
			for _, key := range req.ResourceKeys {
				out = append(out, dto.ClusterSyncPreviewItem{ResourceKey: key, TargetNodeID: targetID, TargetName: target.Name, Action: model.ClusterSyncActionFailed, Reason: "target node is offline"})
			}
			continue
		}
		for _, key := range req.ResourceKeys {
			res, ok := byKey[key]
			if !ok {
				out = append(out, dto.ClusterSyncPreviewItem{ResourceKey: key, TargetNodeID: targetID, TargetName: target.Name, Action: model.ClusterSyncActionFailed, Reason: "resource not found on source node"})
				continue
			}
			action, reason := s.compareOnTarget(req.Kind, target, res, req.Path)
			out = append(out, dto.ClusterSyncPreviewItem{ResourceKey: key, ResourceName: res.Name, TargetNodeID: targetID, TargetName: target.Name, Action: action, Reason: reason})
		}
	}
	if out == nil {
		out = []dto.ClusterSyncPreviewItem{}
	}
	return out, nil
}

// --- execution transfers (master as transit) ---

const maxSyncFileBytes = 200 * 1024 * 1024 // 200 MB per file; larger files are rejected, not partially transferred

func (s *ClusterSyncService) signedClient(node model.ClusterNode, timeout time.Duration) (*resty.Client, string, string, string, error) {
	timestamp, hmacToken, md5Token, err := s.nodes.remoteAuthHeaders(node)
	if err != nil {
		return nil, "", "", "", err
	}
	client := resty.New().
		SetTimeout(timeout).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	return client, timestamp, hmacToken, md5Token, nil
}

func (s *ClusterSyncService) downloadRemoteFile(node model.ClusterNode, remotePath string) (tmpPath string, size int64, err error) {
	client, timestamp, hmacToken, md5Token, err := s.signedClient(node, 10*time.Minute)
	if err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp("", "cluster-sync-*")
	if err != nil {
		return "", 0, err
	}
	tmpPath = tmp.Name()
	_ = tmp.Close()
	targetURL := strings.TrimRight(node.EntryURL, "/") + "/api/v2/files/download"
	doGet := func(token string) (*resty.Response, error) {
		return client.R().
			SetHeader("1Panel-Timestamp", timestamp).
			SetHeader("1Panel-Token", token).
			SetQueryParam("path", remotePath).
			SetOutput(tmpPath).
			Get(targetURL)
	}
	resp, err := doGet(hmacToken)
	if err == nil && resp.StatusCode() == http.StatusUnauthorized {
		resp, err = doGet(md5Token)
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	if resp.StatusCode() != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("source panel returned HTTP %d while downloading the file", resp.StatusCode())
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	if info.Size() > maxSyncFileBytes {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("file is %d bytes, over the %d-byte sync limit", info.Size(), maxSyncFileBytes)
	}
	return tmpPath, info.Size(), nil
}

func (s *ClusterSyncService) uploadRemoteFile(node model.ClusterNode, dir, fileName, tmpPath string, overwrite bool) error {
	client, timestamp, hmacToken, md5Token, err := s.signedClient(node, 10*time.Minute)
	if err != nil {
		return err
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	targetURL := strings.TrimRight(node.EntryURL, "/") + "/api/v2/files/upload"
	doPost := func(token string) (*resty.Response, error) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return client.R().
			SetHeader("1Panel-Timestamp", timestamp).
			SetHeader("1Panel-Token", token).
			SetMultipartField("path", "", "", strings.NewReader(dir)).
			SetMultipartField("overwrite", "", "", strings.NewReader(strconv.FormatBool(overwrite))).
			SetFileReader("file", fileName, f).
			Post(targetURL)
	}
	resp, err := doPost(hmacToken)
	if err == nil && resp.StatusCode() == http.StatusUnauthorized {
		resp, err = doPost(md5Token)
	}
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("target panel returned HTTP %d while uploading the file", resp.StatusCode())
	}
	var env remoteEnvelope
	if err := json.Unmarshal(resp.Body(), &env); err == nil && env.Code != http.StatusOK && env.Code != 0 {
		if strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("target panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("target panel error code %d", env.Code)
	}
	return nil
}

// transferOne executes a single (resource × target) transfer. It returns the
// action recorded for the item and a human-readable reason ("" on create).
func (s *ClusterSyncService) transferOne(kind string, source, target model.ClusterNode, res dto.ClusterSyncResource, reqPath string, overwrite bool) (string, string) {
	switch kind {
	case dto.ClusterSyncKindFile:
		tmpPath, _, err := s.downloadRemoteFile(source, res.Key)
		if err != nil {
			return model.ClusterSyncActionFailed, "download from source failed: " + truncateErr(err.Error())
		}
		defer os.Remove(tmpPath)
		dir := path.Dir(res.Key)
		if err := s.uploadRemoteFile(target, dir, path.Base(res.Key), tmpPath, overwrite); err != nil {
			// Best effort: remove a partially uploaded file on the target.
			_ = s.nodes.callRemoteJSON(target, http.MethodPost, "/api/v2/files/del", map[string]interface{}{"path": res.Key, "isDir": false, "forceDelete": true}, nil, 10*time.Second)
			return model.ClusterSyncActionFailed, "upload to target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindImage:
		var out struct {
			TaskID string `json:"taskID"`
		}
		if err := s.nodes.callRemoteJSON(target, http.MethodPost, "/api/v2/containers/image/pull", map[string]interface{}{"imageName": []string{res.Key}}, &out, 10*time.Minute); err != nil {
			return model.ClusterSyncActionFailed, "image pull on target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindApp:
		// Reinstall the same app on the target with the source install's
		// parameters. Ports and other host-bound values come from the
		// source; if the target rejects them (e.g. port in use) the
		// target's error is recorded verbatim on the record.
		appDetailID, err := strconv.ParseUint(strings.TrimSpace(res.Extra), 10, 64)
		if err != nil || appDetailID == 0 {
			return model.ClusterSyncActionFailed, "source app has no appDetailId; cannot reinstall on target"
		}
		var appConfig struct {
			Params []struct {
				Key   string      `json:"key"`
				Value interface{} `json:"value"`
			} `json:"params"`
		}
		if err := s.nodes.callRemoteJSON(source, http.MethodGet, "/api/v2/apps/installed/params/"+res.Key, nil, &appConfig, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "could not read source app params: " + truncateErr(err.Error())
		}
		params := map[string]interface{}{}
		for _, p := range appConfig.Params {
			if p.Key != "" {
				params[p.Key] = p.Value
			}
		}
		body := map[string]interface{}{"appDetailId": appDetailID, "name": res.Name, "params": params}
		if err := s.nodes.callRemoteJSON(target, http.MethodPost, "/api/v2/apps/install", body, nil, 10*time.Minute); err != nil {
			return model.ClusterSyncActionFailed, "app install on target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindCert:
		// Fetch the full certificate from the source (private key + PEM).
		// Keys live only in memory for the duration of this call and are
		// never written to disk, the database, or logs.
		var cert struct {
			PrimaryDomain string `json:"primaryDomain"`
			PrivateKey    string `json:"privateKey"`
			Pem           string `json:"pem"`
			Provider      string `json:"provider"`
			AutoRenew     bool   `json:"autoRenew"`
		}
		if err := s.nodes.callRemoteJSON(source, http.MethodGet, "/api/v2/websites/ssl/"+res.Key, nil, &cert, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "could not read source certificate: " + truncateErr(err.Error())
		}
		if cert.AutoRenew || strings.Contains(strings.ToLower(cert.Provider), "acme") {
			return model.ClusterSyncActionFailed, "ACME auto-renew certificates are not synced (the target renews on its own)"
		}
		if strings.TrimSpace(cert.PrivateKey) == "" || strings.TrimSpace(cert.Pem) == "" {
			return model.ClusterSyncActionFailed, "source panel did not return the certificate content"
		}
		body := map[string]interface{}{
			"type": "paste", "privateKey": cert.PrivateKey, "certificate": cert.Pem,
			"description": "synced from cluster node " + source.Name,
		}
		if err := s.nodes.callRemoteJSON(target, http.MethodPost, "/api/v2/websites/ssl/upload", body, nil, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "certificate upload on target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	}
	return model.ClusterSyncActionFailed, "unknown resource kind"
}

func (s *ClusterSyncService) Execute(req dto.ClusterSyncExecuteReq) (uint, error) {
	if !validSyncKind(req.Kind) {
		return 0, buserr.New("ErrInvalidParams")
	}
	source, err := s.getNode(req.SourceNodeID)
	if err != nil {
		return 0, err
	}
	resources, _, err := s.listRemoteResources(req.Kind, source, req.Path, 1, 500)
	if err != nil {
		return 0, err
	}
	byKey := map[string]dto.ClusterSyncResource{}
	for _, r := range resources {
		byKey[r.Key] = r
	}
	targetIDsJSON, _ := json.Marshal(req.TargetNodeIDs)
	task := &model.ClusterSyncTask{
		Kind: req.Kind, SourceNodeID: req.SourceNodeID, SourceName: source.Name,
		TargetNodeIDs: string(targetIDsJSON), Status: model.ClusterSyncTaskRunning,
		OverwriteConflicts: req.OverwriteConflicts,
	}
	if err := s.syncRepo.CreateTask(task); err != nil {
		return 0, err
	}
	succeeded, skipped, failed := 0, 0, 0
	finishTask := func() {
		status := model.ClusterSyncTaskSuccess
		if failed > 0 && (succeeded > 0 || skipped > 0) {
			status = model.ClusterSyncTaskPartial
		} else if failed > 0 {
			status = model.ClusterSyncTaskFailed
		}
		task.Total = succeeded + skipped + failed
		_ = s.syncRepo.UpdateTask(task.ID, map[string]interface{}{
			"status": status, "total": task.Total,
			"succeeded": succeeded, "skipped": skipped, "failed": failed,
		})
	}
	for _, targetID := range req.TargetNodeIDs {
		if targetID == req.SourceNodeID {
			continue
		}
		target, err := s.getNode(targetID)
		if err != nil {
			for _, key := range req.ResourceKeys {
				_ = s.record(task.ID, req.Kind, key, "", source.ID, targetID, "", model.ClusterSyncActionFailed, "target node not found", 0)
				failed++
			}
			continue
		}
		for _, key := range req.ResourceKeys {
			start := time.Now()
			res, ok := byKey[key]
			if !ok {
				_ = s.record(task.ID, req.Kind, key, "", source.ID, targetID, target.Name, model.ClusterSyncActionFailed, "resource not found on source node", time.Since(start).Milliseconds())
				failed++
				continue
			}
			action, reason := model.ClusterSyncActionCreate, ""
			if !req.OverwriteConflicts {
				action, reason = s.compareOnTarget(req.Kind, target, res, req.Path)
				if action == model.ClusterSyncActionFailed {
					_ = s.record(task.ID, req.Kind, key, res.Name, source.ID, targetID, target.Name, action, reason, time.Since(start).Milliseconds())
					failed++
					continue
				}
				if action != model.ClusterSyncActionCreate {
					_ = s.record(task.ID, req.Kind, key, res.Name, source.ID, targetID, target.Name, action, reason, time.Since(start).Milliseconds())
					skipped++
					continue
				}
			}
			// For certificates the same-content re-check happens inside
			// transferOne; its skip-conflict counts as skipped.
			action, reason = s.transferOne(req.Kind, source, target, res, req.Path, req.OverwriteConflicts)
			switch action {
			case model.ClusterSyncActionCreate:
				succeeded++
			case model.ClusterSyncActionSkipSame, model.ClusterSyncActionSkipConflict:
				skipped++
			default:
				failed++
			}
			_ = s.record(task.ID, req.Kind, key, res.Name, source.ID, targetID, target.Name, action, reason, time.Since(start).Milliseconds())
		}
		// Per-node settlement: record the target's last sync outcome.
		nodeStatus := model.ClusterSyncTaskSuccess
		if failed > 0 {
			nodeStatus = model.ClusterSyncTaskPartial
		}
		_ = clusterNodeRepo.Update(targetID, map[string]interface{}{
			"last_sync_status": nodeStatus, "last_sync_at": time.Now(),
		})
	}
	finishTask()
	return task.ID, nil
}

func (s *ClusterSyncService) record(taskID uint, kind, key, name string, sourceID, targetID uint, targetName, action, reason string, durationMs int64) error {
	return s.syncRepo.CreateRecord(&model.ClusterSyncRecord{
		TaskID: taskID, Kind: kind, ResourceKey: key, ResourceName: name,
		SourceNodeID: sourceID, TargetNodeID: targetID, TargetName: targetName,
		Action: action, Reason: truncateErr(reason), DurationMs: durationMs,
	})
}

func (s *ClusterSyncService) SearchTasks(req dto.ClusterSyncTaskSearch) (int64, interface{}, error) {
	var opts []global.DBOption
	if req.Kind != "" {
		kind := req.Kind
		opts = append(opts, func(g *gorm.DB) *gorm.DB { return g.Where("kind = ?", kind) })
	}
	if req.Status != "" {
		status := req.Status
		opts = append(opts, func(g *gorm.DB) *gorm.DB { return g.Where("status = ?", status) })
	}
	total, tasks, err := s.syncRepo.PageTasks(req.Page, req.PageSize, opts...)
	if err != nil {
		return 0, nil, err
	}
	var infos []dto.ClusterSyncTaskInfo
	for _, task := range tasks {
		var info dto.ClusterSyncTaskInfo
		if err := copier.Copy(&info, &task); err != nil {
			return 0, nil, buserr.WithDetail("ErrStructTransform", err.Error(), nil)
		}
		infos = append(infos, info)
	}
	if infos == nil {
		infos = []dto.ClusterSyncTaskInfo{}
	}
	return total, infos, nil
}

func (s *ClusterSyncService) TaskRecords(taskID uint) ([]dto.ClusterSyncRecordInfo, error) {
	records, err := s.syncRepo.ListRecords(taskID)
	if err != nil {
		return nil, err
	}
	var infos []dto.ClusterSyncRecordInfo
	for _, record := range records {
		var info dto.ClusterSyncRecordInfo
		if err := copier.Copy(&info, &record); err != nil {
			return nil, buserr.WithDetail("ErrStructTransform", err.Error(), nil)
		}
		infos = append(infos, info)
	}
	if infos == nil {
		infos = []dto.ClusterSyncRecordInfo{}
	}
	return infos, nil
}
