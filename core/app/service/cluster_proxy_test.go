package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/auth"
	"github.com/gorilla/websocket"
)

func TestPanelAPIBase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://192.168.1.10:8443/abc123", "https://192.168.1.10:8443"},
		{"https://192.168.1.10:8443/abc123/", "https://192.168.1.10:8443"},
		{"http://panel.example.com:8080", "http://panel.example.com:8080"},
	}
	for _, c := range cases {
		if got, err := panelAPIBase(c.in); err != nil || got != c.want {
			t.Errorf("panelAPIBase(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"ftp://example.com", "https:///no-host", "not a url ::"} {
		if _, err := panelAPIBase(bad); err == nil {
			t.Errorf("panelAPIBase(%q) = nil error, want error", bad)
		}
	}
}

func TestClusterProxyForwardsWithAPIAuth(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	var gotPath, gotQuery, gotToken, gotTimestamp, gotCurrentNode string
	var gotCookie bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotToken = r.Header.Get("1Panel-Token")
		gotTimestamp = r.Header.Get("1Panel-Timestamp")
		gotCurrentNode = r.Header.Get("CurrentNode")
		_, gotCookie = r.Header["Cookie"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"apps":2}}`))
	}))
	defer remote.Close()

	// EntryURL carries the security entrance: the API call must still hit
	// the remote host root, not /<entrance>/api/v2/...
	node := insertTestNode(t, "node-b", remote.URL+"/abc123", "online")
	_ = node

	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/apps/installed/search?page=1&operateNode=node-b", nil)
	req.Header.Set("Cookie", "session=master-session")
	req.Header.Set("CurrentNode", "node-b")
	req.Header.Set("X-CSRF-Token", "master-csrf")
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false for a registered node")
	}
	resp := rec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"apps":2`) {
		t.Fatalf("body = %s, want remote data", body)
	}
	if gotPath != "/api/v2/apps/installed/search" {
		t.Errorf("remote path = %q, want /api/v2/apps/installed/search (entrance must be stripped)", gotPath)
	}
	if strings.Contains(gotQuery, "operateNode") {
		t.Errorf("remote query = %q, operateNode must be stripped", gotQuery)
	}
	if !strings.Contains(gotQuery, "page=1") {
		t.Errorf("remote query = %q, want original params kept", gotQuery)
	}
	if gotCookie {
		t.Error("master session cookie was forwarded to the remote panel")
	}
	if gotCurrentNode != "local" {
		t.Errorf("CurrentNode forwarded as %q, want local", gotCurrentNode)
	}
	wantToken := auth.GenerateHMACSHA256("test-api-key", "1panel:"+gotTimestamp)
	if gotToken != wantToken {
		t.Errorf("1Panel-Token mismatch: remote must receive the HMAC-SHA256 legacy API auth")
	}
}

func TestClusterProxyUnknownNode(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/node", nil)
	if handled := svc.ProxyRequest(httptest.NewRecorder(), req, "ghost", true); handled {
		t.Error("ProxyRequest handled an unknown node, want false (fall through)")
	}
	if handled := svc.ProxyRequest(httptest.NewRecorder(), req, "local", true); handled {
		t.Error("ProxyRequest handled 'local', want false")
	}
}

func TestClusterProxyNonAdmin(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/apps/installed/search", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", false); !handled {
		t.Fatal("ProxyRequest returned false for a non-admin, want handled")
	}
	// Deliberately 502, not 403: a 401/403 envelope code makes the master
	// frontend redirect to the login page.
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "管理员") {
		t.Errorf("body = %s, want the admin-only message", rec.Body.String())
	}
}

