package v2

import (
	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags AI
// @Summary Load vLLM settings
// @Success 200 {object} dto.VllmSettings
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/settings [get]
func (b *BaseApi) LoadVllmSettings(c *gin.Context) {
	helper.SuccessWithData(c, vllmService.LoadSettings())
}

// @Tags AI
// @Summary Update vLLM settings
// @Accept json
// @Param request body dto.VllmSettings true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/settings/update [post]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"修改 vLLM 设置","formatEN":"update vLLM settings"}
func (b *BaseApi) UpdateVllmSettings(c *gin.Context) {
	var req dto.VllmSettings
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.SaveSettings(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Page vLLM models
// @Accept json
// @Param request body dto.VllmSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/search [post]
func (b *BaseApi) PageVllmModels(c *gin.Context) {
	var req dto.VllmSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, items, err := vllmService.PageModels(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{Total: total, Items: items})
}

// @Tags AI
// @Summary Download a model from ModelScope / Hugging Face
// @Accept json
// @Param request body dto.VllmModelDownload true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/download [post]
// @x-panel-log {"bodyKeys":["repoId","source"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"下载模型 [repoId] ([source])","formatEN":"download model [repoId] ([source])"}
func (b *BaseApi) DownloadVllmModel(c *gin.Context) {
	var req dto.VllmModelDownload
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.DownloadModel(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Register an existing local model directory
// @Accept json
// @Param request body dto.VllmModelImport true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/import [post]
// @x-panel-log {"bodyKeys":["repoId","dir"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"导入本地模型 [repoId] [dir]","formatEN":"import local model [repoId] [dir]"}
func (b *BaseApi) ImportVllmModel(c *gin.Context) {
	var req dto.VllmModelImport
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.ImportModel(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Cancel a model download
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/cancel [post]
func (b *BaseApi) CancelVllmModelDownload(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.CancelModelDownload(req.ID); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Retry / resume a model download
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/retry [post]
func (b *BaseApi) RetryVllmModelDownload(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.RetryModelDownload(req.ID); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Delete a vLLM model record (optionally its files)
// @Accept json
// @Param request body dto.VllmModelDelete true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/models/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"删除模型 [id]","formatEN":"delete model [id]"}
func (b *BaseApi) DeleteVllmModel(c *gin.Context) {
	var req dto.VllmModelDelete
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.DeleteModel(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Page vLLM recipes
// @Accept json
// @Param request body dto.VllmSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/recipes/search [post]
func (b *BaseApi) PageVllmRecipes(c *gin.Context) {
	var req dto.VllmSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, items, err := vllmService.PageRecipes(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{Total: total, Items: items})
}

// @Tags AI
// @Summary Create or update a vLLM recipe
// @Accept json
// @Param request body dto.VllmRecipeSave true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/recipes [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存 vLLM Recipe [name]","formatEN":"save vLLM recipe [name]"}
func (b *BaseApi) SaveVllmRecipe(c *gin.Context) {
	var req dto.VllmRecipeSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.SaveRecipe(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Delete a vLLM recipe
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/recipes/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"删除 vLLM Recipe [id]","formatEN":"delete vLLM recipe [id]"}
func (b *BaseApi) DeleteVllmRecipe(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.DeleteRecipe(req.ID); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Page vLLM deployments
// @Accept json
// @Param request body dto.VllmSearch true "request"
// @Success 200 {object} dto.PageResult
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments/search [post]
func (b *BaseApi) PageVllmDeployments(c *gin.Context) {
	var req dto.VllmSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, items, err := vllmService.PageDeployments(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{Total: total, Items: items})
}

// @Tags AI
// @Summary Create or update a vLLM deployment (local or external)
// @Accept json
// @Param request body dto.VllmDeploymentSave true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments [post]
// @x-panel-log {"bodyKeys":["name"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"保存 vLLM 实例 [name]","formatEN":"save vLLM instance [name]"}
func (b *BaseApi) SaveVllmDeployment(c *gin.Context) {
	var req dto.VllmDeploymentSave
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.SaveDeployment(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Start / stop / restart a vLLM deployment
// @Accept json
// @Param request body dto.VllmOperate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments/operate [post]
// @x-panel-log {"bodyKeys":["id","operate"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"vLLM 实例 [id] 执行 [operate]","formatEN":"vLLM instance [id] [operate]"}
func (b *BaseApi) OperateVllmDeployment(c *gin.Context) {
	var req dto.VllmOperate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.OperateDeployment(req); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Delete a vLLM deployment
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments/del [post]
// @x-panel-log {"bodyKeys":["id"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"删除 vLLM 实例 [id]","formatEN":"delete vLLM instance [id]"}
func (b *BaseApi) DeleteVllmDeployment(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := vllmService.DeleteDeployment(req.ID); err != nil {
		helper.BadRequest(c, err)
		return
	}
	helper.Success(c)
}

// @Tags AI
// @Summary Sync status and load usage of vLLM deployments
// @Accept json
// @Param request body dto.OperateByIDs true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments/stats [post]
func (b *BaseApi) LoadVllmDeploymentStats(c *gin.Context) {
	var req dto.OperateByIDs
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	list, err := vllmService.DeploymentStats(req.IDs)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, list)
}

// @Tags AI
// @Summary Check a vLLM endpoint (GET /v1/models)
// @Accept json
// @Param request body dto.OperateByID true "request"
// @Success 200 {object} dto.VllmHealth
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /ai/vllm/deployments/check [post]
func (b *BaseApi) CheckVllmDeployment(c *gin.Context) {
	var req dto.OperateByID
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := vllmService.CheckDeployment(req.ID)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}
