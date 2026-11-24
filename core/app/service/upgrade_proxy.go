package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/utils/xpack"
)

// Download route of a panel upgrade package. The user picks it in the
// upgrade drawer: the panel's global proxy, the subscription proxy node
// (mihomo container managed by the local agent) or a direct connection.

const (
	upgradeProxyAuto   = "auto"
	upgradeProxyDirect = "direct"
	upgradeProxyPanel  = "panel"
	upgradeProxyNode   = "node"
)

// localProxyNodeStatus is the part of the agent's /settings/proxy-node
// answer used here (agent dto.ProxyNodeStatus).
type localProxyNodeStatus struct {
	SubscriptionURL string `json:"subscriptionUrl"`
	Running         bool   `json:"running"`
	ProxyURL        string `json:"proxyUrl"`
	Selected        string `json:"selected"`
}

// loadLocalProxyNode is swapped in tests.
var loadLocalProxyNode = func() (localProxyNodeStatus, error) {
	var st localProxyNodeStatus
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := (&ClusterSyncService{}).callLocalJSON(ctx, http.MethodGet, "/api/v2/settings/proxy-node", nil, &st, 15*time.Second)
	return st, err
}

func panelProxyAddr() string {
	proxyType, _ := settingRepo.GetValueByKey("ProxyType")
	proxyType = strings.TrimSpace(proxyType)
	if proxyType == "" || proxyType == "close" {
		return ""
	}
	host, _ := settingRepo.GetValueByKey("ProxyUrl")
	port, _ := settingRepo.GetValueByKey("ProxyPort")
	host = strings.TrimSpace(host)
	for _, prefix := range []string{proxyType + "://", "http://", "https://", "socks5://"} {
		host = strings.TrimPrefix(host, prefix)
	}
	if host == "" || strings.TrimSpace(port) == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", proxyType, net.JoinHostPort(strings.TrimRight(host, "/"), strings.TrimSpace(port)))
}

func (u *UpgradeService) LoadUpgradeProxyOptions() dto.UpgradeProxyOptions {
	var opts dto.UpgradeProxyOptions
	if addr := panelProxyAddr(); addr != "" {
		opts.Panel, opts.PanelAddr = true, addr
	}
	st, err := loadLocalProxyNode()
	switch {
	case err != nil:
		opts.NodeError = err.Error()
	case st.SubscriptionURL == "":
		opts.NodeError = "no proxy subscription configured"
	case !st.Running:
		opts.NodeError = "the proxy node container is not running"
	case st.ProxyURL == "":
		opts.NodeError = "the proxy node has no address"
	default:
		opts.Node, opts.NodeName = true, st.Selected
	}
	return opts
}

func directTransport() *http.Transport {
	return &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{},
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
}

// upgradeTransport returns the transport for the chosen route; nil means
// the default (panel proxy when configured, else direct).
func upgradeTransport(mode string) (*http.Transport, error) {
	switch mode {
	case "", upgradeProxyAuto:
		return nil, nil
	case upgradeProxyDirect:
		return directTransport(), nil
	case upgradeProxyPanel:
		if panelProxyAddr() == "" {
			return nil, fmt.Errorf("the panel proxy is not configured (Settings → Panel → Proxy)")
		}
		return xpack.MultiNodeProvider.LoadRequestTransport(), nil
	case upgradeProxyNode:
		st, err := loadLocalProxyNode()
		if err != nil {
			return nil, fmt.Errorf("load the subscription proxy failed: %v", err)
		}
		if st.SubscriptionURL == "" || !st.Running || st.ProxyURL == "" {
			return nil, fmt.Errorf("the subscription proxy is not running (Settings → Panel → Proxy → Subscription)")
		}
		proxyURL, err := url.Parse(st.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid subscription proxy address: %v", err)
		}
		transport := directTransport()
		transport.Proxy = http.ProxyURL(proxyURL)
		return transport, nil
	default:
		return nil, fmt.Errorf("unknown download proxy %q", mode)
	}
}
