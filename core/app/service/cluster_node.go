package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"github.com/go-resty/resty/v2"
	"github.com/jinzhu/copier"
	"golang.org/x/sync/errgroup"
)

type ClusterNodeService struct {
	// testMu guards the per-node test throttle (one probe per node per 5s).
	testMu   sync.Mutex
	lastTest map[uint]time.Time
}

type IClusterNodeService interface {
	SearchWithPage(req dto.ClusterNodeSearch) (int64, interface{}, error)
	Create(req dto.ClusterNodeCreate) error
	Update(req dto.ClusterNodeUpdate) error
	Delete(id uint) error
	TestConnection(id uint) (dto.ClusterNodeInfo, error)
	CheckAll() error
	Overview() (dto.ClusterNodeOverview, error)
	Aggregate() (dto.ClusterAggregateOverview, error)
	ListOptions() ([]dto.ClusterNodeOption, error)
	// ProxyRequest forwards one request to the named panel node. It
	// returns false when the name is not a registered cluster node.
	// isAdmin reports whether the master session belongs to a super admin.
	ProxyRequest(w http.ResponseWriter, r *http.Request, nodeName string, isAdmin bool) bool
}

func NewIClusterNodeService() IClusterNodeService {
	return &ClusterNodeService{lastTest: make(map[uint]time.Time)}
}

func (u *ClusterNodeService) SearchWithPage(req dto.ClusterNodeSearch) (int64, interface{}, error) {
	options := []global.DBOption{
		repo.WithOrderDesc("created_at"),
		clusterNodeRepo.WithByInfo(req.Info),
	}
	if req.Status != "" {
		options = append(options, repo.WithByStatus(req.Status))
	}
	if req.GroupID != 0 {
		options = append(options, repo.WithByGroupID(req.GroupID))
	}
	total, nodes, err := clusterNodeRepo.Page(req.Page, req.PageSize, options...)
	if err != nil {
		return 0, nil, err
	}
	var infos []dto.ClusterNodeInfo
	for _, node := range nodes {
		var item dto.ClusterNodeInfo
		if err := copier.Copy(&item, &node); err != nil {
			return 0, nil, buserr.WithDetail("ErrStructTransform", err.Error(), nil)
		}
		infos = append(infos, item)
	}
	return total, infos, nil
}

func validateEntryURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return buserr.WithDetail("ErrInvalidParams", err.Error(), nil)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return buserr.New("ErrInvalidParams")
	}
	if parsed.Host == "" || parsed.User != nil {
		return buserr.New("ErrInvalidParams")
	}
	return nil
}

func clusterKeyHint(apiKey string) string {
	if len(apiKey) <= 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}

func encryptNodeKey(apiKey string, id uint) (string, error) {
	key, err := encrypt.APIKeyEncryptionKey()
	if err != nil {
		return "", err
	}
	return encrypt.EncryptAPIKey(apiKey, strconv.FormatUint(uint64(id), 10), key)
}

func (u *ClusterNodeService) decryptNodeKey(node model.ClusterNode) (string, error) {
	key, err := encrypt.APIKeyEncryptionKey()
	if err != nil {
		return "", err
	}
	return encrypt.DecryptAPIKey(node.APIKeyCipher, strconv.FormatUint(uint64(node.ID), 10), key)
}

func (u *ClusterNodeService) Create(req dto.ClusterNodeCreate) error {
	if err := validateEntryURL(req.EntryURL); err != nil {
		return err
	}
	if existing, _ := clusterNodeRepo.Get(repo.WithByName(req.Name)); existing.ID != 0 {
		return buserr.New("ErrRecordExist")
	}
	var node model.ClusterNode
	if err := copier.Copy(&node, &req); err != nil {
		return buserr.WithDetail("ErrStructTransform", err.Error(), nil)
	}
	node.EntryURL = strings.TrimRight(strings.TrimSpace(req.EntryURL), "/")
	node.KeyHint = clusterKeyHint(req.APIKey)
	node.Status = model.ClusterNodeStatusUnknown
	// Insert first so the row has an ID; the AES-GCM AAD is bound to that ID.
	if err := clusterNodeRepo.Create(&node); err != nil {
		return err
	}
	cipher, err := encryptNodeKey(req.APIKey, node.ID)
	if err != nil {
		_ = clusterNodeRepo.Delete(repo.WithByID(node.ID))
		return err
	}
	return clusterNodeRepo.Update(node.ID, map[string]interface{}{"api_key_cipher": cipher})
}

