package helper

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
	"golang.org/x/net/proxy"
)

// panelProxyTransport loads the panel's saved global proxy
// (ProxyType / ProxyUrl / ProxyPort / ProxyUser / ProxyPasswd) and returns
// a transport that dials through it. An empty or closed proxy, or a load
// failure, returns nil so the caller keeps a direct connection.
//
// Subscription proxies (ProxyNode*) are a separate feature and are not used
// for panel downloads.
func panelProxyTransport() (transport *http.Transport) {
	defer func() {
		if recover() != nil {
			transport = nil
		}
	}()
	settingRepo := repo.NewISettingRepo()
	proxyType, err := settingRepo.GetValueByKey("ProxyType")
	if err != nil {
		return nil
	}
	proxyType = strings.TrimSpace(proxyType)
	if proxyType == "" || proxyType == "close" {
		return nil
	}
	proxyURL, _ := settingRepo.GetValueByKey("ProxyUrl")
	proxyPort, _ := settingRepo.GetValueByKey("ProxyPort")
	proxyUser, _ := settingRepo.GetValueByKey("ProxyUser")
	proxyPasswd, _ := settingRepo.GetValueByKey("ProxyPasswd")
	if proxyPasswd != "" {
		if plain, decErr := encrypt.StringDecrypt(proxyPasswd); decErr == nil {
			proxyPasswd = plain
		}
	}
	built, err := transportForProxy(proxyType, proxyURL, proxyPort, proxyUser, proxyPasswd)
	if err != nil {
		return nil
	}
	return built
}

// transportForProxy builds an HTTP transport for the panel proxy settings.
// proxyURL may already include the scheme (saved http/https values do).
func transportForProxy(proxyType, proxyURL, proxyPort, proxyUser, proxyPasswd string) (*http.Transport, error) {
	host := strings.TrimSpace(proxyURL)
	host = strings.TrimPrefix(host, proxyType+"://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "socks5://")
	host = strings.TrimRight(host, "/")
	if host == "" || strings.TrimSpace(proxyPort) == "" {
		return nil, fmt.Errorf("proxy address is incomplete")
	}
	addr := net.JoinHostPort(host, strings.TrimSpace(proxyPort))

	base := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}

	switch proxyType {
	case "http", "https":
		parsed, err := url.Parse(proxyType + "://" + addr)
		if err != nil {
			return nil, err
		}
		if proxyUser != "" {
			parsed.User = url.UserPassword(proxyUser, proxyPasswd)
		}
		base.Proxy = http.ProxyURL(parsed)
		return base, nil
	case "socks5":
		var auth *proxy.Auth
		if proxyUser != "" {
			auth = &proxy.Auth{User: proxyUser, Password: proxyPasswd}
		}
		dialer, err := proxy.SOCKS5("tcp", addr, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}
		return base, nil
	default:
		return nil, fmt.Errorf("unsupported proxy type %s", proxyType)
	}
}
