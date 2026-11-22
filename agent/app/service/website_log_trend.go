package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/global"
)

// Visitor trend for the website monitor page.
//
// The trend is aggregated on the server so that 7 / 30 day ranges cover the
// whole period: the current access.log is streamed completely and the
// archives produced by the "cut website log" cron job (tar.gz files under
// <backup>/log/website/<alias>) are read as well. Results are cached per
// website/range and invalidated when any source file changes.

var trendLogRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "[^"]*" \d{3} \S+ "[^"]*" "([^"]*)"`)

const (
	trendCacheTTL   = time.Minute
	trendMaxLineLen = 1024 * 1024
)

type trendCacheEntry struct {
	key  string
	at   time.Time
	data *response.WebsiteLogTrend
}

var (
	trendCacheMu sync.Mutex
	trendCache   = map[string]trendCacheEntry{}
)

type trendBucket struct {
	pv  int
	uv  map[string]struct{}
	ips map[string]struct{}
}

func (w WebsiteService) GetWebsiteLogTrend(req request.WebsiteLogTrendReq) (*response.WebsiteLogTrend, error) {
	website, err := websiteRepo.GetFirst(repo.WithByID(req.ID))
	if err != nil {
		return nil, err
	}
	res := &response.WebsiteLogTrend{Enable: website.AccessLog, Labels: []string{}, PV: []int{}, UV: []int{}, IPs: []int{}}
	now := time.Now()
	start, step, layout, count := trendWindow(req.Range, now)

	sources := []string{path.Join(GetSitePath(website, SiteDir), "log", "access.log")}
	archiveDir := path.Join(global.Dir.LocalBackupDir, "log", "website", website.Alias)
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".gz") {
				continue
			}
			info, err := e.Info()
			// An archive holds lines written before it was cut, so archives
			// cut before the window starts cannot contain data in range.
			if err != nil || info.ModTime().Before(start) {
				continue
			}
			sources = append(sources, filepath.Join(archiveDir, e.Name()))
		}
	}
	cacheKey := fmt.Sprintf("%d|%s|%s", website.ID, req.Range, start.Format(layout))
	fingerprint := trendFingerprint(sources)
	trendCacheMu.Lock()
	if c, ok := trendCache[cacheKey]; ok && c.key == fingerprint && time.Since(c.at) < trendCacheTTL {
		trendCacheMu.Unlock()
		out := *c.data
		out.Enable = website.AccessLog
		return &out, nil
	}
	trendCacheMu.Unlock()

	buckets := make([]trendBucket, count)
	add := func(line string) {
		m := trendLogRe.FindStringSubmatch(line)
		if m == nil {
			return
		}
		t, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[2])
		if err != nil {
			return
		}
		t = t.In(now.Location())
		if t.Before(start) {
			return
		}
		idx := trendIndex(start, t, step)
		if idx < 0 || idx >= count {
			return
		}
		b := &buckets[idx]
		if b.uv == nil {
			b.uv, b.ips = map[string]struct{}{}, map[string]struct{}{}
		}
		b.pv++
		b.ips[m[1]] = struct{}{}
		b.uv[m[1]+"|"+m[3]] = struct{}{}
	}
	for i, src := range sources {
		if i == 0 {
			_ = scanPlainLog(src, add)
		} else {
			_ = scanArchivedAccessLog(src, add)
		}
	}
	for i := 0; i < count; i++ {
		res.Labels = append(res.Labels, trendLabel(start, step, i, layout))
		res.PV = append(res.PV, buckets[i].pv)
		res.UV = append(res.UV, len(buckets[i].uv))
		res.IPs = append(res.IPs, len(buckets[i].ips))
	}
	trendCacheMu.Lock()
	trendCache[cacheKey] = trendCacheEntry{key: fingerprint, at: time.Now(), data: res}
	trendCacheMu.Unlock()
	return res, nil
}

// trendWindow returns the first bucket start, bucket size, label layout and
// bucket count: today → 24 hourly buckets; 7d / 30d → daily buckets
// ending today.
func trendWindow(rangeName string, now time.Time) (time.Time, time.Duration, string, int) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch rangeName {
	case "7d":
		return today.AddDate(0, 0, -6), 24 * time.Hour, "01-02", 7
	case "30d":
		return today.AddDate(0, 0, -29), 24 * time.Hour, "01-02", 30
	default:
		return today, time.Hour, "15:00", 24
	}
}

func trendIndex(start, t time.Time, step time.Duration) int {
	if step == 24*time.Hour {
		// Count calendar days so DST changes do not shift buckets.
		y, m, d := t.Date()
		day := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
		return int(day.Sub(start).Hours()+12) / 24
	}
	return int(t.Sub(start) / step)
}

func trendLabel(start time.Time, step time.Duration, i int, layout string) string {
	if step == 24*time.Hour {
		return start.AddDate(0, 0, i).Format(layout)
	}
	return start.Add(step * time.Duration(i)).Format(layout)
}

func trendFingerprint(sources []string) string {
	sorted := append([]string(nil), sources...)
	sort.Strings(sorted)
	var b strings.Builder
	for _, s := range sorted {
		if info, err := os.Stat(s); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", s, info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

func scanLines(r io.Reader, fn func(string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), trendMaxLineLen)
	for sc.Scan() {
		fn(sc.Text())
	}
	return sc.Err()
}

func scanPlainLog(file string, fn func(string)) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return scanLines(f, fn)
}

// scanArchivedAccessLog reads access.log from a cut-log archive (tar.gz
// with access.log / error.log, or a plain gzip of the log).
func scanArchivedAccessLog(file string, fn func(string)) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	br := bufio.NewReader(gz)
	head, _ := br.Peek(262)
	if len(head) >= 262 && string(head[257:262]) == "ustar" {
		tr := tar.NewReader(br)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if path.Base(hdr.Name) == "access.log" {
				return scanLines(tr, fn)
			}
		}
	}
	return scanLines(br, fn)
}
