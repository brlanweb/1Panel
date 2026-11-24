package service

import (
	"errors"
	"net/http"
	"testing"
)

func TestUpgradeTransportNode(t *testing.T) {
	old := loadLocalProxyNode
	defer func() { loadLocalProxyNode = old }()

	loadLocalProxyNode = func() (localProxyNodeStatus, error) {
		return localProxyNodeStatus{SubscriptionURL: "https://sub", Running: true, ProxyURL: "http://u:p@127.0.0.1:7897"}, nil
	}
	tr, err := upgradeTransport(upgradeProxyNode)
	if err != nil || tr == nil || tr.Proxy == nil {
		t.Fatalf("node transport: %v %v", tr, err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://github.com/x", nil)
	u, _ := tr.Proxy(req)
	if u == nil || u.Host != "127.0.0.1:7897" || u.User.Username() != "u" {
		t.Fatalf("proxy url = %v", u)
	}

	loadLocalProxyNode = func() (localProxyNodeStatus, error) {
		return localProxyNodeStatus{SubscriptionURL: "https://sub", Running: false}, nil
	}
	if _, err := upgradeTransport(upgradeProxyNode); err == nil {
		t.Fatal("a stopped subscription proxy must be rejected")
	}
	loadLocalProxyNode = func() (localProxyNodeStatus, error) { return localProxyNodeStatus{}, errors.New("down") }
	if _, err := upgradeTransport(upgradeProxyNode); err == nil {
		t.Fatal("agent error must be reported")
	}
}

func TestUpgradeTransportDirectAndAuto(t *testing.T) {
	tr, err := upgradeTransport(upgradeProxyDirect)
	if err != nil || tr == nil || tr.Proxy != nil {
		t.Fatalf("direct transport must not use a proxy: %v %v", tr, err)
	}
	if tr, err := upgradeTransport(""); err != nil || tr != nil {
		t.Fatalf("auto must keep the default transport: %v %v", tr, err)
	}
	if _, err := upgradeTransport("bogus"); err == nil {
		t.Fatal("unknown mode must fail")
	}
}