// TestClusterProxyRemoteAuthEnvelope401 is the regression test for the
// "switching a node jumps back to login" bug. The remote panel's BadAuth
// helper always answers HTTP 200 with an envelope {"code":401,...}, and
// the master frontend redirects to login on any 401/403 envelope code.
// The proxy must therefore normalize a remote auth envelope into a 502
// envelope with a clear cause, never passing a 401/403 code through.
func TestClusterProxyRemoteAuthEnvelope401(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"code":401,"message":"ErrApiConfigStatusInvalid","data":null}`))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")
	svc := NewIClusterNodeService()
	for _, path := range []string{"/api/v2/settings/search", "/api/v2/dashboard/current/node"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
			t.Fatalf("ProxyRequest returned false for %s", path)
		}
		resp := rec.Result()
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("%s: status = %d, want 502", path, resp.StatusCode)
		}
		if strings.Contains(string(body), `"code":401`) || strings.Contains(string(body), `"code":403`) {
			t.Errorf("%s: body = %s, must not contain a 401/403 envelope code", path, body)
		}
		if !strings.Contains(string(body), `"code":502`) {
			t.Errorf("%s: body = %s, want a 502 envelope", path, body)
		}
		if !strings.Contains(string(body), "API 接口未开启") {
			t.Errorf("%s: body = %s, want the interface-disabled hint", path, body)
		}
	}
}

func TestClusterProxyRemoteHTTP401Status(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"ErrApiConfigKeyInvalid","data":null}`))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/node", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false")
	}
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if strings.Contains(string(body), `"code":401`) {
		t.Errorf("body = %s, must not contain a 401 envelope code", body)
	}
	if !strings.Contains(string(body), "API Key 错误") {
		t.Errorf("body = %s, want the wrong-key hint", body)
	}
}

func TestClusterProxyRemoteAuthTimeInvalid(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"code":401,"message":"ErrApiConfigKeyTimeInvalid","data":null}`))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/settings/search", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false")
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "时间") {
		t.Errorf("body = %s, want the clock-skew hint", rec.Body.String())
	}
}

// TestClusterProxyNonAuthEnvelopePassthrough guards the other side of the
// normalization: a non-auth remote envelope must pass through byte-for-byte
// (only 401/403 codes are rewritten).
func TestClusterProxyNonAuthEnvelopePassthrough(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"code":500,"message":"boom","data":null}`))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/node", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false")
	}
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 passthrough", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"code":500`) || !strings.Contains(string(body), "boom") {
		t.Errorf("body = %s, want the remote envelope untouched", body)
	}
}

func TestClusterProxyOfflineNode(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	// Port 1 on loopback refuses connections: deterministic offline node.
	_ = insertTestNode(t, "node-down", "http://127.0.0.1:1", "offline")
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/node", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-down", true); !handled {
		t.Fatal("ProxyRequest returned false for an offline node")
	}
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unreachable") {
		t.Errorf("body = %s, want an unreachable error, never local data", rec.Body.String())
	}
}

func TestClusterProxyWebSocket(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	upgrader := websocket.Upgrader{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("1Panel-Token") == "" {
			http.Error(w, "missing api auth", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, append([]byte("echo:"), msg...))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterNodeService()
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !svc.ProxyRequest(w, r, "node-b", true) {
			http.NotFound(w, r)
		}
	}))
	defer master.Close()

	wsURL := "ws" + strings.TrimPrefix(master.URL, "http") + "/api/v2/hosts/terminal/local?operateNode=node-b"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	defer conn.Close()
	_ = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read through proxy: %v", err)
	}
	if string(msg) != "echo:hello" {
		t.Errorf("ws reply = %q, want echo:hello", msg)
	}
}

func TestClusterProxyStreaming(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("remote ResponseWriter is not a Flusher")
			return
		}
		_, _ = w.Write([]byte("chunk1\n"))
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte("chunk2\n"))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/containers/search/log?follow=true", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false")
	}
	if got := rec.Body.String(); got != "chunk1\nchunk2\n" {
		t.Errorf("streamed body = %q, want chunked passthrough", got)
	}
}

