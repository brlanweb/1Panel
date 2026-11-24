package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/shirou/gopsutil/v4/disk"
)

// vLLM model transfer ("send to node").
//
// A model is a directory of large files (safetensors shards are commonly
// 4-10 GB each), so it is NOT packed into one archive like a directory
// sync: that would need the full model size of scratch space on the source,
// the master and the target. Instead every file is moved on its own
// (source -> master temp -> target, chunked + resumable), so the master only
// needs room for the largest single file. Files are written to the same
// absolute path on the target, then the model is registered in the target's
// vLLM model list (POST /ai/vllm/models/import) so it can be deployed there.
//
// Files the target already has with the same size are skipped, which makes a
// re-send after a failure / cancel continue where it stopped.

const (
	// modelDownloadIdle aborts one download attempt when no byte arrives
	// for this long (dead TCP connection); the attempt is then resumed.
	modelDownloadIdle = 2 * time.Minute
	// modelDownloadRetries is the number of consecutive attempts without
	// any progress before a file download fails.
	modelDownloadRetries = 8
	// modelListPageSize is the page size used to walk model directories.
	modelListPageSize = 500
	// modelWalkMaxEntries guards against walking a wrong, huge directory.
	modelWalkMaxEntries = 20000
)

// modelSyncTmpDir is the master's scratch directory for model files. It is
// on the install disk rather than /tmp (often a small tmpfs).
func modelSyncTmpDir() string {
	base := global.CONF.Base.InstallDir
	if base == "" {
		base = "/opt"
	}
	return path.Join(base, "1panel/tmp/cluster-sync-model")
}

// modelFile is one regular file of a model directory.
type modelFile struct {
	Rel  string // path relative to the model directory
	Size int64
}

// listModelResources lists the successfully downloaded / imported vLLM
// models of a node as sync resources (Key = model directory).
func (s *ClusterSyncService) listModelResources(node model.ClusterNode, opts syncListOptions, page, pageSize int) (dto.ClusterSyncResourcePage, error) {
	out := dto.ClusterSyncResourcePage{Items: []dto.ClusterSyncResource{}}
	var result struct {
		Total int64 `json:"total"`
		Items []struct {
			ID       uint   `json:"id"`
			RepoID   string `json:"repoId"`
			Source   string `json:"source"`
			Kind     string `json:"kind"`
			Revision string `json:"revision"`
			Dir      string `json:"dir"`
			Status   string `json:"status"`
			Size     int64  `json:"size"`
		} `json:"items"`
	}
	body := map[string]interface{}{"page": page, "pageSize": pageSize, "info": strings.TrimSpace(opts.Search), "kind": ""}
	if err := s.callNodeJSON(node, http.MethodPost, "/api/v2/ai/vllm/models/search", body, &result, 15*time.Second); err != nil {
		return out, err
	}
	for _, item := range result.Items {
		// Only finished models can be sent; a running download is not a
		// consistent set of files yet.
		if item.Status != "Success" || item.Dir == "" {
			continue
		}
		out.Items = append(out.Items, dto.ClusterSyncResource{
			Key: path.Clean(item.Dir), Name: item.RepoID, Detail: item.Kind, Size: item.Size, Extra: item.Source,
			Meta: map[string]interface{}{"revision": item.Revision, "kind": item.Kind, "source": item.Source},
		})
	}
	out.Total = result.Total
	return out, nil
}

