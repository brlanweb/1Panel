package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/common"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
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
	SearchResources(req dto.ClusterSyncResourceSearch) (dto.ClusterSyncResourcePage, error)
	AppDatabases(req dto.ClusterSyncAppDBReq) ([]dto.ClusterSyncAppDB, error)
	DeleteFile(req dto.ClusterSyncFileDelete) error
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

// localAgentSocketPath is the unix socket of this panel's own agent. The
// core talks to its own agent over this socket without API-key auth (the
// socket itself is the trust boundary, same as proxy_local). Tests point it
// at a temp socket.
var localAgentSocketPath = "/etc/1panel/agent.sock"

func localAgentHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := &net.Dialer{Timeout: timeout}
				return d.DialContext(ctx, "unix", localAgentSocketPath)
			},
		},
	}
}

// localClusterNode is the pseudo-node for this (master) panel. It is never
// stored in cluster_nodes: ID 0 is the local panel by convention everywhere
// in the sync service and API (the DTOs accept 0 as a valid SourceNodeID).
func localClusterNode() model.ClusterNode {
	return model.ClusterNode{ID: 0, Name: "local", Status: model.ClusterNodeStatusOnline}
}

func isLocalNode(node model.ClusterNode) bool { return node.ID == 0 }

// resolveNode resolves a sync node id: 0 is the local panel, anything else
// is a registered cluster node.
func (s *ClusterSyncService) resolveNode(id uint) (model.ClusterNode, error) {
	if id == 0 {
		return localClusterNode(), nil
	}
	return s.getNode(id)
}

// callLocalJSON calls this panel's own agent API over the unix socket and
// unwraps the 1Panel envelope into out, mirroring callRemoteJSON's error
// contract (a non-200 HTTP status or envelope code is an error carrying the
// panel's message).
func (s *ClusterSyncService) callLocalJSON(ctx context.Context, method, path string, body interface{}, out interface{}, timeout time.Duration) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := localAgentHTTPClient(timeout).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var env remoteEnvelope
		if json.Unmarshal(raw, &env) == nil && strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("local panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("local panel returned HTTP %d", resp.StatusCode)
	}
	var env remoteEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("local panel returned an unreadable response")
	}
	if env.Code != http.StatusOK {
		if strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("local panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("local panel error code %d", env.Code)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// callNodeJSON is the node dispatcher for sync JSON calls: the local panel
// goes over the unix socket, registered nodes go through the signed remote
// call.
func (s *ClusterSyncService) callNodeJSON(node model.ClusterNode, method, path string, body interface{}, out interface{}, timeout time.Duration) error {
	if isLocalNode(node) {
		return s.callLocalJSON(context.Background(), method, path, body, out, timeout)
	}
	return s.nodes.callRemoteJSON(node, method, path, body, out, timeout)
}

// syncResourceDir marks a file-kind resource that is a directory (Extra).
const syncResourceDir = "dir"

func validSyncKind(kind string) bool {
	switch kind {
	case dto.ClusterSyncKindFile, dto.ClusterSyncKindImage, dto.ClusterSyncKindApp, dto.ClusterSyncKindCert, dto.ClusterSyncKindPhp:
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

// syncListOptions narrows one resource listing (search / paging / file
// directory and sort order).
type syncListOptions struct {
	Path      string
	Search    string
	SortOrder string
	Page      int
	PageSize  int
}

// listRemoteResources lists one kind of resource on one node, normalized.
// The `path` argument is only used for files (directory to list).
func (s *ClusterSyncService) listRemoteResources(kind string, node model.ClusterNode, reqPath string, page, pageSize int) ([]dto.ClusterSyncResource, int64, error) {
	res, err := s.listResourcesPage(kind, node, syncListOptions{Path: reqPath, Page: page, PageSize: pageSize})
	if err != nil {
		return nil, 0, err
	}
	return res.Items, res.Total, nil
}

func (s *ClusterSyncService) listResourcesPage(kind string, node model.ClusterNode, opts syncListOptions) (dto.ClusterSyncResourcePage, error) {
	page, pageSize := opts.Page, opts.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 100
	}
	search := strings.TrimSpace(opts.Search)
	out := dto.ClusterSyncResourcePage{Items: []dto.ClusterSyncResource{}}
	switch kind {
	case dto.ClusterSyncKindFile:
		dir := strings.TrimSpace(opts.Path)
		if dir == "" {
			dir = "/"
		}
		sortOrder := opts.SortOrder
		if sortOrder != "descending" {
			sortOrder = "ascending"
		}
		// The agent's /api/v2/files/search returns response.FileInfo
		// (files.FileInfo): entries live under "items", not "files".
		var data struct {
			Path      string                   `json:"path"`
			ItemTotal int64                    `json:"itemTotal"`
			Items     []map[string]interface{} `json:"items"`
		}
		// "expand": true is mandatory: without it the agent only stats the
		// directory itself (files.NewFileInfo skips handleExpansion) and
		// "items" is always empty.
		body := map[string]interface{}{"path": dir, "expand": true, "showHidden": true, "page": page, "pageSize": pageSize, "search": search, "containSub": false, "sortBy": "name", "sortOrder": sortOrder, "isDetail": true}
		if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/files/search", body, &data, 15*time.Second); err != nil {
			return out, err
		}
		// Directories are listed first so the panel reads like a file
		// manager; both directories and files can be transferred.
		var dirs, files []dto.ClusterSyncResource
		for _, f := range data.Items {
			name := toString(f["name"])
			if name == "" {
				continue
			}
			fullPath := path.Join(dir, name)
			meta := map[string]interface{}{
				"isDir":     toBool(f["isDir"]),
				"isSymlink": toBool(f["isSymlink"]),
				"linkPath":  toString(f["linkPath"]),
				"extension": toString(f["extension"]),
				"modTime":   toString(f["modTime"]),
			}
			if toBool(f["isDir"]) {
				dirs = append(dirs, dto.ClusterSyncResource{Key: fullPath, Name: name, Detail: dir, Extra: syncResourceDir, Meta: meta})
				continue
			}
			files = append(files, dto.ClusterSyncResource{
				Key: fullPath, Name: name,
				Detail: dir, Size: int64(toFloat(f["size"])), Meta: meta,
			})
		}
		out.Items = append(append(out.Items, dirs...), files...)
		out.Total = data.ItemTotal
		if out.Total < int64(len(out.Items)) {
			out.Total = int64(len(out.Items))
		}
		out.Path = data.Path
		return out, nil
	case dto.ClusterSyncKindImage:
		var result remotePage
		// PageImage requires orderBy + order (oneof) — omitting them is a
		// backend 400, which used to make the image list silently empty.
		if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/containers/image/search", map[string]interface{}{"page": page, "pageSize": pageSize, "name": search, "orderBy": "createdAt", "order": "null"}, &result, 15*time.Second); err != nil {
			return out, err
		}
		// One row per image (id), like the image page: an image is saved
		// and loaded as a whole, with all of its tags.
		for _, raw := range result.Items {
			var item map[string]interface{}
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			id := toString(item["id"])
			if id == "" {
				continue
			}
			var tags []string
			rawTags, _ := item["tags"].([]interface{})
			for _, t := range rawTags {
				if tag := toString(t); tag != "" && tag != "<none>:<none>" {
					tags = append(tags, tag)
				}
			}
			name := shortImageID(id)
			if len(tags) > 0 {
				name = tags[0]
			}
			out.Items = append(out.Items, dto.ClusterSyncResource{
				Key: id, Name: name,
				Detail: strings.Join(tags, ","), Size: int64(toFloat(item["size"])),
				Meta: map[string]interface{}{"tags": tags, "createdAt": toString(item["createdAt"]), "isUsed": toBool(item["isUsed"])},
			})
		}
		out.Total = result.Total
		return out, nil
	case dto.ClusterSyncKindApp:
		var result remotePage
		if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/apps/installed/search", map[string]interface{}{"page": page, "pageSize": pageSize, "name": search, "sync": true}, &result, 20*time.Second); err != nil {
			return out, err
		}
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
			// The agent's AppInstallDTO serializes the detail id as
			// "appDetailID" (capital ID).
			detailID := toString(item["appDetailID"])
			if detailID == "" {
				detailID = toString(item["appDetailId"])
			}
			env, _ := item["env"].(map[string]interface{})
			out.Items = append(out.Items, dto.ClusterSyncResource{
				Key: id, Name: name,
				Detail: key + " " + version,
				Extra:  detailID,
				Meta: map[string]interface{}{
					"appKey": key, "appName": toString(item["appName"]), "version": version,
					"status": toString(item["status"]), "linkDB": toBool(item["linkDB"]),
					"dbType": toString(env["PANEL_DB_TYPE"]), "dbHostName": toString(env["PANEL_DB_HOST_NAME"]),
					"hasDBHost": env["PANEL_DB_HOST"] != nil,
					"hasRedis":  env["PANEL_REDIS_HOST"] != nil || env["REDIS_HOST"] != nil,
					"hasMongo":  env["MONGO_HOST"] != nil,
				},
			})
		}
		out.Total = result.Total
		return out, nil
	case dto.ClusterSyncKindCert:
		var result remotePage
		if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/websites/ssl/search", map[string]interface{}{"page": page, "pageSize": pageSize, "domain": search, "orderBy": "expire_date", "order": "ascending"}, &result, 15*time.Second); err != nil {
			return out, err
		}
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
			out.Items = append(out.Items, dto.ClusterSyncResource{
				Key: id, Name: domain,
				Detail: toString(item["provider"]),
				Extra:  strconv.FormatBool(toBool(item["autoRenew"])),
				Meta: map[string]interface{}{
					"status": toString(item["status"]), "provider": toString(item["provider"]),
					"expireDate": toString(item["expireDate"]), "domains": toString(item["domains"]),
				},
			})
		}
		out.Total = result.Total
		return out, nil
	case dto.ClusterSyncKindPhp:
		// PHP extension templates live in the source panel's own template
		// table. Key is the template id, Detail is the extension list.
		var result remotePage
		if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/runtimes/php/extensions/search", map[string]interface{}{"page": page, "pageSize": pageSize, "name": search}, &result, 15*time.Second); err != nil {
			return out, err
		}
		for _, raw := range result.Items {
			var item map[string]interface{}
			if err := json.Unmarshal(raw, &item); err != nil {
				continue
			}
			id := toString(item["id"])
			name := toString(item["name"])
			if id == "" || name == "" {
				continue
			}
			out.Items = append(out.Items, dto.ClusterSyncResource{
				Key: id, Name: name,
				Detail: toString(item["extensions"]),
			})
		}
		out.Total = result.Total
		return out, nil
	}
	return out, buserr.New("ErrInvalidParams")
}

