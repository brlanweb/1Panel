package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func alertTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alert.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AlertConfig{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldMainDB, oldLog, oldI18n := global.AlertDB, global.DB, global.LOG, global.I18n
	global.AlertDB = db
	global.DB = nil
	global.LOG = logrus.New()
	global.LOG.SetOutput(io.Discard)
	i18n.Init()
	t.Cleanup(func() { global.AlertDB, global.DB, global.LOG, global.I18n = oldDB, oldMainDB, oldLog, oldI18n })
}

// Mirrors exactly what the drawer sends for Telegram (save + test).
func TestTelegramAlertConfigSaveAndTest(t *testing.T) {
	alertTestDB(t)
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	cfg, _ := json.Marshal(map[string]string{
		"displayName": "tg",
		"botToken":    "123456789:AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-ab",
		"chatId":      "-1001234567890",
		"apiBase":     srv.URL,
	})
	svc := NewIAlertService()
	ok, err := svc.TestAlertConfig(dto.AlertConfigTest{Type: constant.Telegram, Config: string(cfg)})
	if err != nil || !ok {
		t.Fatalf("test failed: ok=%v err=%v", ok, err)
	}
	if got["chat_id"] != "-1001234567890" {
		t.Fatalf("payload = %v", got)
	}
	if err := svc.UpdateAlertConfig(dto.AlertConfigUpdate{
		Type: constant.Telegram, Title: "xpack.alert.telegram", Status: constant.AlertEnable,
		Config: string(cfg), DisplayName: "tg",
	}, "admin"); err != nil {
		t.Fatalf("save failed: %v", err)
	}
}
