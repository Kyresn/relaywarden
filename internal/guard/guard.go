// Package guard observes request/response fidelity and records degradation
// signals. It never rewrites routing or retries a request.
package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"cpa-guard/internal/config"
	"cpa-guard/internal/protocol"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

var severityRank = map[string]int{
	SeverityLow: 1, SeverityMedium: 2, SeverityHigh: 3, SeverityCritical: 4,
}

// Signal is one observed degradation event.
type Signal struct {
	Time           time.Time      `json:"time"`
	Severity       string         `json:"severity"`
	Code           string         `json:"code"`
	RequestID      string         `json:"request_id,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	Provider       string         `json:"provider,omitempty"`
	AuthID         string         `json:"auth_id,omitempty"`
	Model          string         `json:"model,omitempty"`
	RequestedModel string         `json:"requested_model,omitempty"`
	ResponseModel  string         `json:"response_model,omitempty"`
	Detail         map[string]any `json:"detail,omitempty"`
}

type profileEntry struct {
	profile protocol.Profile
	created time.Time
	body    []byte
}

type Guard struct {
	store *config.Store

	mu        sync.Mutex
	profiles  map[string]*profileEntry
	signals   []Signal
	counters  map[string]int64
	seen      map[string]time.Time
	cacheMiss map[string]int
	finger    map[string]string
	ttft      map[string]*ttftStat
	lastUsage []pluginapi.UsageRecord

	webhook *webhookQueue
	stop    chan struct{}
	once    sync.Once
}

func New(store *config.Store) *Guard {
	g := &Guard{
		store:     store,
		profiles:  make(map[string]*profileEntry),
		counters:  make(map[string]int64),
		seen:      make(map[string]time.Time),
		cacheMiss: make(map[string]int),
		finger:    make(map[string]string),
		stop:      make(chan struct{}),
	}
	g.webhook = newWebhookQueue(g)
	go g.cleanupLoop()
	return g
}

func (g *Guard) Close() {
	if g == nil {
		return
	}
	g.once.Do(func() {
		close(g.stop)
		g.webhook.Close()
	})
}

func (g *Guard) cfg() *config.Config {
	if g == nil || g.store == nil {
		return nil
	}
	return g.store.Load()
}

// Before records the request profile and optionally injects a stable prompt
// cache key when the client did not send one.
func (g *Guard) Before(req pluginapi.RequestInterceptRequest) []byte {
	cfg := g.cfg()
	if cfg == nil || !cfg.Guard.Enabled {
		return nil
	}
	p := protocol.ExtractProfile(req.Body, req.SourceFormat, req.Model)
	p.SessionID = sessionID(req)
	entry := &profileEntry{profile: p, created: time.Now(), body: append([]byte(nil), req.Body...)}
	g.mu.Lock()
	g.profiles[req.RequestID] = entry
	g.evictProfilesLocked(cfg.Metrics.MaxSignals * 4)
	g.mu.Unlock()

	if !cfg.Guard.InjectPromptCacheKey || p.PromptCacheKey != "" || p.SessionID == "" {
		return nil
	}
	path := cacheKeyPath(p.Format)
	if path == "" {
		return nil
	}
	key := "cpa-guard-" + shortHash(p.SessionID+"|"+p.Model)
	out, err := sjson.SetBytes(req.Body, path, key)
	if err != nil || len(out) == 0 {
		return nil
	}
	g.mu.Lock()
	if entry := g.profiles[req.RequestID]; entry != nil {
		entry.profile.PromptCacheKey = key
		entry.profile.CacheKeyPath = path
	}
	g.mu.Unlock()
	return out
}

// After restores source-level fields that another layer dropped and records
// what could not be restored. It returns a replacement body only when needed.
func (g *Guard) After(req pluginapi.RequestInterceptRequest) []byte {
	cfg := g.cfg()
	if cfg == nil || !cfg.Guard.Enabled {
		return nil
	}
	g.mu.Lock()
	entry := g.profiles[req.RequestID]
	g.mu.Unlock()
	if entry == nil {
		return nil
	}
	if !cfg.Guard.ReinjectSourceParams {
		return nil
	}
	body := req.Body
	var changed bool
	var dropped []string

	restoreString := func(path, value, code string) {
		if path == "" || value == "" {
			return
		}
		cur := gjson.GetBytes(body, path)
		if cur.Exists() {
			if cur.String() != value {
				dropped = append(dropped, code+"_changed")
			}
			return
		}
		next, err := sjson.SetBytes(body, path, value)
		if err == nil && len(next) > 0 {
			body = next
			changed = true
		}
	}
	restoreInt := func(path string, value int64, code string) {
		if path == "" || value <= 0 {
			return
		}
		cur := gjson.GetBytes(body, path)
		if cur.Exists() {
			if cur.Int() < value {
				dropped = append(dropped, code+"_reduced")
			}
			return
		}
		next, err := sjson.SetBytes(body, path, value)
		if err == nil && len(next) > 0 {
			body = next
			changed = true
		}
	}
	restoreBool := func(path string, value bool, code string) {
		if path == "" || !value {
			return
		}
		cur := gjson.GetBytes(body, path)
		if cur.Exists() {
			return
		}
		next, err := sjson.SetBytes(body, path, true)
		if err == nil && len(next) > 0 {
			body = next
			changed = true
		}
	}

	restoreString(entry.profile.ReasoningPath, entry.profile.ReasoningEffort, "reasoning")
	restoreInt(entry.profile.ThinkingPath, entry.profile.ThinkingBudget, "thinking_budget")
	restoreBool(entry.profile.IncludePath, entry.profile.IncludeThoughts, "include_thoughts")
	restoreInt(entry.profile.MaxTokensPath, entry.profile.MaxOutputTokens, "max_tokens")
	restoreString(entry.profile.ServiceTierPath, entry.profile.ServiceTier, "service_tier")
	restoreString(entry.profile.CacheKeyPath, entry.profile.PromptCacheKey, "prompt_cache_key")

	if len(dropped) > 0 && cfg.Guard.Signals.SourceParamDropped {
		g.emit(Signal{
			Severity: SeverityHigh, Code: "source_param_dropped",
			RequestID: req.RequestID, Model: req.Model, RequestedModel: req.RequestedModel,
			Detail: map[string]any{"fields": dropped, "format": req.ToFormat},
		})
	}
	afterProfile := protocol.ExtractProfile(body, req.SourceFormat, req.Model)
	if cfg.Guard.CheckTools && cfg.Guard.Signals.ToolsChanged && !sameStringSet(entry.profile.Tools, afterProfile.Tools) {
		g.emit(Signal{
			Severity: SeverityHigh, Code: "tools_changed",
			RequestID: req.RequestID, Model: req.Model,
			Detail: map[string]any{"before": entry.profile.Tools, "after": afterProfile.Tools},
		})
	}
	if cfg.Guard.Signals.SystemPromptChanged && entry.profile.SystemHash != "" && afterProfile.SystemHash != "" && entry.profile.SystemHash != afterProfile.SystemHash {
		g.emit(Signal{
			Severity: SeverityLow, Code: "system_prompt_changed",
			RequestID: req.RequestID, Model: req.Model,
		})
	}
	if !changed {
		return nil
	}
	return body
}

// ObserveResponse inspects a non-streaming response body.
func (g *Guard) ObserveResponse(req pluginapi.ResponseInterceptRequest) {
	cfg := g.cfg()
	if cfg == nil || !cfg.Guard.Enabled || len(req.Body) == 0 {
		return
	}
	g.mu.Lock()
	entry := g.profiles[req.RequestID]
	g.mu.Unlock()
	if entry == nil {
		return
	}
	p := entry.profile
	body := req.Body

	// Model identity is checked in ObserveUsage, where the record carries both
	// the upstream model and the client-facing alias. The response interceptor
	// only sees the alias and would false-positive on every alias route.

	if cfg.Guard.CheckReasoning && cfg.Guard.Signals.ReasoningStripped && p.HasThinking {
		if !responseHasReasoning(body, p.Format) {
			g.emit(Signal{
				Severity: severityForReasoning(p.Model), Code: "reasoning_stripped",
				RequestID: req.RequestID, Model: p.Model, RequestedModel: req.RequestedModel,
				Detail: map[string]any{"format": p.Format, "requested_effort": p.ReasoningEffort},
			})
		}
	}
	if cfg.Guard.CheckCache && cfg.Guard.Signals.CacheMissStreak {
		cached, prompt := responseCacheTokens(body, p.Format)
		if p.PromptCacheKey != "" && prompt >= int64(cfg.Guard.PromptTokenThreshold) && cached == 0 {
			g.noteCacheMiss(cfg, req.RequestID, p)
		} else if cached > 0 {
			g.clearCacheMiss(p)
		}
	}
	if cfg.Guard.CheckFingerprint && cfg.Guard.Signals.SystemFingerprintChanged {
		if fp := strings.TrimSpace(gjson.GetBytes(body, "system_fingerprint").String()); fp != "" {
			key := p.Model
			g.mu.Lock()
			prev := g.finger[key]
			g.finger[key] = fp
			g.mu.Unlock()
			if prev != "" && prev != fp {
				g.emit(Signal{
					Severity: SeverityMedium, Code: "system_fingerprint_changed",
					RequestID: req.RequestID, Model: p.Model,
					Detail: map[string]any{"previous": prev, "current": fp},
				})
			}
		}
	}
	if cfg.Guard.CheckCacheReason && cfg.Guard.Signals.CacheMissReason {
		if reason := cacheMissReason(body); reason != "" {
			sev := SeverityMedium
			switch reason {
			case "model_changed", "tools_changed", "reasoning_effort_changed", "service_tier_changed":
				sev = SeverityHigh
			}
			g.emit(Signal{Severity: sev, Code: "cache_miss_reason", RequestID: req.RequestID, Model: p.Model, Detail: map[string]any{"reason": reason}})
		}
	}
	if cfg.Guard.CheckTruncation && cfg.Guard.Signals.Truncated {
		if reason := truncationReason(body, p.Format); reason != "" {
			g.emit(Signal{Severity: SeverityLow, Code: "truncated", RequestID: req.RequestID, Model: p.Model, Detail: map[string]any{"reason": reason}})
		}
	}
	if cfg.Guard.CheckEmpty && cfg.Guard.Signals.EmptyOutput {
		if responseEmpty(body, p.Format) {
			g.emit(Signal{Severity: SeverityCritical, Code: "empty_output", RequestID: req.RequestID, Model: p.Model})
		}
	}
}

// ObserveUsage inspects the host's normalized usage record.
func (g *Guard) ObserveUsage(rec pluginapi.UsageRecord) {
	cfg := g.cfg()
	if cfg == nil || !cfg.Guard.Enabled {
		return
	}
	g.mu.Lock()
	g.lastUsage = append(g.lastUsage, rec)
	if len(g.lastUsage) > 20 {
		g.lastUsage = g.lastUsage[len(g.lastUsage)-20:]
	}
	entry := g.profiles[rec.RequestID]
	g.mu.Unlock()

	if cfg.Guard.CheckModel && cfg.Guard.Signals.ModelMismatch && rec.ResponseModel != "" {
		requested := rec.Model
		if requested == "" && entry != nil {
			requested = entry.profile.Model
		}
		if !modelsMatchAny(rec.ResponseModel, requested, rec.Alias) {
			g.emit(Signal{
				Severity: SeverityCritical, Code: "model_mismatch",
				RequestID: rec.RequestID, TraceID: rec.TraceID, Provider: rec.Provider, AuthID: rec.AuthID,
				Model: requested, ResponseModel: rec.ResponseModel,
				Detail: map[string]any{"alias": rec.Alias, "record_model": rec.Model},
			})
		}
	}
	if cfg.Guard.CheckReasoning && cfg.Guard.Signals.ReasoningStripped && entry != nil && entry.profile.HasThinking {
		if rec.Detail.ReasoningTokens == 0 {
			g.emit(Signal{
				Severity: severityForReasoning(rec.Model), Code: "reasoning_stripped",
				RequestID: rec.RequestID, TraceID: rec.TraceID, Provider: rec.Provider, AuthID: rec.AuthID,
				Model: rec.Model, RequestedModel: entry.profile.Model,
				Detail: map[string]any{"reasoning_tokens": 0, "format": entry.profile.Format},
			})
		}
	}
	if cfg.Guard.CheckUsage && cfg.Guard.Signals.UsageInvariantViolation {
		if reason := usageInvariant(rec); reason != "" {
			g.emit(Signal{
				Severity: SeverityHigh, Code: "usage_invariant_violation",
				RequestID: rec.RequestID, TraceID: rec.TraceID, Provider: rec.Provider, AuthID: rec.AuthID,
				Model: rec.Model, Detail: map[string]any{"reason": reason},
			})
		}
	}
	if cfg.Guard.CheckParams && cfg.Guard.Signals.ServiceTierDowngraded {
		if rec.EffectiveServiceTier != "" && rec.ResponseServiceTier != "" &&
			tierRank(rec.ResponseServiceTier) < tierRank(rec.EffectiveServiceTier) {
			g.emit(Signal{
				Severity: SeverityMedium, Code: "service_tier_downgraded",
				RequestID: rec.RequestID, TraceID: rec.TraceID, Provider: rec.Provider, AuthID: rec.AuthID,
				Model: rec.Model, Detail: map[string]any{"effective": rec.EffectiveServiceTier, "response": rec.ResponseServiceTier},
			})
		}
	}
	if cfg.Guard.CheckEmpty && cfg.Guard.Signals.EmptyOutput && rec.Generate && rec.Detail.OutputTokens == 0 && !rec.Failed {
		g.emit(Signal{Severity: SeverityCritical, Code: "empty_output", RequestID: rec.RequestID, Provider: rec.Provider, AuthID: rec.AuthID, Model: rec.Model})
	}
	if cfg.Guard.Signals.UsageMissing && rec.Generate && !rec.Failed &&
		rec.Detail.TotalTokens == 0 && rec.Detail.InputTokens == 0 && rec.Detail.OutputTokens == 0 {
		g.emit(Signal{Severity: SeverityMedium, Code: "usage_missing", RequestID: rec.RequestID, Provider: rec.Provider, AuthID: rec.AuthID, Model: rec.Model})
	}
	if cfg.Guard.CheckTTFT && cfg.Guard.Signals.TTFTAnomaly && rec.TTFT > 0 {
		// A low-confidence signal: only emitted when a per-auth baseline exists.
		g.noteTTFT(cfg, rec)
	}
}

// RecentUsage returns the newest usage records for diagnostics.
func (g *Guard) RecentUsage(limit int) []pluginapi.UsageRecord {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit <= 0 || limit > len(g.lastUsage) {
		limit = len(g.lastUsage)
	}
	out := make([]pluginapi.UsageRecord, limit)
	for i := 0; i < limit; i++ {
		out[i] = g.lastUsage[len(g.lastUsage)-1-i]
	}
	return out
}

// Signals returns the newest signals first.
func (g *Guard) Signals(limit int) []Signal {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit <= 0 || limit > len(g.signals) {
		limit = len(g.signals)
	}
	out := make([]Signal, limit)
	for i := 0; i < limit; i++ {
		out[i] = g.signals[len(g.signals)-1-i]
	}
	return out
}

// Summary returns counters grouped by code.
func (g *Guard) Summary() map[string]int64 {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]int64, len(g.counters))
	for k, v := range g.counters {
		out[k] = v
	}
	return out
}

// Complete releases per-request state after the host reports a terminal outcome.
func (g *Guard) Complete(requestID string) {
	if g == nil || requestID == "" {
		return
	}
	g.mu.Lock()
	delete(g.profiles, requestID)
	for k := range g.seen {
		if strings.HasPrefix(k, "|"+requestID) || strings.HasSuffix(k, "|"+requestID) {
			delete(g.seen, k)
		}
	}
	g.mu.Unlock()
}

func (g *Guard) emit(s Signal) {
	cfg := g.cfg()
	if cfg == nil || !cfg.Guard.Enabled {
		return
	}
	if !signalEnabled(cfg.Guard.Signals, s.Code) {
		return
	}
	if s.Time.IsZero() {
		s.Time = time.Now().UTC()
	}
	key := s.Code + "|" + s.RequestID
	g.mu.Lock()
	if t, ok := g.seen[key]; ok && time.Since(t) < 5*time.Second {
		g.mu.Unlock()
		return
	}
	g.seen[key] = time.Now()
	g.signals = append(g.signals, s)
	g.counters[s.Code]++
	max := cfg.Metrics.MaxSignals
	if max <= 0 {
		max = 1000
	}
	if len(g.signals) > max {
		g.signals = g.signals[len(g.signals)-max:]
	}
	if len(g.seen) > max*4 {
		cut := time.Now().Add(-10 * time.Minute)
		for k, t := range g.seen {
			if t.Before(cut) {
				delete(g.seen, k)
			}
		}
	}
	g.mu.Unlock()

	if cfg.Guard.Alerts.Log {
		log.Printf("cpa-guard signal: code=%s severity=%s model=%s requested=%s auth=%s request=%s",
			s.Code, s.Severity, s.Model, s.RequestedModel, s.AuthID, s.RequestID)
	}
	if cfg.Guard.Alerts.Webhook.Enabled && severityRank[s.Severity] >= severityRank[cfg.Guard.Alerts.MinSeverity] {
		g.webhook.Enqueue(s)
	}
}

func (g *Guard) noteCacheMiss(cfg *config.Config, requestID string, p protocol.Profile) {
	key := p.SessionID + "|" + p.Model + "|" + p.PromptCacheKey
	g.mu.Lock()
	g.cacheMiss[key]++
	n := g.cacheMiss[key]
	g.mu.Unlock()
	if n >= cfg.Guard.CacheMissThreshold {
		g.emit(Signal{
			Severity: SeverityMedium, Code: "cache_miss_streak", RequestID: requestID,
			Model: p.Model, Detail: map[string]any{"consecutive_misses": n, "cache_key": p.PromptCacheKey},
		})
	}
}

func (g *Guard) clearCacheMiss(p protocol.Profile) {
	key := p.SessionID + "|" + p.Model + "|" + p.PromptCacheKey
	g.mu.Lock()
	delete(g.cacheMiss, key)
	g.mu.Unlock()
}

func (g *Guard) evictProfilesLocked(max int) {
	if max <= 0 || len(g.profiles) <= max {
		return
	}
	cut := time.Now().Add(-10 * time.Minute)
	for id, e := range g.profiles {
		if e.created.Before(cut) {
			delete(g.profiles, id)
		}
	}
	if len(g.profiles) <= max {
		return
	}
	ids := make([]string, 0, len(g.profiles))
	for id := range g.profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids[:len(ids)-max] {
		delete(g.profiles, id)
	}
}

func (g *Guard) cleanupLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-t.C:
			cfg := g.cfg()
			max := 4000
			if cfg != nil && cfg.Metrics.MaxSignals > 0 {
				max = cfg.Metrics.MaxSignals * 4
			}
			g.mu.Lock()
			g.evictProfilesLocked(max)
			g.mu.Unlock()
		}
	}
}

func signalEnabled(s config.SignalToggles, code string) bool {
	switch code {
	case "model_mismatch":
		return s.ModelMismatch
	case "reasoning_stripped":
		return s.ReasoningStripped
	case "source_param_dropped":
		return s.SourceParamDropped
	case "cache_miss_streak":
		return s.CacheMissStreak
	case "service_tier_downgraded":
		return s.ServiceTierDowngraded
	case "system_fingerprint_changed":
		return s.SystemFingerprintChanged
	case "cache_miss_reason":
		return s.CacheMissReason
	case "usage_invariant_violation":
		return s.UsageInvariantViolation
	case "tools_changed":
		return s.ToolsChanged
	case "system_prompt_changed":
		return s.SystemPromptChanged
	case "empty_output":
		return s.EmptyOutput
	case "truncated":
		return s.Truncated
	case "usage_missing":
		return s.UsageMissing
	case "ttft_anomaly":
		return s.TTFTAnomaly
	default:
		return true
	}
}

func sessionID(req pluginapi.RequestInterceptRequest) string {
	for _, key := range []string{"execution_session_id", "canonical_session_id", "session_id"} {
		if v, ok := req.Metadata[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	for _, key := range []string{"X-Session-Id", "Session-Id", "X-Conversation-Id", "Conversation-Id"} {
		if v := req.Headers.Get(key); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func cacheKeyPath(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "openai-response", "openai_responses", "responses", "codex":
		return "prompt_cache_key"
	case "openai", "chat":
		return "prompt_cache_key"
	default:
		return ""
	}
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		if seen[v] == 0 {
			return false
		}
		seen[v]--
	}
	return true
}
