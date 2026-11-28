package service

import (
	"testing"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestComputeUsageDailyPersists(t *testing.T) {
	oldDB := global.DB
	defer func() { global.DB = oldDB }()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ComputeUsageDaily{}); err != nil {
		t.Fatal(err)
	}
	global.DB = db
	computeDaily = &computeDailyStore{pending: map[computeDailyKey]*computeAgg{}}

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	log := func(at time.Time, model string, ok bool, p, c int64) dto.ComputeCallLog {
		return dto.ComputeCallLog{Time: at.UnixMilli(), Model: model, Consumer: "app", Success: ok,
			PromptTokens: p, CompletionTokens: c}
	}
	computeDaily.add(log(yesterday, "ds", true, 100, 50))
	computeDaily.flush()
	// Two flushes on the same key must add up (upsert), not overwrite.
	computeDaily.add(log(now, "ds", true, 10, 5))
	computeDaily.flush()
	computeDaily.add(log(now, "ds", false, 1, 0))
	computeDaily.flush()
	// Not yet flushed usage is counted too.
	computeDaily.add(log(now, "other", true, 7, 3))

	var n int64
	db.Model(&model.ComputeUsageDaily{}).Count(&n)
	if n != 2 {
		t.Fatalf("expected 2 persisted rows (yesterday + today for ds), got %d", n)
	}

	all := computeUsageTotals("", "")
	if all.Requests != 4 || all.Failures != 1 || all.TotalTokens != 176 {
		t.Fatalf("all-time totals wrong: %+v", all)
	}
	if all.TodayRequests != 3 || all.TodayTokens != 26 {
		t.Fatalf("today totals wrong: %+v", all)
	}
	if all.Since != computeDay(yesterday.UnixMilli()) {
		t.Fatalf("since wrong: %s", all.Since)
	}
	ds := computeUsageTotals("ds", "")
	if ds.Requests != 3 || ds.TotalTokens != 166 {
		t.Fatalf("model filter wrong: %+v", ds)
	}
}

// Non-streamed answers arrive whole: their first byte is not a time to
// first token and must not be averaged as one.
func TestComputeTTFTStreamOnly(t *testing.T) {
	computeCallLogs = &computeCallLogStore{ring: make([]dto.ComputeCallLog, 16), minutes: map[int64]map[computeAggKey]*computeAgg{}}
	computeDaily = &computeDailyStore{pending: map[computeDailyKey]*computeAgg{}}
	now := time.Now().UnixMilli()
	computeCallLogs.add(dto.ComputeCallLog{Time: now, Model: "m", Success: true, Stream: true, TTFTMs: 400, LatencyMs: 5000})
	computeCallLogs.add(dto.ComputeCallLog{Time: now, Model: "m", Success: true, Stream: false, TTFTMs: 0, LatencyMs: 9000})
	res := (&ComputeService{}).Analytics(dto.ComputeAnalyticsReq{})
	if res.Summary.AvgTTFTMs != 400 {
		t.Fatalf("avg TTFT must only use streamed calls, got %.0f", res.Summary.AvgTTFTMs)
	}
	if res.Summary.AvgLatencyMs != 7000 {
		t.Fatalf("latency covers all calls, got %.0f", res.Summary.AvgLatencyMs)
	}
}