func TestClusterProxyFileDownload(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	var gotQuery string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/api/v2/files/download" {
			t.Errorf("remote path = %q, want /api/v2/files/download", r.URL.Path)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="backup.tar.gz"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("fake-backup-bytes"))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterNodeService()
	// A browser download is a plain navigation: no CurrentNode header, the
	// node travels only in the operateNode query parameter.
	req := httptest.NewRequest(http.MethodGet, "/api/v2/files/download?path=%2Fopt%2Fbackup.tar.gz&operateNode=node-b", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "node-b", true); !handled {
		t.Fatal("ProxyRequest returned false")
	}
	resp := rec.Result()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake-backup-bytes" {
		t.Errorf("body = %q, want the remote file bytes", body)
	}
	if got := resp.Header.Get("Content-Disposition"); got != `attachment; filename="backup.tar.gz"` {
		t.Errorf("Content-Disposition = %q, want the remote filename preserved", got)
	}
	if strings.Contains(gotQuery, "operateNode") {
		t.Errorf("remote query = %q, operateNode must be stripped", gotQuery)
	}
	if !strings.Contains(gotQuery, "path=") {
		t.Errorf("remote query = %q, want the original path param kept", gotQuery)
	}
}

func TestClusterListOptions(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	_ = insertTestNode(t, "node-b", "https://10.0.0.2:8443", "online")
	_ = insertTestNode(t, "node-c", "https://10.0.0.3:8443", "offline")
	_ = insertTestNode(t, "node-d", "https://10.0.0.4:8443", "unknown")
	svc := NewIClusterNodeService()
	options, err := svc.ListOptions()
	if err != nil {
		t.Fatalf("ListOptions: %v", err)
	}
	if len(options) != 3 {
		t.Fatalf("len(options) = %d, want 3", len(options))
	}
	want := map[string]string{"node-b": "Healthy", "node-c": "Unhealthy", "node-d": "Unknown"}
	for _, o := range options {
		if want[o.Name] != o.Status {
			t.Errorf("node %s status = %q, want %q", o.Name, o.Status, want[o.Name])
		}
		if !o.IsBound {
			t.Errorf("node %s IsBound = false, want true", o.Name)
		}
	}
}

func TestClusterProxyTerminalForwardsWithAPIAuth(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()

	upgrader := websocket.Upgrader{}
	var gotToken string
	var leakedIdentity bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("1Panel-Token")
		if r.Header.Get("X-Panel-User-ID") != "" || r.Header.Get("X-Panel-Auth-Session-ID") != "" || r.Header.Get("X-Panel-Auth-Lease-Until") != "" {
			leakedIdentity = true
		}
		if r.URL.Path != "/api/v2/hosts/terminal/local" {
			http.NotFound(w, r)
			return
		}
		if gotToken == "" {
			http.Error(w, "missing api auth", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, append([]byte("term-echo:"), msg...))
	}))
	defer remote.Close()
	_ = insertTestNode(t, "node-b", remote.URL, "online")

	svc := NewIClusterNodeService()
	master := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !svc.ProxyRequest(w, r, "node-b", true) {
			http.NotFound(w, r)
		}
	}))
	defer master.Close()

	wsURL := "ws" + strings.TrimPrefix(master.URL, "http") + "/api/v2/hosts/terminal/local?cols=80&rows=24&operateNode=node-b"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial terminal through proxy: %v", err)
	}
	defer conn.Close()
	_ = conn.WriteMessage(websocket.TextMessage, []byte("ls"))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read terminal through proxy: %v", err)
	}
	if string(msg) != "term-echo:ls" {
		t.Errorf("ws reply = %q, want term-echo:ls", msg)
	}
	if gotToken == "" {
		t.Error("remote terminal upgrade carried no 1Panel-Token (API Key auth missing)")
	}
	if leakedIdentity {
		t.Error("master-session terminal identity headers leaked to the remote panel")
	}
}

func TestClusterProxyTerminalUnknownNode(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	svc := NewIClusterNodeService()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/hosts/terminal/local?operateNode=ghost", nil)
	rec := httptest.NewRecorder()
	if handled := svc.ProxyRequest(rec, req, "ghost", true); handled {
		t.Error("ProxyRequest returned true for an unregistered node terminal request; Proxy() must answer 502 instead")
	}
}
