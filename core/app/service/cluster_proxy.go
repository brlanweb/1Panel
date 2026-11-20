package service

import (
	"crypto/tls"
	"fmt"
	"net"
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
		})
	}
	return options, nil
}

func clusterProxyTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
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

// ProxyRequest forwards the incoming request to the registered panel node
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":403,"message":"only master admins can operate a cluster node"}`))
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
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			req.Header.Del("X-CSRF-Token")
			req.Header.Del("X-Panel-User")
		},
		Transport: clusterProxyTransport(),
		// Flush streaming responses (SSE logs, downloads) immediately.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			proxyError(w, nodeName, err)
		},
	}
	proxy.ServeHTTP(w, r)
	return true
}
