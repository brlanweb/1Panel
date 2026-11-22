package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto"
	providercatalog "github.com/1Panel-dev/1Panel/agent/app/provider"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
	"github.com/1Panel-dev/1Panel/agent/global"
	"github.com/1Panel-dev/1Panel/agent/utils/req_helper"
)

// Website and model API probes. They only run on the master panel: a ticker
// wakes every few seconds, picks the rules whose interval has elapsed (with
// a random offset on first run so rules do not fire in lockstep), and runs
// them with bounded concurrency. A rule never overlaps with itself.

const (
	probeTick          = 5 * time.Second
	probeConcurrency   = 8
	probeMaxBodyBytes  = 1 << 20
	probeMaxErrorBytes = 300
)

type clusterProbeScheduler struct {
	mu      sync.Mutex
	nextRun map[uint]time.Time
	running map[uint]bool
	sem     chan struct{}
	started bool
}

var probeScheduler = &clusterProbeScheduler{
	nextRun: map[uint]time.Time{},
	running: map[uint]bool{},
	sem:     make(chan struct{}, probeConcurrency),
}

// StartClusterProbeScheduler starts the probe loop once (master only).
func StartClusterProbeScheduler() {
	if !global.IsMaster {
		return
	}
	probeScheduler.mu.Lock()
	if probeScheduler.started {
		probeScheduler.mu.Unlock()
		return
	}
	probeScheduler.started = true
	probeScheduler.mu.Unlock()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				global.LOG.Errorf("cluster probe scheduler panic: %v", r)
			}
		}()
		ticker := time.NewTicker(probeTick)
		defer ticker.Stop()
		for range ticker.C {
			probeScheduler.tick()
		}
	}()
}

// resetProbeSchedule makes a rule run soon after it was created or edited.
func resetProbeSchedule(alertID uint) {
	probeScheduler.mu.Lock()
	delete(probeScheduler.nextRun, alertID)
	probeScheduler.mu.Unlock()
}

func (p *clusterProbeScheduler) tick() {
	if global.AlertDB == nil {
		return
	}
	rules, err := alertRepo.List(repo.WithByStatus(constant.AlertEnable), alertRepo.WithByTypeIn([]string{dto.ClusterAlertSite, dto.ClusterAlertModel}))
	if err != nil {
		global.LOG.Errorf("load probe rules failed: %v", err)
		return
	}
	now := time.Now()
	active := make(map[uint]bool, len(rules))
	p.mu.Lock()
	var due []dto.AlertDTO
	for _, rule := range rules {
		active[rule.ID] = true
		cycle := time.Duration(maxUint(rule.Cycle, 1)) * time.Minute
		next, ok := p.nextRun[rule.ID]
		if !ok {
			// Spread the first run over up to 30s (or the interval).
			spread := cycle
			if spread > 30*time.Second {
				spread = 30 * time.Second
			}
			p.nextRun[rule.ID] = now.Add(time.Duration(rand.Int63n(int64(spread))))
			continue
		}
		if now.Before(next) || p.running[rule.ID] {
			continue
		}
		p.running[rule.ID] = true
		p.nextRun[rule.ID] = now.Add(cycle)
		due = append(due, dto.AlertDTO{
			ID: rule.ID, Type: rule.Type, Cycle: rule.Cycle, Count: rule.Count, Method: rule.Method,
			Title: rule.Title, Status: rule.Status, SendCount: rule.SendCount, AdvancedParams: rule.AdvancedParams,
		})
	}
	for id := range p.nextRun {
		if !active[id] {
			delete(p.nextRun, id)
		}
	}
	p.mu.Unlock()

	for _, rule := range due {
		rule := rule
		go func() {
			p.sem <- struct{}{}
			defer func() {
				<-p.sem
				p.mu.Lock()
				delete(p.running, rule.ID)
				p.mu.Unlock()
				if r := recover(); r != nil {
					global.LOG.Errorf("cluster probe %d panic: %v", rule.ID, r)
				}
			}()
			result := runClusterProbe(rule)
			processClusterCheck(rule, []dto.ClusterAlertTargetResult{result})
		}()
	}
}

func maxUint(value, min uint) uint {
	if value < min {
		return min
	}
	return value
}

func runClusterProbe(rule dto.AlertDTO) dto.ClusterAlertTargetResult {
	params := parseClusterAlertParams(rule.AdvancedParams)
	switch rule.Type {
	case dto.ClusterAlertSite:
		return probeSite(params)
	case dto.ClusterAlertModel:
		return probeModel(params)
	}
	return dto.ClusterAlertTargetResult{Key: "unknown", Failed: true, Error: "unknown probe type"}
}

// ------------------------------------------------------------- http client

