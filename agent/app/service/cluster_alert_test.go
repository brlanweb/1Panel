package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newClusterStateDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alert.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AlertTargetState{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func withSendWindow(t *testing.T, allowed bool) {
	t.Helper()
	prev := clusterSendAllowed
	clusterSendAllowed = func(string) bool { return allowed }
	t.Cleanup(func() { clusterSendAllowed = prev })
}

func clusterRule(count, sendCount uint, cooldownHours int) dto.AlertDTO {
	return dto.AlertDTO{
		ID: 1, Type: dto.ClusterAlertSite, Cycle: 1, Count: count, SendCount: sendCount,
		AdvancedParams: fmt.Sprintf(`{"cooldownHours":%d}`, cooldownHours),
	}
}

func fail(key string) []dto.ClusterAlertTargetResult {
	return []dto.ClusterAlertTargetResult{{Key: key, Label: key, Failed: true, Error: "down"}}
}

func ok(key string) []dto.ClusterAlertTargetResult {
	return []dto.ClusterAlertTargetResult{{Key: key, Label: key}}
}

func TestClusterStateThresholdAndCooldown(t *testing.T) {
	withSendWindow(t, true)
	db := newClusterStateDB(t)
	rule := clusterRule(3, 2, 24)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	step := func(i int) time.Time { return now.Add(time.Duration(i) * time.Minute) }

	// Two failures: below the threshold, no alert.
	for i := 0; i < 2; i++ {
		d, err := applyClusterCheck(db, rule, fail("a"), step(i))
		if err != nil || len(d.alerts) != 0 {
			t.Fatalf("check %d: unexpected alert %v %v", i, d.alerts, err)
		}
	}
	// Third failure: first alert.
	if d, _ := applyClusterCheck(db, rule, fail("a"), step(2)); len(d.alerts) != 1 {
		t.Fatalf("expected first alert, got %v", d.alerts)
	}
	// Fourth: second (last) alert, cooldown starts.
	if d, _ := applyClusterCheck(db, rule, fail("a"), step(3)); len(d.alerts) != 1 {
		t.Fatalf("expected second alert, got %v", d.alerts)
	}
	// In cooldown: silent.
	if d, _ := applyClusterCheck(db, rule, fail("a"), step(30)); len(d.alerts) != 0 {
		t.Fatalf("expected silence during cooldown, got %v", d.alerts)
	}
	// After 24h cooldown: a new round starts.
	if d, _ := applyClusterCheck(db, rule, fail("a"), step(3+24*60+1)); len(d.alerts) != 1 {
		t.Fatalf("expected alert after cooldown, got %v", d.alerts)
	}
	// Recovery: one recovery notice and full reset.
	d, _ := applyClusterCheck(db, rule, ok("a"), step(3+24*60+2))
	if len(d.recoveries) != 1 {
		t.Fatalf("expected recovery, got %v", d.recoveries)
	}
	var state model.AlertTargetState
	db.Where("alert_id = ? AND target_key = ?", 1, "a").First(&state)
	if state.Status != clusterTargetOK || state.SentCount != 0 || state.CooldownUntil != 0 || state.ConsecutiveFail != 0 {
		t.Fatalf("state not reset: %+v", state)
	}
	// A second OK does not repeat the recovery notice.
	if d, _ := applyClusterCheck(db, rule, ok("a"), step(3+24*60+3)); len(d.recoveries) != 0 {
		t.Fatalf("unexpected second recovery")
	}
}

func TestClusterStatePerTargetIsolation(t *testing.T) {
	withSendWindow(t, true)
	db := newClusterStateDB(t)
	rule := clusterRule(1, 1, 24)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	if d, _ := applyClusterCheck(db, rule, fail("a"), now); len(d.alerts) != 1 {
		t.Fatal("a should alert")
	}
	// Target a is in cooldown; b must still alert.
	targets := []dto.ClusterAlertTargetResult{
		{Key: "a", Failed: true}, {Key: "b", Failed: true},
	}
	d, _ := applyClusterCheck(db, rule, targets, now.Add(time.Minute))
	if len(d.alerts) != 1 || d.alerts[0].label != "b" {
		t.Fatalf("expected only b to alert, got %+v", d.alerts)
	}
}

func TestClusterStateRecoveryWithoutAlertIsSilent(t *testing.T) {
	withSendWindow(t, true)
	db := newClusterStateDB(t)
	rule := clusterRule(3, 1, 24)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	applyClusterCheck(db, rule, fail("a"), now)
	d, _ := applyClusterCheck(db, rule, ok("a"), now.Add(time.Minute))
	if len(d.recoveries) != 0 {
		t.Fatal("recovery must not be sent when no alert went out")
	}
}

func TestClusterStateOutsideSendWindow(t *testing.T) {
	withSendWindow(t, false)
	db := newClusterStateDB(t)
	rule := clusterRule(1, 2, 24)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	if d, _ := applyClusterCheck(db, rule, fail("a"), now); len(d.alerts) != 0 {
		t.Fatal("must not alert outside the send window")
	}
	var state model.AlertTargetState
	db.First(&state)
	if state.SentCount != 0 || state.Status != clusterTargetFail {
		t.Fatalf("budget must not be consumed outside the window: %+v", state)
	}
}

func TestParseExpectStatus(t *testing.T) {
	ranges, err := parseExpectStatus("200,3xx, 401-403")
	if err != nil {
		t.Fatal(err)
	}
	for code, want := range map[int]bool{200: true, 302: true, 402: true, 404: false, 500: false} {
		if statusAccepted(code, ranges) != want {
			t.Fatalf("status %d: want %v", code, want)
		}
	}
	if _, err := parseExpectStatus("abc"); err == nil {
		t.Fatal("expected error")
	}
	def, _ := parseExpectStatus("")
	if !statusAccepted(301, def) || statusAccepted(404, def) {
		t.Fatal("default range must be 200-399")
	}
}

func TestValidateModelResponse(t *testing.T) {
	cases := []struct {
		provider string
		body     string
		strict   bool
		ok       bool
	}{
		{"openai", `{"choices":[{"message":{"content":"pong"}}]}`, true, true},
		{"openai", `{"choices":[]}`, false, false},
		{"openai", `{"choices":[{"message":{"content":""}}]}`, true, false},
		{"anthropic", `{"type":"message","content":[{"type":"text","text":"hi"}]}`, true, true},
		{"anthropic", `{"type":"error"}`, false, false},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`, true, true},
		{"gemini", `{"candidates":[]}`, false, false},
	}
	for _, c := range cases {
		err := validateModelResponse(dto.ClusterAlertParams{Provider: c.provider, StrictContent: c.strict}, []byte(c.body))
		if (err == nil) != c.ok {
			t.Fatalf("%s %s: got %v", c.provider, c.body, err)
		}
	}
}
