package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/req_helper"
)

// Fork upgrade progress.
//
// The download phase runs while the panel stays usable (SystemStatus stays
// "Free"), so the upgrade drawer can show byte progress and a readable
// error. Only after the package is downloaded, checksum-verified and its
// layout checked does the panel switch to "Upgrading" and replace files.
// A failure is kept in memory for the drawer and persisted in the
// UpgradeLastResult setting so it survives a reload.

const (
	upgradeStageIdle        = "idle"
	upgradeStageDownloading = "downloading"
	upgradeStageVerifying   = "verifying"
	upgradeStageInstalling  = "installing"
	upgradeStageFailed      = "failed"

	settingUpgradeLastResult = "UpgradeLastResult"
)

// Tunables (swapped in tests).
var (
	forkDownloadTimeout  = 60 * time.Minute
	forkDownloadStall    = 2 * time.Minute
	forkDownloadAttempts = 4
	forkDownloadRetryGap = 5 * time.Second
)

var forkUpgrade struct {
	sync.Mutex
	state dto.UpgradeStatus
	last  time.Time
	start time.Time
	base  int64
}

func setUpgradeState(mut func(s *dto.UpgradeStatus)) {
	forkUpgrade.Lock()
	defer forkUpgrade.Unlock()
	mut(&forkUpgrade.state)
	forkUpgrade.state.UpdatedAt = time.Now()
}

// beginUpgrade claims the single upgrade slot.
func beginUpgrade(version string) error {
	forkUpgrade.Lock()
	defer forkUpgrade.Unlock()
	switch forkUpgrade.state.Stage {
	case upgradeStageDownloading, upgradeStageVerifying, upgradeStageInstalling:
		return fmt.Errorf("an upgrade to %s is already running", forkUpgrade.state.Version)
	}
	forkUpgrade.state = dto.UpgradeStatus{Stage: upgradeStageDownloading, Version: version, UpdatedAt: time.Now()}
	forkUpgrade.start = time.Now()
	forkUpgrade.base = 0
	return nil
}

func failUpgrade(version string, err error) {
	msg := err.Error()
	global.LOG.Errorf("upgrade to %s failed: %s", version, msg)
	setUpgradeState(func(s *dto.UpgradeStatus) {
		s.Stage = upgradeStageFailed
		s.Error = msg
	})
	if raw, mErr := json.Marshal(dto.UpgradeStatus{Stage: upgradeStageFailed, Version: version, Error: msg, UpdatedAt: time.Now()}); mErr == nil {
		_ = settingRepo.UpdateOrCreate(settingUpgradeLastResult, string(raw))
	}
	_ = settingRepo.Update("SystemStatus", "Free")
}

func reportDownload(done, total int64) {
	forkUpgrade.Lock()
	defer forkUpgrade.Unlock()
	now := time.Now()
	s := &forkUpgrade.state
	s.Done, s.Total = done, total
	if span := now.Sub(forkUpgrade.start).Seconds(); span > 1 {
		s.Rate = int64(float64(done-forkUpgrade.base) / span)
	}
	s.UpdatedAt = now
}

// LoadUpgradeStatus returns the live state; when nothing runs it reports
// the persisted result of the last failed attempt (if any).
func (u *UpgradeService) LoadUpgradeStatus() dto.UpgradeStatus {
	forkUpgrade.Lock()
	state := forkUpgrade.state
	forkUpgrade.Unlock()
	state.CurrentVersion = global.CONF.Base.Version
	if state.Stage != "" && state.Stage != upgradeStageIdle {
		return state
	}
	if raw, err := settingRepo.GetValueByKey(settingUpgradeLastResult); err == nil && raw != "" {
		var last dto.UpgradeStatus
		if json.Unmarshal([]byte(raw), &last) == nil && compareForkVersion(last.Version, state.CurrentVersion) > 0 {
			last.CurrentVersion = state.CurrentVersion
			return last
		}
	}
	return dto.UpgradeStatus{Stage: upgradeStageIdle, CurrentVersion: state.CurrentVersion}
}

