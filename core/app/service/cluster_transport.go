package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/model"
)

// Unified persistent HTTP/TLS transport pool for all cluster traffic.
//
// Previously every callRemote / ProxyRequest / localAgentHTTPClient created a
// fresh http.Transport (forcing a new TCP+TLS handshake on every 1s monitor
// poll and accumulating TIME_WAIT sockets). This pool reuses persistent
// Keep-Alive transports across all cluster calls and reverse-proxy requests.
type clusterTransportPool struct {
	mu                 sync.RWMutex
	panelHTTP          *http.Transport
	panelWS            *http.Transport
	localUnixTransport *http.Transport
	localUnixPath      string
}

var sharedClusterTransports = &clusterTransportPool{}

func (p *clusterTransportPool) getPanelTransport(isWS bool) *http.Transport {
	p.mu.RLock()
	if isWS && p.panelWS != nil {
		t := p.panelWS
		p.mu.RUnlock()
		return t
	}
	if !isWS && p.panelHTTP != nil {
		t := p.panelHTTP
		p.mu.RUnlock()
		return t
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if isWS {
		if p.panelWS == nil {
			p.panelWS = &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
				TLSHandshakeTimeout: 5 * time.Second,
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
			}
		}
		return p.panelWS
	}
	if p.panelHTTP == nil {
		p.panelHTTP = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          128,
			MaxIdleConnsPerHost:   32,
			ForceAttemptHTTP2:     true,
		}
	}
	return p.panelHTTP
}

// TransportFor returns a pooled http.Transport for the node and traffic kind.
func (p *clusterTransportPool) TransportFor(_ model.ClusterNode, isWS bool) *http.Transport {
	return p.getPanelTransport(isWS)
}

// LocalAgentTransport returns a pooled unix-socket transport for the master's
// own local agent.
func (p *clusterTransportPool) LocalAgentTransport(sockPath string) *http.Transport {
	p.mu.RLock()
	if p.localUnixTransport != nil && p.localUnixPath == sockPath {
		tr := p.localUnixTransport
		p.mu.RUnlock()
		return tr
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.localUnixTransport != nil && p.localUnixPath == sockPath {
		return p.localUnixTransport
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", sockPath)
		},
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     60 * time.Second,
	}
	p.localUnixPath = sockPath
	p.localUnixTransport = tr
	return tr
}

// nodeAPIBase returns the scheme://host base URL for calling a cluster node.
func nodeAPIBase(node model.ClusterNode) (string, error) {
	switch node.EffectiveMode() {
	case model.ClusterNodeModeAgent:
		host := nodeEndpointHost(node)
		if host == "" {
			return "", fmt.Errorf("missing agent host for node %s", node.Name)
		}
		port := node.AgentPort
		if port <= 0 {
			port = 9999
		}
		return "https://" + net.JoinHostPort(host, strconv.Itoa(port)), nil
	default:
		return panelAPIBase(node.EntryURL)
	}
}

func nodeEndpointHost(node model.ClusterNode) string {
	raw := strings.TrimSpace(node.EntryURL)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
			return parsed.Hostname()
		}
	}
	if h, _, err := net.SplitHostPort(raw); err == nil && h != "" {
		return h
	}
	return strings.Trim(raw, "[]")
}
