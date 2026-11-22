package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/app/task"
	"github.com/1Panel-dev/1Panel/agent/buserr"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/cmd"
	"github.com/1Panel-dev/1Panel/agent/utils/compose"
	"github.com/1Panel-dev/1Panel/agent/utils/docker"
	"github.com/1Panel-dev/1Panel/agent/utils/files"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

// vLLM management (open-source re-implementation of the AI → vLLM page):
//
//   - settings: model / tokenizer directories and ModelScope / Hugging Face
//     endpoints + tokens (stored in the agent settings table);
//   - models: download a repo with the official CLI run inside a throwaway
//     python container (nothing installed on the host), or register an
//     existing local directory;
//   - recipes: reusable launch templates (image + engine args + env);
//   - deployments: vllm/vllm-openai served through docker compose with GPU
//     reservation, or "external" instances that only record an endpoint.

const (
	vllmSettingModelDir        = "VllmModelDir"
	vllmSettingTokenizerDir    = "VllmTokenizerDir"
	vllmSettingHFEndpoint      = "VllmHFEndpoint"
	vllmSettingHFToken         = "VllmHFToken"
	vllmSettingModelScopeURL   = "VllmModelScopeURL"
	vllmSettingModelScopeToken = "VllmModelScopeToken"
	vllmSettingProxyMode       = "VllmProxyMode"
	vllmSettingProxyURL        = "VllmProxyURL"

	defaultModelScopeURL = "https://www.modelscope.cn"
	defaultVllmImage     = "vllm/vllm-openai:latest"
	vllmDownloaderImage  = "python:3.12-slim"
	vllmContainerModels  = "/models"

	vllmStatusDownloading = "Downloading"
)

var vllmNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
var vllmRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type VllmService struct {
	repo repo.IVllmRepo
}

type IVllmService interface {
	LoadSettings() dto.VllmSettings
	SaveSettings(req dto.VllmSettings) error

	PageModels(req dto.VllmSearch) (int64, []model.VllmModel, error)
	DownloadModel(req dto.VllmModelDownload) error
	ImportModel(req dto.VllmModelImport) error
	CancelModelDownload(id uint) error
	RetryModelDownload(id uint) error
	DeleteModel(req dto.VllmModelDelete) error

	PageRecipes(req dto.VllmSearch) (int64, []model.VllmRecipe, error)
	SaveRecipe(req dto.VllmRecipeSave) error
	DeleteRecipe(id uint) error

	PageDeployments(req dto.VllmSearch) (int64, []dto.VllmDeploymentInfo, error)
	SaveDeployment(req dto.VllmDeploymentSave) error
	OperateDeployment(req dto.VllmOperate) error
	DeleteDeployment(id uint) error
	DeploymentStats(ids []uint) ([]dto.VllmDeploymentStats, error)
	CheckDeployment(id uint) (dto.VllmHealth, error)
}

func NewIVllmService() IVllmService {
	return &VllmService{repo: repo.NewIVllmRepo()}
}

// ---------------------------------------------------------------- settings

func (s *VllmService) LoadSettings() dto.VllmSettings {
	get := func(key, def string) string {
		if v, err := settingRepo.GetValueByKey(key); err == nil && strings.TrimSpace(v) != "" {
			return v
		}
		return def
	}
	return dto.VllmSettings{
		ModelDir:        get(vllmSettingModelDir, path.Join(global.Dir.DataDir, "ai/vllm/models")),
		TokenizerDir:    get(vllmSettingTokenizerDir, path.Join(global.Dir.DataDir, "ai/vllm/tokenizers")),
		HFEndpoint:      get(vllmSettingHFEndpoint, ""),
		HFToken:         get(vllmSettingHFToken, ""),
		ModelScopeURL:   get(vllmSettingModelScopeURL, defaultModelScopeURL),
		ModelScopeToken: get(vllmSettingModelScopeToken, ""),
		ProxyMode:       get(vllmSettingProxyMode, "none"),
		ProxyURL:        get(vllmSettingProxyURL, ""),
	}
}

