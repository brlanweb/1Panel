package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
)

// Prepared models for deployment plans.
//
// A plan names the model by the repo id of AI → vLLM → Models (the "model
// preparation" page of every panel host). Before an instance starts, every
// host of the instance must hold the model; hosts that miss it get a copy
// from a host that has it:
//
//   1. direct: a one-shot, read-only rsync daemon on the source bound to
//      the fabric (RoCE / optical) address, pulled by the target — fast and
//      does not touch the master;
//   2. relay: when rsync is missing on a panel pair, files go through the
//      master (cluster model transfer, chunked and resumable).
//
// The copy lands under the same absolute path (multi-host engines need one
// path on every rank) and is registered in the target's model list.

const (
	computeSyncIdle   = 10 * time.Minute // no byte for this long = failed
	computeSyncMax    = 6 * time.Hour
	computeSyncPoll   = 10 * time.Second
	computeModelsWait = 20 * time.Second
)

type computePreparedModel struct {
	ID       uint   `json:"id"`
	RepoID   string `json:"repoId"`
	Kind     string `json:"kind"`
	Dir      string `json:"dir"`
	Status   string `json:"status"`
	Size     int64  `json:"size"`
	RecipeID uint   `json:"recipeId"`
}

type computeRecipe struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	GPUType string `json:"gpuType"`
	Image   string `json:"image"`
	Args    string `json:"args"`
	Env     string `json:"env"`
}

// computeHostModels lists the finished models of a panel host.
func computeHostModels(h model.ComputeHost) ([]computePreparedModel, error) {
	var page computeVllmPage[computePreparedModel]
	body := map[string]interface{}{"page": 1, "pageSize": 1000, "kind": "model"}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/models/search", body, &page, computeModelsWait); err != nil {
		return nil, err
	}
	res := make([]computePreparedModel, 0, len(page.Items))
	for _, m := range page.Items {
		if m.Status == "Success" && m.Dir != "" && (m.Kind == "" || m.Kind == "model") {
			m.Dir = filepath.Clean(m.Dir)
			res = append(res, m)
		}
	}
	return res, nil
}

func computeHostRecipes(h model.ComputeHost) map[uint]computeRecipe {
	var page computeVllmPage[computeRecipe]
	body := map[string]interface{}{"page": 1, "pageSize": 500}
	res := map[uint]computeRecipe{}
	if err := computePanelCall(h, http.MethodPost, "/api/v2/ai/vllm/recipes/search", body, &page, computeModelsWait); err != nil {
		return res
	}
	for _, r := range page.Items {
		res[r.ID] = r
	}
	return res
}

type computeHostModelSet struct {
	models []computePreparedModel
	err    error
}

// computeScanModels lists the prepared models of the given panel hosts in
// parallel (hosts reached over SSH have no model list and are skipped).
func computeScanModels(hosts map[uint]model.ComputeHost, ids []uint) map[uint]computeHostModelSet {
	res := map[uint]computeHostModelSet{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		h, ok := hosts[id]
		if !ok || !h.IsPanel() {
			continue
		}
		wg.Add(1)
		go func(h model.ComputeHost) {
			defer wg.Done()
			list, err := computeHostModels(h)
			mu.Lock()
			res[h.ID] = computeHostModelSet{models: list, err: err}
			mu.Unlock()
		}(h)
	}
	wg.Wait()
	return res
}