func shortImageID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// lookupFile finds one file or directory by its full path on a node by
// listing its parent directory filtered by name (so it works in
// directories with more entries than one listing page).
func (s *ClusterSyncService) lookupFile(node model.ClusterNode, fullPath string) (dto.ClusterSyncResource, bool, error) {
	fullPath = path.Clean(fullPath)
	name := path.Base(fullPath)
	if fullPath == "/" || name == "" {
		return dto.ClusterSyncResource{}, false, nil
	}
	page, err := s.listResourcesPage(dto.ClusterSyncKindFile, node, syncListOptions{Path: path.Dir(fullPath), Search: name, Page: 1, PageSize: 500})
	if err != nil {
		return dto.ClusterSyncResource{}, false, err
	}
	for _, item := range page.Items {
		if item.Name == name {
			return item, true, nil
		}
	}
	return dto.ClusterSyncResource{}, false, nil
}

// resolveResources maps the requested keys to source resources. Files are
// looked up one by one (keys are full paths, possibly in a large
// directory); the other kinds come from one listing.
func (s *ClusterSyncService) resolveResources(kind string, source model.ClusterNode, keys []string, reqPath string) (map[string]dto.ClusterSyncResource, error) {
	byKey := map[string]dto.ClusterSyncResource{}
	if kind == dto.ClusterSyncKindFile {
		for _, key := range keys {
			res, ok, err := s.lookupFile(source, key)
			if err != nil {
				return nil, err
			}
			if ok {
				byKey[key] = res
			}
		}
		return byKey, nil
	}
	resources, _, err := s.listRemoteResources(kind, source, reqPath, 1, 500)
	if err != nil {
		return nil, err
	}
	for _, r := range resources {
		byKey[r.Key] = r
	}
	return byKey, nil
}

func (s *ClusterSyncService) ListResources(req dto.ClusterSyncResourceSearch) ([]dto.ClusterSyncResource, error) {
	page, err := s.SearchResources(req)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *ClusterSyncService) SearchResources(req dto.ClusterSyncResourceSearch) (dto.ClusterSyncResourcePage, error) {
	if !validSyncKind(req.Kind) {
		return dto.ClusterSyncResourcePage{}, buserr.New("ErrInvalidParams")
	}
	node, err := s.resolveNode(req.SourceNodeID)
	if err != nil {
		return dto.ClusterSyncResourcePage{}, err
	}
	if node.Status == model.ClusterNodeStatusOffline {
		return dto.ClusterSyncResourcePage{}, buserr.New("ErrInvalidParams")
	}
	return s.listResourcesPage(req.Kind, node, syncListOptions{Path: req.Path, Search: req.Search, SortOrder: req.SortOrder, Page: req.Page, PageSize: req.PageSize})
}

// AppDatabases lists the database services (type + name) that exist on
// every given node, for the app-transfer dependency selector.
func (s *ClusterSyncService) AppDatabases(req dto.ClusterSyncAppDBReq) ([]dto.ClusterSyncAppDB, error) {
	types := strings.TrimSpace(req.Types)
	if types == "" {
		types = "mysql,mariadb,postgresql"
	}
	var common map[string]dto.ClusterSyncAppDB
	for _, id := range req.NodeIDs {
		node, err := s.resolveNode(id)
		if err != nil {
			return nil, err
		}
		var list []struct {
			Type     string `json:"type"`
			Database string `json:"database"`
		}
		if err := s.callNodeJSON(node, http.MethodGet, "/api/v2/databases/db/list/"+url.PathEscape(types), nil, &list, 15*time.Second); err != nil {
			return nil, fmt.Errorf("%s: %s", node.Name, truncateErr(err.Error()))
		}
		found := map[string]dto.ClusterSyncAppDB{}
		for _, item := range list {
			if item.Database == "" {
				continue
			}
			key := item.Type + "/" + item.Database
			if common == nil {
				found[key] = dto.ClusterSyncAppDB{Type: item.Type, Database: item.Database}
			} else if v, ok := common[key]; ok {
				found[key] = v
			}
		}
		common = found
	}
	out := []dto.ClusterSyncAppDB{}
	for _, v := range common {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Database < out[j].Database
	})
	return out, nil
}

// DeleteFile deletes one file or directory on a node (file-transfer page).
// The node's own delete policy applies (protected paths, recycle bin).
func (s *ClusterSyncService) DeleteFile(req dto.ClusterSyncFileDelete) error {
	node, err := s.resolveNode(req.NodeID)
	if err != nil {
		return err
	}
	p := path.Clean(strings.TrimSpace(req.Path))
	if p == "/" || !strings.HasPrefix(p, "/") {
		return buserr.New("ErrInvalidParams")
	}
	return s.callNodeJSON(node, http.MethodPost, "/api/v2/files/del", map[string]interface{}{"path": p, "isDir": req.IsDir, "forceDelete": false}, nil, 60*time.Second)
}