func (s *VllmService) SaveSettings(req dto.VllmSettings) error {
	for _, dir := range []string{req.ModelDir, req.TokenizerDir} {
		if dir == "" || !filepath.IsAbs(dir) || filepath.Clean(dir) == "/" {
			return buserr.WithDetail("ErrInvalidParams", "directory must be an absolute path other than /", nil)
		}
	}
	for _, u := range []string{req.HFEndpoint, req.ModelScopeURL} {
		if u == "" {
			continue
		}
		if p, err := url.Parse(u); err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return buserr.WithDetail("ErrInvalidParams", "invalid URL: "+u, nil)
		}
	}
	switch req.ProxyMode {
	case "", "none":
		req.ProxyMode = "none"
	case "node":
	case "custom":
		p, err := url.Parse(strings.TrimSpace(req.ProxyURL))
		if err != nil || p.Host == "" || (p.Scheme != "http" && p.Scheme != "https" && p.Scheme != "socks5" && p.Scheme != "socks5h") {
			return buserr.WithDetail("ErrInvalidParams", "proxy URL must look like http://host:port or socks5://host:port", nil)
		}
	default:
		return buserr.WithDetail("ErrInvalidParams", "invalid proxy mode", nil)
	}
	pairs := map[string]string{
		vllmSettingProxyMode:       req.ProxyMode,
		vllmSettingProxyURL:        strings.TrimSpace(req.ProxyURL),
		vllmSettingModelDir:        filepath.Clean(req.ModelDir),
		vllmSettingTokenizerDir:    filepath.Clean(req.TokenizerDir),
		vllmSettingHFEndpoint:      strings.TrimRight(req.HFEndpoint, "/"),
		vllmSettingHFToken:         req.HFToken,
		vllmSettingModelScopeURL:   strings.TrimRight(req.ModelScopeURL, "/"),
		vllmSettingModelScopeToken: req.ModelScopeToken,
	}
	for k, v := range pairs {
		if err := settingRepo.UpdateOrCreate(k, v); err != nil {
			return err
		}
	}
	return nil
}

// vllmProxy describes the proxy used by vLLM. It is deliberately scoped:
// only model downloads (env of the short-lived downloader container) and
// vLLM image pulls (an in-process registry client) use it. The Docker
// daemon, the panel process environment and the running vLLM deployments are
// never proxied, so every other feature keeps connecting directly.
type vllmProxy struct {
	// containerURL is reachable from containers attached to network.
	containerURL string
	network      string
	// hostURL is reachable from the agent process (image pulls).
	hostURL string
}

// loadVllmProxyFor resolves the proxy of one download: nil uses the vLLM
// settings, false disables the proxy, true forces it (custom proxy when one
// is configured, otherwise the subscription node).
func (s *VllmService) loadVllmProxyFor(useProxy *bool) (*vllmProxy, error) {
	if useProxy == nil {
		return s.loadVllmProxy()
	}
	if !*useProxy {
		return nil, nil
	}
	st := s.LoadSettings()
	if st.ProxyMode == "custom" && st.ProxyURL != "" {
		return s.loadVllmProxyMode("custom")
	}
	return s.loadVllmProxyMode("node")
}

func (s *VllmService) loadVllmProxy() (*vllmProxy, error) {
	return s.loadVllmProxyMode(s.LoadSettings().ProxyMode)
}

func (s *VllmService) loadVllmProxyMode(mode string) (*vllmProxy, error) {
	st := s.LoadSettings()
	switch mode {
	case "node":
		svc := NewIProxyNodeService()
		containerURL, err := svc.ProxyURLForContainer()
		if err != nil {
			return nil, err
		}
		hostURL, err := svc.ProxyURLForHost()
		if err != nil {
			return nil, err
		}
		return &vllmProxy{containerURL: containerURL, network: "1panel-network", hostURL: hostURL}, nil
	case "custom":
		if st.ProxyURL == "" {
			return nil, nil
		}
		return &vllmProxy{containerURL: st.ProxyURL, hostURL: st.ProxyURL}, nil
	default:
		return nil, nil
	}
}

// containerEnv is injected only into the model downloader container.
func (p *vllmProxy) containerEnv() map[string]string {
	if p == nil || p.containerURL == "" {
		return nil
	}
	u := p.containerURL
	noProxy := "localhost,127.0.0.1,::1"
	return map[string]string{
		"HTTP_PROXY": u, "HTTPS_PROXY": u, "ALL_PROXY": u,
		"http_proxy": u, "https_proxy": u, "all_proxy": u,
		"NO_PROXY": noProxy, "no_proxy": noProxy,
	}
}

// pullImage pulls imageName through the proxy (no-op without proxy or when
// the image already exists; compose/docker then pull directly as usual).
func (p *vllmProxy) pullImage(ctx context.Context, t *task.Task, imageName string) error {
	if p == nil || p.hostURL == "" {
		return nil
	}
	return pullImageViaProxy(ctx, t, imageName, p.hostURL)
}

// ------------------------------------------------------------------ models

func (s *VllmService) PageModels(req dto.VllmSearch) (int64, []model.VllmModel, error) {
	return s.repo.PageModels(req.Page, req.PageSize, vllmModelOpts(req)...)
}

