package alert

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/utils/bark"
	"github.com/1Panel-dev/1Panel/agent/utils/telegram"
	"github.com/1Panel-dev/1Panel/agent/utils/webhook_sender"
)

// Direct channels are delivered by the panel itself (no paid provider):
// Bark, WeCom group robot and Telegram Bot.

// TelegramConfig is the stored config of a Telegram Bot channel.
type TelegramConfig struct {
	DisplayName string `json:"displayName"`
	BotToken    string `json:"botToken"`
	ChatID      string `json:"chatId"`
	APIBase     string `json:"apiBase"`
}

// IsDirectChannel reports whether a config type is sent by this package.
func IsDirectChannel(configType string) bool {
	switch configType {
	case constant.Bark, constant.WeCom, constant.Telegram:
		return true
	default:
		return false
	}
}

// CreateDirectAlertLog sends one alert through a direct channel and stores
// the delivery log.
func CreateDirectAlertLog(create dto.AlertLogCreate, alert dto.AlertDTO, params []dto.Param, transport *http.Transport, agentInfo *dto.AgentInfo, config model.AlertConfig) error {
	if config.Type == constant.Bark {
		return CreateBarkAlertLog(create, alert, params, transport, agentInfo, config)
	}
	var alertLog model.AlertLog
	content := GetAlertLogContent(create, alert, params, agentInfo)
	if content == "" {
		content = i18n.GetMsgWithMap("CommonAlert", map[string]interface{}{"msg": alert.Title})
	}
	if err := SendDirectMessage(config, i18n.GetMsgByKey("PanelAlertTitle"), content, transport); err != nil {
		create.Message = err.Error()
		create.Status = constant.AlertError
		return SaveAlertLog(create, &alertLog)
	}
	create.Status = constant.AlertSuccess
	return SaveAlertLog(create, &alertLog)
}

// SendDirectMessage delivers title/body through the channel described by
// config (also used by the "test" button).
func SendDirectMessage(config model.AlertConfig, title, body string, transport *http.Transport) error {
	body = webhook_sender.NormalizeToText(body)
	switch config.Type {
	case constant.Bark:
		var cfg dto.AlertWebhookConfig
		if err := json.Unmarshal([]byte(config.Config), &cfg); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Url) == "" {
			return fmt.Errorf("bark config url is required")
		}
		return bark.SendMessage(cfg.Url, title, body, transport)
	case constant.WeCom:
		var cfg dto.AlertWebhookConfig
		if err := json.Unmarshal([]byte(config.Config), &cfg); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Url) == "" {
			return fmt.Errorf("WeCom webhook url is required")
		}
		payload, err := webhook_sender.BuildWebhookPayload(constant.WeCom, title, body)
		if err != nil {
			return err
		}
		return webhook_sender.SendWebhookRequest(constant.WeCom, strings.TrimSpace(cfg.Url), payload, transport)
	case constant.Telegram:
		var cfg TelegramConfig
		if err := json.Unmarshal([]byte(config.Config), &cfg); err != nil {
			return err
		}
		return telegram.SendMessage(cfg.APIBase, cfg.BotToken, cfg.ChatID, title, body, transport)
	default:
		return fmt.Errorf("unsupported direct alert channel: %s", config.Type)
	}
}