// syncTransferOpts carries the per-request options of one transfer.
type syncTransferOpts struct {
	Path       string // file: source directory
	TargetPath string // file: target directory ("" = same as source)
	Overwrite  bool
	AppDBHost  string // app: database service name on the target
	// Step reports the stage being run (nil = no reporting); it also
	// clears any byte progress of the previous stage.
	Step func(stage string)
	// Progress reports byte progress of the current stage (nil = none).
	Progress func(done, total, rate int64)
}

func (o syncTransferOpts) step(stage string) {
	if o.Step != nil {
		o.Step(stage)
	}
}

// Stage timeouts. A node task (export / compress / load / restore) that
// does not finish in time fails the item instead of keeping the sync task
// "running" for hours; tests shorten them.
var (
	syncShortTaskTimeout = 30 * time.Minute
	syncLongTaskTimeout  = 2 * time.Hour
)

func (o syncTransferOpts) fileTargetDir(res dto.ClusterSyncResource) string {
	if dir := strings.TrimSpace(o.TargetPath); dir != "" {
		return path.Clean(dir)
	}
	return path.Dir(res.Key)
}

// appKeyVersion splits the listing's "appKey version" detail.
func appKeyVersion(res dto.ClusterSyncResource) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(res.Detail), " ", 2)
	if len(parts) != 2 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func imageTags(res dto.ClusterSyncResource) []string {
	var tags []string
	for _, t := range strings.Split(res.Detail, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// syncTaskPollInterval is how often a node task is polled (tests shorten it).
var syncTaskPollInterval = 2 * time.Second

// syncTaskMissingLimit is how long a node task may stay unlisted.
var syncTaskMissingLimit = time.Minute

// waitNodeTask polls a node's task log (POST /logs/tasks/search) until the
// async task created with taskID succeeds or fails.
func (s *ClusterSyncService) waitNodeTask(node model.ClusterNode, taskID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	missing := 0
	for {
		var page struct {
			Items []struct {
				Status   string `json:"status"`
				ErrorMsg string `json:"errorMsg"`
			} `json:"items"`
		}
		err := s.callNodeJSON(node, http.MethodPost, "/api/v2/logs/tasks/search", map[string]interface{}{"taskID": taskID, "page": 1, "pageSize": 1}, &page, 15*time.Second)
		if err == nil && len(page.Items) == 0 {
			// The task row is created synchronously by the node before
			// it answers; still missing after a minute means the request
			// never started a task (or the node cannot list it).
			missing++
			if time.Duration(missing)*syncTaskPollInterval > syncTaskMissingLimit {
				return fmt.Errorf("task %s not found on node %s", taskID, node.Name)
			}
		}
		if err == nil && len(page.Items) > 0 {
			switch page.Items[0].Status {
			case "Success":
				return nil
			case "Failed", "Canceled":
				msg := strings.TrimSpace(page.Items[0].ErrorMsg)
				if msg == "" {
					msg = "task " + strings.ToLower(page.Items[0].Status)
				}
				return errors.New(msg)
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("task %s did not finish: %s", taskID, truncateErr(err.Error()))
			}
			return fmt.Errorf("task %s did not finish in time", taskID)
		}
		time.Sleep(syncTaskPollInterval)
	}
}

// syncTmpDir is the scratch directory used on nodes for archives.
const syncTmpDir = "/tmp/1panel-cluster-sync"

func (s *ClusterSyncService) removeNodeFile(node model.ClusterNode, p string) {
	_ = s.callNodeJSON(node, http.MethodPost, "/api/v2/files/del", map[string]interface{}{"path": p, "isDir": false, "forceDelete": true}, nil, 30*time.Second)
}

// compareOnTarget computes the preview action for one resource on one target.
func (s *ClusterSyncService) compareOnTarget(kind string, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (action, reason string) {
	switch kind {
	case dto.ClusterSyncKindFile:
		dst := path.Join(opts.fileTargetDir(res), path.Base(res.Key))
		item, ok, err := s.lookupFile(target, dst)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target directory: " + truncateErr(err.Error())
		}
		if !ok {
			return model.ClusterSyncActionCreate, ""
		}
		if res.Extra == syncResourceDir {
			if item.Extra != syncResourceDir {
				return model.ClusterSyncActionSkipConflict, "target has a file with the same name as the directory"
			}
			// Directories are merged: new files are added, existing
			// target files are kept unless overwrite is requested.
			return model.ClusterSyncActionCreate, ""
		}
		if item.Extra == syncResourceDir {
			return model.ClusterSyncActionSkipConflict, "target has a directory with the same name"
		}
		if item.Size == res.Size {
			return model.ClusterSyncActionSkipSame, "target already has the same file (same size)"
		}
		return model.ClusterSyncActionSkipConflict, "target file differs (different size)"
	case dto.ClusterSyncKindImage:
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target images: " + truncateErr(err.Error())
		}
		srcTags := imageTags(res)
		tagOwner := map[string]string{}
		var same *dto.ClusterSyncResource
		for i := range items {
			for _, t := range imageTags(items[i]) {
				tagOwner[t] = items[i].Key
			}
			if items[i].Key == res.Key {
				same = &items[i]
			}
		}
		for _, t := range srcTags {
			if owner, ok := tagOwner[t]; ok && owner != res.Key {
				return model.ClusterSyncActionSkipConflict, "target has a different image tagged " + t
			}
		}
		if same != nil {
			have := map[string]bool{}
			for _, t := range imageTags(*same) {
				have[t] = true
			}
			for _, t := range srcTags {
				if !have[t] {
					return model.ClusterSyncActionCreate, ""
				}
			}
			return model.ClusterSyncActionSkipSame, "target already has the same image"
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindApp:
		if reason := appUnsupportedReason(res); reason != "" {
			return model.ClusterSyncActionFailed, reason
		}
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target apps: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if strings.EqualFold(item.Name, res.Name) {
				if item.Detail == res.Detail {
					return model.ClusterSyncActionSkipSame, "target already has this app installed with the same name and version"
				}
				return model.ClusterSyncActionSkipConflict, "target already has an app installed with the same name"
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
				return model.ClusterSyncActionSkipConflict, "target already has a certificate for this domain"
			}
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindPhp:
		items, _, err := s.listRemoteResources(kind, target, "", 1, 500)
		if err != nil {
			return model.ClusterSyncActionFailed, "could not list target PHP templates: " + truncateErr(err.Error())
		}
		for _, item := range items {
			if item.Name == res.Name {
				if item.Detail == res.Detail {
					return model.ClusterSyncActionSkipSame, "target already has the same PHP extension template"
				}
				return model.ClusterSyncActionSkipConflict, "target has a PHP template with the same name but different extensions"
			}
		}
		return model.ClusterSyncActionCreate, ""
	}
	return model.ClusterSyncActionFailed, "unknown resource kind"
}

// appUnsupportedReason mirrors the page's rule: only apps without resource
// dependencies, or depending on MySQL / MariaDB / PostgreSQL, are migrated.
func appUnsupportedReason(res dto.ClusterSyncResource) string {
	key, _ := appKeyVersion(res)
	if key == "openresty" {
		return "OpenResty is not migrated (it is bound to the panel's websites)"
	}
	if res.Meta == nil {
		return ""
	}
	if toBool(res.Meta["hasRedis"]) || toBool(res.Meta["hasMongo"]) {
		return "only apps depending on MySQL, MariaDB or PostgreSQL can be migrated"
	}
	if dbType := strings.ToLower(toString(res.Meta["dbType"])); dbType != "" {
		switch dbType {
		case "mysql", "mariadb", "postgresql":
		default:
			return "only apps depending on MySQL, MariaDB or PostgreSQL can be migrated"
		}
	}
	return ""
}

func syncOptsFromPreview(req dto.ClusterSyncPreviewReq) syncTransferOpts {
	return syncTransferOpts{Path: req.Path, TargetPath: req.TargetPath, AppDBHost: req.AppDBHost}
}

func (s *ClusterSyncService) Preview(req dto.ClusterSyncPreviewReq) ([]dto.ClusterSyncPreviewItem, error) {
	if !validSyncKind(req.Kind) {
		return nil, buserr.New("ErrInvalidParams")
	}
	source, err := s.resolveNode(req.SourceNodeID)
	if err != nil {
		return nil, err
	}
	byKey, err := s.resolveResources(req.Kind, source, req.ResourceKeys, req.Path)
	if err != nil {
		return nil, err
	}
	opts := syncOptsFromPreview(req)
	var out []dto.ClusterSyncPreviewItem
	for _, targetID := range req.TargetNodeIDs {
		if targetID == req.SourceNodeID {
			continue
		}
		target, err := s.resolveNode(targetID)
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
			action, reason := s.compareOnTarget(req.Kind, target, res, opts)
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

func (s *ClusterSyncService) downloadRemoteFile(node model.ClusterNode, remotePath string, maxBytes int64, progress func(done, total int64)) (tmpPath string, size int64, err error) {
	timestamp, hmacToken, md5Token, err := s.nodes.remoteAuthHeaders(node)
	if err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp("", "cluster-sync-*")
	if err != nil {
		return "", 0, err
	}
	tmpPath = tmp.Name()
	_ = tmp.Close()
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	targetURL := apiBase + "/api/v2/files/download?path=" + url.QueryEscape(remotePath)
	client := &http.Client{
		Timeout:   2 * time.Hour,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: http.ProxyFromEnvironment},
	}
	doGet := func(token string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, targetURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("1Panel-Timestamp", timestamp)
		req.Header.Set("1Panel-Token", token)
		return client.Do(req)
	}
	resp, err := doGet(hmacToken)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		resp, err = doGet(md5Token)
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("source panel returned HTTP %d while downloading the file", resp.StatusCode)
	}
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	pw := &progressWriter{w: out, total: resp.ContentLength, progress: progress}
	if pw.total < 0 {
		pw.total = 0
	}
	_, copyErr := io.Copy(pw, io.LimitReader(resp.Body, maxBytes+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if copyErr != nil {
			return "", 0, copyErr
		}
		return "", 0, closeErr
	}
	// The agent answers errors as a JSON envelope with HTTP 200 (bad API
	// key, sensitive path, missing file). Without this check the JSON
	// error body would be saved as the file and "synced" as corrupt
	// content; the local branch already guards the same way.
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		raw, _ := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)
		var env remoteEnvelope
		if json.Unmarshal(raw, &env) == nil && strings.TrimSpace(env.Message) != "" {
			return "", 0, fmt.Errorf("source panel error: %s", truncateErr(env.Message))
		}
		return "", 0, fmt.Errorf("source panel returned an unreadable download response")
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	if info.Size() > maxBytes {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("file is %d bytes, over the %d-byte sync limit", info.Size(), maxBytes)
	}
	return tmpPath, info.Size(), nil
}

// Chunked upload tuning. The target panel's HTTP server closes any request
// after ReadTimeout (600 s), so on a slow / lossy link one big multipart POST
// fails with "use of closed network connection". Chunks are sized so each
// request finishes well inside that window: start small, grow while chunks
// are fast, shrink when they are slow.
var (
	syncChunkMin      int64 = 256 << 10
	syncChunkStart    int64 = 1 << 20
	syncChunkMax      int64 = 16 << 20
	syncChunkFast           = 15 * time.Second
	syncChunkSlow           = 90 * time.Second
	syncChunkTimeout        = 5 * time.Minute
	syncChunkRetries        = 6
	syncChunkRetryGap       = 3 * time.Second
)

// errChunkUnsupported: the target agent has no resumable chunk upload
// (older 1Panel); the caller falls back to a single multipart POST.
var errChunkUnsupported = errors.New("chunk upload not supported by target")

func (s *ClusterSyncService) uploadRemoteFile(node model.ClusterNode, dir, fileName, tmpPath string, overwrite bool, progress func(sent, total int64)) error {
	err := s.uploadRemoteFileChunked(node, dir, fileName, tmpPath, overwrite, progress)
	if !errors.Is(err, errChunkUnsupported) {
		return err
	}
	return s.uploadRemoteFileSingle(node, dir, fileName, tmpPath, overwrite)
}

// uploadRemoteFileChunked streams tmpPath to the target through
// /api/v2/files/chunkupload in resumable mode (uploadID + offset + fileSize):
// each chunk is its own short request, a failed chunk is retried at the same
// offset (the agent truncates and rewrites it), and the file only appears in
// dir once the final chunk lands.
func (s *ClusterSyncService) uploadRemoteFileChunked(node model.ClusterNode, dir, fileName, tmpPath string, overwrite bool, progress func(sent, total int64)) error {
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		return err
	}
	targetURL := apiBase + "/api/v2/files/chunkupload"
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	total := info.Size()
	if total == 0 {
		// The resumable API rejects empty files.
		return errChunkUnsupported
	}
	client := &http.Client{
		Timeout:   syncChunkTimeout,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: http.ProxyFromEnvironment},
	}
	uploadID := "cluster-sync-" + strings.ReplaceAll(uuid.NewString(), "-", "")

	postChunk := func(index int, offset int64, data []byte, final bool) (int, []byte, error) {
		count := index + 2
		if final {
			count = index + 1
		}
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fields := [][2]string{
			{"path", dir}, {"filename", fileName}, {"uploadID", uploadID},
			{"chunkIndex", strconv.Itoa(index)}, {"chunkCount", strconv.Itoa(count)},
			{"offset", strconv.FormatInt(offset, 10)}, {"fileSize", strconv.FormatInt(total, 10)},
			{"overwrite", strconv.FormatBool(overwrite)},
		}
		for _, kv := range fields {
			if err := mw.WriteField(kv[0], kv[1]); err != nil {
				return 0, nil, err
			}
		}
		part, err := mw.CreateFormFile("chunk", fileName)
		if err != nil {
			return 0, nil, err
		}
		if _, err := part.Write(data); err != nil {
			return 0, nil, err
		}
		if err := mw.Close(); err != nil {
			return 0, nil, err
		}
		// Fresh signature per request: a long transfer outlives one timestamp.
		timestamp, hmacToken, md5Token, err := s.nodes.remoteAuthHeaders(node)
		if err != nil {
			return 0, nil, err
		}
		// Report bytes as they leave so a slow chunk still shows movement.
		overhead := int64(buf.Len() - len(data))
		send := func(token string) (int, []byte, error) {
			var reqBody io.Reader = bytes.NewReader(buf.Bytes())
			if progress != nil {
				reqBody = &countingReader{r: reqBody, onRead: func(n int64) {
					sent := n - overhead
					if sent < 0 {
						sent = 0
					}
					if sent > int64(len(data)) {
						sent = int64(len(data))
					}
					progress(offset+sent, total)
				}}
			}
			req, err := http.NewRequest(http.MethodPost, targetURL, reqBody)
			if err != nil {
				return 0, nil, err
			}
			req.ContentLength = int64(buf.Len())
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("1Panel-Timestamp", timestamp)
			req.Header.Set("1Panel-Token", token)
			resp, err := client.Do(req)
			if err != nil {
				return 0, nil, err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			return resp.StatusCode, body, err
		}
		code, body, err := send(hmacToken)
		if err == nil && code == http.StatusUnauthorized {
			code, body, err = send(md5Token)
		}
		return code, body, err
	}

	chunkSize := syncChunkStart
	var offset int64
	for index := 0; offset < total; index++ {
		size := chunkSize
		if offset+size > total {
			size = total - offset
		}
		final := offset+size == total
		data := make([]byte, size)
		if _, err := f.ReadAt(data, offset); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		var lastErr error
		var took time.Duration
		ok := false
		for attempt := 0; attempt < syncChunkRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(syncChunkRetryGap * time.Duration(attempt))
			}
			started := time.Now()
			code, body, err := postChunk(index, offset, data, final)
			took = time.Since(started)
			if err != nil {
				lastErr = err
				continue
			}
			if code == http.StatusNotFound && index == 0 {
				return errChunkUnsupported
			}
			var env struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    struct {
					Retryable *bool `json:"retryable"`
				} `json:"data"`
			}
			_ = json.Unmarshal(body, &env)
			if code == http.StatusOK && (env.Code == http.StatusOK || env.Code == 0) {
				ok = true
				break
			}
			msg := strings.TrimSpace(env.Message)
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d", code)
			}
			lastErr = fmt.Errorf("target panel error: %s", truncateErr(msg))
			// 4xx (bad params, file exists, auth) and errors the agent marks
			// as not retryable will fail the same way again.
			if (code >= 400 && code < 500 && code != http.StatusRequestTimeout && code != http.StatusTooManyRequests) ||
				(env.Data.Retryable != nil && !*env.Data.Retryable) {
				break
			}
		}
		if !ok {
			return fmt.Errorf("chunk %d at offset %d/%d failed: %w", index, offset, total, lastErr)
		}
		offset += size
		if progress != nil {
			progress(offset, total)
		}
		switch {
		case took < syncChunkFast && chunkSize < syncChunkMax:
			chunkSize *= 2
		case took > syncChunkSlow && chunkSize > syncChunkMin:
			chunkSize /= 2
		}
	}
	return nil
}