func vllmModelOpts(req dto.VllmSearch) []repo.DBOption {
	var opts []repo.DBOption
	if req.Info != "" {
		info := "%" + req.Info + "%"
		opts = append(opts, func(db *gorm.DB) *gorm.DB { return db.Where("repo_id LIKE ?", info) })
	}
	if req.Kind != "" {
		kind := req.Kind
		opts = append(opts, func(db *gorm.DB) *gorm.DB { return db.Where("kind = ?", kind) })
	}
	return opts
}

// modelTargetDir is <modelDir|tokenizerDir>/<org>/<name>.
func (s *VllmService) modelTargetDir(kind, repoID string) string {
	st := s.LoadSettings()
	base := st.ModelDir
	if kind == "tokenizer" {
		base = st.TokenizerDir
	}
	return path.Join(base, repoID)
}

var vllmDownloads = struct {
	sync.Mutex
	cancel map[uint]context.CancelFunc
}{cancel: map[uint]context.CancelFunc{}}

func (s *VllmService) DownloadModel(req dto.VllmModelDownload) error {
	req.RepoID = strings.TrimSpace(req.RepoID)
	if !vllmRepoPattern.MatchString(req.RepoID) {
		return buserr.WithDetail("ErrInvalidParams", "repository id must look like org/name", nil)
	}
	if req.Revision != "" && !regexp.MustCompile(`^[A-Za-z0-9_.\-/]{1,100}$`).MatchString(req.Revision) {
		return buserr.WithDetail("ErrInvalidParams", "invalid revision", nil)
	}
	if existing, _ := s.repo.GetModel(func(db *gorm.DB) *gorm.DB {
		return db.Where("repo_id = ? AND kind = ? AND status = ?", req.RepoID, req.Kind, vllmStatusDownloading)
	}); existing != nil {
		return buserr.WithDetail("ErrInvalidParams", req.RepoID+" is already downloading", nil)
	}
	item := &model.VllmModel{
		RepoID: req.RepoID, Source: req.Source, Kind: req.Kind, Revision: req.Revision,
		Dir: s.modelTargetDir(req.Kind, req.RepoID), Status: vllmStatusDownloading,
		TaskID: req.TaskID, RecipeID: req.RecipeID,
	}
	if old, _ := s.repo.GetModel(func(db *gorm.DB) *gorm.DB {
		return db.Where("repo_id = ? AND kind = ?", req.RepoID, req.Kind)
	}); old != nil {
		item.ID, item.CreatedAt = old.ID, old.CreatedAt
	}
	if err := s.repo.SaveModel(item); err != nil {
		return err
	}
	go s.runModelDownload(item, req.UseProxy)
	return nil
}

func (s *VllmService) RetryModelDownload(id uint) error {
	item, err := s.repo.GetModel(repo.WithByID(id))
	if err != nil {
		return err
	}
	if item.Source == "local" {
		return buserr.WithDetail("ErrInvalidParams", "local models cannot be downloaded", nil)
	}
	return s.DownloadModel(dto.VllmModelDownload{RepoID: item.RepoID, Source: item.Source, Kind: item.Kind, Revision: item.Revision, RecipeID: item.RecipeID})
}

func (s *VllmService) CancelModelDownload(id uint) error {
	vllmDownloads.Lock()
	cancel, ok := vllmDownloads.cancel[id]
	vllmDownloads.Unlock()
	if !ok {
		return buserr.WithDetail("ErrInvalidParams", "no running download", nil)
	}
	cancel()
	return nil
}

