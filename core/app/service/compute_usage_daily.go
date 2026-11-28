package service

import (
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/global"
	"gorm.io/gorm/clause"
)

// Persisted usage of the model router.
//
// Call logs and the per-minute aggregates stay in memory (see
// compute_calllog.go). Calls and tokens are also summed per day, model and
// caller and flushed to compute_usage_dailies with the consumer usage
// (every computeUsageFlush), so the totals keep growing across days and
// restarts. At most one row per day × model × caller: the table stays tiny.

type computeDailyKey struct {
	day, model, consumer string
}

type computeDailyStore struct {
	mu      sync.Mutex
	pending map[computeDailyKey]*computeAgg
}

var computeDaily = &computeDailyStore{pending: map[computeDailyKey]*computeAgg{}}

func computeDay(ms int64) string {
	return time.UnixMilli(ms).Format("2006-01-02")
}

func (s *computeDailyStore) add(l dto.ComputeCallLog) {
	k := computeDailyKey{computeDay(l.Time), l.Model, l.Consumer}
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.pending[k]
	if a == nil {
		a = &computeAgg{}
		s.pending[k] = a
	}
	a.req++
	if !l.Success {
		a.fail++
	}
	a.prompt += l.PromptTokens
	a.completion += l.CompletionTokens
}

// snapshot copies the not yet flushed counters (for reads).
func (s *computeDailyStore) snapshot() map[computeDailyKey]computeAgg {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make(map[computeDailyKey]computeAgg, len(s.pending))
	for k, a := range s.pending {
		res[k] = *a
	}
	return res
}

// flush upserts the pending counters; on a database error they are put
// back so nothing is lost, only delayed.
func (s *computeDailyStore) flush() {
	s.mu.Lock()
	batch := s.pending
	s.pending = map[computeDailyKey]*computeAgg{}
	s.mu.Unlock()
	if len(batch) == 0 || global.DB == nil {
		s.restore(batch)
		return
	}
	failed := map[computeDailyKey]*computeAgg{}
	for k, a := range batch {
		row := model.ComputeUsageDaily{Day: k.day, Model: k.model, Consumer: k.consumer,
			Requests: a.req, Failures: a.fail, PromptTokens: a.prompt, CompletionTokens: a.completion}
		err := global.DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "day"}, {Name: "model"}, {Name: "consumer"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"requests":          computeExpr("requests + ?", a.req),
				"failures":          computeExpr("failures + ?", a.fail),
				"prompt_tokens":     computeExpr("prompt_tokens + ?", a.prompt),
				"completion_tokens": computeExpr("completion_tokens + ?", a.completion),
			}),
		}).Create(&row).Error
		if err != nil {
			global.LOG.Errorf("compute usage daily flush %s/%s/%s: %v", k.day, k.model, k.consumer, err)
			failed[k] = a
		}
	}
	s.restore(failed)
}

func (s *computeDailyStore) restore(batch map[computeDailyKey]*computeAgg) {
	if len(batch) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, a := range batch {
		if cur := s.pending[k]; cur != nil {
			computeAggAdd(cur, a)
		} else {
			s.pending[k] = a
		}
	}
}

// computeUsageTotals sums the persisted plus pending usage, optionally
// filtered by model / caller: the all-time totals and those of today.
func computeUsageTotals(modelName, consumer string) dto.ComputeUsageTotals {
	var res dto.ComputeUsageTotals
	today := computeDay(time.Now().UnixMilli())
	add := func(day string, req, fail, prompt, completion int64) {
		res.Requests += req
		res.Failures += fail
		res.PromptTokens += prompt
		res.CompletionTokens += completion
		if day == today {
			res.TodayRequests += req
			res.TodayTokens += prompt + completion
		}
		if res.Since == "" || day < res.Since {
			res.Since = day
		}
	}
	if global.DB != nil {
		var rows []model.ComputeUsageDaily
		q := global.DB.Model(&model.ComputeUsageDaily{})
		if modelName != "" {
			q = q.Where("model = ?", modelName)
		}
		if consumer != "" {
			q = q.Where("consumer = ?", consumer)
		}
		if err := q.Find(&rows).Error; err == nil {
			for _, r := range rows {
				add(r.Day, r.Requests, r.Failures, r.PromptTokens, r.CompletionTokens)
			}
		}
	}
	for k, a := range computeDaily.snapshot() {
		if (modelName != "" && k.model != modelName) || (consumer != "" && k.consumer != consumer) {
			continue
		}
		add(k.day, a.req, a.fail, a.prompt, a.completion)
	}
	res.TotalTokens = res.PromptTokens + res.CompletionTokens
	return res
}