// uploadRemoteFileSingle is the legacy single-request multipart upload.
func (s *ClusterSyncService) uploadRemoteFileSingle(node model.ClusterNode, dir, fileName, tmpPath string, overwrite bool) error {
	timestamp, hmacToken, md5Token, err := s.nodes.remoteAuthHeaders(node)
	if err != nil {
		return err
	}
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		return err
	}
	targetURL := apiBase + "/api/v2/files/upload"
	client := &http.Client{
		Timeout:   2 * time.Hour,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: http.ProxyFromEnvironment},
	}
	// Stream the multipart body from the temp file through a pipe. resty
	// builds multipart bodies in memory (and copies them once more), which
	// for an image tarball exhausted RAM on small (1 GB) panels.
	doPost := func(token string) (int, []byte, error) {
		f, err := os.Open(tmpPath)
		if err != nil {
			return 0, nil, err
		}
		defer f.Close()
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		go func() {
			err := mw.WriteField("path", dir)
			if err == nil {
				err = mw.WriteField("overwrite", strconv.FormatBool(overwrite))
			}
			if err == nil {
				var part io.Writer
				if part, err = mw.CreateFormFile("file", fileName); err == nil {
					_, err = io.Copy(part, f)
				}
			}
			if err == nil {
				err = mw.Close()
			}
			_ = pw.CloseWithError(err)
		}()
		req, err := http.NewRequest(http.MethodPost, targetURL, pr)
		if err != nil {
			_ = pr.Close()
			return 0, nil, err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("1Panel-Timestamp", timestamp)
		req.Header.Set("1Panel-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			_ = pr.Close()
			return 0, nil, err
		}
		defer resp.Body.Close()
		_ = pr.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, body, err
	}
	code, body, err := doPost(hmacToken)
	if err == nil && code == http.StatusUnauthorized {
		code, body, err = doPost(md5Token)
	}
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("target panel returned HTTP %d while uploading the file", code)
	}
	var env remoteEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Code != http.StatusOK && env.Code != 0 {
		if strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("target panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("target panel error code %d", env.Code)
	}
	return nil
}