// runModelDownload runs the official CLI in a disposable container. The
// CLIs resume partial downloads, so retry after a failure continues.
func (s *VllmService) runModelDownload(item *model.VllmModel, useProxy *bool) {
	ctx, cancel := context.WithCancel(context.Background())
	vllmDownloads.Lock()
	vllmDownloads.cancel[item.ID] = cancel
	vllmDownloads.Unlock()
	defer func() {
		vllmDownloads.Lock()
		delete(vllmDownloads.cancel, item.ID)
		vllmDownloads.Unlock()
		cancel()
	}()

	st := s.LoadSettings()
	containerName := fmt.Sprintf("1panel-vllm-download-%d", item.ID)
	args := []string{"run", "--rm", "--name", containerName, "-v", item.Dir + ":/target"}
	proxy, proxyErr := s.loadVllmProxyFor(useProxy)
	if proxyErr != nil {
		item.Status, item.Message = constant.StatusFailed, truncateVllmMsg(proxyErr.Error())
		_ = s.repo.SaveModel(item)
		return
	}
	if proxy != nil && proxy.network != "" {
		args = append(args, "--network", proxy.network)
	}
	for k, v := range proxy.containerEnv() {
		args = append(args, "-e", k+"="+v)
	}
	var script string
	switch item.Source {
	case "huggingface":
		if st.HFEndpoint != "" {
			args = append(args, "-e", "HF_ENDPOINT="+st.HFEndpoint)
		}
		if st.HFToken != "" {
			args = append(args, "-e", "HF_TOKEN="+st.HFToken)
		}
		args = append(args, "-e", "HF_HUB_ENABLE_HF_TRANSFER=0")
		rev := ""
		if item.Revision != "" {
			rev = " --revision " + shellQuote(item.Revision)
		}
		script = "pip install -q -U 'huggingface_hub[cli]' && huggingface-cli download " + shellQuote(item.RepoID) + rev + " --local-dir /target"
		if item.Kind == "tokenizer" {
			script += ` --include "*.json" "*.model" "*.txt" "*.tiktoken" "*.py"`
		}
	default: // modelscope
		if st.ModelScopeURL != "" && st.ModelScopeURL != defaultModelScopeURL {
			args = append(args, "-e", "MODELSCOPE_DOMAIN="+strings.TrimPrefix(strings.TrimPrefix(st.ModelScopeURL, "https://"), "http://"))
		}
		if st.ModelScopeToken != "" {
			args = append(args, "-e", "MODELSCOPE_API_TOKEN="+st.ModelScopeToken)
		}
		rev := ""
		if item.Revision != "" {
			rev = " --revision " + shellQuote(item.Revision)
		}
		login := ""
		if st.ModelScopeToken != "" {
			login = "modelscope login --token \"$MODELSCOPE_API_TOKEN\" >/dev/null && "
		}
		script = "pip install -q -U modelscope && " + login + "modelscope download --model " + shellQuote(item.RepoID) + rev + " --local_dir /target"
		if item.Kind == "tokenizer" {
			script += ` --include "*.json" "*.model" "*.txt" "*.tiktoken" "*.py"`
		}
	}
	args = append(args, vllmDownloaderImage, "sh", "-c", script)

	finish := func(status, msg string) {
		item.Status, item.Message = status, msg
		item.Size = dirSize(item.Dir)
		_ = s.repo.SaveModel(item)
	}
	if err := os.MkdirAll(item.Dir, 0o755); err != nil {
		finish(constant.StatusFailed, err.Error())
		return
	}

	dlTask, err := task.NewTaskWithOps(item.RepoID, task.TaskPull, task.TaskScopeAI, item.TaskID, item.ID)
	if err != nil {
		finish(constant.StatusFailed, err.Error())
		return
	}
	dlTask.AddSubTaskWithOps(task.GetTaskName(item.RepoID, task.TaskPull, task.TaskScopeAI), func(t *task.Task) error {
		t.Logf("download %s (%s) from %s into %s", item.RepoID, item.Kind, item.Source, item.Dir)
		if err := proxy.pullImage(ctx, t, vllmDownloaderImage); err != nil {
			return err
		}
		mgr := cmd.NewCommandMgr(cmd.WithTask(*t), cmd.WithContext(ctx), cmd.WithTimeout(24*time.Hour))
		return mgr.Run("docker", args...)
	}, nil, 0, 24*time.Hour)
	err = dlTask.Execute()
	switch {
	case ctx.Err() != nil:
		_ = cmd.NewCommandMgr(cmd.WithTimeout(30*time.Second)).Run("docker", "rm", "-f", containerName)
		finish(constant.StatusCanceled, "cancelled; run again to resume")
	case err != nil:
		finish(constant.StatusFailed, truncateVllmMsg(err.Error()))
	default:
		finish(constant.StatusSuccess, "")
	}
}

func (s *VllmService) ImportModel(req dto.VllmModelImport) error {
	dir := filepath.Clean(req.Dir)
	if !filepath.IsAbs(dir) || dir == "/" {
		return buserr.WithDetail("ErrInvalidParams", "directory must be an absolute path other than /", nil)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return buserr.WithDetail("ErrInvalidParams", "directory does not exist: "+dir, nil)
	}
	if _, err := os.Stat(path.Join(dir, "config.json")); err != nil && req.Kind == "model" {
		return buserr.WithDetail("ErrInvalidParams", "config.json not found in "+dir+" (not a Hugging Face format model)", nil)
	}
	item := &model.VllmModel{RepoID: strings.TrimSpace(req.RepoID), Source: "local", Kind: req.Kind, Dir: dir, Status: constant.StatusSuccess, Size: dirSize(dir)}
	return s.repo.SaveModel(item)
}

