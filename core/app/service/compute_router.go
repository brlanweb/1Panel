package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/app/model"
	"github.com/1Panel-dev/1Panel/core/app/repo"
	"github.com/1Panel-dev/1Panel/core/buserr"
	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/1Panel-dev/1Panel/core/utils/encrypt"
)

// Model router: an OpenAI compatible data plane on its own listener.
//
//   POST /v1/chat/completions, /v1/completions, /v1/embeddings, ...
//   GET  /v1/models
//
// Callers (new-api, VPS services) authenticate with a consumer key. The
// request body is read once (bounded) to find "model"; the upstream is
// picked by a load score from live engine metrics and in-flight counts,
// and the response is streamed back unbuffered (SSE friendly). Failures
// before the first response byte are retried on another instance and feed
// a small circuit breaker that ejects an instance for a while.

const (
	computeSettingRouter = "ComputeRouter"
	computeSettingSched  = "ComputeScheduler"
	computeMaxBody       = 32 << 20
	computeEjectAfter    = 3
	computeEjectFor      = 30 * time.Second
	computeUsageFlush    = 10 * time.Second
)

type computeRouterConf struct {
	Enabled           bool   `json:"enabled"`
	Listen            string `json:"listen"`
	MaxRetries        int    `json:"maxRetries"`
	RequestTimeout    int    `json:"requestTimeout"`
	LogRetentionHours int    `json:"logRetentionHours"`
}

type computeRouterSrv struct {
	mu      sync.Mutex
	srv     *http.Server
	conf    computeRouterConf
	running bool
	lastErr string

	cacheMu   sync.RWMutex
	cacheAt   time.Time
	insts     []model.ComputeInstance
	consumers map[string]model.ComputeConsumer // keyHash -> consumer
	keys      map[uint]string                  // instance id -> upstream key

	rlMu sync.Mutex
	rl   map[uint]*computeBucket

	usageMu sync.Mutex
	usage   map[uint]*computeUsage
}

type computeBucket struct {
	tokens float64
	last   time.Time
}

type computeUsage struct {
	req, fail, prompt, completion int64
	last                          time.Time
}

var computeRouter = &computeRouterSrv{rl: map[uint]*computeBucket{}, usage: map[uint]*computeUsage{}}
var computeRouterDirty atomic.Bool

func computeRouterInvalidate() { computeRouterDirty.Store(true) }

func computeDefaultRouterConf() computeRouterConf {
	return computeRouterConf{Enabled: false, Listen: "0.0.0.0:18080", MaxRetries: 1, RequestTimeout: 600, LogRetentionHours: computeLogDefaultHours}
}

func computeLoadRouterConf() computeRouterConf {
	conf := computeDefaultRouterConf()
	if v, err := settingRepo.GetValueByKey(computeSettingRouter); err == nil && v != "" {
		_ = json.Unmarshal([]byte(v), &conf)
	}
	if conf.RequestTimeout <= 0 {
		conf.RequestTimeout = 600
	}
	return conf
}

// computeRouterApply (re)starts or stops the listener to match settings.
func computeRouterApply() error {
	conf := computeLoadRouterConf()
	computeSetLogRetention(conf.LogRetentionHours)
	r := computeRouter
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.srv != nil && (!conf.Enabled || conf.Listen != r.conf.Listen) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = r.srv.Shutdown(ctx)
		cancel()
		r.srv, r.running = nil, false
	}
	r.conf = conf
	if !conf.Enabled || r.srv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", conf.Listen)
	if err != nil {
		r.lastErr = err.Error()
		return err
	}
	srv := &http.Server{Handler: http.HandlerFunc(r.serve), ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second}
	r.srv, r.running, r.lastErr = srv, true, ""
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			global.LOG.Errorf("compute router stopped: %v", err)
			r.mu.Lock()
			r.running, r.lastErr = false, err.Error()
			r.mu.Unlock()
		}
	}()
	global.LOG.Infof("compute model router listening on %s", conf.Listen)
	return nil
}