func probeHTTPClient(params dto.ClusterAlertParams, timeout time.Duration, followRedirect, verifyTLS bool) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if params.UseProxy {
		proxyURL, err := loadProbeProxyURL()
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: !verifyTLS} //nolint:gosec // user-chosen per rule
	transport.DisableKeepAlives = true
	client := &http.Client{Timeout: timeout, Transport: transport}
	if !followRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	} else {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
	}
	return client, nil
}

// loadProbeProxyURL prefers the subscription proxy node, then the panel
// proxy settings.
func loadProbeProxyURL() (*url.URL, error) {
	if raw, err := NewIProxyNodeService().ProxyURLForHost(); err == nil && raw != "" {
		return url.Parse(raw)
	}
	address, err := req_helper.LoadRuntimeProxyURL()
	if err != nil || address == nil {
		return nil, errors.New("no proxy is configured (Settings → Panel → Proxy)")
	}
	return address, nil
}

func probeTimeout(params dto.ClusterAlertParams, defaultSec int) time.Duration {
	sec := params.TimeoutSec
	if sec <= 0 {
		sec = defaultSec
	}
	return time.Duration(sec) * time.Second
}

// ------------------------------------------------------------------ website

type statusRange struct{ min, max int }

func parseExpectStatus(raw string) ([]statusRange, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []statusRange{{200, 399}}, nil
	}
	var ranges []statusRange
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if len(lower) == 3 && strings.HasSuffix(lower, "xx") && lower[0] >= '1' && lower[0] <= '5' {
			base := int(lower[0]-'0') * 100
			ranges = append(ranges, statusRange{base, base + 99})
			continue
		}
		if bounds := strings.SplitN(part, "-", 2); len(bounds) == 2 {
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || lo < 100 || hi > 599 || lo > hi {
				return nil, clusterParamErr("invalid expected status: " + part)
			}
			ranges = append(ranges, statusRange{lo, hi})
			continue
		}
		code, err := strconv.Atoi(part)
		if err != nil || code < 100 || code > 599 {
			return nil, clusterParamErr("invalid expected status: " + part)
		}
		ranges = append(ranges, statusRange{code, code})
	}
	if len(ranges) == 0 {
		return []statusRange{{200, 399}}, nil
	}
	return ranges, nil
}

func statusAccepted(code int, ranges []statusRange) bool {
	for _, item := range ranges {
		if code >= item.min && code <= item.max {
			return true
		}
	}
	return false
}

func probeSite(params dto.ClusterAlertParams) dto.ClusterAlertTargetResult {
	result := dto.ClusterAlertTargetResult{Key: "site", Label: params.URL}
	fail := func(format string, args ...interface{}) dto.ClusterAlertTargetResult {
		result.Failed = true
		result.Error = truncateRunes(fmt.Sprintf(format, args...), probeMaxErrorBytes)
		return result
	}
	ranges, err := parseExpectStatus(params.ExpectStatus)
	if err != nil {
		return fail("%v", err)
	}
	timeout := probeTimeout(params, 10)
	client, err := probeHTTPClient(params, timeout, params.FollowRedirect, params.VerifyTLS)
	if err != nil {
		return fail("%v", err)
	}
	method := params.Method
	if method == "" {
		method = http.MethodGet
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, params.URL, nil)
	if err != nil {
		return fail("%v", err)
	}
	req.Header.Set("User-Agent", "1Panel-SiteProbe/1.0")
	for key, value := range params.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fail("%s", simplifyProbeError(err))
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	result.Value = fmt.Sprintf("HTTP %d, %dms", resp.StatusCode, elapsed.Milliseconds())
	if !statusAccepted(resp.StatusCode, ranges) {
		return fail("unexpected status HTTP %d", resp.StatusCode)
	}
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" && method != http.MethodHead {
		body, err := io.ReadAll(io.LimitReader(resp.Body, probeMaxBodyBytes))
		if err != nil {
			return fail("read body: %s", simplifyProbeError(err))
		}
		if !bytes.Contains(body, []byte(keyword)) {
			return fail("keyword %q not found", keyword)
		}
	}
	return result
}

func simplifyProbeError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return "timeout"
		}
		return urlErr.Err.Error()
	}
	return err.Error()
}

// -------------------------------------------------------------------- model

func isProbeableModelAPIType(apiType string) bool {
	switch apiType {
	case "openai-completions", "openai-responses", "anthropic-messages", "gemini-generate-content":
		return true
	}
	return false
}

// responseFormat maps a model service API type to the response shape
// checked by validateModelResponse.
func responseFormat(apiType string) string {
	switch apiType {
	case "anthropic-messages":
		return "anthropic"
	case "gemini-generate-content":
		return "gemini"
	case "openai-responses":
		return "openai-responses"
	}
	return "openai"
}

