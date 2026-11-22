package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/constant"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/req_helper"
)

// Fork release channel.
//
// This build is a fork of 1Panel (cluster management). The upstream update
// channel (resource.fit2cloud.com latest / latest.current) must never be
// used: it offers upstream releases, and "upgrading" to one replaces the
// fork binaries and silently drops every fork feature. Update checks,
// release notes, the changelog and the upgrade download therefore all come
// from the fork's own GitHub Releases. A new version is offered ONLY when
// the fork publishes a release whose tag is newer than the running version.
//
// Release contract (see scripts/fork-release.sh):
//   - tag = panel version, e.g. v2.1.5-cluster.2 (it becomes SystemVersion)
//   - draft releases are ignored; "pre-release" releases are the test
//     channel and only offered when developer mode is enabled
//   - asset 1panel-<tag>-linux-<arch>.tar.gz with the same layout as the
//     upstream package (1panel-core, 1panel-agent, 1pctl, initscript/,
//     lang/, GeoIP.mmdb inside a 1panel-<tag>-linux-<arch>/ directory)
//   - the release body is the release note (markdown)

const (
	// Setting keys (seeded by migration AddForkReleaseSettings).
	settingForkReleaseRepo = "ForkReleaseRepo"
	settingForkReleaseAPI  = "ForkReleaseAPI"

	defaultForkReleaseRepo = "brlanweb/1Panel"
	defaultForkReleaseAPI  = "https://api.github.com"

	forkReleaseCacheTTL = 10 * time.Minute
)

type forkReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type forkRelease struct {
	TagName     string             `json:"tag_name"`
	Name        string             `json:"name"`
	Body        string             `json:"body"`
	Draft       bool               `json:"draft"`
	Prerelease  bool               `json:"prerelease"`
	PublishedAt string             `json:"published_at"`
	Assets      []forkReleaseAsset `json:"assets"`
}

var forkReleaseCache struct {
	sync.Mutex
	key      string
	at       time.Time
	releases []forkRelease
}

// forkReleaseFetcher / forkLatestReleaseFetcher are swapped in tests.
var (
	forkReleaseFetcher       = fetchForkReleases
	forkLatestReleaseFetcher = fetchForkLatestRelease
)

// fetchForkLatestRelease resolves the latest release tag from the
// github.com/<repo>/releases/latest redirect (no API, no rate limit).
func fetchForkLatestRelease(repoName string) (forkRelease, error) {
	client := req_helper.NewDownloadClientWithProxy(constant.TimeOut20s * time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer client.CloseIdleConnections()
	resp, err := client.Head(fmt.Sprintf("https://github.com/%s/releases/latest", repoName))
	if err != nil {
		return forkRelease{}, err
	}
	_ = resp.Body.Close()
	location := resp.Header.Get("Location")
	idx := strings.LastIndex(location, "/releases/tag/")
	if resp.StatusCode < 300 || resp.StatusCode >= 400 || idx < 0 {
		return forkRelease{}, fmt.Errorf("resolve latest release failed: HTTP %d", resp.StatusCode)
	}
	tag := strings.TrimSpace(location[idx+len("/releases/tag/"):])
	if tag == "" {
		return forkRelease{}, fmt.Errorf("resolve latest release failed: empty tag")
	}
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s/", repoName, tag)
	release := forkRelease{TagName: tag, Name: tag}
	for _, arch := range []string{"amd64", "arm64"} {
		name := fmt.Sprintf("1panel-%s-linux-%s.tar.gz", tag, arch)
		release.Assets = append(release.Assets,
			forkReleaseAsset{Name: name, BrowserDownloadURL: base + name},
			forkReleaseAsset{Name: name + ".sha256", BrowserDownloadURL: base + name + ".sha256"},
		)
	}
	return release, nil
}

func forkReleaseConfig() (repoName, apiBase string) {
	repoName, apiBase = defaultForkReleaseRepo, defaultForkReleaseAPI
	if v, err := settingRepo.GetValueByKey(settingForkReleaseRepo); err == nil && strings.TrimSpace(v) != "" {
		repoName = strings.Trim(strings.TrimSpace(v), "/")
	}
	if v, err := settingRepo.GetValueByKey(settingForkReleaseAPI); err == nil && strings.TrimSpace(v) != "" {
		apiBase = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	return repoName, apiBase
}

func fetchForkReleases(apiBase, repoName string) ([]forkRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=30", apiBase, repoName)
	status, body, err := req_helper.HandleRequestWithProxy(url, http.MethodGet, constant.TimeOut20s)
	if err != nil {
		return nil, fmt.Errorf("load releases of %s failed: %v", repoName, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("load releases of %s failed: HTTP %d", repoName, status)
	}
	var list []forkRelease
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse releases of %s failed: %v", repoName, err)
	}
	return list, nil
}

// loadForkReleases returns the published (non-draft) releases, newest
// version first, cached for forkReleaseCacheTTL to stay far below the
// unauthenticated GitHub API limit (60 requests / hour / IP).
func loadForkReleases() ([]forkRelease, error) {
	repoName, apiBase := forkReleaseConfig()
	key := apiBase + "|" + repoName
	forkReleaseCache.Lock()
	defer forkReleaseCache.Unlock()
	if forkReleaseCache.key == key && time.Since(forkReleaseCache.at) < forkReleaseCacheTTL {
		return forkReleaseCache.releases, nil
	}
	list, err := forkReleaseFetcher(apiBase, repoName)
	if err != nil {
		// The unauthenticated GitHub API allows 60 requests / hour / IP and
		// is often exhausted on shared egress IPs. Fall back to the
		// /releases/latest redirect, which has no such limit, and build the
		// asset URLs from the fixed naming scheme of fork-release.yml.
		fallback, fbErr := forkLatestReleaseFetcher(repoName)
		if fbErr != nil {
			return nil, fmt.Errorf("%v; fallback: %v", err, fbErr)
		}
		logForkReleaseError(fmt.Errorf("%v; using the releases/latest redirect instead", err))
		list = []forkRelease{fallback}
	}
	published := make([]forkRelease, 0, len(list))
	for _, item := range list {
		if item.Draft || strings.TrimSpace(item.TagName) == "" {
			continue
		}
		published = append(published, item)
	}
	sort.SliceStable(published, func(i, j int) bool {
		return compareForkVersion(published[i].TagName, published[j].TagName) > 0
	})
	forkReleaseCache.key = key
	forkReleaseCache.at = time.Now()
	forkReleaseCache.releases = published
	return published, nil
}

func resetForkReleaseCache() {
	forkReleaseCache.Lock()
	forkReleaseCache.key = ""
	forkReleaseCache.releases = nil
	forkReleaseCache.Unlock()
}

// splitVersion splits "v2.1.5-cluster.2" into ["2","1","5","cluster","2"].
func splitVersion(v string) []string {
	v = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(v)), "v")
	return strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// compareForkVersion returns 1 if a > b, -1 if a < b, 0 if equal.
// Segments compare numerically when both are numbers, a number beats a
// word, and when one version is a prefix of the other the longer one is
// newer — so a fork build "v2.1.5-cluster.1" is newer than the upstream
// "v2.1.5" it is based on, and "cluster.2" is newer than "cluster.1".
func compareForkVersion(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				if na > nb {
					return 1
				}
				return -1
			}
		case errA == nil:
			return 1
		case errB == nil:
			return -1
		default:
			if pa[i] != pb[i] {
				if pa[i] > pb[i] {
					return 1
				}
				return -1
			}
		}
	}
	switch {
	case len(pa) > len(pb):
		return 1
	case len(pa) < len(pb):
		return -1
	}
	return 0
}