func (r *computeRouterSrv) refresh() {
	r.cacheMu.RLock()
	fresh := time.Since(r.cacheAt) < 5*time.Second && !computeRouterDirty.Load()
	r.cacheMu.RUnlock()
	if fresh {
		return
	}
	insts, err := repo.ComputeInstanceRepo.List()
	if err != nil {
		return
	}
	cons, _ := repo.ComputeConsumerRepo.List()
	cm := make(map[string]model.ComputeConsumer, len(cons))
	for _, c := range cons {
		cm[c.KeyHash] = c
	}
	keys := map[uint]string{}
	for _, in := range insts {
		keys[in.ID] = computeInstanceKey(in)
	}
	r.cacheMu.Lock()
	r.insts, r.consumers, r.keys, r.cacheAt = insts, cm, keys, time.Now()
	r.cacheMu.Unlock()
	computeRouterDirty.Store(false)
}

func computeHashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func computeRandomKey(prefix string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func computeEncrypt(s string) (string, error) { return encrypt.StringEncrypt(s) }

func computeWriteErr(w http.ResponseWriter, code int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"message": msg, "type": typ}})
}

func (r *computeRouterSrv) auth(req *http.Request) (model.ComputeConsumer, bool) {
	key := strings.TrimSpace(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	if key == "" {
		key = req.Header.Get("x-api-key")
	}
	if key == "" {
		return model.ComputeConsumer{}, false
	}
	r.cacheMu.RLock()
	c, ok := r.consumers[computeHashKey(key)]
	r.cacheMu.RUnlock()
	return c, ok && c.Enabled
}

func (r *computeRouterSrv) allow(c model.ComputeConsumer) bool {
	if c.RPM <= 0 {
		return true
	}
	r.rlMu.Lock()
	defer r.rlMu.Unlock()
	b := r.rl[c.ID]
	now := time.Now()
	if b == nil {
		b = &computeBucket{tokens: float64(c.RPM), last: now}
		r.rl[c.ID] = b
	}
	b.tokens = math.Min(float64(c.RPM), b.tokens+now.Sub(b.last).Minutes()*float64(c.RPM))
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func computeModelAllowed(c model.ComputeConsumer, m string) bool {
	if strings.TrimSpace(c.AllowedModels) == "" {
		return true
	}
	for _, a := range strings.Split(c.AllowedModels, ",") {
		if strings.TrimSpace(a) == m {
			return true
		}
	}
	return false
}

func (r *computeRouterSrv) serve(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/healthz" {
		_, _ = w.Write([]byte("ok"))
		return
	}
	r.refresh()
	c, ok := r.auth(req)
	if !ok {
		computeWriteErr(w, http.StatusUnauthorized, "invalid_api_key", "invalid api key")
		return
	}
	if req.Method == http.MethodGet && req.URL.Path == "/v1/models" {
		r.listModels(w, c)
		return
	}
	if !strings.HasPrefix(req.URL.Path, "/v1/") {
		computeWriteErr(w, http.StatusNotFound, "not_found", "unsupported path")
		return
	}
	start := time.Now()
	if !r.allow(c) {
		computeCallLogs.add(dto.ComputeCallLog{Time: start.UnixMilli(), Consumer: c.Name, ConsumerID: c.ID, Path: req.URL.Path,
			Status: http.StatusTooManyRequests, Error: "rate limit exceeded", ClientIP: computeClientIP(req)})
		computeWriteErr(w, http.StatusTooManyRequests, "rate_limit", "rate limit exceeded")
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, computeMaxBody+1))
	if err != nil || len(body) > computeMaxBody {
		computeWriteErr(w, http.StatusRequestEntityTooLarge, "invalid_request", "body too large")
		return
	}
	var head struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &head)
	if head.Model == "" {
		computeWriteErr(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	if !computeModelAllowed(c, head.Model) {
		computeWriteErr(w, http.StatusForbidden, "model_not_allowed", "model not allowed for this key")
		return
	}
	call := &computeCall{log: dto.ComputeCallLog{Time: start.UnixMilli(), Consumer: c.Name, ConsumerID: c.ID,
		Model: head.Model, Path: req.URL.Path, Stream: head.Stream, ClientIP: computeClientIP(req)}}
	defer func() {
		call.log.LatencyMs = time.Since(start).Milliseconds()
		computeCallLogs.add(call.log)
	}()
	tried := map[uint]bool{}
	retries := r.conf.MaxRetries
	var lastErr string
	for attempt := 0; attempt <= retries; attempt++ {
		in, ok := r.pick(head.Model, tried)
		if !ok {
			break
		}
		tried[in.ID] = true
		call.log.Retries = len(tried) - 1
		done, retryable, errMsg := r.forward(w, req, in, body, c, head.Stream, call, start)
		if done {
			return
		}
		lastErr = errMsg
		if !retryable {
			return
		}
	}
	r.addUsage(c.ID, 0, 0, true)
	if len(tried) == 0 {
		call.fail(http.StatusServiceUnavailable, "no healthy instance")
		computeWriteErr(w, http.StatusServiceUnavailable, "model_unavailable", "no healthy instance for model "+head.Model)
		return
	}
	call.fail(http.StatusBadGateway, lastErr)
	computeWriteErr(w, http.StatusBadGateway, "upstream_error", lastErr)
}

// computeCall is the call log entry of one router request.
type computeCall struct {
	log dto.ComputeCallLog
}

func (c *computeCall) fail(status int, msg string) {
	c.log.Status, c.log.Success, c.log.Error = status, false, computeTrim(msg, 300)
}

func computeClientIP(req *http.Request) string {
	if xf := req.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	if xr := req.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

// computeLoadScore: queue dominates, then running + our own in-flight,
// then the recent time to first token (every 500 ms counts like one
// running request, so a slow replica — e.g. one whose prefill competes
// with long prompts — gets less new traffic), then KV pressure.
func computeLoadScore(m dto.ComputeEngineMetrics, inflight int64) float64 {
	score := m.Waiting*4 + m.Running + float64(inflight)*1.5
	if m.TTFTMs > 0 {
		score += math.Min(m.TTFTMs/500, 20)
	}
	if m.KVUsage > 0.9 {
		score += (m.KVUsage - 0.9) * 100
	}
	return score
}

// pick chooses an instance with the lowest load score among the healthy,
// routable ones; ties are broken randomly weighted by Weight.
func (r *computeRouterSrv) pick(modelName string, tried map[uint]bool) (model.ComputeInstance, bool) {
	r.cacheMu.RLock()
	insts := r.insts
	r.cacheMu.RUnlock()
	now := time.Now().UnixMilli()
	type cand struct {
		in    model.ComputeInstance
		score float64
	}
	var cands []cand
	for _, in := range insts {
		if in.ServedName != modelName || !in.RouteEnabled || in.Weight <= 0 || tried[in.ID] {
			continue
		}
		if in.Status != model.ComputeInstRunning && in.Status != model.ComputeInstUnknown {
			continue
		}
		st := computeState.inst(in.ID)
		if st.ejectUntil.Load() > now {
			continue
		}
		m := computeState.instMetrics(in.ID)
		if m.SampledAt > 0 && !m.Healthy {
			continue
		}
		score := (computeLoadScore(m, st.inflight.Load()) + 1) * 100 / float64(in.Weight)
		cands = append(cands, cand{in, score})
	}
	if len(cands) == 0 {
		return model.ComputeInstance{}, false
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score < cands[j].score })
	best := cands[0].score
	var near []cand
	for _, c := range cands {
		if c.score <= best*1.1+0.01 {
			near = append(near, c)
		}
	}
	return near[mrand.IntN(len(near))].in, true
}

var computeUpstream = &http.Transport{
	Proxy:                 nil,
	DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConnsPerHost:   64,
	IdleConnTimeout:       90 * time.Second,
	ResponseHeaderTimeout: 0,
	DisableCompression:    true,
}

var computeHopHeaders = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Content-Length"}

// forward returns done (response written), retryable (nothing written yet).
func (r *computeRouterSrv) forward(w http.ResponseWriter, req *http.Request, in model.ComputeInstance, body []byte,
	c model.ComputeConsumer, stream bool, call *computeCall, start time.Time) (bool, bool, string) {
	st := computeState.inst(in.ID)
	st.inflight.Add(1)
	defer st.inflight.Add(-1)

	ctx, cancel := context.WithTimeout(req.Context(), time.Duration(r.conf.RequestTimeout)*time.Second)
	defer cancel()
	// One pool may hold engines serving the model under another name: the
	// client asks for the pool name, the engine must see its own.
	body = computeRewriteModel(body, in)
	injected := false
	if stream {
		body, injected = computeStreamUsage(body)
	}
	url := strings.TrimRight(in.Endpoint, "/") + req.URL.Path
	if req.URL.RawQuery != "" {
		url += "?" + req.URL.RawQuery
	}
	up, err := http.NewRequestWithContext(ctx, req.Method, url, bytes.NewReader(body))
	if err != nil {
		return false, false, err.Error()
	}
	for k, vs := range req.Header {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "x-api-key") {
			continue
		}
		for _, v := range vs {
			up.Header.Add(k, v)
		}
	}
	for _, h := range computeHopHeaders {
		up.Header.Del(h)
	}
	r.cacheMu.RLock()
	key := r.keys[in.ID]
	r.cacheMu.RUnlock()
	if key != "" {
		up.Header.Set("Authorization", "Bearer "+key)
	}
	up.ContentLength = int64(len(body))
	resp, err := computeUpstream.RoundTrip(up)
	if err != nil {
		r.fail(st)
		if req.Context().Err() != nil {
			return true, false, "client gone"
		}
		return false, true, fmt.Sprintf("%s: %v", in.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		r.fail(st)
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return false, true, fmt.Sprintf("%s: http %d %s", in.Name, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	st.failStreak.Store(0)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	for _, h := range computeHopHeaders {
		w.Header().Del(h)
	}
	w.Header().Set("X-Upstream-Instance", in.Name)
	w.WriteHeader(resp.StatusCode)
	call.log.Instance, call.log.Status = in.Name, resp.StatusCode
	usage := &computeUsageSniffer{}
	errTail := &computeErrTail{on: resp.StatusCode >= 400}
	var filter *computeUsageFilter
	if injected && resp.StatusCode < 400 {
		filter = &computeUsageFilter{}
	}
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	var readErr error
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if call.log.TTFTMs == 0 {
				// First body byte: for SSE the first token event (role /
				// content), for JSON the whole answer is ready.
				call.log.TTFTMs = max(time.Since(start).Milliseconds(), 1)
			}
			usage.feed(chunk)
			errTail.feed(chunk)
			if filter != nil {
				chunk = filter.feed(chunk)
			}
			if len(chunk) > 0 {
				if _, werr := w.Write(chunk); werr != nil {
					break
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				readErr = rerr
			}
			break
		}
	}
	if filter != nil {
		if rest := filter.flush(); len(rest) > 0 {
			_, _ = w.Write(rest)
		}
	}
	_ = stream
	call.log.PromptTokens, call.log.CompletionTokens = usage.prompt, usage.completion
	call.log.Success = resp.StatusCode < 400 && readErr == nil
	switch {
	case resp.StatusCode >= 400:
		call.log.Error = computeTrim(errTail.String(), 300)
	case readErr != nil:
		call.log.Error = "stream interrupted: " + computeTrim(readErr.Error(), 200)
		if req.Context().Err() != nil {
			call.log.Error = "client closed the connection"
		}
	}
	r.addUsage(c.ID, usage.prompt, usage.completion, !call.log.Success)
	return true, false, ""
}

// computeErrTail keeps the first bytes of an error response body.
type computeErrTail struct {
	on  bool
	buf []byte
}

func (t *computeErrTail) feed(b []byte) {
	if !t.on || len(t.buf) >= 512 {
		return
	}
	t.buf = append(t.buf, b[:min(len(b), 512-len(t.buf))]...)
}

func (t *computeErrTail) String() string { return strings.TrimSpace(string(t.buf)) }

// computeRewriteModel replaces the top-level "model" of a JSON request body
// with the instance's upstream model name. Every other field, and its raw
// encoding (numbers, unicode, key order of nested objects), is kept as is.
// Bodies that are not a JSON object are returned untouched.
func computeRewriteModel(body []byte, in model.ComputeInstance) []byte {
	upstream := computeUpstreamModel(in)
	if upstream == in.ServedName {
		return body
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	if _, ok := obj["model"]; !ok {
		return body
	}
	name, err := json.Marshal(upstream)
	if err != nil {
		return body
	}
	obj["model"] = name
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// computeStreamUsage asks the engine to append a usage chunk to a stream
// (stream_options.include_usage), so streamed calls are counted too. A
// request that already sets stream_options is left as is; vLLM / SGLang
// send the usage as the last data chunk before [DONE].
func computeStreamUsage(body []byte) ([]byte, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, false
	}
	if _, ok := obj["stream_options"]; ok {
		return body, false
	}
	obj["stream_options"] = json.RawMessage(`{"include_usage":true}`)
	out, err := json.Marshal(obj)
	if err != nil {
		return body, false
	}
	return out, true
}

// computeUsageFilter removes the usage-only SSE event ("choices": [] plus
// "usage") that the router asked for on behalf of a client that did not:
// some clients index choices[0] of every chunk. Events are passed through
// as soon as they are complete; only a partial event is held back.
type computeUsageFilter struct {
	pending []byte
}

var computeSSESep = []byte("\n\n")

func (f *computeUsageFilter) feed(b []byte) []byte {
	f.pending = append(f.pending, b...)
	var out []byte
	for {
		i := bytes.Index(f.pending, computeSSESep)
		if i < 0 {
			break
		}
		ev := f.pending[:i+2]
		if !computeUsageOnlyEvent(ev) {
			out = append(out, ev...)
		}
		f.pending = f.pending[i+2:]
	}
	return out
}

func (f *computeUsageFilter) flush() []byte {
	out := f.pending
	f.pending = nil
	return out
}

func computeUsageOnlyEvent(ev []byte) bool {
	if !bytes.Contains(ev, []byte(`"usage"`)) {
		return false
	}
	data := bytes.TrimSpace(ev)
	if !bytes.HasPrefix(data, []byte("data:")) {
		return false
	}
	var chunk struct {
		Choices []json.RawMessage `json:"choices"`
		Usage   json.RawMessage   `json:"usage"`
	}
	if json.Unmarshal(bytes.TrimSpace(data[5:]), &chunk) != nil {
		return false
	}
	return len(chunk.Choices) == 0 && len(chunk.Usage) > 0 && string(chunk.Usage) != "null"
}

func (r *computeRouterSrv) fail(st *computeInstState) {
	if st.failStreak.Add(1) >= computeEjectAfter {
		st.ejectUntil.Store(time.Now().Add(computeEjectFor).UnixMilli())
		st.failStreak.Store(0)
	}
}

func (r *computeRouterSrv) listModels(w http.ResponseWriter, c model.ComputeConsumer) {
	r.cacheMu.RLock()
	insts := r.insts
	r.cacheMu.RUnlock()
	seen := map[string]bool{}
	var data []map[string]interface{}
	for _, in := range insts {
		if seen[in.ServedName] || !in.RouteEnabled || !computeModelAllowed(c, in.ServedName) {
			continue
		}
		seen[in.ServedName] = true
		data = append(data, map[string]interface{}{"id": in.ServedName, "object": "model", "owned_by": "xpanel"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": data})
}

// computeUsageSniffer finds the last "usage" object in a JSON or SSE body
// without buffering the whole response (keeps a short tail).
type computeUsageSniffer struct {
	tail               []byte
	prompt, completion int64
}

func (s *computeUsageSniffer) feed(b []byte) {
	s.tail = append(s.tail, b...)
	for {
		i := bytes.Index(s.tail, []byte(`"usage"`))
		if i < 0 {
			if len(s.tail) > 64 {
				s.tail = append([]byte(nil), s.tail[len(s.tail)-64:]...)
			}
			return
		}
		j := bytes.IndexByte(s.tail[i:], '}')
		if j < 0 {
			if len(s.tail) > 8192 {
				s.tail = nil
			}
			return
		}
		var u struct {
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
		}
		start := bytes.IndexByte(s.tail[i:], '{')
		if start >= 0 && start < j {
			if json.Unmarshal(s.tail[i+start:i+j+1], &u) == nil && (u.Prompt > 0 || u.Completion > 0) {
				s.prompt, s.completion = u.Prompt, u.Completion
			}
		}
		s.tail = s.tail[i+j+1:]
	}
}

func (r *computeRouterSrv) addUsage(id uint, prompt, completion int64, failed bool) {
	r.usageMu.Lock()
	defer r.usageMu.Unlock()
	u := r.usage[id]
	if u == nil {
		u = &computeUsage{}
		r.usage[id] = u
	}
	u.req++
	if failed {
		u.fail++
	}
	u.prompt += prompt
	u.completion += completion
	u.last = time.Now()
}

func computeUsageFlushLoop() {
	ticker := time.NewTicker(computeUsageFlush)
	defer ticker.Stop()
	for range ticker.C {
		r := computeRouter
		r.usageMu.Lock()
		batch := r.usage
		r.usage = map[uint]*computeUsage{}
		r.usageMu.Unlock()
		for id, u := range batch {
			_ = global.DB.Model(&model.ComputeConsumer{}).Where("id = ?", id).Updates(map[string]interface{}{
				"requests":          computeExpr("requests + ?", u.req),
				"failures":          computeExpr("failures + ?", u.fail),
				"prompt_tokens":     computeExpr("prompt_tokens + ?", u.prompt),
				"completion_tokens": computeExpr("completion_tokens + ?", u.completion),
				"last_used_at":      u.last,
			}).Error
		}
	}
}

// ---- service methods ------------------------------------------------------

func (u *ComputeService) ListConsumers() ([]model.ComputeConsumer, error) {
	return repo.ComputeConsumerRepo.List(repo.WithComputeOrder("name asc"))
}

func (u *ComputeService) SaveConsumer(req dto.ComputeConsumerReq) (dto.ComputeConsumerKeyRes, error) {
	if dup, err := repo.ComputeConsumerRepo.Get(repo.WithByName(req.Name)); err == nil && dup.ID != req.ID {
		return dto.ComputeConsumerKeyRes{}, buserr.New("ErrRecordExist")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	defer computeRouterInvalidate()
	if req.ID > 0 {
		return dto.ComputeConsumerKeyRes{ID: req.ID}, repo.ComputeConsumerRepo.Update(req.ID, map[string]interface{}{
			"name": req.Name, "allowed_models": req.AllowedModels, "rpm": req.RPM, "enabled": enabled, "remark": req.Remark})
	}
	key := computeRandomKey("sk-xp-")
	c := model.ComputeConsumer{Name: req.Name, KeyHash: computeHashKey(key), KeyHint: key[:9] + "…" + key[len(key)-4:],
		AllowedModels: req.AllowedModels, RPM: req.RPM, Enabled: enabled, Remark: req.Remark}
	if err := repo.ComputeConsumerRepo.Create(&c); err != nil {
		return dto.ComputeConsumerKeyRes{}, err
	}
	return dto.ComputeConsumerKeyRes{ID: c.ID, Key: key}, nil
}

func (u *ComputeService) ResetConsumerKey(id uint) (dto.ComputeConsumerKeyRes, error) {
	if _, err := repo.ComputeConsumerRepo.Get(repo.WithByID(id)); err != nil {
		return dto.ComputeConsumerKeyRes{}, buserr.New("ErrRecordNotFound")
	}
	key := computeRandomKey("sk-xp-")
	defer computeRouterInvalidate()
	return dto.ComputeConsumerKeyRes{ID: id, Key: key}, repo.ComputeConsumerRepo.Update(id, map[string]interface{}{
		"key_hash": computeHashKey(key), "key_hint": key[:9] + "…" + key[len(key)-4:]})
}

func (u *ComputeService) DeleteConsumer(id uint) error {
	defer computeRouterInvalidate()
	return repo.ComputeConsumerRepo.Delete(repo.WithByID(id))
}

func (u *ComputeService) GetRouterSetting() (dto.ComputeRouterSetting, error) {
	conf := computeLoadRouterConf()
	sched := computeLoadSchedConf()
	r := computeRouter
	r.mu.Lock()
	running, lastErr := r.running, r.lastErr
	r.mu.Unlock()
	return dto.ComputeRouterSetting{Enabled: conf.Enabled, Listen: conf.Listen, MaxRetries: conf.MaxRetries,
		RequestTimeout: conf.RequestTimeout, LogRetentionHours: int(computeLogRetention().Hours()), Running: running,
		Error: lastErr, SchedulerPaused: sched.Paused}, nil
}

func (u *ComputeService) UpdateRouterSetting(req dto.ComputeRouterSetting) error {
	if _, _, err := net.SplitHostPort(req.Listen); err != nil {
		return buserr.WithDetail("ErrInvalidParams", "listen", nil)
	}
	retention := req.LogRetentionHours
	if retention <= 0 {
		retention = computeLogDefaultHours
	}
	conf := computeRouterConf{Enabled: req.Enabled, Listen: req.Listen, MaxRetries: req.MaxRetries, RequestTimeout: req.RequestTimeout,
		LogRetentionHours: min(retention, computeLogMaxHours)}
	b, _ := json.Marshal(conf)
	if err := settingRepo.UpdateOrCreate(computeSettingRouter, string(b)); err != nil {
		return err
	}
	sb, _ := json.Marshal(computeSchedConf{Paused: req.SchedulerPaused})
	if err := settingRepo.UpdateOrCreate(computeSettingSched, string(sb)); err != nil {
		return err
	}
	computeCallLogs.prune(time.Now())
	return computeRouterApply()
}
