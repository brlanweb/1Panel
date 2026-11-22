package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/buserr"
)

const testClashYAML = "proxies:\n  - {name: a, type: ss, server: 1.1.1.1, port: 1, cipher: aes-128-gcm, password: x}\n"

func TestDownloadClashProxiesRetriesUserAgents(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.UserAgent())
		// emulate a WAF that only lets one known Clash client through
		if r.UserAgent() != "mihomo/1.19.0" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(testClashYAML))
	}))
	defer srv.Close()
	proxies, err := downloadClashProxies(srv.URL)
	if err != nil || len(proxies) != 1 {
		t.Fatalf("proxies=%v err=%v (UAs tried %v)", proxies, err, seen)
	}
}

func TestDownloadClashProxiesForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := downloadClashProxies(srv.URL)
	var be buserr.BusinessError
	if !errors.As(err, &be) || be.Msg != "ErrProxySubscription403" {
		t.Fatalf("want ErrProxySubscription403, got %v", err)
	}
}
