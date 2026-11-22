package service

import (
	"fmt"
	"testing"
)

func TestCompareForkVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v2.1.5-cluster.2", "v2.1.5-cluster.1", 1},
		{"v2.1.5-cluster.1", "v2.1.5", 1}, // fork build is newer than its upstream base
		{"v2.1.5", "v2.1.5-cluster.1", -1},
		{"v2.2.0", "v2.1.9-cluster.9", 1},
		{"v2.1.10", "v2.1.9", 1}, // numeric, not lexical
		{"v2.1.5-cluster.1", "v2.1.5-cluster.1", 0},
		{"2.1.5", "v2.1.5", 0},
	}
	for _, c := range cases {
		if got := compareForkVersion(c.a, c.b); got != c.want {
			t.Errorf("compareForkVersion(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func withFakeReleases(t *testing.T, releases []forkRelease) {
	t.Helper()
	old := forkReleaseFetcher
	forkReleaseFetcher = func(apiBase, repoName string) ([]forkRelease, error) { return releases, nil }
	resetForkReleaseCache()
	t.Cleanup(func() {
		forkReleaseFetcher = old
		resetForkReleaseCache()
	})
}

func TestForkUpgradeInfoOnlyOffersNewerForkReleases(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withFakeReleases(t, []forkRelease{
		{TagName: "v2.1.5-cluster.3", Body: "draft", Draft: true},
		{TagName: "v2.1.5-cluster.2", Body: "test build", Prerelease: true},
		{TagName: "v2.1.5-cluster.1", Body: "stable notes"},
		{TagName: "v2.1.5-cluster.0", Body: "old"},
	})

	// Running the newest stable fork release: nothing to offer (an
	// upstream v2.1.6 published elsewhere is irrelevant — it is not in
	// the fork channel at all).
	info, err := forkUpgradeInfo("v2.1.5-cluster.1", false)
	if err != nil {
		t.Fatal(err)
	}
	if info.LatestVersion != "" || info.TestVersion != "" {
		t.Fatalf("info = %+v, want no update", info)
	}

	// Older install: the stable release is offered, drafts never are,
	// pre-releases only in developer mode.
	info, _ = forkUpgradeInfo("v2.1.5-cluster.0", false)
	if info.LatestVersion != "v2.1.5-cluster.1" || info.ReleaseNote != "stable notes" || info.TestVersion != "" {
		t.Fatalf("info = %+v, want latest v2.1.5-cluster.1 only", info)
	}
	info, _ = forkUpgradeInfo("v2.1.5-cluster.0", true)
	if info.TestVersion != "v2.1.5-cluster.2" {
		t.Fatalf("developer info = %+v, want test v2.1.5-cluster.2", info)
	}

	// A panel still reporting the upstream base version is offered the
	// fork release, never an upstream one.
	info, _ = forkUpgradeInfo("v2.1.5", false)
	if info.LatestVersion != "v2.1.5-cluster.1" {
		t.Fatalf("info = %+v, want v2.1.5-cluster.1", info)
	}
}

func TestForkPackageURLRequiresMatchingAsset(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	withFakeReleases(t, []forkRelease{
		{TagName: "v2.1.5-cluster.1", Assets: []forkReleaseAsset{
			{Name: "1panel-v2.1.5-cluster.1-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.invalid/amd64.tgz"},
		}},
	})
	url, name, err := forkPackageURL("v2.1.5-cluster.1", "amd64")
	if err != nil || url != "https://example.invalid/amd64.tgz" || name != "1panel-v2.1.5-cluster.1-linux-amd64.tar.gz" {
		t.Fatalf("amd64: url=%q name=%q err=%v", url, name, err)
	}
	if _, _, err := forkPackageURL("v2.1.5-cluster.1", "arm64"); err == nil {
		t.Fatal("arm64 without asset must fail instead of downloading something else")
	}
	if _, _, err := forkPackageURL("v2.1.6", "amd64"); err == nil {
		t.Fatal("a tag that is not a fork release must fail")
	}
}

// When the GitHub API fails (e.g. the 60 req/h unauthenticated limit), the
// update check must fall back to the releases/latest redirect instead of
// reporting "already the latest version".
func TestForkReleasesFallBackWhenAPIFails(t *testing.T) {
	restore := setupSyncTestDB(t)
	defer restore()
	oldAPI, oldLatest := forkReleaseFetcher, forkLatestReleaseFetcher
	forkReleaseFetcher = func(apiBase, repoName string) ([]forkRelease, error) {
		return nil, fmt.Errorf("load releases failed: HTTP 403")
	}
	forkLatestReleaseFetcher = func(repoName string) (forkRelease, error) {
		return forkRelease{TagName: "v2.5.1-cluster.9"}, nil
	}
	resetForkReleaseCache()
	defer func() {
		forkReleaseFetcher, forkLatestReleaseFetcher = oldAPI, oldLatest
		resetForkReleaseCache()
	}()

	info, err := forkUpgradeInfo("v2.5.1-cluster.3", false)
	if err != nil {
		t.Fatalf("forkUpgradeInfo: %v", err)
	}
	if info.LatestVersion != "v2.5.1-cluster.9" {
		t.Fatalf("LatestVersion = %q, want the redirect fallback v2.5.1-cluster.9", info.LatestVersion)
	}

	forkLatestReleaseFetcher = func(repoName string) (forkRelease, error) {
		return forkRelease{}, fmt.Errorf("resolve latest release failed: HTTP 404")
	}
	resetForkReleaseCache()
	if _, err := forkUpgradeInfo("v2.5.1-cluster.3", false); err == nil {
		t.Fatal("both channels failing must be an error, not \"no update\"")
	}
}