func (s *VllmService) DeleteModel(req dto.VllmModelDelete) error {
	item, err := s.repo.GetModel(repo.WithByID(req.ID))
	if err != nil {
		return err
	}
	if item.Status == vllmStatusDownloading {
		return buserr.WithDetail("ErrInvalidParams", "cancel the download first", nil)
	}
	if used, _ := s.repo.GetDeployment(func(db *gorm.DB) *gorm.DB { return db.Where("model_id = ?", item.ID) }); used != nil {
		return buserr.WithDetail("ErrInvalidParams", "model is used by deployment "+used.Name, nil)
	}
	// Only directories we created (under the managed model dirs) are deleted.
	if req.DeleteFile && item.Source != "local" {
		st := s.LoadSettings()
		for _, base := range []string{st.ModelDir, st.TokenizerDir} {
			if strings.HasPrefix(item.Dir, filepath.Clean(base)+"/") {
				_ = os.RemoveAll(item.Dir)
				break
			}
		}
	}
	return s.repo.DeleteModel(repo.WithByID(item.ID))
}

// ----------------------------------------------------------------- recipes

func (s *VllmService) PageRecipes(req dto.VllmSearch) (int64, []model.VllmRecipe, error) {
	var opts []repo.DBOption
	if req.Info != "" {
		info := "%" + req.Info + "%"
		opts = append(opts, func(db *gorm.DB) *gorm.DB { return db.Where("name LIKE ? OR model_repo LIKE ?", info, info) })
	}
	if req.Kind != "" {
		kind := req.Kind
		opts = append(opts, func(db *gorm.DB) *gorm.DB { return db.Where("gpu_type = ?", kind) })
	}
	return s.repo.PageRecipes(req.Page, req.PageSize, opts...)
}

func (s *VllmService) SaveRecipe(req dto.VllmRecipeSave) error {
	if _, err := parseVllmArgs(req.Args); err != nil {
		return err
	}
	if _, err := parseVllmEnv(req.Env); err != nil {
		return err
	}
	item := &model.VllmRecipe{}
	if req.ID > 0 {
		old, err := s.repo.GetRecipe(repo.WithByID(req.ID))
		if err != nil {
			return err
		}
		item = old
	}
	item.Name, item.GPUType, item.ModelRepo, item.Image = req.Name, req.GPUType, req.ModelRepo, req.Image
	item.Args, item.Env, item.Description = req.Args, req.Env, req.Description
	return s.repo.SaveRecipe(item)
}

func (s *VllmService) DeleteRecipe(id uint) error {
	return s.repo.DeleteRecipe(repo.WithByID(id))
}

// ------------------------------------------------------------- deployments

func (s *VllmService) PageDeployments(req dto.VllmSearch) (int64, []dto.VllmDeploymentInfo, error) {
	var opts []repo.DBOption
	if req.Info != "" {
		info := "%" + req.Info + "%"
		opts = append(opts, func(db *gorm.DB) *gorm.DB { return db.Where("name LIKE ?", info) })
	}
	total, items, err := s.repo.PageDeployments(req.Page, req.PageSize, opts...)
	if err != nil {
		return 0, nil, err
	}
	res := make([]dto.VllmDeploymentInfo, 0, len(items))
	for _, item := range items {
		info := dto.VllmDeploymentInfo{VllmDeployment: item, Endpoint: vllmEndpoint(&item)}
		info.APIKey = maskSecret(item.APIKey)
		if m, err := s.repo.GetModel(repo.WithByID(item.ModelID)); err == nil {
			info.ModelName = m.RepoID
		}
		res = append(res, info)
	}
	return total, res, nil
}

func vllmEndpoint(d *model.VllmDeployment) string {
	if d.External {
		return d.BaseURL
	}
	host := d.HostIP
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/v1", host, d.Port)
}

