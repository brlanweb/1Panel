package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const DefaultAPIBase = "https://api.telegram.org"

var botTokenPattern = regexp.MustCompile(`^[0-9]{3,20}:[A-Za-z0-9_-]{20,100}$`)

// ValidBotToken reports whether the token looks like "<bot id>:<secret>".
func ValidBotToken(token string) bool {
	return botTokenPattern.MatchString(strings.TrimSpace(token))
}

// NormalizeAPIBase validates an optional custom Bot API base URL (a reverse
// proxy or a self-hosted telegram-bot-api server) and falls back to the
// official endpoint.
func NormalizeAPIBase(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return DefaultAPIBase, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("invalid Telegram API address")
	}
	return raw, nil
}

// SendMessage posts a plain-text message through the Bot API sendMessage
// method. The bot token is never included in returned errors.
func SendMessage(apiBase, botToken, chatID, title, body string, transport *http.Transport) error {
	botToken = strings.TrimSpace(botToken)
	chatID = strings.TrimSpace(chatID)
	if !ValidBotToken(botToken) {
		return fmt.Errorf("invalid Telegram bot token")
	}
	if chatID == "" {
		return fmt.Errorf("Telegram chat id is required")
	}
	base, err := NormalizeAPIBase(apiBase)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(title + "\n" + body)
	if len([]rune(text)) > 4000 {
		text = string([]rune(text)[:4000])
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	client := &http.Client{Timeout: 15 * time.Second}
	if transport != nil {
		cloned := transport.Clone()
		if cloned.Proxy == nil {
			// Honour HTTPS_PROXY / ALL_PROXY: api.telegram.org is often only
			// reachable through a proxy.
			cloned.Proxy = http.ProxyFromEnvironment
		}
		client.Transport = cloned
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+botToken+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create Telegram request failed")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		// The URL contains the token; never surface it.
		return fmt.Errorf("Telegram request failed: %s", strings.ReplaceAll(err.Error(), botToken, "***"))
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(raw, &result)
	if resp.StatusCode != http.StatusOK || !result.OK {
		if result.Description != "" {
			return fmt.Errorf("Telegram rejected the message: %s", result.Description)
		}
		return fmt.Errorf("Telegram response status code: %d", resp.StatusCode)
	}
	return nil
}