func (u *ClusterNodeService) Update(req dto.ClusterNodeUpdate) error {
	if err := validateEntryURL(req.EntryURL); err != nil {
		return err
	}
	node, _ := clusterNodeRepo.Get(repo.WithByID(req.ID))
	if node.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	if existing, _ := clusterNodeRepo.Get(repo.WithByName(req.Name), repo.WithoutByName(node.Name)); existing.ID != 0 {
		return buserr.New("ErrRecordExist")
	}
	upMap := map[string]interface{}{
		"name":      req.Name,
		"entry_url": strings.TrimRight(strings.TrimSpace(req.EntryURL), "/"),
		"remark":    req.Remark,
		"group_id":  req.GroupID,
	}
	if strings.TrimSpace(req.APIKey) != "" {
		cipher, err := encryptNodeKey(strings.TrimSpace(req.APIKey), node.ID)
		if err != nil {
			return err
		}
		upMap["api_key_cipher"] = cipher
		upMap["key_hint"] = clusterKeyHint(strings.TrimSpace(req.APIKey))
	}
	return clusterNodeRepo.Update(req.ID, upMap)
}

func (u *ClusterNodeService) Delete(id uint) error {
	node, _ := clusterNodeRepo.Get(repo.WithByID(id))
	if node.ID == 0 {
		return buserr.New("ErrRecordNotFound")
	}
	// Only the registry row is removed; nothing on the remote panel is touched.
	return clusterNodeRepo.Delete(repo.WithByID(id))
}

func (u *ClusterNodeService) Overview() (dto.ClusterNodeOverview, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return dto.ClusterNodeOverview{}, err
	}
	var overview dto.ClusterNodeOverview
	overview.Total = int64(len(nodes))
	for _, node := range nodes {
		switch node.Status {
		case model.ClusterNodeStatusOnline:
			overview.Online++
		case model.ClusterNodeStatusOffline:
			overview.Offline++
		default:
			overview.Unknown++
		}
	}
	return overview, nil
}

// remoteEnvelope is the response envelope every 1Panel core API returns
// ({code, message, data}). Remote calls are only trusted when the HTTP
// status is 200 AND code is 200.
type remoteEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// callRemote performs one signed request against a remote panel's core API
// (path is relative to the panel entry URL, e.g. "/api/v2/settings/search").
// It signs with the legacy HMAC-SHA256 scheme first and falls back to the
// legacy MD5 scheme on HTTP 401. The API key is never logged. timeout
// bounds the whole request. Note the remote panel answers auth failures as
// HTTP 200 with an envelope {"code":401,...}, so callers must parse the
// envelope — the HTTP status alone never proves the key works.
func (u *ClusterNodeService) callRemote(node model.ClusterNode, method, path string, body interface{}, timeout time.Duration) (int, []byte, error) {
	apiKey, err := u.decryptNodeKey(node)
	if err != nil {
		return 0, nil, fmt.Errorf("credential decrypt failed")
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	// The API lives at the panel origin; the entrance path segment (if any)
	// must not be part of the target, or the remote panel answers NoRoute.
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		return 0, nil, err
	}
	targetURL := apiBase + path
	client := resty.New().
		SetTimeout(timeout).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	// Remote panels usually use self-signed certificates; TLS verification is
	// skipped (same as the existing multi-node transport). Only add nodes you trust.
	doRequest := func(token string) (*resty.Response, error) {
		req := client.R().
			SetHeader("1Panel-Timestamp", timestamp).
			SetHeader("1Panel-Token", token)
		if body != nil {
			req = req.SetHeader("Content-Type", "application/json").SetBody(body)
		}
		switch method {
		case http.MethodGet:
			return req.Get(targetURL)
		case http.MethodPost:
			return req.Post(targetURL)
		default:
			return nil, fmt.Errorf("unsupported method %s", method)
		}
	}
	resp, err := doRequest(auth.GenerateHMACSHA256(apiKey, "1panel:"+timestamp))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode() == http.StatusUnauthorized {
		resp, err = doRequest(auth.GenerateMD5("1panel" + apiKey + timestamp))
		if err != nil {
			return 0, nil, err
		}
	}
	return resp.StatusCode(), resp.Body(), nil
}

