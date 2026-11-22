package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/utils/docker"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// proxyPullOptions tunes pullImageViaProxy.
type proxyPullOptions struct {
	// Force downloads even when the image already exists locally (explicit
	// "pull image" requests); otherwise an existing image is reused, like
	// compose's default pull_policy (missing).
	Force bool
	// Username / Password authenticate against the registry.
	Username string
	Password string
	// Insecure allows plain-HTTP registries.
	Insecure bool
}

// pullImageViaProxy downloads an image from its registry through proxyURL
// and loads it into the local Docker engine.
//
// The proxy is scoped to this single download: neither the Docker daemon
// configuration nor the process environment is touched, so every other
// panel feature (app store, docker pulls elsewhere, upgrades, …) keeps
// connecting directly.
func pullImageViaProxy(ctx context.Context, t *task.Task, imageName, proxyURL string, o ...proxyPullOptions) error {
	var opt proxyPullOptions
	if len(o) > 0 {
		opt = o[0]
	}
	cli, err := docker.NewDockerClient()
	if err != nil {
		return err
	}
	defer cli.Close()
	if !opt.Force {
		if _, err := cli.ImageInspect(ctx, imageName); err == nil {
			logPull(t, "image %s already exists, skip pulling", imageName)
			return nil
		}
	}

	proxy, err := url.Parse(proxyURL)
	if err != nil || proxy.Host == "" {
		return fmt.Errorf("invalid proxy url")
	}
	var nameOpts []name.Option
	if opt.Insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	ref, err := name.ParseReference(imageName, nameOpts...)
	if err != nil {
		return fmt.Errorf("invalid image name %s: %v", imageName, err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(transport),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: runtime.GOARCH}),
	}
	if opt.Username != "" || opt.Password != "" {
		opts = append(opts, remote.WithAuth(&authn.Basic{Username: opt.Username, Password: opt.Password}))
	} else {
		opts = append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	logPull(t, "pull image %s through the proxy node", imageName)
	img, err := remote.Image(ref, opts...)
	if err != nil {
		return fmt.Errorf("pull image %s through proxy failed: %v", imageName, err)
	}

	pr, pw := io.Pipe()
	updates := make(chan v1.Update, 16)
	go func() {
		err := tarball.Write(ref, img, pw, tarball.WithProgress(updates))
		// tarball never closes the progress channel itself.
		close(updates)
		pw.CloseWithError(err)
	}()
	go func() {
		lastPct, lastLog := int64(-1), time.Time{}
		for u := range updates {
			if u.Error != nil || u.Total <= 0 {
				continue
			}
			pct := u.Complete * 100 / u.Total
			if pct != lastPct && (pct == 100 || time.Since(lastLog) > 5*time.Second) {
				lastPct, lastLog = pct, time.Now()
				logPull(t, "downloading %s: %d%% (%.1f / %.1f MB)", imageName, pct,
					float64(u.Complete)/1024/1024, float64(u.Total)/1024/1024)
			}
		}
	}()
	resp, err := cli.ImageLoad(ctx, pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return fmt.Errorf("load image %s failed: %v", imageName, err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("load image %s failed: %v", imageName, err)
	}
	if _, err := cli.ImageInspect(ctx, imageName); err != nil {
		return fmt.Errorf("image %s not found after loading: %v", imageName, err)
	}
	logPull(t, "image %s pulled", imageName)
	return nil
}

func logPull(t *task.Task, format string, v ...any) {
	if t != nil {
		t.Logf(format, v...)
	}
}
