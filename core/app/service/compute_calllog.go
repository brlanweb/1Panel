package service

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
)

// Call logs of the model router — memory only.
//
// Every request through the router is recorded twice:
//   - as a detail entry in a bounded ring (newest computeLogCap entries),
//     for the call log list;
//   - into per-minute aggregates keyed by model and caller, for the
//     analytics page; aggregates stay exact even when the ring wrapped.
// Nothing is written to the database. A janitor drops everything older
// than the retention window (router setting, default 24 h, max 72 h).

const (
	computeLogCap          = 100000
	computeLogDefaultHours = 24
	computeLogMaxHours     = 72
	computeLogJanitor      = 10 * time.Minute
)

type computeAggKey struct {
	model, consumer string
}

type computeAgg struct {
	req, fail, prompt, completion int64
	ttftSum, ttftN                int64
	latSum                        int64
}

type computeCallLogStore struct {
	mu      sync.RWMutex
	ring    []dto.ComputeCallLog
	head    int // next write position
	size    int
	nextID  uint64
	minutes map[int64]map[computeAggKey]*computeAgg // unix minute -> aggregates
}

var computeCallLogs = &computeCallLogStore{ring: make([]dto.ComputeCallLog, computeLogCap), minutes: map[int64]map[computeAggKey]*computeAgg{}}

// computeLogHours is the retention window, loaded with the router
// settings (computeRouterApply) so reading it never hits the database.
var computeLogHours atomic.Int32

func computeSetLogRetention(h int) {
	if h <= 0 {
		h = computeLogDefaultHours
	}
	computeLogHours.Store(int32(min(h, computeLogMaxHours)))
}

func computeLogRetention() time.Duration {
	h := int(computeLogHours.Load())
	if h <= 0 {
		h = computeLogDefaultHours
	}
	return time.Duration(h) * time.Hour
}

func (s *computeCallLogStore) add(l dto.ComputeCallLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	l.ID = s.nextID
	s.ring[s.head] = l
	s.head = (s.head + 1) % len(s.ring)
	if s.size < len(s.ring) {
		s.size++
	}
	minute := l.Time / 60000
	m := s.minutes[minute]
	if m == nil {
		m = map[computeAggKey]*computeAgg{}
		s.minutes[minute] = m
	}
	k := computeAggKey{l.Model, l.Consumer}
	a := m[k]
	if a == nil {
		a = &computeAgg{}
		m[k] = a
	}
	a.req++
	if !l.Success {
		a.fail++
	}
	a.prompt += l.PromptTokens
	a.completion += l.CompletionTokens
	if l.TTFTMs > 0 {
		a.ttftSum += l.TTFTMs
		a.ttftN++
	}
	a.latSum += l.LatencyMs
}

// prune drops entries older than the retention window.
func (s *computeCallLogStore) prune(now time.Time) {
	cut := now.Add(-computeLogRetention()).UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	for m := range s.minutes {
		if (m+1)*60000 <= cut {
			delete(s.minutes, m)
		}
	}
	// The ring is ordered by time: drop from the oldest end.
	for s.size > 0 {
		oldest := (s.head - s.size + len(s.ring)) % len(s.ring)
		if s.ring[oldest].Time >= cut {
			break
		}
		s.ring[oldest] = dto.ComputeCallLog{}
		s.size--
	}
}

func computeCallLogJanitor() {
	ticker := time.NewTicker(computeLogJanitor)
	defer ticker.Stop()
	for now := range ticker.C {
		computeCallLogs.prune(now)
	}
}

// each walks the ring from newest to oldest until fn returns false.
func (s *computeCallLogStore) each(fn func(l *dto.ComputeCallLog) bool) {
	for i := 0; i < s.size; i++ {
		idx := (s.head - 1 - i + 2*len(s.ring)) % len(s.ring)
		if !fn(&s.ring[idx]) {
			return
		}
	}
}

func (u *ComputeService) PageCallLogs(req dto.ComputeCallLogSearch) dto.ComputeCallLogPage {
	s := computeCallLogs
	s.mu.RLock()
	defer s.mu.RUnlock()
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 500 {
		req.PageSize = 20
	}
	from := (req.Page - 1) * req.PageSize
	res := dto.ComputeCallLogPage{Items: []dto.ComputeCallLog{}}
	model := strings.TrimSpace(req.Model)
	consumer := strings.TrimSpace(req.Consumer)
	if cut := time.Now().Add(-computeLogRetention()).UnixMilli(); req.Start < cut {
		req.Start = cut
	}
	s.each(func(l *dto.ComputeCallLog) bool {
		if req.End > 0 && l.Time > req.End {
			return true
		}
		if req.Start > 0 && l.Time < req.Start {
			return false
		}
		if (model != "" && l.Model != model) || (consumer != "" && l.Consumer != consumer) {
			return true
		}
		if (req.Status == "success" && !l.Success) || (req.Status == "failed" && l.Success) {
			return true
		}
		if res.Total >= int64(from) && len(res.Items) < req.PageSize {
			res.Items = append(res.Items, *l)
		}
		res.Total++
		return true
	})
	return res
}

// computeBucketSec picks a bucket size giving at most ~120 points.
func computeBucketSec(span time.Duration) int64 {
	for _, m := range []int64{1, 5, 10, 15, 30, 60, 120, 180, 360} {
		if span <= time.Duration(m*120)*time.Minute {
			return m * 60
		}
	}
	return 720 * 60
}

