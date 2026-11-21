package service

import (
	"strings"
	"testing"
)

func TestParseClashProxies(t *testing.T) {
	raw := []byte(`
port: 7890
proxies:
  - {name: "HK 01", type: ss, server: 1.1.1.1, port: 443, cipher: aes-128-gcm, password: x}
  - {name: "HK 01", type: ss, server: 1.1.1.2, port: 443, cipher: aes-128-gcm, password: x}
  - {name: "DIRECT", type: ss, server: 1.1.1.3, port: 443, cipher: aes-128-gcm, password: x}
  - {name: "", type: ss, server: 1.1.1.4, port: 443}
  - {name: " JP 02 ", type: vmess, server: 2.2.2.2, port: 443, uuid: abc}
proxy-groups:
  - {name: G, type: select, proxies: ["HK 01"]}
`)
	got, err := parseClashProxies(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0]["name"] != "HK 01" || got[1]["name"] != "JP 02" {
		t.Fatalf("unexpected proxies: %v", got)
	}
	if _, err := parseClashProxies([]byte("dm1lc3M6Ly9hYmM=")); err == nil {
		t.Fatal("base64 (non-clash) subscription must be rejected")
	}
}

func TestIsVirtualIface(t *testing.T) {
	for _, name := range []string{"lo", "docker0", "br-1a2b", "veth123", "cni0"} {
		if !isVirtualIface(name) {
			t.Fatalf("%s should be virtual", name)
		}
	}
	for _, name := range []string{"eth0", "ens3", "enp0s31f6", "wlan0", "bond0"} {
		if isVirtualIface(name) {
			t.Fatalf("%s should be physical", name)
		}
	}
	if strings.Contains(proxyNodeContainer, " ") {
		t.Fatal("container name must not contain spaces")
	}
}
