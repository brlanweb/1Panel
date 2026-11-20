package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateEntryURL(t *testing.T) {
	valid := []string{
		"https://192.168.1.10:8443",
		"http://panel.example.com:8080/abc123",
		"https://[2001:db8::1]:8443",
	}
	for _, u := range valid {
		if err := validateEntryURL(u); err != nil {
			t.Errorf("validateEntryURL(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"",
		"ftp://example.com",
		"file:///etc/passwd",
		"https://user:pass@example.com",
		"example.com:8080",
		"//example.com",
	}
	for _, u := range invalid {
		if err := validateEntryURL(u); err == nil {
			t.Errorf("validateEntryURL(%q) = nil, want error", u)
		}
	}
}

func TestClusterKeyHint(t *testing.T) {
	if got := clusterKeyHint("abcdefgh"); got != "****efgh" {
		t.Errorf("clusterKeyHint = %q, want ****efgh", got)
	}
	if got := clusterKeyHint("abc"); got != "****" {
		t.Errorf("clusterKeyHint(short) = %q, want ****", got)
	}
}

func TestTruncateErr(t *testing.T) {
	if got := truncateErr(strings.Repeat("x", 600)); len(got) != 500 {
		t.Errorf("truncateErr len = %d, want 500", len(got))
	}
}

// TestProbeNodeAuthFailures is the regression test for the second half of
// the "switching jumps back to login" bug: the old probe only looked at the
// HTTP status of GET /api/v2/health/check, but the remote panel's BadAuth
// helper always answers HTTP 200 with an envelope {"code":401,...}, so a
// disabled API interface or a wrong key was reported as "online" and only
// failed at switch time. The probe now POSTs /api/v2/settings/search —
// the same auth path the switch itself uses — and parses the envelope.
func TestProbeNodeAuthFailures(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	svc := &ClusterNodeService{lastTest: make(map[uint]time.Time)}

	cases := []struct {
		name       string
		envelope   string
		wantOnline bool
		wantHint   string
	}{
		{"valid key", `{"code":200,"message":"success","data":{"defaultNetwork":"eth0"}}`, true, ""},
		{"interface disabled", `{"code":401,"message":"ErrApiConfigStatusInvalid","data":null}`, false, "API 接口未开启"},
		{"wrong key", `{"code":401,"message":"ErrApiConfigKeyInvalid","data":null}`, false, "API Key 错误"},
		{"time invalid", `{"code":401,"message":"ErrApiConfigKeyTimeInvalid","data":null}`, false, "时间"},
		{"key expired", `{"code":401,"message":"ErrApiConfigKeyExpired","data":null}`, false, "过期"},
		{"IP whitelist", `{"code":401,"message":"ErrApiConfigIPInvalid","data":null}`, false, "白名单"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v2/settings/search" || r.Method != http.MethodPost {
					t.Errorf("probe hit %s %s, want POST /api/v2/settings/search", r.Method, r.URL.Path)
				}
				if r.Header.Get("1Panel-Token") == "" || r.Header.Get("1Panel-Timestamp") == "" {
					t.Error("probe sent no API auth headers")
				}
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = fmt.Fprint(w, tc.envelope)
			}))
			defer remote.Close()
			node := insertTestNode(t, "node-"+strings.ReplaceAll(tc.name, " ", "-"), remote.URL, "unknown")
			_, apiDisabled, errMsg := svc.probeNode(node)
			if tc.wantOnline {
				if apiDisabled || errMsg != "" {
					t.Errorf("probeNode = apiDisabled=%v err=%q, want online", apiDisabled, errMsg)
				}
				return
			}
			if !apiDisabled {
				t.Errorf("probeNode apiDisabled = false, want true (auth failure must be distinct from offline)")
			}
			if !strings.Contains(errMsg, tc.wantHint) {
				t.Errorf("probeNode err = %q, want it to contain %q", errMsg, tc.wantHint)
			}
		})
	}
}
