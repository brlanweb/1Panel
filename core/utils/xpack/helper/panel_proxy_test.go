package helper

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTransportForProxyHTTP(t *testing.T) {
	var gotProxy bool
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProxy = true
		if r.Method == http.MethodConnect {
			http.Error(w, "CONNECT is not used by this test", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer proxySrv.Close()

	proxyHost, proxyPort, err := net.SplitHostPort(mustHostPort(proxySrv.URL))
	if err != nil {
		t.Fatal(err)
	}
	transport, err := transportForProxy("http", "http://"+proxyHost, proxyPort, "user", "secret")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/pkg", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if !gotProxy {
		t.Fatal("request did not go through the proxy")
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	user := proxyURL.User.Username()
	pass, _ := proxyURL.User.Password()
	if user != "user" || pass != "secret" {
		t.Fatalf("proxy credentials = %s:%s", user, pass)
	}
}

func TestTransportForProxySOCKS5(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "socks-ok")
	}))
	defer target.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go serveTestSOCKS5(ln)

	proxyHost, proxyPort, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	transport, err := transportForProxy("socks5", proxyHost, proxyPort, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "socks-ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestTransportForProxyClosed(t *testing.T) {
	if _, err := transportForProxy("close", "127.0.0.1", "7890", "", ""); err == nil {
		t.Fatal("close should not produce a proxy transport")
	}
	if _, err := transportForProxy("http", "", "7890", "", ""); err == nil {
		t.Fatal("empty host should fail")
	}
}

func mustHostPort(rawURL string) string {
	return rawURL[len("http://"):]
}

// serveTestSOCKS5 accepts username/password CONNECT and dials the target.
func serveTestSOCKS5(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 258)
			if _, err := io.ReadFull(c, buf[:2]); err != nil {
				return
			}
			nmethods := int(buf[1])
			if _, err := io.ReadFull(c, buf[:nmethods]); err != nil {
				return
			}
			if _, err := c.Write([]byte{0x05, 0x02}); err != nil {
				return
			}
			if _, err := io.ReadFull(c, buf[:2]); err != nil || buf[0] != 0x01 {
				return
			}
			ulen := int(buf[1])
			if _, err := io.ReadFull(c, buf[:ulen]); err != nil || string(buf[:ulen]) != "user" {
				return
			}
			if _, err := io.ReadFull(c, buf[:1]); err != nil {
				return
			}
			plen := int(buf[0])
			if _, err := io.ReadFull(c, buf[:plen]); err != nil || string(buf[:plen]) != "pass" {
				return
			}
			if _, err := c.Write([]byte{0x01, 0x00}); err != nil {
				return
			}
			if _, err := io.ReadFull(c, buf[:4]); err != nil || buf[1] != 0x01 {
				return
			}
			var host string
			switch buf[3] {
			case 0x01:
				if _, err := io.ReadFull(c, buf[:4]); err != nil {
					return
				}
				host = net.IP(buf[:4]).String()
			case 0x03:
				if _, err := io.ReadFull(c, buf[:1]); err != nil {
					return
				}
				n := int(buf[0])
				if _, err := io.ReadFull(c, buf[:n]); err != nil {
					return
				}
				host = string(buf[:n])
			default:
				return
			}
			if _, err := io.ReadFull(c, buf[:2]); err != nil {
				return
			}
			port := int(buf[0])<<8 | int(buf[1])
			dest, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
			if err != nil {
				_, _ = c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
				return
			}
			defer dest.Close()
			if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
				return
			}
			errc := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(dest, c); errc <- struct{}{} }()
			go func() { _, _ = io.Copy(c, dest); errc <- struct{}{} }()
			<-errc
		}(conn)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