// walkModelDir lists every regular file under root on a node. Hidden
// download caches and partial uploads are skipped. A missing root returns
// (nil, nil).
func (s *ClusterSyncService) walkModelDir(ctx context.Context, node model.ClusterNode, root string) ([]modelFile, error) {
	root = path.Clean(root)
	var files []modelFile
	queue := []string{root}
	entries := 0
	for len(queue) > 0 {
		if ctx.Err() != nil {
			return nil, errSyncCancelled
		}
		dir := queue[0]
		queue = queue[1:]
		for page := 1; ; page++ {
			res, err := s.listResourcesPage(dto.ClusterSyncKindFile, node, syncListOptions{Path: dir, Page: page, PageSize: modelListPageSize})
			if err != nil {
				if dir == root && isMissingPathErr(err) {
					return nil, nil
				}
				return nil, fmt.Errorf("list %s on %s: %s", dir, node.Name, truncateErr(err.Error()))
			}
			// The agent answers an empty path when the directory is gone.
			if dir == root && page == 1 && res.Path == "" && len(res.Items) == 0 {
				return nil, nil
			}
			for _, item := range res.Items {
				entries++
				if entries > modelWalkMaxEntries {
					return nil, fmt.Errorf("%s has more than %d entries, it does not look like a model directory", root, modelWalkMaxEntries)
				}
				rel := strings.TrimPrefix(item.Key, root+"/")
				if skipModelEntry(rel, item.Name) {
					continue
				}
				if item.Extra == syncResourceDir {
					// A symlinked directory could loop; models never need one.
					if item.Meta != nil && toBool(item.Meta["isSymlink"]) {
						continue
					}
					queue = append(queue, item.Key)
					continue
				}
				files = append(files, modelFile{Rel: rel, Size: item.Size})
			}
			if len(res.Items) < modelListPageSize || int64(page*modelListPageSize) >= res.Total {
				break
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

// skipModelEntry excludes download-tool caches and partial files.
func skipModelEntry(rel, name string) bool {
	if rel == ".cache" || strings.HasPrefix(rel, ".cache/") {
		return true // huggingface-cli --local-dir metadata
	}
	return strings.HasPrefix(name, "._____temp") || // modelscope partial
		strings.HasPrefix(name, ".1panel-upload-") || // our own chunk upload parts
		strings.HasSuffix(name, ".incomplete") ||
		strings.HasSuffix(name, ".lock")
}

func isMissingPathErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file") || strings.Contains(msg, "not exist") || strings.Contains(msg, "errpathnotfound") || strings.Contains(msg, "not found")
}

// transferModel copies one model directory from source to the same path
// on target and registers it in the target's vLLM model list.
func (s *ClusterSyncService) transferModel(source, target model.ClusterNode, res dto.ClusterSyncResource, opts syncTransferOpts) (string, string) {
	ctx := opts.ctx()
	root := path.Clean(res.Key)
	if !path.IsAbs(root) || root == "/" {
		return model.ClusterSyncActionFailed, "invalid model directory " + root
	}
	opts.step("list files on " + source.Name)
	srcFiles, err := s.walkModelDir(ctx, source, root)
	if err != nil {
		return model.ClusterSyncActionFailed, err.Error()
	}
	if len(srcFiles) == 0 {
		return model.ClusterSyncActionFailed, "model directory is empty or missing on the source: " + root
	}
	opts.step("compare files on " + target.Name)
	dstFiles, err := s.walkModelDir(ctx, target, root)
	if err != nil {
		return model.ClusterSyncActionFailed, err.Error()
	}
	have := make(map[string]int64, len(dstFiles))
	for _, f := range dstFiles {
		have[f.Rel] = f.Size
	}

	var todo []modelFile
	var totalBytes, largest int64
	conflicts := 0
	for _, f := range srcFiles {
		if size, ok := have[f.Rel]; ok {
			if size == f.Size {
				continue // already there (e.g. resumed send)
			}
			if !opts.Overwrite {
				conflicts++
				continue
			}
		}
		todo = append(todo, f)
		totalBytes += f.Size
		if f.Size > largest {
			largest = f.Size
		}
	}
	if conflicts > 0 {
		return model.ClusterSyncActionSkipConflict, fmt.Sprintf("%d file(s) already exist on the target with a different size; enable overwrite to replace them", conflicts)
	}

	if len(todo) > 0 {
		tmpDir := modelSyncTmpDir()
		if err := os.MkdirAll(tmpDir, 0o700); err != nil {
			return model.ClusterSyncActionFailed, "could not create master temp directory: " + err.Error()
		}
		// The master only holds one file at a time.
		if usage, err := disk.Usage(tmpDir); err == nil && usage.Free < uint64(largest)+512<<20 {
			return model.ClusterSyncActionFailed, fmt.Sprintf("not enough free space on the master (%s): %d MB free, the largest model file needs %d MB",
				tmpDir, usage.Free>>20, largest>>20)
		}
		report := opts.byteProgress()
		var doneBase int64
		for i, f := range todo {
			if ctx.Err() != nil {
				return model.ClusterSyncActionFailed, errSyncCancelled.Error()
			}
			label := fmt.Sprintf("(%d/%d) %s", i+1, len(todo), f.Rel)
			// Both legs (download to master, upload to target) count half
			// of the file so the bar covers the whole model once.
			var dl, ul int64
			emit := func() {
				if report != nil {
					report(doneBase+(dl+ul)/2, totalBytes)
				}
			}
			opts.step(label + ": download from " + source.Name)
			tmpPath, err := s.downloadModelFile(ctx, source, path.Join(root, f.Rel), f.Size, tmpDir, func(done int64) { dl = done; emit() })
			if err != nil {
				if ctx.Err() != nil {
					return model.ClusterSyncActionFailed, errSyncCancelled.Error()
				}
				return model.ClusterSyncActionFailed, fmt.Sprintf("download %s from source failed: %s", f.Rel, truncateErr(err.Error()))
			}
			opts.step(label + ": upload to " + target.Name)
			dl = f.Size
			dstDir := path.Dir(path.Join(root, f.Rel))
			upErr := s.uploadNodeFile(ctx, target, dstDir, path.Base(f.Rel), tmpPath, true, func(sent, _ int64) { ul = sent; emit() })
			_ = os.Remove(tmpPath)
			if upErr != nil {
				if ctx.Err() != nil {
					return model.ClusterSyncActionFailed, errSyncCancelled.Error()
				}
				return model.ClusterSyncActionFailed, fmt.Sprintf("upload %s to target failed: %s", f.Rel, truncateErr(upErr.Error()))
			}
			doneBase += f.Size
			dl, ul = 0, 0
			emit()
		}
	}

	opts.step("register model on " + target.Name)
	body := map[string]interface{}{
		"repoId": res.Name, "kind": modelKind(res), "dir": root,
		"source": res.Extra, "revision": toString(res.Meta["revision"]),
	}
	if err := s.callNodeJSON(target, http.MethodPost, "/api/v2/ai/vllm/models/import", body, nil, 2*time.Minute); err != nil {
		return model.ClusterSyncActionFailed, "files copied to " + root + ", but registering the model in the target vLLM list failed (upgrade the target panel or import it manually): " + truncateErr(err.Error())
	}
	if len(todo) == 0 {
		return model.ClusterSyncActionSkipSame, "target already has all model files; registered in its vLLM model list"
	}
	return model.ClusterSyncActionCreate, ""
}

func modelKind(res dto.ClusterSyncResource) string {
	if res.Detail == "tokenizer" {
		return "tokenizer"
	}
	return "model"
}

// downloadModelFile streams one file of a node into a temp file on the
// master. Each attempt asks for the remaining bytes (HTTP Range), so a
// connection cut by the remote server's write timeout, a network drop or
// an idle stall resumes instead of restarting the multi-GB file.
func (s *ClusterSyncService) downloadModelFile(ctx context.Context, node model.ClusterNode, remotePath string, expected int64, tmpDir string, progress func(done int64)) (string, error) {
	tmp, err := os.CreateTemp(tmpDir, "model-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()
	if expected == 0 {
		ok = true
		return tmpPath, nil
	}

	var offset int64
	failures := 0
	var lastErr error
	for offset < expected {
		if ctx.Err() != nil {
			return "", errSyncCancelled
		}
		if failures >= modelDownloadRetries {
			return "", fmt.Errorf("giving up after %d attempts without progress: %v", failures, lastErr)
		}
		if failures > 0 {
			select {
			case <-ctx.Done():
				return "", errSyncCancelled
			case <-time.After(time.Duration(failures) * 3 * time.Second):
			}
		}
		n, restart, err := s.downloadModelRange(ctx, node, remotePath, offset, tmp, func(written int64) {
			if progress != nil {
				progress(offset + written)
			}
		})
		if restart {
			// The server ignored the Range header and sent the whole
			// file from byte 0: the bytes were written from offset 0.
			offset = 0
		}
		offset += n
		if err != nil {
			if errors.Is(err, errSyncCancelled) || ctx.Err() != nil {
				return "", errSyncCancelled
			}
			lastErr = err
			if n > 0 {
				failures = 0
			} else {
				failures++
			}
			// Hard errors (missing file, auth) will not get better.
			if errors.Is(err, errModelDownloadFatal) {
				return "", err
			}
			continue
		}
		if offset < expected {
			// The stream ended early without an error: resume.
			if n == 0 {
				failures++
				lastErr = fmt.Errorf("stream ended at %d of %d bytes", offset, expected)
			}
			continue
		}
	}
	if offset > expected {
		return "", fmt.Errorf("received %d bytes, expected %d (the source file changed during the transfer)", offset, expected)
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	ok = true
	return tmpPath, nil
}

var errModelDownloadFatal = errors.New("download failed")

// downloadModelRange performs one GET of remotePath from offset and writes
// the body into out at offset. It returns the number of bytes written and
// whether the server restarted the file from 0 (no Range support).
func (s *ClusterSyncService) downloadModelRange(ctx context.Context, node model.ClusterNode, remotePath string, offset int64, out *os.File, progress func(written int64)) (int64, bool, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Idle watchdog: cancel the attempt when the body stalls.
	var mu sync.Mutex
	watchdog := time.AfterFunc(modelDownloadIdle, cancel)
	defer watchdog.Stop()
	touch := func() {
		mu.Lock()
		watchdog.Reset(modelDownloadIdle)
		mu.Unlock()
	}

	resp, err := s.openNodeDownload(attemptCtx, node, remotePath, offset)
	if err != nil {
		if ctx.Err() != nil {
			return 0, false, errSyncCancelled
		}
		return 0, false, err
	}
	defer resp.Body.Close()
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var env remoteEnvelope
		if json.Unmarshal(raw, &env) == nil && strings.TrimSpace(env.Message) != "" {
			return 0, false, fmt.Errorf("%w: %s", errModelDownloadFatal, truncateErr(env.Message))
		}
		return 0, false, fmt.Errorf("%w: unreadable download response", errModelDownloadFatal)
	}
	restart := false
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		if offset > 0 {
			restart = true
			offset = 0
			if err := out.Truncate(0); err != nil {
				return 0, false, err
			}
		}
	case http.StatusRequestedRangeNotSatisfiable:
		return 0, false, fmt.Errorf("%w: the source file is smaller than expected (changed during the transfer?)", errModelDownloadFatal)
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return 0, false, fmt.Errorf("%w: source panel returned HTTP %d", errModelDownloadFatal, resp.StatusCode)
	default:
		return 0, false, fmt.Errorf("source panel returned HTTP %d", resp.StatusCode)
	}
	buf := make([]byte, 1<<20)
	var written int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			touch()
			if _, werr := out.WriteAt(buf[:n], offset+written); werr != nil {
				return written, restart, fmt.Errorf("%w: write master temp file: %v", errModelDownloadFatal, werr)
			}
			written += int64(n)
			progress(written)
		}
		if rerr == io.EOF {
			return written, restart, nil
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return written, restart, errSyncCancelled
			}
			return written, restart, rerr
		}
	}
}

