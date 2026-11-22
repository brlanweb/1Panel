package v2

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/i18n"
	"github.com/1Panel-dev/1Panel/agent/init/validator"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercises the HTTP handlers with the exact JSON the alert drawer posts
// for a Telegram channel.
func TestTelegramAlertConfigHandlers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alert.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AlertConfig{}); err != nil {
		t.Fatal(err)
	}
	oldAlert, oldDB, oldLog, oldValid, oldI18n := global.AlertDB, global.DB, global.LOG, global.VALID, global.I18n
	global.AlertDB, global.DB = db, nil
	global.LOG = logrus.New()
	global.LOG.SetOutput(io.Discard)
	validator.Init()
	i18n.Init()
	t.Cleanup(func() {
		global.AlertDB, global.DB, global.LOG, global.VALID, global.I18n = oldAlert, oldDB, oldLog, oldValid, oldI18n
	})

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer tg.Close()
	cfg, _ := json.Marshal(map[string]string{
		"displayName": "tg", "botToken": "123456789:AAEhBOweik6ad9r_QXMENQjcrGbqCr4K-ab",
		"chatId": "-1001234567890", "apiBase": tg.URL,
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := BaseApi{}
	r.POST("/test", api.TestAlertConfig)
	r.POST("/update", api.UpdateAlertConfig)
	post := func(path string, body any) map[string]any {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw))))
		var res map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		return res
	}
	if res := post("/test", map[string]any{"type": "telegram", "config": string(cfg)}); res["code"] != float64(200) || res["data"] != true {
		t.Fatalf("test response: %v", res)
	}
	if res := post("/update", map[string]any{
		"id": 0, "type": "telegram", "title": "xpack.alert.telegram", "status": "Enable",
		"config": string(cfg), "displayName": "tg",
	}); res["code"] != float64(200) {
		t.Fatalf("update response: %v", res)
	}
}