// downloadNodeFile / uploadNodeFile dispatch file transfers by node: the
// local panel goes over the unix socket, registered nodes use the signed
// remote calls.
func (s *ClusterSyncService) downloadNodeFile(node model.ClusterNode, remotePath string, maxBytes int64, progress func(done, total int64)) (string, int64, error) {
	if isLocalNode(node) {
		return s.downloadLocalFile(remotePath, maxBytes)
	}
	return s.downloadRemoteFile(node, remotePath, maxBytes, progress)
}

func (s *ClusterSyncService) uploadNodeFile(node model.ClusterNode, dir, fileName, tmpPath string, overwrite bool, progress func(sent, total int64)) error {
	if isLocalNode(node) {
		return s.uploadLocalFile(dir, fileName, tmpPath, overwrite)
	}
	return s.uploadRemoteFile(node, dir, fileName, tmpPath, overwrite, progress)
}

// byteProgress returns a throttled byte-progress callback for a transfer
// stage: it writes done/total and the recent speed (bytes/s, averaged over
// a sliding ~30 s window) to the task row at most every 2 s.
func (o syncTransferOpts) byteProgress() func(done, total int64) {
	if o.Progress == nil {
		return nil
	}
	type sample struct {
		at   time.Time
		done int64
	}
	window := []sample{{at: time.Now()}}
	var last time.Time
	return func(done, total int64) {
		now := time.Now()
		window = append(window, sample{now, done})
		for len(window) > 2 && now.Sub(window[1].at) > 30*time.Second {
			window = window[1:]
		}
		if done < total && now.Sub(last) < 2*time.Second {
			return
		}
		last = now
		var rate int64
		if span := now.Sub(window[0].at).Seconds(); span > 0 {
			rate = int64(float64(done-window[0].done) / span)
		}
		o.Progress(done, total, rate)
	}
}

// countingReader reports the running byte count read through it.
type countingReader struct {
	r      io.Reader
	n      int64
	onRead func(n int64)
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	if n > 0 && c.onRead != nil {
		c.onRead(c.n)
	}
	return n, err
}

// progressWriter counts bytes written through it (download progress).
type progressWriter struct {
	w        io.Writer
	done     int64
	total    int64
	progress func(done, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.progress != nil {
		p.progress(p.done, p.total)
	}
	return n, err
}

// downloadLocalFile fetches a file from this panel's own agent over the unix
// socket and streams it to a temp file (same 200 MB cap as the remote path).
// The agent answers errors as a JSON envelope even with HTTP 200, so a JSON
// content type is parsed as an error instead of being saved as file bytes.
func (s *ClusterSyncService) downloadLocalFile(remotePath string, maxBytes int64) (string, int64, error) {
	tmp, err := os.CreateTemp("", "cluster-sync-*")
	if err != nil {
		return "", 0, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	targetURL := "http://localhost/api/v2/files/download?path=" + url.QueryEscape(remotePath)
	resp, err := localAgentHTTPClient(2 * time.Hour).Get(targetURL)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("local panel returned HTTP %d while downloading the file", resp.StatusCode)
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		raw, _ := io.ReadAll(resp.Body)
		_ = os.Remove(tmpPath)
		var env remoteEnvelope
		if json.Unmarshal(raw, &env) == nil && strings.TrimSpace(env.Message) != "" {
			return "", 0, fmt.Errorf("local panel error: %s", truncateErr(env.Message))
		}
		return "", 0, fmt.Errorf("local panel returned an unreadable download response")
	}
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, err
	}
	written, copyErr := io.CopyN(out, resp.Body, maxBytes+1)
	_ = out.Close()
	if copyErr != nil && copyErr != io.EOF {
		_ = os.Remove(tmpPath)
		return "", 0, copyErr
	}
	if written > maxBytes {
		_ = os.Remove(tmpPath)
		return "", 0, fmt.Errorf("file is over the %d-byte sync limit", maxBytes)
	}
	return tmpPath, written, nil
}

