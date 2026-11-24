package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/global"
)

// panelAPIBase returns the API base (scheme://host) of a panel entry URL.
//
// The security entrance is a UI-only entry path: the panel UI lives at
// /<entrance> but the API lives at the host root (/api/v2/...). A request to
// /<entrance>/api/v2/... matches no API route and falls through to NoRoute,
// so the entrance path segment must be stripped before building any API
// target. Jump links that open the panel UI keep the full EntryURL.
func panelAPIBase(entryURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(entryURL))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// ResolveClusterNode looks a registered panel node up by name.
// "local" and "" are the master panel itself, never a cluster node.
func (u *ClusterNodeService) ResolveClusterNode(name string) (model.ClusterNode, error) {
	if name == "" || name == "local" {
		return model.ClusterNode{}, fmt.Errorf("local node")
	}
	node, err := clusterNodeRepo.Get(repo.WithByName(name))
	if err != nil || node.ID == 0 {
		return model.ClusterNode{}, fmt.Errorf("node %q is not a registered cluster node", name)
	}
	return node, nil
}

// ListOptions returns the bottom-switcher options for the registered panel
// nodes: name, panel address, and health mapped onto the official switcher
// vocabulary (online -> Healthy, offline -> Unhealthy, anything else ->
// Unknown). The master ("local") entry is added by the frontend.
func (u *ClusterNodeService) ListOptions() ([]dto.ClusterNodeOption, error) {
	nodes, err := clusterNodeRepo.GetList()
	if err != nil {
		return nil, err
	}
	options := make([]dto.ClusterNodeOption, 0, len(nodes))
	for _, node := range nodes {
		status := "Unknown"
		switch node.Status {
		case model.ClusterNodeStatusOnline:
			status = "Healthy"
		case model.ClusterNodeStatusOffline:
			status = "Unhealthy"
		}
		options = append(options, dto.ClusterNodeOption{
			ID:      node.ID,
			Name:    node.Name,
			Addr:    strings.TrimRight(node.EntryURL, "/"),
			Status:  status,
			IsBound: true,
			Version: node.Version,
			Mode:    node.EffectiveMode(),
		})
	}
	return options, nil
}

func clusterProxyTransport() *http.Transport {
	return sharedClusterTransports.getPanelTransport(false)
}

func clusterProxyWebsocketTransport() *http.Transport {
	return sharedClusterTransports.getPanelTransport(true)
}

func isWebsocketUpgrade(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, token := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return true
		}
	}
	return false
}

// clusterProxyTransportFor picks the websocket transport for upgrades and
// the bounded transport for everything else.
func clusterProxyTransportFor(r *http.Request) *http.Transport {
	if isWebsocketUpgrade(r) {
		return clusterProxyWebsocketTransport()
	}
	return clusterProxyTransport()
}