func (s *VllmService) SaveDeployment(req dto.VllmDeploymentSave) error {
	if !vllmNamePattern.MatchString(req.Name) {
		return buserr.WithDetail("ErrInvalidParams", "name may only contain letters, digits, '.', '_' and '-'", nil)
	}
	var item *model.VllmDeployment
	if req.ID > 0 {
		old, err := s.repo.GetDeployment(repo.WithByID(req.ID))
		if err != nil {
			return err
		}
		item = old
		if item.External != req.External {
			return buserr.WithDetail("ErrInvalidParams", "cannot switch between local and external", nil)
		}
	} else {
		if dup, _ := s.repo.GetDeployment(repo.WithByName(req.Name)); dup != nil {
			return buserr.New("ErrNameIsExist")
		}
		item = &model.VllmDeployment{Name: req.Name, External: req.External}
	}
	// A masked key coming back from the list means "unchanged".
	if req.APIKey != "" && !strings.Contains(req.APIKey, "****") {
		item.APIKey = req.APIKey
	}
	if req.External {
		p, err := url.Parse(req.BaseURL)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return buserr.WithDetail("ErrInvalidParams", "invalid base URL", nil)
		}
		item.BaseURL = strings.TrimRight(req.BaseURL, "/")
		item.ServedName = req.ServedName
		item.Status = constant.StatusRunning
		if h, _ := checkVllmEndpoint(item); !h.Healthy {
			item.Status, item.Message = constant.StatusError, h.Message
		}
		return s.repo.SaveDeployment(item)
	}

	mdl, err := s.repo.GetModel(repo.WithByID(req.ModelID))
	if err != nil {
		return buserr.WithDetail("ErrInvalidParams", "model not found", nil)
	}
	if mdl.Status != constant.StatusSuccess || mdl.Kind != "model" {
		return buserr.WithDetail("ErrInvalidParams", "model "+mdl.RepoID+" is not ready", nil)
	}
	if req.Port <= 0 || req.Port > 65535 {
		return buserr.WithDetail("ErrInvalidParams", "invalid port", nil)
	}
	if req.Port != item.Port {
		if err := checkPortExist(req.Port); err != nil {
			return err
		}
		if dup, _ := s.repo.GetDeployment(func(db *gorm.DB) *gorm.DB {
			return db.Where("port = ? AND external = ? AND id <> ?", req.Port, false, item.ID)
		}); dup != nil {
			return buserr.New("ErrPortInUsed")
		}
	}
	if req.GPUDevices != "" && req.GPUDevices != "all" && !regexp.MustCompile(`^\d+(,\d+)*$`).MatchString(req.GPUDevices) {
		return buserr.WithDetail("ErrInvalidParams", "GPU devices must be 'all' or indexes like 0,1", nil)
	}
	if _, err := parseVllmArgs(req.Args); err != nil {
		return err
	}
	if _, err := parseVllmEnv(req.Env); err != nil {
		return err
	}
	if req.GPUMemoryUtil != "" {
		if f, err := strconv.ParseFloat(req.GPUMemoryUtil, 64); err != nil || f <= 0 || f > 1 {
			return buserr.WithDetail("ErrInvalidParams", "GPU memory utilization must be in (0, 1]", nil)
		}
	}
	item.Image = strings.TrimSpace(req.Image)
	if item.Image == "" {
		item.Image = defaultVllmImage
	}
	item.HostIP, item.Port = req.HostIP, req.Port
	item.ModelID, item.ModelPath = mdl.ID, mdl.Dir
	item.ServedName = strings.TrimSpace(req.ServedName)
	if item.ServedName == "" {
		item.ServedName = path.Base(mdl.RepoID)
	}
	item.GPUType, item.GPUDevices = req.GPUType, req.GPUDevices
	item.TensorParallel, item.MaxModelLen = req.TensorParallel, req.MaxModelLen
	item.GPUMemoryUtil, item.ShmSize = req.GPUMemoryUtil, req.ShmSize
	item.Args, item.Env, item.RecipeID = req.Args, req.Env, req.RecipeID
	item.ContainerName = "1panel-vllm-" + item.Name
	item.Dir = path.Join(global.Dir.DataDir, "ai/vllm/deployments", item.Name)
	composeContent, err := buildVllmCompose(item)
	if err != nil {
		return err
	}
	item.DockerCompose = composeContent
	item.Status, item.Message = constant.StatusStarting, ""
	if err := os.MkdirAll(item.Dir, 0o755); err != nil {
		return err
	}
	if err := files.NewFileOp().SaveFile(path.Join(item.Dir, "docker-compose.yml"), composeContent, 0o644); err != nil {
		return err
	}
	if err := s.repo.SaveDeployment(item); err != nil {
		return err
	}
	operate := task.TaskCreate
	if req.ID > 0 {
		operate = task.TaskUpdate
	}
	go s.runDeploymentTask(item, req.TaskID, operate)
	return nil
}

func (s *VllmService) runDeploymentTask(item *model.VllmDeployment, taskID, operate string) {
	t, err := task.NewTaskWithOps(item.Name, operate, task.TaskScopeAI, taskID, item.ID)
	if err != nil {
		item.Status, item.Message = constant.StatusError, err.Error()
		_ = s.repo.SaveDeployment(item)
		return
	}
	composePath := path.Join(item.Dir, "docker-compose.yml")
	t.AddSubTaskWithOps(task.GetTaskName(item.Name, operate, task.TaskScopeAI), func(t *task.Task) error {
		// The proxy only covers pulling the vLLM image; the deployed
		// container itself runs without any proxy.
		proxy, err := s.loadVllmProxy()
		if err != nil {
			return err
		}
		if err := proxy.pullImage(context.Background(), t, item.Image); err != nil {
			return err
		}
		_, _ = compose.Down(composePath)
		return compose.UpWithTask(composePath, t, false)
	}, nil, 0, 2*time.Hour)
	if err := t.Execute(); err != nil {
		item.Status, item.Message = constant.StatusError, truncateVllmMsg(err.Error())
	} else {
		item.Status, item.Message = constant.StatusRunning, ""
	}
	_ = s.repo.SaveDeployment(item)
}

