package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func trendLine(ip string, t time.Time, ua string) string {
	return ip + ` - - [` + t.Format("02/Jan/2006:15:04:05 -0700") + `] "GET / HTTP/1.1" 200 12 "-" "` + ua + `"`
}

func TestTrendWindow(t *testing.T) {
	now := time.Date(2026, 11, 21, 15, 30, 0, 0, time.Local)
	start, step, _, count := trendWindow("7d", now)
	if count != 7 || step != 24*time.Hour || start.Day() != 15 {
		t.Fatalf("7d window: %v %v %d", start, step, count)
	}
	start, _, _, count = trendWindow("30d", now)
	if count != 30 || !start.Equal(time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("30d window: %v %d", start, count)
	}
	start, step, _, count = trendWindow("today", now)
	if count != 24 || step != time.Hour || start.Hour() != 0 {
		t.Fatalf("today window: %v %v %d", start, step, count)
	}
	if idx := trendIndex(start, now, step); idx != 15 {
		t.Fatalf("hour index = %d", idx)
	}
}

func TestScanArchivedAccessLog(t *testing.T) {
	now := time.Now()
	content := trendLine("1.1.1.1", now, "UA") + "\n" + trendLine("2.2.2.2", now, "UA") + "\n"

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{{"error.log", "boom\n"}, {"access.log", content}} {
		_ = tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(f.body))
	}
	_ = tw.Close()
	_ = gz.Close()
	file := filepath.Join(t.TempDir(), "site_log_x.gz")
	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if err := scanArchivedAccessLog(file, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 access lines from tar.gz, got %v", lines)
	}
	m := trendLogRe.FindStringSubmatch(lines[0])
	if m == nil || m[1] != "1.1.1.1" || m[3] != "UA" {
		t.Fatalf("regex did not match combined log line: %q", lines[0])
	}
}