// proxyError writes a 1Panel-envelope error for a failed proxied request.
// It never falls back to local data: showing master data while the user
// believes they are operating node B would silently apply changes to the
// wrong panel.
func proxyError(w http.ResponseWriter, nodeName string, err error) {
	if global.LOG != nil {
		global.LOG.Warnf("cluster proxy to node %q failed: %v", nodeName, err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = fmt.Fprintf(w, `{"code":502,"message":"node %s unreachable: %s. Switch back to the master node and retry."}`,
		nodeName, truncateErr(err.Error()))
}

// remoteAuthFailureHint maps a remote 1Panel auth error key (the `message`
// field of its error envelope) onto a Chinese hint telling the operator
// what to check on the remote panel. It returns "" for keys that are not
// auth failures. The timestamp case covers both an expired ApiKeyValidityTime
// window and clock skew between the two panels (the remote rejects a
// timestamp more than 60s in the future, or older than its validity window).
func remoteAuthFailureHint(msgKey string) string {
	switch strings.TrimSpace(msgKey) {
	case "ErrApiConfigStatusInvalid":
		return "远程面板 API 接口未开启，请在远程面板「面板设置 > API 接口」中开启后再切换"
	case "ErrApiConfigKeyInvalid":
		return "远程面板 API Key 错误，请检查节点表单里粘贴的 Key 是否为该远程面板的接口密钥"
	case "ErrApiConfigKeyTimeInvalid":
		return "远程面板 API Key 时间无效，请检查主节点与远程面板的服务器时间是否同步（时间相差过大时远程会拒绝请求）"
	case "ErrApiConfigKeyExpired":
		return "远程面板 API Key 已过期，请在远程面板重新生成"
	case "ErrApiConfigIPInvalid":
		return "远程面板 IP 白名单拒绝了主节点的访问，请把主节点出口 IP 加入远程面板 API 接口的白名单"
	default:
		return ""
	}
}

// remoteAuthFailureEnvelope builds the 502 envelope the master returns when
// a remote panel rejects the proxied API-key auth. The code is deliberately
// 502, never 401/403: the master's frontend treats any 401/403 envelope
// code as "my own session expired" and redirects to the login page
// (frontend/src/utils/auth-response.ts), which is exactly the bug this
// fixes — a remote auth problem must surface as a node error, never as a
// master login redirect.
func remoteAuthFailureEnvelope(msgKey string) []byte {
	hint := remoteAuthFailureHint(msgKey)
	if hint == "" {
		hint = "远程面板拒绝了 API 访问（接口未开启或 API Key 错误/过期/白名单），请在远程面板设置里检查"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"code":    http.StatusBadGateway,
		"message": hint,
		"data":    nil,
	})
	return body
}

// maxProxiedAuthSniffBytes bounds how much of a proxied JSON body is read
// while looking for an auth-failure envelope. Auth envelopes are tiny
// (<1KB); anything larger cannot be one, so it is passed through untouched.
const maxProxiedAuthSniffBytes = 1024 * 1024

// replaceProxiedBody swaps a proxied response body (used for normalized
// auth failures). It fixes Content-Length and drops Content-Encoding so the
// replacement is served exactly as written.
func replaceProxiedBody(resp *http.Response, status int, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Del("Content-Encoding")
	resp.StatusCode = status
	resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
}

// normalizeProxiedAuthFailure rewrites a remote auth failure into a 502
// envelope. It runs as the ReverseProxy ModifyResponse hook.
//
// Why this is needed: the remote panel's BadAuth helper always answers
// HTTP 200 with an envelope {"code":401,...} (core/app/api/v2/helper
// ErrorWithDetail answers ctx.JSON(http.StatusOK, ...) for every error),
// so a naive pass-through hands the master frontend an envelope code 401,
// and its response interceptor redirects to the login page. Streaming
// responses (SSE logs, downloads, websockets) are never buffered or
// rewritten: they are not JSON, and an auth failure on those paths arrives
// as an HTTP 401/403 status instead, which is rewritten from the status
// alone. A non-JSON 200 body and any non-auth JSON envelope pass through
// byte-for-byte.
func normalizeProxiedAuthFailure(resp *http.Response) error {
	if resp.StatusCode == http.StatusSwitchingProtocols {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msgKey := ""
		if body, err := io.ReadAll(io.LimitReader(resp.Body, maxProxiedAuthSniffBytes)); err == nil {
			var env remoteEnvelope
			if json.Unmarshal(body, &env) == nil {
				msgKey = env.Message
			}
		}
		_ = resp.Body.Close()
		replaceProxiedBody(resp, http.StatusBadGateway, remoteAuthFailureEnvelope(msgKey))
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProxiedAuthSniffBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxProxiedAuthSniffBytes {
		// Too large to be an auth envelope: replay it untouched.
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), resp.Body))
		return nil
	}
	_ = resp.Body.Close()
	var env remoteEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	if env.Code != http.StatusUnauthorized && env.Code != http.StatusForbidden {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	replaceProxiedBody(resp, http.StatusBadGateway, remoteAuthFailureEnvelope(env.Message))
	return nil
}

