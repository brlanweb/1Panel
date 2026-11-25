package version

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
)

// GetUpgradeVersionInfo feeds the "panel update" alert. Fork build: it
// reads the fork release channel configured on the master core
// (ForkReleaseRepo / ForkReleaseAPI settings) instead of the upstream
// package repository, so the alert can never announce an upstream
// release.
func GetUpgradeVersionInfo() (*dto.UpgradeInfo, error) {
	var currentVersion model.Setting
	if err := global.CoreDB.Model(&model.Setting{}).Where("key = ?", "SystemVersion").First(&currentVersion).Error; err != nil {
		global.LOG.Errorf("load %s from db setting failed, err: %v", "SystemVersion", err)
		return nil, err
	}
	var developerMode model.Setting
	if err := global.CoreDB.Model(&model.Setting{}).Where("key = ?", "DeveloperMode").First(&developerMode).Error; err != nil {
		global.LOG.Errorf("load %s from db setting failed, err: %v", "DeveloperMode", err)
		return nil, err
	}
	repoName, apiBase := forkReleaseConfig()
	developer := global.CONF.Base.Mode == "dev" || developerMode.Value == constant.StatusEnable
	return forkUpgradeInfo(apiBase, repoName, currentVersion.Value, developer)
}

func forkReleaseConfig() (string, string) {
	repoName, apiBase := "brlanweb/1Panel", "https://api.github.com"
	var item model.Setting
	if err := global.CoreDB.Model(&model.Setting{}).Where("key = ?", "ForkReleaseRepo").First(&item).Error; err == nil && strings.TrimSpace(item.Value) != "" {
		repoName = strings.Trim(strings.TrimSpace(item.Value), "/")
	}
	item = model.Setting{}
	if err := global.CoreDB.Model(&model.Setting{}).Where("key = ?", "ForkReleaseAPI").First(&item).Error; err == nil && strings.TrimSpace(item.Value) != "" {
		apiBase = strings.TrimRight(strings.TrimSpace(item.Value), "/")
	}
	return repoName, apiBase
}

type forkRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func forkUpgradeInfo(apiBase, repoName, currentVersion string, developer bool) (*dto.UpgradeInfo, error) {
	_, body, err := HandleRequest(fmt.Sprintf("%s/repos/%s/releases?per_page=30", apiBase, repoName), http.MethodGet, constant.TimeOut20s)
	if err != nil {
		return nil, fmt.Errorf("load fork releases failed: %v", err)
	}
	var list []forkRelease
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse fork releases failed: %v", err)
	}
	var info dto.UpgradeInfo
	for _, item := range list {
		if item.Draft || compareForkVersion(item.TagName, currentVersion) <= 0 {
			continue
		}
		if item.Prerelease {
			if developer && (info.TestVersion == "" || compareForkVersion(item.TagName, info.TestVersion) > 0) {
				info.TestVersion = item.TagName
			}
			continue
		}
		if info.LatestVersion == "" || compareForkVersion(item.TagName, info.LatestVersion) > 0 {
			info.LatestVersion = item.TagName
			info.ReleaseNote = item.Body
		}
	}
	return &info, nil
}

// compareForkVersion mirrors core/app/service/fork_release.go.
func compareForkVersion(a, b string) int {
	split := func(v string) []string {
		v = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(v)), "v")
		return strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	}
	pa, pb := split(a), split(b)
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
	// Pre-release suffixes come before the release they lead to.
	pre := func(s string) bool {
		switch s {
		case "alpha", "beta", "rc", "pre", "preview", "dev":
			return true
		}
		return strings.HasPrefix(s, "rc") || strings.HasPrefix(s, "beta") || strings.HasPrefix(s, "alpha")
	}
	switch {
	case len(pa) > len(pb):
		if pre(pa[len(pb)]) {
			return -1
		}
		return 1
	case len(pa) < len(pb):
		if pre(pb[len(pa)]) {
			return 1
		}
		return -1
	}
	return 0
}

func HandleRequest(url, method string, timeout int) (int, []byte, error) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("handle request failed, error message: %v", r)
			return
		}
	}()

	transport := loadRequestTransport()
	client := http.Client{Timeout: time.Duration(timeout) * time.Second, Transport: transport}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, errors.New(resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}

func loadRequestTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       15 * time.Second,
	}
}