// downloadForkPackage downloads url to dst with progress, stall detection
// and HTTP Range resume between attempts.
func downloadForkPackage(url, dst string, transport *http.Transport, progress func(done, total int64)) error {
	part := dst + ".part"
	_ = os.Remove(part)
	var lastErr error
	for attempt := 1; attempt <= forkDownloadAttempts; attempt++ {
		if attempt > 1 {
			if global.LOG != nil {
				global.LOG.Warnf("download %s attempt %d/%d after: %v", path.Base(dst), attempt, forkDownloadAttempts, lastErr)
			}
			time.Sleep(forkDownloadRetryGap)
		}
		done, err := downloadForkPackageOnce(url, part, transport, progress)
		if err == nil {
			return os.Rename(part, dst)
		}
		lastErr = err
		var perm permanentDownloadError
		if errors.As(err, &perm) {
			break
		}
		_ = done
	}
	_ = os.Remove(part)
	return fmt.Errorf("download %s failed: %w", path.Base(dst), lastErr)
}

type permanentDownloadError struct{ error }

func downloadForkPackageOnce(url, part string, transport *http.Transport, progress func(done, total int64)) (int64, error) {
	var offset int64
	if info, err := os.Stat(part); err == nil {
		offset = info.Size()
	}
	ctx, cancel := context.WithTimeout(context.Background(), forkDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return offset, permanentDownloadError{err}
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	client := req_helper.NewDownloadClientWithTransport(transport, forkDownloadTimeout)
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return offset, err
	}
	defer resp.Body.Close()

	flag := os.O_CREATE | os.O_WRONLY
	var total int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flag |= os.O_APPEND
		if resp.ContentLength >= 0 {
			total = offset + resp.ContentLength
		}
	case http.StatusOK:
		flag |= os.O_TRUNC
		offset = 0
		total = resp.ContentLength
	case http.StatusNotFound, http.StatusForbidden, http.StatusUnauthorized, http.StatusGone:
		return offset, permanentDownloadError{fmt.Errorf("HTTP %d", resp.StatusCode)}
	default:
		return offset, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	out, err := os.OpenFile(part, flag, 0o600)
	if err != nil {
		return offset, permanentDownloadError{err}
	}
	defer out.Close()

	// Stall watchdog: cancel when no byte arrives for forkDownloadStall.
	var mu sync.Mutex
	lastByte := time.Now()
	stalled := false
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				mu.Lock()
				idle := time.Since(lastByte)
				mu.Unlock()
				if idle > forkDownloadStall {
					mu.Lock()
					stalled = true
					mu.Unlock()
					cancel()
					return
				}
			}
		}
	}()

	done := offset
	buf := make([]byte, 256<<10)
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return done, permanentDownloadError{wErr}
			}
			done += int64(n)
			mu.Lock()
			lastByte = time.Now()
			mu.Unlock()
			if progress != nil {
				progress(done, total)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			mu.Lock()
			wasStalled := stalled
			mu.Unlock()
			if wasStalled {
				return done, fmt.Errorf("no data received for %s", forkDownloadStall)
			}
			return done, rErr
		}
	}
	if total > 0 && done != total {
		return done, fmt.Errorf("incomplete download: %d of %d bytes", done, total)
	}
	return done, out.Sync()
}

// verifyForkPackageChecksum checks the package against the release's
// "<file>.sha256" asset. Releases without that asset are accepted (older
// manually published releases) but a mismatch is always fatal.
func verifyForkPackageChecksum(tag, fileName, pkgPath string, transport *http.Transport) error {
	release, err := findForkRelease(tag)
	if err != nil {
		return err
	}
	var sumURL string
	for _, asset := range release.Assets {
		if asset.Name == fileName+".sha256" {
			sumURL = asset.BrowserDownloadURL
		}
	}
	if sumURL == "" {
		logForkReleaseError(fmt.Errorf("release %s has no %s.sha256, skipping checksum verification", tag, fileName))
		return nil
	}
	status, body, err := req_helper.HandleRequestWithTransport(sumURL, http.MethodGet, transport, 60)
	if err != nil {
		return fmt.Errorf("load checksum failed: %v", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("load checksum failed: HTTP %d", status)
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return fmt.Errorf("checksum asset %s.sha256 is malformed", fileName)
	}
	want := strings.ToLower(fields[0])
	f, err := os.Open(pkgPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", fileName, want, got)
	}
	return nil
}

// checkForkPackageLayout makes sure every file the installer copies exists
// before anything on the system is touched.
func checkForkPackageLayout(dir string, svcInfo serviceInfo) error {
	required := []string{
		"1panel-core", "1panel-agent", "1pctl", "lang", "GeoIP.mmdb",
		path.Join("initscript", svcInfo.selCoreName),
		path.Join("initscript", svcInfo.selAgentName),
	}
	var missing []string
	for _, item := range required {
		if _, err := os.Stat(path.Join(dir, item)); err != nil {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("upgrade package is missing %s", strings.Join(missing, ", "))
	}
	return nil
}