// forkUpgradeInfo: LatestVersion is the newest stable fork release newer
// than current; TestVersion the newest pre-release newer than current
// (only with developer mode). NewVersion is never set (the fork has no
// separate "patch of the current minor" channel).
func forkUpgradeInfo(currentVersion string, developer bool) (*dto.UpgradeInfo, error) {
	releases, err := loadForkReleases()
	if err != nil {
		return nil, err
	}
	var info dto.UpgradeInfo
	for _, item := range releases {
		if compareForkVersion(item.TagName, currentVersion) <= 0 {
			continue
		}
		if item.Prerelease {
			if developer && info.TestVersion == "" {
				info.TestVersion = item.TagName
			}
			continue
		}
		if info.LatestVersion == "" {
			info.LatestVersion = item.TagName
			info.ReleaseNote = item.Body
		}
	}
	if info.LatestVersion == "" && info.TestVersion != "" {
		info.ReleaseNote = forkReleaseBody(releases, info.TestVersion)
	}
	return &info, nil
}

func forkReleaseBody(releases []forkRelease, tag string) string {
	for _, item := range releases {
		if item.TagName == tag {
			return item.Body
		}
	}
	return ""
}

func findForkRelease(tag string) (forkRelease, error) {
	releases, err := loadForkReleases()
	if err != nil {
		return forkRelease{}, err
	}
	for _, item := range releases {
		if item.TagName == tag {
			return item, nil
		}
	}
	return forkRelease{}, fmt.Errorf("release %s not found in the fork release channel", tag)
}

// forkPackageURL returns the download URL of the release package for arch.
func forkPackageURL(tag, arch string) (string, string, error) {
	release, err := findForkRelease(tag)
	if err != nil {
		return "", "", err
	}
	fileName := fmt.Sprintf("1panel-%s-linux-%s.tar.gz", tag, arch)
	for _, asset := range release.Assets {
		if asset.Name == fileName && asset.BrowserDownloadURL != "" {
			return asset.BrowserDownloadURL, fileName, nil
		}
	}
	return "", "", fmt.Errorf("release %s has no asset %s", tag, fileName)
}

func forkReleaseNotesList() ([]dto.ReleasesNotes, error) {
	releases, err := loadForkReleases()
	if err != nil {
		return nil, err
	}
	notes := make([]dto.ReleasesNotes, 0, len(releases))
	for _, item := range releases {
		createdAt := item.PublishedAt
		if t, err := time.Parse(time.RFC3339, item.PublishedAt); err == nil {
			createdAt = t.Format("2006-01-02")
		}
		notes = append(notes, dto.ReleasesNotes{
			Version:   item.TagName,
			CreatedAt: createdAt,
			Content:   item.Body,
		})
	}
	return notes, nil
}

func logForkReleaseError(err error) {
	if err != nil && global.LOG != nil {
		global.LOG.Warnf("fork release channel: %v", err)
	}
}