func computeAllHostIDs(hosts map[uint]model.ComputeHost) []uint {
	ids := make([]uint, 0, len(hosts))
	for id := range hosts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// ListModels aggregates the prepared models of all panel hosts by repo id.
func (u *ComputeService) ListModels() (dto.ComputeModelList, error) {
	hosts := computeHostMap()
	scan := computeScanModels(hosts, computeAllHostIDs(hosts))
	byRepo := map[string]*dto.ComputeModelItem{}
	var errs []string
	recipeHost := map[string]uint{}
	recipeID := map[string]uint{}
	for _, id := range computeAllHostIDs(hosts) {
		set, ok := scan[id]
		if !ok {
			continue
		}
		h := hosts[id]
		if set.err != nil {
			errs = append(errs, h.Name+": "+computeTrim(set.err.Error(), 200))
			continue
		}
		for _, m := range set.models {
			key := m.RepoID
			if key == "" {
				key = m.Dir
			}
			item := byRepo[key]
			if item == nil {
				item = &dto.ComputeModelItem{RepoID: key, Name: path.Base(key)}
				byRepo[key] = item
			}
			item.Hosts = append(item.Hosts, dto.ComputeModelHost{HostID: id, HostName: h.Name, Dir: m.Dir, Size: m.Size, RecipeID: m.RecipeID})
			if m.RecipeID > 0 && recipeID[key] == 0 {
				recipeHost[key], recipeID[key] = id, m.RecipeID
			}
		}
	}
	// Recipes are per host; read each host once.
	recipes := map[uint]map[uint]computeRecipe{}
	for key, hid := range recipeHost {
		if recipes[hid] == nil {
			recipes[hid] = computeHostRecipes(hosts[hid])
		}
		if r, ok := recipes[hid][recipeID[key]]; ok {
			byRepo[key].Recipe = &dto.ComputeModelRecipe{Name: r.Name, GPUType: r.GPUType, Image: r.Image, Args: r.Args, Env: r.Env}
		}
	}
	res := dto.ComputeModelList{Items: make([]dto.ComputeModelItem, 0, len(byRepo)), Errors: errs}
	for _, item := range byRepo {
		res.Items = append(res.Items, *item)
	}
	sort.Slice(res.Items, func(i, j int) bool { return res.Items[i].RepoID < res.Items[j].RepoID })
	return res, nil
}

// computeModelMatch reports whether a prepared model is the plan's model:
// same repo id, or (plans without a repo) the same directory.
func computeModelMatch(prof model.ComputeProfile, m computePreparedModel) bool {
	if prof.ModelRepo != "" {
		return m.RepoID == prof.ModelRepo || (m.RepoID == "" && m.Dir == prof.ModelRepo)
	}
	return filepath.Clean(m.Dir) == filepath.Clean(prof.ModelPath)
}

// computeModelPlacement is where the model of a plan is (and is missing).
type computeModelPlacement struct {
	Have    map[uint]string // hostID -> model directory
	Missing []uint          // hosts of the instance without the model
	Unknown []string        // hosts that could not be checked
}

// computeLocateModel finds the plan's model on the given hosts. Panel hosts
// are checked through their model list; SSH hosts by config.json under the
// plan's model path.
func computeLocateModel(prof model.ComputeProfile, ids []uint, hosts map[uint]model.ComputeHost) computeModelPlacement {
	res := computeModelPlacement{Have: map[uint]string{}}
	scan := computeScanModels(hosts, ids)
	for _, id := range ids {
		h, ok := hosts[id]
		if !ok {
			continue
		}
		if h.IsPanel() {
			set := scan[id]
			if set.err != nil {
				res.Unknown = append(res.Unknown, h.Name+": "+computeTrim(set.err.Error(), 120))
			}
			found := ""
			for _, m := range set.models {
				if computeModelMatch(prof, m) {
					found = m.Dir
					if found == filepath.Clean(prof.ModelPath) {
						break
					}
				}
			}
			if found == "" && set.err == nil && computePathHasModel(h, prof.ModelPath) {
				// Present on disk but not registered in the model list.
				found = filepath.Clean(prof.ModelPath)
			}
			if found != "" {
				res.Have[id] = found
				continue
			}
		} else if computePathHasModel(h, prof.ModelPath) {
			res.Have[id] = filepath.Clean(prof.ModelPath)
			continue
		}
		res.Missing = append(res.Missing, id)
	}
	return res
}

func computePathHasModel(h model.ComputeHost, dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	script := fmt.Sprintf("test -f %s && echo OK || echo MISSING", computeShellQuote(path.Join(dir, "config.json")))
	out, err := computeExec(h, script, false, 15*time.Second)
	return err == nil && strings.Contains(out, "OK")
}

// computeModelSource picks the host to copy the model from: a member of
// the instance that has it (head first), otherwise any host that has it,
// preferring one with a fabric link to the targets.
func computeModelSource(prof model.ComputeProfile, ids []uint, targets []uint, hosts map[uint]model.ComputeHost,
	place computeModelPlacement) (uint, string, bool) {
	for _, id := range ids {
		if dir, ok := place.Have[id]; ok {
			return id, dir, true
		}
	}
	others := make([]uint, 0)
	member := map[uint]bool{}
	for _, id := range ids {
		member[id] = true
	}
	for _, id := range computeAllHostIDs(hosts) {
		if !member[id] {
			others = append(others, id)
		}
	}
	elsewhere := computeLocateModel(prof, others, hosts)
	var best uint
	bestDir := ""
	for _, id := range others {
		dir, ok := elsewhere.Have[id]
		if !ok {
			continue
		}
		if best == 0 {
			best, bestDir = id, dir
		}
		for _, t := range targets {
			if _, _, ok := computeLinkBetween(id, t); ok {
				return id, dir, true
			}
		}
	}
	return best, bestDir, best > 0
}

// computeLinkBetween returns the fabric addresses (a side, b side) of an
// ACTIVE link between two hosts.
func computeLinkBetween(a, b uint) (string, string, bool) {
	links, _ := computeLinks()
	for _, l := range links {
		if l.State != "ACTIVE" || l.IPA == "" || l.IPB == "" {
			continue
		}
		if l.HostAID == a && l.HostBID == b {
			return l.IPA, l.IPB, true
		}
		if l.HostAID == b && l.HostBID == a {
			return l.IPB, l.IPA, true
		}
	}
	return "", "", false
}

// computeEnsureModel makes sure every host of the instance has the model
// under one path and returns that path. The head's copy wins; when the head
// has none, the path of the source copy is used.
func computeEnsureModel(run *computeJobRun, prof model.ComputeProfile, ids []uint, hosts map[uint]model.ComputeHost) (string, error) {
	place := computeLocateModel(prof, ids, hosts)
	if dir, ok := place.Have[ids[0]]; ok && len(ids) == 1 {
		return dir, nil
	}
	var srcID uint
	want, ok := place.Have[ids[0]]
	if ok {
		srcID = ids[0]
	} else {
		var found bool
		srcID, want, found = computeModelSource(prof, ids, ids, hosts, place)
		if !found {
			return "", computeNoModelErr(prof, place)
		}
	}
	src := hosts[srcID]
	for _, id := range ids {
		if id == srcID || place.Have[id] == want {
			continue
		}
		dst := hosts[id]
		if err := run.step(fmt.Sprintf("sync model %s → %s", src.Name, dst.Name), func() (string, error) {
			return computeSyncModel(run, prof, src, dst, want)
		}); err != nil {
			return "", err
		}
	}
	return want, nil
}

func computeNoModelErr(prof model.ComputeProfile, place computeModelPlacement) error {
	name := prof.ModelRepo
	if name == "" {
		name = prof.ModelPath
	}
	msg := fmt.Sprintf("model %s is not prepared on any host (AI → vLLM → Models)", name)
	if len(place.Unknown) > 0 {
		msg += "; not checked: " + strings.Join(place.Unknown, "; ")
	}
	return fmt.Errorf("%s", msg)
}

func computeRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func computeHasRsync(h model.ComputeHost) bool {
	out, err := computeExec(h, "command -v rsync >/dev/null 2>&1 && echo OK || echo NO", false, 15*time.Second)
	return err == nil && strings.Contains(out, "OK")
}

// computeSyncModel copies dir from src to the same path on dst and
// registers it there.
func computeSyncModel(run *computeJobRun, prof model.ComputeProfile, src, dst model.ComputeHost, dir string) (string, error) {
	var msg string
	var err error
	if computeHasRsync(src) && computeHasRsync(dst) {
		msg, err = computeRsyncModel(run, src, dst, dir)
	} else if src.IsPanel() && dst.IsPanel() {
		msg, err = computeRelayModel(run, prof, src, dst, dir)
	} else {
		return "", fmt.Errorf("rsync is required on %s and %s to copy the model over SSH (apt install rsync / yum install rsync)", src.Name, dst.Name)
	}
	if err != nil {
		return msg, err
	}
	if dst.IsPanel() {
		repoID := prof.ModelRepo
		if repoID == "" {
			repoID = path.Base(dir)
		}
		body := map[string]interface{}{"repoId": repoID, "kind": "model", "dir": dir}
		if rerr := computePanelCall(dst, http.MethodPost, "/api/v2/ai/vllm/models/import", body, nil, 2*time.Minute); rerr != nil {
			msg += "\nregister in AI → vLLM → Models failed: " + rerr.Error()
		} else {
			msg += "\nregistered in AI → vLLM → Models of " + dst.Name
		}
	}
	return msg, nil
}

// computeRsyncModel: the source serves dir read-only through a one-shot
// rsync daemon (random port, random password, bound to the fabric address
// when the two hosts share a link); the target pulls in the background and
// is polled until rsync exits.
func computeRsyncModel(run *computeJobRun, src, dst model.ComputeHost, dir string) (string, error) {
	bind, _, fabric := computeLinkBetween(src.ID, dst.ID)
	if !fabric {
		bind = src.Addr
	}
	if bind == "" {
		return "", fmt.Errorf("no address of %s to serve the model from", src.Name)
	}
	pw := computeRandHex(16)
	var srvDir string
	var port int
	var lastErr error
	for attempt := 0; attempt < 3 && srvDir == ""; attempt++ {
		port = 20000 + mrand.IntN(20000)
		script := fmt.Sprintf(`set -e
D=$(mktemp -d /tmp/xp-msync.XXXXXX)
cat > "$D/rsyncd.conf" <<'XPEOF'
pid file = %[1]s/pid
use chroot = no
uid = 0
gid = 0
address = %[2]s
port = %[3]d
[m]
path = %[4]s
read only = yes
list = no
auth users = xp
secrets file = %[1]s/secrets
exclude = .cache/ *.incomplete *.lock ._____temp* .1panel-upload-*
XPEOF
sed -i "s#%[1]s#$D#g" "$D/rsyncd.conf"
printf 'xp:%%s\n' %[5]s > "$D/secrets"
chmod 600 "$D/secrets"
nohup timeout %[6]d rsync --daemon --no-detach --config="$D/rsyncd.conf" >"$D/log" 2>&1 </dev/null &
echo $! > "$D/tpid"
sleep 2
if ! kill -0 $! 2>/dev/null; then cat "$D/log"; rm -rf "$D"; exit 1; fi
echo "XPDIR=$D"
`, "@D@", bind, port, dir, computeShellQuote(pw), int(computeSyncMax.Seconds()))
		out, err := computeExec(src, script, true, time.Minute)
		if err != nil {
			lastErr = fmt.Errorf("start rsync daemon on %s: %v %s", src.Name, err, strings.TrimSpace(out))
			continue
		}
		srvDir = computeMarker(out, "XPDIR=")
	}
	if srvDir == "" {
		return "", lastErr
	}
	defer func() {
		_, _ = computeExec(src, fmt.Sprintf(`D=%s; for f in pid tpid; do [ -f "$D/$f" ] && kill $(cat "$D/$f") 2>/dev/null; done; rm -rf "$D"`, computeShellQuote(srvDir)), true, 30*time.Second)
	}()

	total := computeDirBytes(src, dir)
	url := fmt.Sprintf("rsync://xp@%s/m/", net.JoinHostPort(bind, strconv.Itoa(port)))
	pull := fmt.Sprintf(`set -e
mkdir -p %[1]s
D=$(mktemp -d /tmp/xp-mpull.XXXXXX)
printf '%%s' %[2]s > "$D/pw"
chmod 600 "$D/pw"
nohup sh -c 'rsync -a --partial --inplace --whole-file --password-file="$1/pw" "$2" "$3/" >"$1/log" 2>&1; echo $? > "$1/rc"' xp "$D" %[3]s %[1]s </dev/null >/dev/null 2>&1 &
echo "XPDIR=$D"
`, computeShellQuote(dir), computeShellQuote(pw), computeShellQuote(url))
	out, err := computeExec(dst, pull, true, time.Minute)
	if err != nil {
		return "", fmt.Errorf("start rsync on %s: %v %s", dst.Name, err, strings.TrimSpace(out))
	}
	cliDir := computeMarker(out, "XPDIR=")
	if cliDir == "" {
		return "", fmt.Errorf("start rsync on %s: %s", dst.Name, strings.TrimSpace(out))
	}
	defer func() {
		_, _ = computeExec(dst, fmt.Sprintf(`rm -rf %s`, computeShellQuote(cliDir)), true, 30*time.Second)
	}()

	via := "management network"
	if fabric {
		via = "fabric " + bind
	}
	start := time.Now()
	lastBytes, lastMove := int64(-1), time.Now()
	poll := fmt.Sprintf(`D=%s; echo "RC=$(cat "$D/rc" 2>/dev/null)"; echo "BYTES=$(du -sb %s 2>/dev/null | cut -f1)"; tail -c 400 "$D/log" 2>/dev/null`,
		computeShellQuote(cliDir), computeShellQuote(dir))
	for time.Since(start) < computeSyncMax {
		time.Sleep(computeSyncPoll)
		out, err := computeExec(dst, poll, true, 30*time.Second)
		if err != nil {
			continue
		}
		rc := computeMarker(out, "RC=")
		bytes, _ := strconv.ParseInt(computeMarker(out, "BYTES="), 10, 64)
		if bytes != lastBytes {
			lastBytes, lastMove = bytes, time.Now()
		}
		speed := float64(bytes) / time.Since(start).Seconds()
		progress := fmt.Sprintf("%s → %s via %s: %s", src.Name, dst.Name, via, computeSyncProgress(bytes, total, speed))
		run.progress(progress)
		if rc != "" {
			if rc != "0" {
				return progress, fmt.Errorf("rsync exited with %s: %s", rc, computeTrim(computeAfterMarkers(out), 400))
			}
			return progress + fmt.Sprintf(", done in %s", time.Since(start).Round(time.Second)), nil
		}
		if time.Since(lastMove) > computeSyncIdle {
			_, _ = computeExec(dst, fmt.Sprintf(`pkill -f %s 2>/dev/null || true`, computeShellQuote(url)), true, 15*time.Second)
			return progress, fmt.Errorf("no progress for %s", computeSyncIdle)
		}
	}
	return "", fmt.Errorf("model copy not finished after %s", computeSyncMax)
}

// computeRelayModel copies through the master (cluster model transfer).
func computeRelayModel(run *computeJobRun, prof model.ComputeProfile, src, dst model.ComputeHost, dir string) (string, error) {
	srcNode, err := computePanelNode(src)
	if err != nil {
		return "", err
	}
	dstNode, err := computePanelNode(dst)
	if err != nil {
		return "", err
	}
	repoID := prof.ModelRepo
	if repoID == "" {
		repoID = path.Base(dir)
	}
	stage := ""
	opts := syncTransferOpts{
		Ctx:  context.Background(),
		Step: func(s string) { stage = s; run.progress("relay via master: " + s) },
		Progress: func(done, total, rate int64) {
			run.progress(fmt.Sprintf("relay via master: %s · %s", stage, computeSyncProgress(done, total, float64(rate))))
		},
	}
	res := dto.ClusterSyncResource{Key: dir, Name: repoID, Detail: "model", Extra: "local", Meta: map[string]interface{}{}}
	action, msg := NewIClusterSyncService().(*ClusterSyncService).transferModel(srcNode, dstNode, res, opts)
	if action == model.ClusterSyncActionFailed || action == model.ClusterSyncActionSkipConflict {
		return msg, fmt.Errorf("relay %s → %s: %s", src.Name, dst.Name, msg)
	}
	return fmt.Sprintf("copied %s → %s through the master (rsync not installed)", src.Name, dst.Name), nil
}

func computeDirBytes(h model.ComputeHost, dir string) int64 {
	out, err := computeExec(h, fmt.Sprintf(`du -sb %s 2>/dev/null | cut -f1`, computeShellQuote(dir)), true, 2*time.Minute)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

func computeSyncProgress(done, total int64, speed float64) string {
	gib := func(b int64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }
	s := gib(done)
	if total > 0 {
		pct := float64(done) * 100 / float64(total)
		if pct > 100 {
			pct = 100
		}
		s += fmt.Sprintf(" / %s (%.0f%%)", gib(total), pct)
	}
	if speed > 0 {
		s += fmt.Sprintf(", %.0f MiB/s", speed/(1<<20))
	}
	return s
}

func computeMarker(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	return ""
}

func computeAfterMarkers(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "RC=") || strings.HasPrefix(line, "BYTES=") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

// ---- runtime selection ----------------------------------------------------

// computeAutoRuntime picks the built-in runtime for a plan from its hosts:
// NPU hosts → vllm-ascend; several hosts → vllm-ray-multinode; one panel
// host → vllm-panel (visible in AI → vLLM); one SSH host → vllm-single.
func computeAutoRuntime(ids []uint, nodeCount int, hosts map[uint]model.ComputeHost) (model.ComputeRuntime, error) {
	runtimes, err := repo.ComputeRuntimeRepo.List()
	if err != nil {
		return model.ComputeRuntime{}, err
	}
	device := computeKindGPU
	panel := true
	for _, id := range ids {
		if h, ok := hosts[id]; ok && !h.IsPanel() {
			panel = false
		}
		if computeAccelCount(computeState.hostRealtime(id), computeKindNPU) > 0 {
			device = computeKindNPU
		}
	}
	want := "vllm-single"
	switch {
	case device == computeKindNPU:
		want = "vllm-ascend"
	case nodeCount > 1:
		want = "vllm-ray-multinode"
	case panel && len(ids) > 0:
		want = "vllm-panel"
	}
	for _, rt := range runtimes {
		if rt.Name == want {
			return rt, nil
		}
	}
	return model.ComputeRuntime{}, fmt.Errorf("built-in runtime %s not found", want)
}