// buildVllmCompose renders the compose project for a local instance. All
// user values go into the YAML structure (never into a shell string), and
// engine args are passed as an argv list.
func buildVllmCompose(d *model.VllmDeployment) (string, error) {
	command := []string{
		"--model", path.Join(vllmContainerModels, "model"),
		"--served-model-name", d.ServedName,
		"--host", "0.0.0.0",
		"--port", "8000",
	}
	if d.TensorParallel > 1 {
		command = append(command, "--tensor-parallel-size", strconv.Itoa(d.TensorParallel))
	}
	if d.MaxModelLen > 0 {
		command = append(command, "--max-model-len", strconv.Itoa(d.MaxModelLen))
	}
	if d.GPUMemoryUtil != "" {
		command = append(command, "--gpu-memory-utilization", d.GPUMemoryUtil)
	}
	if d.APIKey != "" {
		command = append(command, "--api-key", d.APIKey)
	}
	extra, err := parseVllmArgs(d.Args)
	if err != nil {
		return "", err
	}
	command = append(command, extra...)
	env, err := parseVllmEnv(d.Env)
	if err != nil {
		return "", err
	}
	port := fmt.Sprintf("%d:8000", d.Port)
	if d.HostIP != "" {
		port = fmt.Sprintf("%s:%d:8000", d.HostIP, d.Port)
	}
	shm := d.ShmSize
	if shm == "" {
		shm = "16g"
	}
	service := map[string]interface{}{
		"image":          d.Image,
		"container_name": d.ContainerName,
		"restart":        "unless-stopped",
		"ipc":            "host",
		"shm_size":       shm,
		"ports":          []string{port},
		"volumes":        []string{d.ModelPath + ":" + path.Join(vllmContainerModels, "model") + ":ro"},
		"command":        command,
		"labels":         map[string]string{"createdBy": "1Panel", "1panel.vllm": d.Name},
	}
	if len(env) > 0 {
		service["environment"] = env
	}
	switch d.GPUType {
	case "", "nvidia":
		dev := map[string]interface{}{"driver": "nvidia", "capabilities": []string{"gpu"}}
		if d.GPUDevices == "" || d.GPUDevices == "all" {
			dev["count"] = "all"
		} else {
			dev["device_ids"] = strings.Split(d.GPUDevices, ",")
		}
		service["deploy"] = map[string]interface{}{"resources": map[string]interface{}{"reservations": map[string]interface{}{"devices": []interface{}{dev}}}}
	case "cpu":
		// CPU build: no device reservation.
	default:
		// Other accelerators (ascend / intel) need vendor-specific device
		// mounts; put them in a recipe's env/args or edit the compose file.
	}
	project := map[string]interface{}{"services": map[string]interface{}{d.Name: service}}
	out, err := yaml.Marshal(project)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *VllmService) OperateDeployment(req dto.VllmOperate) error {
	item, err := s.repo.GetDeployment(repo.WithByID(req.ID))
	if err != nil {
		return err
	}
	if item.External {
		return buserr.WithDetail("ErrInvalidParams", "external instances are not managed by the panel", nil)
	}
	composePath := path.Join(item.Dir, "docker-compose.yml")
	var out string
	switch req.Operate {
	case "start":
		out, err = compose.Up(composePath)
		item.Status = constant.StatusRunning
	case "stop":
		out, err = compose.Down(composePath)
		item.Status = constant.StatusStopped
	case "restart":
		out, err = compose.Restart(composePath)
		item.Status = constant.StatusRunning
	}
	item.Message = ""
	if err != nil {
		item.Status, item.Message = constant.StatusError, truncateVllmMsg(out+" "+err.Error())
	}
	_ = s.repo.SaveDeployment(item)
	return err
}

func (s *VllmService) DeleteDeployment(id uint) error {
	item, err := s.repo.GetDeployment(repo.WithByID(id))
	if err != nil {
		return err
	}
	if !item.External && item.Dir != "" {
		_, _ = compose.Down(path.Join(item.Dir, "docker-compose.yml"))
		if strings.HasPrefix(item.Dir, path.Join(global.Dir.DataDir, "ai/vllm/deployments")+"/") {
			_ = os.RemoveAll(item.Dir)
		}
	}
	return s.repo.DeleteDeployment(repo.WithByID(id))
}