// uploadLocalFile uploads a temp file to this panel's own agent over the
// unix socket (multipart, streamed — the file is never buffered in memory).
func (s *ClusterSyncService) uploadLocalFile(dir, fileName, tmpPath string, overwrite bool) error {
	f, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer f.Close()
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		_ = mw.WriteField("path", dir)
		_ = mw.WriteField("overwrite", strconv.FormatBool(overwrite))
		if part, err := mw.CreateFormFile("file", fileName); err == nil {
			_, _ = io.Copy(part, f)
		}
		_ = mw.Close()
	}()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://localhost/api/v2/files/upload", pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := localAgentHTTPClient(2 * time.Hour).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local panel returned HTTP %d while uploading the file", resp.StatusCode)
	}
	var env remoteEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Code != http.StatusOK && env.Code != 0 {
		if strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("local panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("local panel error code %d", env.Code)
	}
	return nil
}

// transferOne executes a single (resource × target) transfer. It returns the
// action recorded for the item and a human-readable reason ("" on create).
func (s *ClusterSyncService) transferOne(kind string, source, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (string, string) {
	switch kind {
	case dto.ClusterSyncKindFile:
		if res.Extra == syncResourceDir {
			return s.transferDir(source, target, res, opts)
		}
		opts.step("download from " + source.Name)
		tmpPath, _, err := s.downloadNodeFile(source, res.Key, maxSyncFileBytes, opts.byteProgress())
		if err != nil {
			return model.ClusterSyncActionFailed, "download from source failed: " + truncateErr(err.Error())
		}
		defer os.Remove(tmpPath)
		dir := opts.fileTargetDir(res)
		opts.step("upload to " + target.Name)
		if err := s.uploadNodeFile(target, dir, path.Base(res.Key), tmpPath, opts.Overwrite, opts.byteProgress()); err != nil {
			return model.ClusterSyncActionFailed, "upload to target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindImage:
		return s.transferImage(source, target, res, opts)
	case dto.ClusterSyncKindApp:
		return s.transferApp(source, target, res, opts)
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
		if err := s.callNodeJSON(source, http.MethodGet, "/api/v2/websites/ssl/"+res.Key, nil, &cert, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "could not read source certificate: " + truncateErr(err.Error())
		}
		// ACME / auto-renew certificates are copied as a static pasted
		// certificate; renewal stays with the source panel.
		if strings.TrimSpace(cert.PrivateKey) == "" || strings.TrimSpace(cert.Pem) == "" {
			return model.ClusterSyncActionFailed, "source panel did not return the certificate content"
		}
		body := map[string]interface{}{
			"type": "paste", "privateKey": cert.PrivateKey, "certificate": cert.Pem,
			"description": syncCertDescription(source.Name, cert.AutoRenew),
		}
		if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/websites/ssl/upload", body, nil, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "certificate upload on target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	case dto.ClusterSyncKindPhp:
		// Write path is the open-source import endpoint: it creates the
		// template, returns "skipped" when the target already has the same
		// content, and fails with ErrPHPExtensionsConflict on a name
		// clash with different extensions.
		var status string
		err := s.callNodeJSON(target, http.MethodPost, "/api/v2/runtimes/php/extensions/import", map[string]interface{}{"name": res.Name, "extensions": res.Detail}, &status, 10*time.Second)
		if err == nil {
			if status == "skipped" {
				return model.ClusterSyncActionSkipSame, "target already has the same PHP extension template"
			}
			return model.ClusterSyncActionCreate, ""
		}
		if !strings.Contains(err.Error(), "ErrPHPExtensionsConflict") {
			return model.ClusterSyncActionFailed, "PHP template import on target failed: " + truncateErr(err.Error())
		}
		if !opts.Overwrite {
			return model.ClusterSyncActionSkipConflict, "target has a PHP template with the same name but different extensions"
		}
		targetItems, _, listErr := s.listRemoteResources(kind, target, "", 1, 500)
		if listErr != nil {
			return model.ClusterSyncActionFailed, "could not list target PHP templates for overwrite: " + truncateErr(listErr.Error())
		}
		var targetID uint64
		for _, item := range targetItems {
			if item.Name == res.Name {
				targetID, _ = strconv.ParseUint(strings.TrimSpace(item.Key), 10, 64)
				break
			}
		}
		if targetID == 0 {
			return model.ClusterSyncActionFailed, "target PHP template disappeared before overwrite"
		}
		if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/runtimes/php/extensions/update", map[string]interface{}{"id": targetID, "extensions": res.Detail}, nil, 10*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "PHP template update on target failed: " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionCreate, ""
	}
	return model.ClusterSyncActionFailed, "unknown resource kind"
}

// maxSyncArchiveBytes caps directory archives and image tarballs that
// transit through the master.
const maxSyncArchiveBytes = 20 * 1024 * 1024 * 1024

func newSyncTaskID() string { return uuid.NewString() }

func (s *ClusterSyncService) ensureNodeDir(node model.ClusterNode, dir string) error {
	if item, ok, err := s.lookupFile(node, dir); err == nil && ok && item.Extra == syncResourceDir {
		return nil
	}
	err := s.callNodeJSON(node, http.MethodPost, "/api/v2/files", map[string]interface{}{"path": dir, "isDir": true, "mode": 0755}, nil, 15*time.Second)
	if err != nil {
		// Created concurrently by another transfer: fine if it exists now.
		if item, ok, lookErr := s.lookupFile(node, dir); lookErr == nil && ok && item.Extra == syncResourceDir {
			return nil
		}
		return fmt.Errorf("could not create %s on %s: %s", dir, node.Name, truncateErr(err.Error()))
	}
	return nil
}

// moveArchive downloads an archive from one node and uploads it into
// syncTmpDir on another, removing the master's temp copy.
func (s *ClusterSyncService) moveArchive(source, target model.ClusterNode, srcPath string, opts syncTransferOpts) (string, error) {
	opts.step("download from " + source.Name)
	tmpPath, _, err := s.downloadNodeFile(source, srcPath, maxSyncArchiveBytes, opts.byteProgress())
	if err != nil {
		return "", fmt.Errorf("download from source failed: %s", truncateErr(err.Error()))
	}
	defer os.Remove(tmpPath)
	name := path.Base(srcPath)
	opts.step("upload to " + target.Name)
	if err := s.uploadNodeFile(target, syncTmpDir, name, tmpPath, true, opts.byteProgress()); err != nil {
		return "", fmt.Errorf("upload to target failed: %s", truncateErr(err.Error()))
	}
	return path.Join(syncTmpDir, name), nil
}

// transferDir packs a directory on the source (files/compress, tar.gz),
// moves the archive through the master and unpacks it into the target
// directory (files/decompress). Existing target files are kept unless
// overwrite is requested.
func (s *ClusterSyncService) transferDir(source, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (string, string) {
	if err := s.ensureNodeDir(source, syncTmpDir); err != nil {
		return model.ClusterSyncActionFailed, err.Error()
	}
	archive := "sync-" + common.RandStrAndNum(12) + ".tar.gz"
	taskID := newSyncTaskID()
	srcArchive := path.Join(syncTmpDir, archive)
	opts.step("compress on " + source.Name)
	if err := s.callNodeJSON(source, http.MethodPost, "/api/v2/files/compress", map[string]interface{}{
		"files": []string{res.Key}, "dst": syncTmpDir, "type": "tar.gz", "name": archive, "replace": true, "taskID": taskID,
	}, nil, 30*time.Second); err != nil {
		return model.ClusterSyncActionFailed, "compress on source failed: " + truncateErr(err.Error())
	}
	defer s.removeNodeFile(source, srcArchive)
	if err := s.waitNodeTask(source, taskID, syncLongTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "compress on source failed: " + truncateErr(err.Error())
	}
	dstArchive, err := s.moveArchive(source, target, srcArchive, opts)
	if err != nil {
		return model.ClusterSyncActionFailed, err.Error()
	}
	defer s.removeNodeFile(target, dstArchive)
	overwrite := opts.Overwrite
	taskID = newSyncTaskID()
	opts.step("decompress on " + target.Name)
	if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/files/decompress", map[string]interface{}{
		"path": dstArchive, "dst": opts.fileTargetDir(res), "type": "tar.gz", "overwrite": &overwrite, "taskID": taskID,
	}, nil, 30*time.Second); err != nil {
		return model.ClusterSyncActionFailed, "decompress on target failed: " + truncateErr(err.Error())
	}
	if err := s.waitNodeTask(target, taskID, syncLongTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "decompress on target failed: " + truncateErr(err.Error())
	}
	return model.ClusterSyncActionCreate, ""
}

// transferImage copies one image (with all its tags) without a registry:
// docker save on the source, transit through the master, docker load on
// the target, then re-tag so every source tag exists on the target.
func (s *ClusterSyncService) transferImage(source, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (string, string) {
	tags := imageTags(res)
	ref := res.Key
	if len(tags) > 0 {
		ref = tags[0]
	}
	if err := s.ensureNodeDir(source, syncTmpDir); err != nil {
		return model.ClusterSyncActionFailed, "image export on source failed: " + err.Error()
	}
	name := "sync-image-" + common.RandStrAndNum(12)
	srcTar := path.Join(syncTmpDir, name+".tar")
	taskID := newSyncTaskID()
	opts.step("export " + ref + " on " + source.Name)
	if err := s.callNodeJSON(source, http.MethodPost, "/api/v2/containers/image/save", map[string]interface{}{
		"tagName": ref, "path": syncTmpDir, "name": name, "taskID": taskID,
	}, nil, 30*time.Second); err != nil {
		return model.ClusterSyncActionFailed, "image export on source failed: " + truncateErr(err.Error())
	}
	defer s.removeNodeFile(source, srcTar)
	if err := s.waitNodeTask(source, taskID, syncShortTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "image export on source failed: " + truncateErr(err.Error())
	}
	dstTar, err := s.moveArchive(source, target, srcTar, opts)
	if err != nil {
		return model.ClusterSyncActionFailed, err.Error()
	}
	defer s.removeNodeFile(target, dstTar)
	taskID = newSyncTaskID()
	opts.step("import on " + target.Name)
	if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/containers/image/load", map[string]interface{}{
		"paths": []string{dstTar}, "taskID": taskID,
	}, nil, 30*time.Second); err != nil {
		return model.ClusterSyncActionFailed, "image import on target failed: " + truncateErr(err.Error())
	}
	if err := s.waitNodeTask(target, taskID, syncShortTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "image import on target failed: " + truncateErr(err.Error())
	}
	if len(tags) > 1 {
		opts.step("tag on " + target.Name)
		// Keep any tags the target already had on this image.
		all := append([]string{}, tags...)
		if items, _, err := s.listRemoteResources(dto.ClusterSyncKindImage, target, "", 1, 500); err == nil {
			for _, item := range items {
				if item.Key == res.Key {
					all = append(all, imageTags(item)...)
				}
			}
		}
		seen := map[string]bool{}
		var uniq []string
		for _, t := range all {
			if !seen[t] {
				seen[t] = true
				uniq = append(uniq, t)
			}
		}
		if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/containers/image/tag", map[string]interface{}{"sourceID": res.Key, "tags": uniq}, nil, 60*time.Second); err != nil {
			return model.ClusterSyncActionFailed, "image imported but tagging on target failed: " + truncateErr(err.Error())
		}
	}
	return model.ClusterSyncActionCreate, ""
}

