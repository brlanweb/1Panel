package model

// VllmModel is a model (or tokenizer) prepared on this node: downloaded
// from ModelScope / Hugging Face into the model directory, or registered
// from an existing local directory.
type VllmModel struct {
	BaseModel
	RepoID   string `json:"repoId"`   // e.g. Qwen/Qwen3-8B
	Source   string `json:"source"`   // modelscope | huggingface | local
	Kind     string `json:"kind"`     // model | tokenizer
	Revision string `json:"revision"` // branch / tag, "" = default
	Dir      string `json:"dir"`      // absolute path on the host
	Status   string `json:"status"`   // Downloading | Success | Failed | Canceled
	Message  string `json:"message"`
	Size     int64  `json:"size"`
	TaskID   string `json:"taskID"`
	RecipeID uint   `json:"recipeId"`
}

// VllmRecipe is a saved vLLM launch template (engine args) that can be
// reused when deploying an instance.
type VllmRecipe struct {
	BaseModel
	Name        string `json:"name"`
	GPUType     string `json:"gpuType"` // nvidia | ascend | intel | cpu
	ModelRepo   string `json:"modelRepo"`
	Image       string `json:"image"`
	Args        string `json:"args"` // extra engine args, one per line
	Env         string `json:"env"`  // KEY=VALUE per line
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
}

// VllmDeployment is a vLLM OpenAI-compatible server. Local instances run as
// a docker compose project; external instances only record an endpoint.
type VllmDeployment struct {
	BaseModel
	Name           string `json:"name"`
	External       bool   `json:"external"`
	BaseURL        string `json:"baseUrl"` // external: http://host:port/v1
	APIKey         string `json:"apiKey"`
	Image          string `json:"image"`
	ContainerName  string `json:"containerName"`
	HostIP         string `json:"hostIP"`
	Port           int    `json:"port"`
	ModelID        uint   `json:"modelId"`
	ModelPath      string `json:"modelPath"`
	ServedName     string `json:"servedName"`
	GPUType        string `json:"gpuType"`
	GPUDevices     string `json:"gpuDevices"` // "all" or comma separated indexes
	TensorParallel int    `json:"tensorParallel"`
	MaxModelLen    int    `json:"maxModelLen"`
	GPUMemoryUtil  string `json:"gpuMemoryUtil"`
	ShmSize        string `json:"shmSize"`
	Args           string `json:"args"`
	Env            string `json:"env"`
	RecipeID       uint   `json:"recipeId"`
	Dir            string `json:"dir"`
	DockerCompose  string `json:"dockerCompose"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}