// callRemoteJSON calls a remote JSON API and unmarshals its `data` field
// into out. A non-200 HTTP status or a non-200 envelope code is an error
// whose message is the remote message (truncated), never the raw body.
func (u *ClusterNodeService) callRemoteJSON(node model.ClusterNode, method, path string, body interface{}, out interface{}, timeout time.Duration) error {
	status, raw, err := u.callRemote(node, method, path, body, timeout)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("remote panel returned HTTP %d", status)
	}
	var env remoteEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("remote panel returned an unreadable response")
	}
	if env.Code != http.StatusOK {
		if strings.TrimSpace(env.Message) != "" {
			return fmt.Errorf("remote panel error: %s", truncateErr(env.Message))
		}
		return fmt.Errorf("remote panel error code %d", env.Code)
	}
	if out == nil {
		return nil
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// remoteAuthHeaders returns the timestamp and both legacy signature tokens
// for a node, for callers that must build their own request (streaming
// download, multipart upload). The key itself is never returned.
func (u *ClusterNodeService) remoteAuthHeaders(node model.ClusterNode) (timestamp, hmacToken, md5Token string, err error) {
	apiKey, err := u.decryptNodeKey(node)
	if err != nil {
		return "", "", "", fmt.Errorf("credential decrypt failed")
	}
	timestamp = strconv.FormatInt(time.Now().Unix(), 10)
	return timestamp, auth.GenerateHMACSHA256(apiKey, "1panel:"+timestamp), auth.GenerateMD5("1panel" + apiKey + timestamp), nil
}

// remotePageTotal calls a remote paged search endpoint with a pageSize of 1
// and returns only its total, so aggregate counts stay cheap.
func (u *ClusterNodeService) remotePageTotal(node model.ClusterNode, path string, body map[string]interface{}) (int, error) {
	var page struct {
		Total int64 `json:"total"`
	}
	if err := u.callRemoteJSON(node, http.MethodPost, path, body, &page, 10*time.Second); err != nil {
		return 0, err
	}
	return int(page.Total), nil
}

// Aggregate refreshes one live snapshot per node (CPU/memory from the remote
// GET /dashboard/current/node, app/website totals from the remote paged
// search endpoints) and persists the snapshot on the node row. A node whose
// refresh fails keeps its last good snapshot and is reported with
// FetchError instead of failing the whole page. Nodes stored as offline are
// not probed; their snapshot reports the stored LastError.
func (u *ClusterNodeService) Aggregate() (dto.ClusterAggregateOverview, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return dto.ClusterAggregateOverview{}, err
	}
	overview, err := u.Overview()
	if err != nil {
		return dto.ClusterAggregateOverview{}, err
	}
	result := dto.ClusterAggregateOverview{ClusterNodeOverview: overview}
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(5)
	var mu sync.Mutex
	for _, node := range nodes {
		node := node
		group.Go(func() error {
			snap := dto.ClusterNodeSnapshot{
				NodeID: node.ID, Name: node.Name, Status: node.Status,
				CpuUsedPercent: node.CpuUsedPercent, MemUsedPercent: node.MemUsedPercent,
				AppCount: node.AppCount, WebsiteCount: node.WebsiteCount,
				RefreshedAt: time.Now().Format(time.RFC3339),
			}
			if node.Status == model.ClusterNodeStatusOffline {
				snap.FetchError = node.LastError
				if snap.FetchError == "" {
					snap.FetchError = "node is offline (last check failed)"
				}
				mu.Lock()
				result.Nodes = append(result.Nodes, snap)
				mu.Unlock()
				return nil
			}
			var current struct {
				CPUUsedPercent    float64 `json:"cpuUsedPercent"`
				CPUTotal          int     `json:"cpuTotal"`
				MemoryUsedPercent float64 `json:"memoryUsedPercent"`
				MemoryTotal       uint64  `json:"memoryTotal"`
			}
			if err := u.callRemoteJSON(node, http.MethodGet, "/api/v2/dashboard/current/node", nil, &current, 10*time.Second); err != nil {
				snap.FetchError = truncateErr(err.Error())
			} else {
				snap.CpuUsedPercent = current.CPUUsedPercent
				snap.MemUsedPercent = current.MemoryUsedPercent
				snap.CpuTotal = current.CPUTotal
				snap.MemTotal = current.MemoryTotal
			}
			if snap.FetchError == "" {
				if total, err := u.remotePageTotal(node, "/api/v2/apps/installed/search", map[string]interface{}{"page": 1, "pageSize": 1, "all": true}); err != nil {
					snap.FetchError = truncateErr(err.Error())
				} else {
					snap.AppCount = total
				}
			}
			if snap.FetchError == "" {
				if total, err := u.remotePageTotal(node, "/api/v2/websites/search", map[string]interface{}{"page": 1, "pageSize": 1, "orderBy": "created_at", "order": "descending"}); err != nil {
					snap.FetchError = truncateErr(err.Error())
				} else {
					snap.WebsiteCount = total
				}
			}
			if snap.FetchError == "" {
				_ = clusterNodeRepo.Update(node.ID, map[string]interface{}{
					"cpu_used_percent": snap.CpuUsedPercent,
					"mem_used_percent": snap.MemUsedPercent,
					"app_count":        snap.AppCount,
					"website_count":    snap.WebsiteCount,
				})
			}
			mu.Lock()
			result.Nodes = append(result.Nodes, snap)
			if snap.FetchError == "" {
				result.TotalApps += snap.AppCount
				result.TotalWebsites += snap.WebsiteCount
			}
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return dto.ClusterAggregateOverview{}, err
	}
	sort.SliceStable(result.Nodes, func(i, j int) bool { return result.Nodes[i].NodeID < result.Nodes[j].NodeID })
	return result, nil
}

// probeNode checks the remote panel's API auth using the same path the
// node switch uses: POST /api/v2/settings/search, the agent endpoint the
// switch probe (loadGlobalSetting) reads right before switching. The old
// probe used GET /api/v2/health/check and only looked at the HTTP status,
// which never caught a bad key: the remote panel's BadAuth helper always
// answers HTTP 200 with an envelope {"code":401,...}, so a disabled API
// interface or a wrong key was reported as "online" and only failed later,
// at switch time, with a login redirect.
//
// The probe therefore parses the envelope: code 200 means the key works;
// code 401/403 means the key is rejected, mapped onto a specific hint by
// remoteAuthFailureHint (interface disabled / wrong key / time invalid /
// expired / IP whitelist). Any other failure is "offline" with the remote
// message attached. The key itself is never logged.
func (u *ClusterNodeService) probeNode(node model.ClusterNode) (latencyMs int64, apiDisabled bool, errMsg string) {
	start := time.Now()
	status, raw, err := u.callRemote(node, http.MethodPost, "/api/v2/settings/search", map[string]interface{}{}, 5*time.Second)
	latencyMs = time.Since(start).Milliseconds()
	if err != nil {
		return latencyMs, false, truncateErr(err.Error())
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return latencyMs, true, remoteAuthFailureHint("")
	}
	if status != http.StatusOK {
		return latencyMs, false, fmt.Sprintf("remote panel returned HTTP %d", status)
	}
	var env remoteEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return latencyMs, false, "remote panel returned an unreadable response"
	}
	switch env.Code {
	case http.StatusOK:
		return latencyMs, false, ""
	case http.StatusUnauthorized, http.StatusForbidden:
		hint := remoteAuthFailureHint(env.Message)
		if hint == "" {
			hint = "远程面板拒绝了 API 访问（接口未开启或 API Key 错误/过期/白名单），请在远程面板设置里检查"
		}
		return latencyMs, true, hint
	default:
		if strings.TrimSpace(env.Message) != "" {
			return latencyMs, false, fmt.Sprintf("remote panel error: %s", truncateErr(env.Message))
		}
		return latencyMs, false, fmt.Sprintf("remote panel error code %d", env.Code)
	}
}

func truncateErr(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}

func (u *ClusterNodeService) checkAndPersist(node model.ClusterNode) dto.ClusterNodeInfo {
	latency, _, errMsg := u.probeNode(node)
	now := time.Now()
	status := model.ClusterNodeStatusOnline
	if errMsg != "" {
		status = model.ClusterNodeStatusOffline
	}
	_ = clusterNodeRepo.Update(node.ID, map[string]interface{}{
		"status":        status,
		"latency_ms":    latency,
		"last_check_at": now,
		"last_error":    errMsg,
	})
	updated, _ := clusterNodeRepo.Get(repo.WithByID(node.ID))
	var info dto.ClusterNodeInfo
	_ = copier.Copy(&info, &updated)
	return info
}

func (u *ClusterNodeService) TestConnection(id uint) (dto.ClusterNodeInfo, error) {
	node, _ := clusterNodeRepo.Get(repo.WithByID(id))
	if node.ID == 0 {
		return dto.ClusterNodeInfo{}, buserr.New("ErrRecordNotFound")
	}
	// Throttle: a repeat test within 5s returns the stored result.
	u.testMu.Lock()
	last, ok := u.lastTest[id]
	if ok && time.Since(last) < 5*time.Second {
		u.testMu.Unlock()
		var info dto.ClusterNodeInfo
		_ = copier.Copy(&info, &node)
		return info, nil
	}
	u.lastTest[id] = time.Now()
	u.testMu.Unlock()
	return u.checkAndPersist(node), nil
}

func (u *ClusterNodeService) CheckAll() error {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return err
	}
	group, _ := errgroup.WithContext(context.Background())
	group.SetLimit(5)
	for _, node := range nodes {
		node := node
		group.Go(func() error {
			u.checkAndPersist(node)
			return nil
		})
	}
	return group.Wait()
}
