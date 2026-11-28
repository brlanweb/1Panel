package service

import (
	"net/url"
	"reflect"
	"testing"
)

func TestJoinLineWriter(t *testing.T) {
	var lines []string
	w := &joinLineWriter{onLine: func(s string) { lines = append(lines, s) }, all: &computeLimitedBuffer{max: 1 << 20}}
	_, _ = w.Write([]byte("a\nb"))
	_, _ = w.Write([]byte("c\r\nprogress 10%\rprogress 20%"))
	w.flush()
	want := []string{"a", "bc", "", "progress 10%", "progress 20%"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %q", lines)
	}
	if w.all.buf.String() != "a\nbc\r\nprogress 10%\rprogress 20%" {
		t.Fatalf("raw output lost: %q", w.all.buf.String())
	}
}

func TestJoinLogMasksPassword(t *testing.T) {
	clusterJoins.Lock()
	clusterJoins.hosts[999999] = &clusterJoinState{Password: "s3cretpw", Running: true}
	clusterJoins.Unlock()
	defer func() {
		clusterJoins.Lock()
		delete(clusterJoins.hosts, 999999)
		clusterJoins.Unlock()
	}()
	joinLog(999999, "\x1b[32m[1Panel] password: s3cretpw\x1b[0m")
	joinLog(999999, "   ")
	st := (&ClusterNodeService{}).JoinClusterStatus(999999)
	if len(st.Logs) != 1 || st.Logs[0] != "[1Panel] password: ********" {
		t.Fatalf("got %q", st.Logs)
	}
	if st.Password != "" {
		t.Fatal("password must not be returned while running / when not installed")
	}
}

func TestJoinURLPort(t *testing.T) {
	for raw, want := range map[string]string{
		"https://1.2.3.4:8443/x": "8443",
		"https://1.2.3.4":        "443",
		"http://1.2.3.4":         "80",
	} {
		u, _ := url.Parse(raw)
		if got := joinURLPort(u); got != want {
			t.Fatalf("%s: got %s", raw, got)
		}
	}
}