// transferApp migrates one installed app: install the same app version on
// the target with the source's parameters (database dependency switched to
// the selected target database service), then back the app up on the
// source and restore that backup on the target so data moves too.
func (s *ClusterSyncService) transferApp(source, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (string, string) {
	if reason := appUnsupportedReason(res); reason != "" {
		return model.ClusterSyncActionFailed, reason
	}
	appKey, version := appKeyVersion(res)
	if appKey == "" || version == "" {
		return model.ClusterSyncActionFailed, "source app has no key/version"
	}
	var detail struct {
		ID uint `json:"id"`
	}
	if err := s.callNodeJSON(target, http.MethodGet, "/api/v2/apps/detail/node/"+url.PathEscape(appKey)+"/"+url.PathEscape(version), nil, &detail, 20*time.Second); err != nil || detail.ID == 0 {
		msg := "app " + appKey + " " + version + " is not available in the target app store (sync the app store first)"
		if err != nil {
			msg += ": " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionFailed, msg
	}
	var appConfig struct {
		Params []struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		} `json:"params"`
	}
	if err := s.callNodeJSON(source, http.MethodGet, "/api/v2/apps/installed/params/"+res.Key, nil, &appConfig, 20*time.Second); err != nil {
		return model.ClusterSyncActionFailed, "could not read source app params: " + truncateErr(err.Error())
	}
	params := map[string]interface{}{}
	for _, p := range appConfig.Params {
		if p.Key != "" {
			params[p.Key] = p.Value
		}
	}
	linkDB := res.Meta != nil && (toBool(res.Meta["linkDB"]) || toBool(res.Meta["hasDBHost"]))
	if _, ok := params["PANEL_DB_HOST"]; ok || linkDB {
		if strings.TrimSpace(opts.AppDBHost) == "" {
			return model.ClusterSyncActionFailed, "select the database the app should use on the target"
		}
		// The agent maps a database service name to its address/port.
		params["PANEL_DB_HOST"] = opts.AppDBHost
	}
	taskID := newSyncTaskID()
	body := map[string]interface{}{"appDetailId": detail.ID, "name": res.Name, "params": params, "taskID": taskID}
	if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/apps/install", body, nil, 5*time.Minute); err != nil {
		return model.ClusterSyncActionFailed, "app install on target failed: " + truncateErr(err.Error())
	}
	if err := s.waitNodeTask(target, taskID, syncLongTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "app install on target failed: " + truncateErr(err.Error())
	}
	// Data: back up on the source and restore on the target.
	var record struct {
		DownloadAccountID uint   `json:"downloadAccountID"`
		FileDir           string `json:"fileDir"`
		FileName          string `json:"fileName"`
	}
	if err := s.callNodeJSON(source, http.MethodPost, "/api/v2/backups/backup", map[string]interface{}{
		"type": "app", "name": appKey, "detailName": res.Name, "isImmediate": true, "taskID": newSyncTaskID(),
		"description": "cluster app transfer to " + target.Name,
	}, &record, time.Hour); err != nil {
		return model.ClusterSyncActionFailed, "app installed on target, but the source backup failed (data not migrated): " + truncateErr(err.Error())
	}
	if record.FileName == "" {
		return model.ClusterSyncActionFailed, "app installed on target, but the source backup returned no file (data not migrated)"
	}
	accountID := record.DownloadAccountID
	if accountID == 0 {
		accountID = 1
	}
	var backupPath string
	if err := s.callNodeJSON(source, http.MethodPost, "/api/v2/backups/record/download", map[string]interface{}{
		"downloadAccountID": accountID, "fileDir": record.FileDir, "fileName": record.FileName,
	}, &backupPath, 30*time.Minute); err != nil || backupPath == "" {
		msg := "app installed on target, but the source backup could not be located (data not migrated)"
		if err != nil {
			msg += ": " + truncateErr(err.Error())
		}
		return model.ClusterSyncActionFailed, msg
	}
	dstBackup, err := s.moveArchive(source, target, backupPath, opts)
	if err != nil {
		return model.ClusterSyncActionFailed, "app installed on target, but moving the backup failed (data not migrated): " + err.Error()
	}
	defer s.removeNodeFile(target, dstBackup)
	taskID = newSyncTaskID()
	if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/backups/recover/byupload", map[string]interface{}{
		"downloadAccountID": 1, "type": "app", "name": appKey, "detailName": res.Name, "file": dstBackup, "taskID": taskID,
	}, nil, 5*time.Minute); err != nil {
		return model.ClusterSyncActionFailed, "app installed on target, but restoring data failed: " + truncateErr(err.Error())
	}
	if err := s.waitNodeTask(target, taskID, syncLongTaskTimeout); err != nil {
		return model.ClusterSyncActionFailed, "app installed on target, but restoring data failed: " + truncateErr(err.Error())
	}
	return model.ClusterSyncActionCreate, ""
}