func probeModel(params dto.ClusterAlertParams) dto.ClusterAlertTargetResult {
	result := dto.ClusterAlertTargetResult{Key: "model", Label: params.Model}
	fail := func(format string, args ...interface{}) dto.ClusterAlertTargetResult {
		result.Failed = true
		result.Error = truncateRunes(fmt.Sprintf(format, args...), probeMaxErrorBytes)
		return result
	}
	account, err := agentAccountRepo.GetFirst(repo.WithByID(params.AccountID))
	if err != nil || account == nil || account.ID == 0 {
		return fail("model service #%d no longer exists", params.AccountID)
	}
	result.Label = account.Name + " / " + params.Model
	if !isProbeableModelAPIType(account.APIType) {
		return fail("model service API type %s cannot be checked", account.APIType)
	}
	apiKey := account.APIKey
	timeout := probeTimeout(params, 30)
	client, err := probeHTTPClient(params, timeout, true, true)
	if err != nil {
		return fail("%v", err)
	}
	built := providercatalog.BuildVerifyRequest(account.Provider, account.APIType, account.AuthMode, account.BaseURL, apiKey, params.Model)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, built.Method, built.URL, bytes.NewReader(built.Body))
	if err != nil {
		return fail("%v", err)
	}
	for key, value := range built.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", "application/json")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return fail("%s", redactKey(simplifyProbeError(err), apiKey))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, probeMaxBodyBytes))
	result.Value = fmt.Sprintf("HTTP %d, %dms", resp.StatusCode, time.Since(start).Milliseconds())
	if resp.StatusCode == http.StatusTooManyRequests && !params.Treat429AsFail {
		// Rate limited means the endpoint and the key work.
		return result
	}
	if resp.StatusCode != http.StatusOK {
		return fail("HTTP %d: %s", resp.StatusCode, redactKey(extractAPIError(body), apiKey))
	}
	check := params
	check.Provider = responseFormat(account.APIType)
	if err := validateModelResponse(check, body); err != nil {
		return fail("%v", err)
	}
	return result
}

// validateModelResponse checks the response shape. With strictContent the
// reply must also contain text.
func validateModelResponse(params dto.ClusterAlertParams, body []byte) error {
	switch params.Provider {
	case "openai":
		var res struct {
			Choices []struct {
				Message struct {
					Content          interface{} `json:"content"`
					ReasoningContent string      `json:"reasoning_content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &res); err != nil || len(res.Choices) == 0 {
			return errors.New("invalid response: no choices")
		}
		if params.StrictContent {
			text, _ := res.Choices[0].Message.Content.(string)
			if strings.TrimSpace(text) == "" && strings.TrimSpace(res.Choices[0].Message.ReasoningContent) == "" {
				return errors.New("empty reply content")
			}
		}
	case "openai-responses":
		var res struct {
			Status     string `json:"status"`
			OutputText string `json:"output_text"`
			Output     []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := json.Unmarshal(body, &res); err != nil || (res.Status == "" && len(res.Output) == 0) {
			return errors.New("invalid response: no output")
		}
		if params.StrictContent {
			if strings.TrimSpace(res.OutputText) != "" {
				return nil
			}
			for _, item := range res.Output {
				for _, part := range item.Content {
					if strings.TrimSpace(part.Text) != "" {
						return nil
					}
				}
			}
			return errors.New("empty reply content")
		}
	case "anthropic":
		var res struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &res); err != nil || res.Type != "message" {
			return errors.New("invalid response: not a message")
		}
		if params.StrictContent {
			for _, item := range res.Content {
				if strings.TrimSpace(item.Text) != "" {
					return nil
				}
			}
			return errors.New("empty reply content")
		}
	case "gemini":
		var res struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &res); err != nil || len(res.Candidates) == 0 {
			return errors.New("invalid response: no candidates")
		}
		if params.StrictContent {
			for _, part := range res.Candidates[0].Content.Parts {
				if strings.TrimSpace(part.Text) != "" {
					return nil
				}
			}
			return errors.New("empty reply content")
		}
	}
	return nil
}

func extractAPIError(body []byte) string {
	var res struct {
		Error interface{} `json:"error"`
	}
	if json.Unmarshal(body, &res) == nil && res.Error != nil {
		switch value := res.Error.(type) {
		case string:
			return truncateRunes(value, 200)
		case map[string]interface{}:
			if msg, ok := value["message"].(string); ok {
				return truncateRunes(msg, 200)
			}
		}
	}
	return truncateRunes(strings.TrimSpace(string(body)), 200)
}

func redactKey(text, key string) string {
	if key == "" {
		return text
	}
	return strings.ReplaceAll(text, key, "***")
}
