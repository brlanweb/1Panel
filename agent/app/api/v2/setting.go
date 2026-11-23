package v2

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/ssh"
	"github.com/gin-gonic/gin"
)

// @Tags System Setting
// @Summary Load system setting info
// @Success 200 {object} dto.SettingInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/search [post]
func (b *BaseApi) GetSettingInfo(c *gin.Context) {
	setting, err := settingService.GetSettingInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// @Tags System Setting
// @Summary Get terminal AI setting info
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/terminal/ai/search [post]
func (b *BaseApi) GetTerminalAISettingInfo(c *gin.Context) {
	setting, err := settingService.GetTerminalAIInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// @Tags System Setting
// @Summary Load system available status
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/search/available [get]
func (b *BaseApi) GetSystemAvailable(c *gin.Context) {
	helper.Success(c)
}

// shanghaiCompositeURL is the Eastmoney quote for the Shanghai Composite
// (secid 1.000001). f43 is the last price, f60 yesterday's close, f170 the
// change in percent. The browser cannot call it directly because of CORS,
// so the panel reads it once per page load and only returns the direction.
const shanghaiCompositeURL = "https://push2.eastmoney.com/api/qt/stock/get?secid=1.000001&fields=f43,f60,f170&fltt=2"

// @Tags System Setting
// @Summary Whether the Shanghai Composite is up, read once per page load
// @Success 200 {object} map[string]bool
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/market [get]
func (b *BaseApi) GetShanghaiComposite(c *gin.Context) {
	rising := false
	client := &http.Client{Timeout: 4 * time.Second}
	req, err := http.NewRequest(http.MethodGet, shanghaiCompositeURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Referer", "https://quote.eastmoney.com/")
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			var quote struct {
				Data struct {
					Price         float64 `json:"f43"`
					PreviousClose float64 `json:"f60"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &quote) == nil && quote.Data.PreviousClose > 0 {
				rising = quote.Data.Price > quote.Data.PreviousClose
			}
		}
	}
	helper.SuccessWithData(c, gin.H{"rising": rising})
}

// @Tags System Setting
// @Summary Update system setting
// @Accept json
// @Param request body dto.AgentSettingUpdate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/update [post]
// @x-panel-log {"bodyKeys":["key","value"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"修改系统配置 [key] => [value]","formatEN":"update system setting [key] => [value]"}
func (b *BaseApi) UpdateSetting(c *gin.Context) {
	var req dto.AgentSettingUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	if err := settingService.Update(req.Key, req.Value); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags System Setting
// @Summary Update terminal AI setting
// @Accept json
// @Param request body dto.TerminalAIInfo true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/terminal/ai/update [post]
// @x-panel-log {"bodyKeys":["aiStatus","aiAccountId"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"更新终端 AI 设置 [aiStatus][aiAccountId]","formatEN":"update terminal AI setting [aiStatus][aiAccountId]"}
func (b *BaseApi) UpdateTerminalAISetting(c *gin.Context) {
	var req dto.TerminalAIInfo
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	if err := settingService.UpdateTerminalAI(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags System Setting
// @Summary Get file manage AI setting info
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/files/ai/search [post]
func (b *BaseApi) GetFileManageAISettingInfo(c *gin.Context) {
	setting, err := settingService.GetFileManageAIInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// @Tags System Setting
// @Summary Update file manage AI setting
// @Accept json
// @Param request body dto.FileManageAIInfo true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/files/ai/update [post]
// @x-panel-log {"bodyKeys":["aiStatus","aiAccountId"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"更新文件管理 AI 设置 [aiStatus][aiAccountId]","formatEN":"update file manage AI setting [aiStatus][aiAccountId]"}
func (b *BaseApi) UpdateFileManageAISetting(c *gin.Context) {
	var req dto.FileManageAIInfo
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := settingService.UpdateFileManageAI(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags System Setting
// @Summary Load file history setting info
// @Success 200 {object} response.FileHistorySettingInfo
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/file-history/search [post]
func (b *BaseApi) GetFileHistorySettingInfo(c *gin.Context) {
	setting, err := settingService.GetFileHistorySettingInfo()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, setting)
}

// @Tags System Setting
// @Summary Update file history setting
// @Accept json
// @Param request body request.FileHistorySettingUpdate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/file-history/update [post]
func (b *BaseApi) UpdateFileHistorySetting(c *gin.Context) {
	var req request.FileHistorySettingUpdate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := settingService.UpdateFileHistorySetting(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags System Setting
// @Summary Load website dir
// @Success 200 {string} path
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/website/dir [get]
func (b *BaseApi) LoadWebsiteDir(c *gin.Context) {
	helper.SuccessWithData(c, settingService.GetWebsiteDir())
}

// @Tags System Setting
// @Summary Load local backup dir
// @Success 200 {string} path
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/basedir [get]
func (b *BaseApi) LoadBaseDir(c *gin.Context) {
	helper.SuccessWithData(c, global.Dir.DataDir)
}

// @Tags System Setting
// @Summary Load local conn
// @Success 200 {object} dto.SSHConnData
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/ssh/conn [get]
func (b *BaseApi) LoadLocalConn(c *gin.Context) {
	helper.SuccessWithData(c, settingService.GetLocalConn())
}

// @Tags System Setting
// @Summary Check local conn
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/ssh/check [post]
func (b *BaseApi) CheckLocalConn(c *gin.Context) {
	client, err := loadLocalConn()
	if err == nil && client != nil {
		client.Close()
	}
	helper.SuccessWithData(c, err == nil)
}

// @Tags System Setting
// @Summary Update local is conn
// @Accept json
// @Param request body dto.SSHDefaultConn true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/ssh/default [post]
// @x-panel-log {"bodyKeys":["defaultConn"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"本地终端默认连接 [defaultConn]","formatEN":"update system default conn [defaultConn]"}
func (b *BaseApi) SetDefaultIsConn(c *gin.Context) {
	var req dto.SSHDefaultConn
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	if err := settingService.SetDefaultIsConn(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags System Setting
// @Summary Check local conn info
// @Success 200 {boolean} isOk
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/ssh/check/info [post]
func (b *BaseApi) CheckLocalConnByInfo(c *gin.Context) {
	var req dto.SSHConnData
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	helper.SuccessWithData(c, settingService.TestConnByInfo(req))
}

// @Tags System Setting
// @Summary Save local conn info
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/ssh [post]
func (b *BaseApi) SaveLocalConn(c *gin.Context) {
	var req dto.SSHConnData
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	if err := settingService.SaveConnInfo(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

func loadLocalConn() (*ssh.SSHClient, error) {
	connInDB, err := settingService.GetLocalConnForSSH()
	if err != nil {
		return nil, err
	}
	sshInfo := ssh.ConnInfo{
		Addr:       connInDB.Addr,
		Port:       int(connInDB.Port),
		User:       connInDB.User,
		AuthMode:   connInDB.AuthMode,
		Password:   connInDB.Password,
		PrivateKey: []byte(connInDB.PrivateKey),
		PassPhrase: []byte(connInDB.PassPhrase),
	}
	return ssh.NewClient(sshInfo)
}

// @Tags System Setting
// @Summary Save common description
// @Accept json
// @Param request body dto.CommonDescription true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /settings/description/save [post]
func (b *BaseApi) SaveDescription(c *gin.Context) {
	var req dto.CommonDescription
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}

	if err := settingService.SaveDescription(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}