func syncCertDescription(sourceName string, autoRenew bool) string {
	desc := "synced from cluster node " + sourceName
	if autoRenew {
		desc += " (auto-renewed on source; re-sync after renewal)"
	}
	return desc
}

func (s *ClusterSyncService) Execute(req dto.ClusterSyncExecuteReq) (uint, error) {
	if !validSyncKind(req.Kind) {
		return 0, buserr.New("ErrInvalidParams")
	}
	source, err := s.resolveNode(req.SourceNodeID)
	if err != nil {
		return 0, err
	}
	byKey, err := s.resolveResources(req.Kind, source, req.ResourceKeys, req.Path)
	if err != nil {
		return 0, err
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
	if syncExecuteAsync {
		// Image transfers and app migrations take minutes; the task id is
		// returned at once and the page polls the task.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					global.LOG.Errorf("cluster sync task %d panicked: %v", task.ID, r)
					_ = s.syncRepo.UpdateTask(task.ID, map[string]interface{}{"status": model.ClusterSyncTaskFailed})
				}
			}()
			s.runTask(task, req, source, byKey)
		}()
		return task.ID, nil
	}
	s.runTask(task, req, source, byKey)
	return task.ID, nil
}

// failStaleTasks marks tasks that stopped reporting as failed: a task whose
// goroutine died with the panel (restart, OOM kill) would otherwise stay
// "running" forever. Running tasks bump updated_at at every stage, and no
// stage waits longer than syncLongTaskTimeout.
func (s *ClusterSyncService) failStaleTasks() {
	cutoff := time.Now().Add(-(syncLongTaskTimeout + 15*time.Minute))
	_ = global.DB.Model(&model.ClusterSyncTask{}).
		Where("status = ? AND updated_at < ?", model.ClusterSyncTaskRunning, cutoff).
		Updates(map[string]interface{}{"status": model.ClusterSyncTaskFailed, "step": "interrupted (panel restarted or task stalled)"}).Error
}

// syncExecuteAsync runs Execute's transfers in the background. Tests turn
// it off to assert the finished task synchronously.
var syncExecuteAsync = true

func (s *ClusterSyncService) runTask(task *model.ClusterSyncTask, req dto.ClusterSyncExecuteReq, source model.ClusterNode, byKey map[string]dto.ClusterSyncResource) {
	opts := syncTransferOpts{Path: req.Path, TargetPath: req.TargetPath, Overwrite: req.OverwriteConflicts, AppDBHost: req.AppDBHost}
	succeeded, skipped, failed := 0, 0, 0
	var itemLabel string
	opts.Step = func(stage string) {
		_ = s.syncRepo.UpdateTask(task.ID, map[string]interface{}{
			"step": truncateErr(itemLabel + ": " + stage), "progress_done": 0, "progress_total": 0, "progress_rate": 0,
		})
	}
	opts.Progress = func(done, total, rate int64) {
		_ = s.syncRepo.UpdateTask(task.ID, map[string]interface{}{
			"progress_done": done, "progress_total": total, "progress_rate": rate,
		})
	}
	for _, targetID := range req.TargetNodeIDs {
		if targetID == req.SourceNodeID {
			continue
		}
		target, err := s.resolveNode(targetID)
		if err != nil {
			for _, key := range req.ResourceKeys {
				_ = s.record(task.ID, req.Kind, key, "", source.ID, targetID, "", model.ClusterSyncActionFailed, "target node not found", 0)
				failed++
			}
			continue
		}
		failedBefore := failed
		for _, key := range req.ResourceKeys {
			start := time.Now()
			res, ok := byKey[key]
			if !ok {
				_ = s.record(task.ID, req.Kind, key, "", source.ID, targetID, target.Name, model.ClusterSyncActionFailed, "resource not found on source node", time.Since(start).Milliseconds())
				failed++
				continue
			}
			action, reason := s.compareOnTarget(req.Kind, target, res, opts)
			if action == model.ClusterSyncActionFailed {
				_ = s.record(task.ID, req.Kind, key, res.Name, source.ID, targetID, target.Name, action, reason, time.Since(start).Milliseconds())
				failed++
				continue
			}
			// Same content is always skipped; a conflict is skipped unless
			// the caller explicitly opted into overwriting (never for apps:
			// an existing install is not replaced).
			if action == model.ClusterSyncActionSkipSame ||
				(action == model.ClusterSyncActionSkipConflict && (!req.OverwriteConflicts || req.Kind == dto.ClusterSyncKindApp)) {
				_ = s.record(task.ID, req.Kind, key, res.Name, source.ID, targetID, target.Name, action, reason, time.Since(start).Milliseconds())
				skipped++
				continue
			}
			itemLabel = res.Name + " → " + target.Name
			action, reason = s.transferOne(req.Kind, source, target, res, opts)
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
		nodeStatus := model.ClusterSyncTaskSuccess
		if failed > failedBefore {
			nodeStatus = model.ClusterSyncTaskPartial
		}
		if targetID != 0 {
			_ = clusterNodeRepo.Update(targetID, map[string]interface{}{
				"last_sync_status": nodeStatus, "last_sync_at": time.Now(),
			})
		}
	}
	status := model.ClusterSyncTaskSuccess
	if failed > 0 && (succeeded > 0 || skipped > 0) {
		status = model.ClusterSyncTaskPartial
	} else if failed > 0 {
		status = model.ClusterSyncTaskFailed
	}
	task.Total = succeeded + skipped + failed
	_ = s.syncRepo.UpdateTask(task.ID, map[string]interface{}{
		"status": status, "total": task.Total, "step": "",
		"progress_done": 0, "progress_total": 0, "progress_rate": 0,
		"succeeded": succeeded, "skipped": skipped, "failed": failed,
	})
}

func (s *ClusterSyncService) record(taskID uint, kind, key, name string, sourceID, targetID uint, targetName, action, reason string, durationMs int64) error {
	return s.syncRepo.CreateRecord(&model.ClusterSyncRecord{
		TaskID: taskID, Kind: kind, ResourceKey: key, ResourceName: name,
		SourceNodeID: sourceID, TargetNodeID: targetID, TargetName: targetName,
		Action: action, Reason: truncateErr(reason), DurationMs: durationMs,
	})
}

func (s *ClusterSyncService) SearchTasks(req dto.ClusterSyncTaskSearch) (int64, interface{}, error) {
	s.failStaleTasks()
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
