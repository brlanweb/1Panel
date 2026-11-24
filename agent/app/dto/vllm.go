package dto

import "github.com/1Panel-dev/1Panel/agent/app/model"

type VllmSettings struct {
	ModelDir        string `json:"modelDir"`
	TokenizerDir    string `json:"tokenizerDir"`
	HFEndpoint      string `json:"hfEndpoint"`
	HFToken         string `json:"hfToken"`
	ModelScopeURL   string `json:"modelScopeUrl"`
	ModelScopeToken string `json:"modelScopeToken"`
	// ProxyMode: "" / none = direct; "node" = the proxy node configured in
	// Settings → Proxy node; "custom" = ProxyURL.
	ProxyMode string `json:"proxyMode"`
	// The proxy only applies to model downloads and vLLM image pulls; it is
	// never applied globally or to running deployments.
	ProxyURL string `json:"proxyUrl"`
}

type VllmSearch struct {
	PageInfo
	Info string `json:"info"`
	Kind string `json:"kind"`
}

type VllmModelDownload struct {
	RepoID   string `json:"repoId" validate:"required"`
	Source   string `json:"source" validate:"required,oneof=modelscope huggingface"`
	Kind     string `json:"kind" validate:"required,oneof=model tokenizer"`
	Revision string `json:"revision"`
	RecipeID uint   `json:"recipeId"`
	TaskID   string `json:"taskID"`
	// UseProxy overrides the default proxy choice of the vLLM settings for
	// this download: true = through the proxy (subscription node unless a
	// custom proxy is configured), false = direct, nil = settings default.
	UseProxy *bool `json:"useProxy"`
}

type VllmModelImport struct {
	RepoID string `json:"repoId" validate:"required"`
	Kind   string `json:"kind" validate:"required,oneof=model tokenizer"`
	Dir    string `json:"dir" validate:"required"`
	// Source / Revision are set when a cluster master copies a downloaded
	// model to this node, so the record keeps its origin (and "resume"
	// keeps working). Empty Source = a local directory registered by hand.
	Source   string `json:"source" validate:"omitempty,oneof=local modelscope huggingface"`
	Revision string `json:"revision"`
}

type VllmModelDelete struct {
	ID         uint `json:"id" validate:"required"`
	DeleteFile bool `json:"deleteFile"`
}

type VllmRecipeSave struct {
	ID          uint   `json:"id"`
	Name        string `json:"name" validate:"required"`
	GPUType     string `json:"gpuType" validate:"required"`
	ModelRepo   string `json:"modelRepo"`
	Image       string `json:"image"`
	Args        string `json:"args"`
	Env         string `json:"env"`
	Description string `json:"description"`
}

type VllmDeploymentSave struct {
	ID             uint   `json:"id"`
	Name           string `json:"name" validate:"required"`
	External       bool   `json:"external"`
	BaseURL        string `json:"baseUrl"`
	APIKey         string `json:"apiKey"`
	Image          string `json:"image"`
	HostIP         string `json:"hostIP"`
	Port           int    `json:"port"`
	ModelID        uint   `json:"modelId"`
	ServedName     string `json:"servedName"`
	GPUType        string `json:"gpuType"`
	GPUDevices     string `json:"gpuDevices"`
	TensorParallel int    `json:"tensorParallel"`
	MaxModelLen    int    `json:"maxModelLen"`
	GPUMemoryUtil  string `json:"gpuMemoryUtil"`
	ShmSize        string `json:"shmSize"`
	Args           string `json:"args"`
	Env            string `json:"env"`
	RecipeID       uint   `json:"recipeId"`
	TaskID         string `json:"taskID"`
}

type VllmOperate struct {
	ID      uint   `json:"id" validate:"required"`
	Operate string `json:"operate" validate:"required,oneof=start stop restart"`
}

type VllmDeploymentInfo struct {
	model.VllmDeployment
	ModelName string `json:"modelName"`
	Endpoint  string `json:"endpoint"`
}

type VllmDeploymentStats struct {
	ID         uint    `json:"id"`
	Status     string  `json:"status"`
	Message    string  `json:"message"`
	Healthy    bool    `json:"healthy"`
	CPUPercent float64 `json:"cpuPercent"`
	MemoryMB   float64 `json:"memoryMB"`
}

type VllmHealth struct {
	Healthy bool     `json:"healthy"`
	Models  []string `json:"models"`
	Message string   `json:"message"`
}