// DeploymentStats syncs container state and returns usage for the list.
func (s *VllmService) DeploymentStats(ids []uint) ([]dto.VllmDeploymentStats, error) {
	var opts []repo.DBOption
	if len(ids) > 0 {
		opts = append(opts, repo.WithByIDs(ids))
	}
	items, err := s.repo.ListDeployments(opts...)
	if err != nil {
		return nil, err
	}
	res := make([]dto.VllmDeploymentStats, 0, len(items))
	var names []string
	for _, item := range items {
		if !item.External {
			names = append(names, item.ContainerName)
		}
	}
	states := map[string]string{}
	ids2 := map[string]string{}
	if len(names) > 0 {
		if cli, err := docker.NewClient(); err == nil {
			if list, err := cli.ListContainersByName(names); err == nil {
				for _, c := range list {
					for _, n := range c.Names {
						states[strings.TrimPrefix(n, "/")] = c.State
						ids2[strings.TrimPrefix(n, "/")] = c.ID
					}
				}
			}
			cli.Close()
		}
	}
	for i := range items {
		item := &items[i]
		st := dto.VllmDeploymentStats{ID: item.ID, Status: item.Status, Message: item.Message}
		if item.External {
			h, _ := checkVllmEndpoint(item)
			st.Healthy = h.Healthy
			if h.Healthy {
				st.Status = constant.StatusRunning
			} else {
				st.Status, st.Message = constant.StatusError, h.Message
			}
			res = append(res, st)
			continue
		}
		if item.Status != constant.StatusStarting {
			newStatus := constant.StatusStopped
			switch states[item.ContainerName] {
			case "running":
				newStatus = constant.StatusRunning
			case "restarting":
				newStatus = constant.StatusRestarting
			case "exited", "dead":
				newStatus = constant.StatusError
			}
			if newStatus != item.Status {
				item.Status = newStatus
				_ = s.repo.SaveDeployment(item)
			}
			st.Status = item.Status
		}
		if id := ids2[item.ContainerName]; id != "" && states[item.ContainerName] == "running" {
			if stats, err := NewIContainerService().ContainerStats(id); err == nil {
				st.CPUPercent, st.MemoryMB = stats.CPUPercent, stats.Memory
			}
			h, _ := checkVllmEndpoint(item)
			st.Healthy = h.Healthy
		}
		res = append(res, st)
	}
	return res, nil
}

func (s *VllmService) CheckDeployment(id uint) (dto.VllmHealth, error) {
	item, err := s.repo.GetDeployment(repo.WithByID(id))
	if err != nil {
		return dto.VllmHealth{}, err
	}
	return checkVllmEndpoint(item)
}

// checkVllmEndpoint calls GET {endpoint}/models (OpenAI-compatible).
func checkVllmEndpoint(d *model.VllmDeployment) (dto.VllmHealth, error) {
	endpoint := vllmEndpoint(d)
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(endpoint, "/")+"/models", nil)
	if err != nil {
		return dto.VllmHealth{Message: err.Error()}, nil
	}
	if d.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.APIKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return dto.VllmHealth{Message: truncateVllmMsg(err.Error())}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return dto.VllmHealth{Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}, nil
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &list)
	h := dto.VllmHealth{Healthy: true}
	for _, m := range list.Data {
		h.Models = append(h.Models, m.ID)
	}
	return h, nil
}

// ----------------------------------------------------------------- helpers

// parseVllmArgs splits engine args: one flag per line ("--max-num-seqs 64"
// or "--enable-prefix-caching"); blank lines and # comments are ignored.
// Only flags (starting with --) and their values are accepted.
func parseVllmArgs(raw string) ([]string, error) {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if !strings.HasPrefix(fields[0], "--") {
			return nil, buserr.WithDetail("ErrInvalidParams", "each arg line must start with --: "+line, nil)
		}
		if strings.Contains(fields[0], "=") {
			out = append(out, fields[0])
			fields = fields[1:]
		}
		out = append(out, fields...)
	}
	return out, nil
}

func parseVllmEnv(raw string) (map[string]string, error) {
	env := map[string]string{}
	keyPattern := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || !keyPattern.MatchString(k) {
			return nil, buserr.WithDetail("ErrInvalidParams", "env lines must be KEY=VALUE: "+line, nil)
		}
		env[k] = strings.TrimSpace(v)
	}
	return env, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func truncateVllmMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1000 {
		return s[len(s)-1000:]
	}
	return s
}

func dirSize(dir string) int64 {
	var size int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
