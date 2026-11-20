package service

import (
	"strings"
	"testing"
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