// openNodeDownload issues GET /api/v2/files/download for a node, asking
// for the bytes from offset on.
func (s *ClusterSyncService) openNodeDownload(ctx context.Context, node model.ClusterNode, remotePath string, offset int64) (*http.Response, error) {
	query := "/api/v2/files/download?path=" + url.QueryEscape(remotePath)
	newReq := func(base string) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+query, nil)
		if err != nil {
			return nil, err
		}
		if offset > 0 {
			req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		}
		return req, nil
	}
	if isLocalNode(node) {
		req, err := newReq("http://localhost")
		if err != nil {
			return nil, err
		}
		// No overall timeout: the idle watchdog bounds stalls.
		return localAgentHTTPClient(0).Do(req)
	}
	apiBase, err := panelAPIBase(node.EntryURL)
	if err != nil {
		return nil, err
	}
	timestamp, hmacToken, md5Token, err := s.nodes.remoteAuthHeaders(node)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: time.Minute,
	}}
	do := func(token string) (*http.Response, error) {
		req, err := newReq(apiBase)
		if err != nil {
			return nil, err
		}
		req.Header.Set("1Panel-Timestamp", timestamp)
		req.Header.Set("1Panel-Token", token)
		return client.Do(req)
	}
	resp, err := do(hmacToken)
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		resp, err = do(md5Token)
	}
	return resp, err
}