// named nodeName and streams the remote response back unchanged.
//
// It returns false when nodeName is not a registered cluster node, so the
// caller can fall through to the previous (xpack stub) behaviour. It
// returns true when the request was proxied or answered with an error.
//
// Auth: the master session was already validated by Proxy() before this is
// called, and isAdmin reports whether that session belongs to a super
// admin (the stored remote API key is equivalent to super admin on the
// target panel, so only master admins may operate a node through the
// proxy). The remote panel is authenticated with the node's stored Legacy
// API Key (HMAC-SHA256). The master session cookie / CSRF token must never
// reach the remote panel, and the key itself is never logged.
func (u *ClusterNodeService) ProxyRequest(w http.ResponseWriter, r *http.Request, nodeName string, isAdmin bool) bool {
	node, err := u.ResolveClusterNode(nodeName)
	if err != nil {
		return false
	}
	if !isAdmin {
		// Deliberately 502, not 403: a 401/403 envelope code makes the
		// master frontend redirect to the login page (see
		// remoteAuthFailureEnvelope). This is a master-side refusal, not
		// a proxied failure, but it must surface as an error, never as a
		// login redirect.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":502,"message":"只有主节点管理员才能操作集群节点"}`))
		return true
	}
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		proxyError(w, nodeName, err)
		return true
	}
	target, err := url.Parse(apiBase)
	if err != nil {
		proxyError(w, nodeName, err)
		return true
	}
	apiKey, err := u.decryptNodeKey(node)
	if err != nil {
		proxyError(w, nodeName, fmt.Errorf("credential decrypt failed"))
		return true
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	token := auth.GenerateHMACSHA256(apiKey, "1panel:"+timestamp)

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Keep the original path and query, but the remote panel must
			// see this as a local request: drop the node selector so its
			// own proxy does not try to resolve the name again.
			query := req.URL.Query()
			query.Del("operateNode")
			req.URL.RawQuery = query.Encode()
			req.Host = target.Host
			req.Header.Set("CurrentNode", "local")
			req.Header.Set("1Panel-Timestamp", timestamp)
			req.Header.Set("1Panel-Token", token)
			// Websocket upgrades (terminals) must not carry the browser's
			// cross-site origin headers: the remote core's
			// WebSocketOriginGuard rejects any upgrade whose Origin host
			// differs from the request host, and any Sec-Fetch-Site other
			// than same-origin. The browser's handshake carries the
			// master's Origin (and Sec-Fetch-Site: cross-site) while this
			// proxy rewrites req.Host to the remote host, so every proxied
			// terminal upgrade was 403'd before reaching the agent — while
			// the plain-GET preflight and plain HTTP/SSE logs passed the
			// guard untouched, which is exactly the "logs work, terminal
			// doesn't connect" symptom. Dropping both headers is what the
			// guard itself allows (absent Origin passes); the remote
			// agent's own upgrader accepts any origin, and auth is still
			// enforced by the API key above. Non-upgrade requests keep
			// their headers untouched.
			if isWebsocketUpgrade(req) {
				req.Header.Del("Origin")
				req.Header.Del("Sec-Fetch-Site")
			}
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			req.Header.Del("X-CSRF-Token")
			req.Header.Del("X-Panel-User")
			// Master-session terminal identity headers must never reach
			// the remote panel: the remote panel derives its own terminal
			// identity (and 90s lease) from the API Key above. Leaking
			// the master's session hash would also let the remote agent
			// mistake this request for a master-local terminal session.
			req.Header.Del("X-Panel-User-ID")
			req.Header.Del("X-Panel-Auth-Session-ID")
			req.Header.Del("X-Panel-Auth-Lease-Until")
		},
		// Websocket upgrades (terminals) use the no-timeout transport:
		// a shell session is long-lived and idle by nature, and the
		// regular transport's 60s response-header / 90s idle limits
		// must never apply to it. Plain HTTP/SSE/download requests keep
		// the bounded transport.
		Transport: clusterProxyTransportFor(r),
		// Normalize remote auth failures into 502 envelopes so a remote
		// 401 never becomes a master login redirect. Streaming bodies
		// (SSE, downloads, websockets) pass through untouched.
		ModifyResponse: normalizeProxiedAuthFailure,
		// Flush streaming responses (SSE logs, downloads) immediately.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			proxyError(w, nodeName, err)
		},
	}
	proxy.ServeHTTP(w, r)
	return true
}