func (u *ComputeService) Analytics(req dto.ComputeAnalyticsReq) dto.ComputeAnalytics {
	now := time.Now()
	retention := computeLogRetention()
	minStart := now.Add(-retention).UnixMilli()
	end := req.End
	if end <= 0 || end > now.UnixMilli() {
		end = now.UnixMilli()
	}
	start := req.Start
	if start <= 0 || start < minStart {
		start = minStart
	}
	if start >= end {
		start = end - 60000
	}
	bucket := computeBucketSec(time.Duration(end-start) * time.Millisecond)
	res := dto.ComputeAnalytics{Start: start, End: end, BucketSec: bucket, Retention: int(retention.Hours()),
		Buckets: []dto.ComputeAnalyticsBucket{}, ByModel: []dto.ComputeAnalyticsShare{}, ByConsumer: []dto.ComputeAnalyticsShare{}}

	type acc struct {
		computeAgg
		byModel map[string]int64
	}
	firstBucket := start / 1000 / bucket * bucket
	buckets := map[int64]*acc{}
	byModel := map[string]*computeAgg{}
	byConsumer := map[string]*computeAgg{}
	models, consumers := map[string]bool{}, map[string]bool{}
	var total computeAgg
	var firstSeen int64

	s := computeCallLogs
	s.mu.RLock()
	for minute, m := range s.minutes {
		t := minute * 60000
		if t+60000 <= start || t > end {
			continue
		}
		for k, a := range m {
			models[k.model], consumers[k.consumer] = true, true
			if (req.Model != "" && k.model != req.Model) || (req.Consumer != "" && k.consumer != req.Consumer) {
				continue
			}
			if firstSeen == 0 || t < firstSeen {
				firstSeen = t
			}
			b := minute * 60 / bucket * bucket
			ba := buckets[b]
			if ba == nil {
				ba = &acc{byModel: map[string]int64{}}
				buckets[b] = ba
			}
			computeAggAdd(&ba.computeAgg, a)
			ba.byModel[k.model] += a.prompt + a.completion
			for _, pair := range []struct {
				m   map[string]*computeAgg
				key string
			}{{byModel, k.model}, {byConsumer, k.consumer}} {
				x := pair.m[pair.key]
				if x == nil {
					x = &computeAgg{}
					pair.m[pair.key] = x
				}
				computeAggAdd(x, a)
			}
			computeAggAdd(&total, a)
		}
	}
	s.mu.RUnlock()

	for b := firstBucket; b*1000 <= end; b += bucket {
		item := dto.ComputeAnalyticsBucket{Time: b * 1000, ByModel: map[string]int64{}}
		if a := buckets[b]; a != nil {
			item.Requests, item.Failures = a.req, a.fail
			item.Tokens = a.prompt + a.completion
			item.AvgTTFTMs = computeAvg(a.ttftSum, a.ttftN)
			item.AvgLatencyMs = computeAvg(a.latSum, a.req)
			item.ByModel = a.byModel
		}
		res.Buckets = append(res.Buckets, item)
	}
	sum := &res.Summary
	sum.Requests, sum.Failures = total.req, total.fail
	sum.PromptTokens, sum.CompletionTokens = total.prompt, total.completion
	sum.TotalTokens = total.prompt + total.completion
	if total.req > 0 {
		sum.SuccessRate = float64(total.req-total.fail) * 100 / float64(total.req)
		// RPM over the part of the window that has traffic (from the first
		// request on), so a fresh router is not averaged over empty hours.
		from := max(start, firstSeen)
		minutes := float64(end-from) / 60000
		if minutes < 1 {
			minutes = 1
		}
		sum.AvgRPM = float64(total.req) / minutes
	}
	sum.AvgTTFTMs = computeAvg(total.ttftSum, total.ttftN)
	sum.AvgLatencyMs = computeAvg(total.latSum, total.req)
	res.ByModel = computeShares(byModel)
	res.ByConsumer = computeShares(byConsumer)
	res.Models, res.Consumers = computeKeys(models), computeKeys(consumers)
	return res
}

func computeAggAdd(dst *computeAgg, a *computeAgg) {
	dst.req += a.req
	dst.fail += a.fail
	dst.prompt += a.prompt
	dst.completion += a.completion
	dst.ttftSum += a.ttftSum
	dst.ttftN += a.ttftN
	dst.latSum += a.latSum
}

func computeAvg(sum, n int64) float64 {
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}

func computeShares(m map[string]*computeAgg) []dto.ComputeAnalyticsShare {
	res := make([]dto.ComputeAnalyticsShare, 0, len(m))
	for k, a := range m {
		res = append(res, dto.ComputeAnalyticsShare{Name: k, Requests: a.req, Tokens: a.prompt + a.completion,
			Failures: a.fail, AvgTTFT: computeAvg(a.ttftSum, a.ttftN)})
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].Tokens != res[j].Tokens {
			return res[i].Tokens > res[j].Tokens
		}
		return res[i].Requests > res[j].Requests
	})
	return res
}

func computeKeys(m map[string]bool) []string {
	res := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			res = append(res, k)
		}
	}
	sort.Strings(res)
	return res
}
