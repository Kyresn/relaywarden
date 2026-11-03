// Package routing keeps per-auth latency/health windows and implements the
// cache-affinity + latency-aware scheduler decision.
package routing

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"cpa-guard/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	maxKeys       = 500
	maxAffinities = 10000
)

type sample struct {
	ttft  time.Duration
	ok    bool
	at    time.Time
	model string
}

type authWindow struct {
	samples []sample
	signals int
}

type affinity struct {
	authID  string
	expires time.Time
}

// Stats is the in-memory routing state.
type Stats struct {
	mu        sync.Mutex
	windows   map[string]*authWindow
	affinity  map[string]affinity
	lastEvict time.Time
}

func New() *Stats {
	return &Stats{
		windows:  make(map[string]*authWindow),
		affinity: make(map[string]affinity),
	}
}

// Observe records one completed request for an auth/model pair.
func (s *Stats) Observe(rec pluginapi.UsageRecord, cfg *config.RoutingConfig) {
	if s == nil || rec.AuthID == "" {
		return
	}
	ttft := rec.TTFT
	if ttft <= 0 {
		ttft = rec.Latency
	}
	now := time.Now()
	key := authKey(rec.AuthID, rec.Model)
	s.mu.Lock()
	w := s.windows[key]
	if w == nil {
		w = &authWindow{}
		s.windows[key] = w
	}
	w.samples = append(w.samples, sample{ttft: ttft, ok: !rec.Failed, at: now, model: rec.Model})
	limit := 20
	ttl := 15 * time.Minute
	if cfg != nil {
		if cfg.Window > 0 {
			limit = cfg.Window
		}
		if cfg.TTLSeconds > 0 {
			ttl = time.Duration(cfg.TTLSeconds) * time.Second
		}
	}
	cut := now.Add(-ttl)
	first := 0
	for first < len(w.samples) && w.samples[first].at.Before(cut) {
		first++
	}
	if first > 0 {
		w.samples = append([]sample(nil), w.samples[first:]...)
	}
	if len(w.samples) > limit {
		w.samples = append([]sample(nil), w.samples[len(w.samples)-limit:]...)
	}
	if rec.SessionID != "" && cfg != nil && cfg.CacheAffinity {
		ttl := time.Duration(cfg.AffinityTTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}
		s.affinity[sessionKey(rec.SessionID, rec.Model)] = affinity{authID: rec.AuthID, expires: now.Add(ttl)}
	}
	s.evictLocked(now)
	s.mu.Unlock()
}

// NoteSignal records one degradation signal for an auth/model pair.
func (s *Stats) NoteSignal(authID, model string) {
	if s == nil || authID == "" {
		return
	}
	s.mu.Lock()
	w := s.windows[authKey(authID, model)]
	if w == nil {
		w = &authWindow{}
		s.windows[authKey(authID, model)] = w
	}
	w.signals++
	s.mu.Unlock()
}

// Pick returns a selected auth ID. ok=false means the caller should fall back
// to the host's built-in scheduler.
func (s *Stats) Pick(req pluginapi.SchedulerPickRequest, cfg *config.RoutingConfig) (string, bool) {
	if s == nil || cfg == nil || !cfg.Enabled || cfg.Strategy == "off" || len(req.Candidates) == 0 {
		return "", false
	}
	now := time.Now()
	session := requestSession(req)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Cache affinity: keep a healthy session on its previous auth unless it is
	// clearly slower than the best candidate.
	if cfg.CacheAffinity && cfg.Strategy != "latency_only" && session != "" {
		if a, ok := s.affinity[sessionKey(session, req.Model)]; ok && a.expires.After(now) {
			for _, c := range req.Candidates {
				if c.ID != a.authID || !candidateUsable(c) {
					continue
				}
				bound := s.scoreForLocked(a.authID, req.Model, req.Candidates, cfg)
				best := s.bestLocked(req.Candidates, req.Model, cfg)
				if bound.samples < cfg.MinSamples || best.samples == 0 || bound.score <= best.score+0.15 {
					return a.authID, true
				}
			}
		}
	}

	bestID, bestScore := "", math.MaxFloat64
	warm := false
	for _, c := range req.Candidates {
		if !candidateUsable(c) {
			continue
		}
		st := s.scoreForLocked(c.ID, req.Model, req.Candidates, cfg)
		if st.samples >= cfg.MinSamples {
			warm = true
		}
		if st.score < bestScore || (st.score == bestScore && c.ID < bestID) {
			bestScore = st.score
			bestID = c.ID
		}
	}
	if !warm || bestID == "" {
		return "", false
	}
	return bestID, true
}

type summary struct {
	authID  string
	samples int
	score   float64
	avgTTFT time.Duration
	p95TTFT time.Duration
	errRate float64
	degrade float64
}

func (s *Stats) summarizeLocked(authID, model string, cfg *config.RoutingConfig) summary {
	w := s.windows[authKey(authID, model)]
	if w == nil {
		return summary{score: 0.5}
	}
	var ttfts []time.Duration
	errs := 0
	for _, sm := range w.samples {
		if sm.ok {
			ttfts = append(ttfts, sm.ttft)
		} else {
			errs++
		}
	}
	out := summary{samples: len(w.samples)}
	if len(w.samples) > 0 {
		out.errRate = float64(errs) / float64(len(w.samples))
	}
	if len(ttfts) > 0 {
		var sum time.Duration
		for _, t := range ttfts {
			sum += t
		}
		out.avgTTFT = sum / time.Duration(len(ttfts))
		sorted := append([]time.Duration(nil), ttfts...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		idx := int(float64(len(sorted)-1) * 0.95)
		out.p95TTFT = sorted[idx]
	}
	if len(w.samples) > 0 {
		out.degrade = float64(w.signals) / float64(len(w.samples))
		if out.degrade > 1 {
			out.degrade = 1
		}
	}
	// Cold candidates get a neutral score so they are not punished for having
	// no samples; warm candidates are scored relative to the fastest.
	if out.samples < cfg.MinSamples || out.avgTTFT <= 0 {
		out.score = 0.5
		return out
	}
	out.score = out.errRate*cfg.Weights.Error + out.degrade*cfg.Weights.Degrade
	return out
}

func (s *Stats) bestLocked(candidates []pluginapi.SchedulerAuthCandidate, model string, cfg *config.RoutingConfig) summary {
	best := summary{score: math.MaxFloat64}
	for _, c := range candidates {
		if !candidateUsable(c) {
			continue
		}
		st := s.scoreForLocked(c.ID, model, candidates, cfg)
		if st.score < best.score || (st.score == best.score && c.ID < best.authID) {
			best = st
			best.authID = c.ID
		}
	}
	if best.score == math.MaxFloat64 {
		return summary{}
	}
	return best
}

func (s *Stats) scoreForLocked(authID, model string, candidates []pluginapi.SchedulerAuthCandidate, cfg *config.RoutingConfig) summary {
	st := s.summarizeLocked(authID, model, cfg)
	st.authID = authID
	if st.samples < cfg.MinSamples || st.avgTTFT <= 0 {
		st.score = 0.5
		return st
	}
	fastest := time.Duration(0)
	for _, c := range candidates {
		if !candidateUsable(c) {
			continue
		}
		other := s.summarizeLocked(c.ID, model, cfg)
		if other.samples >= cfg.MinSamples && other.avgTTFT > 0 && (fastest == 0 || other.avgTTFT < fastest) {
			fastest = other.avgTTFT
		}
	}
	if fastest <= 0 {
		st.score = 0.5
		return st
	}
	ratio := float64(st.avgTTFT) / float64(fastest)
	norm := math.Min(1, math.Max(0, ratio-1))
	st.score = cfg.Weights.TTFT*norm + st.errRate*cfg.Weights.Error + st.degrade*cfg.Weights.Degrade
	return st
}

func (s *Stats) evictLocked(now time.Time) {
	if now.Sub(s.lastEvict) < time.Minute {
		return
	}
	s.lastEvict = now
	for k, a := range s.affinity {
		if a.expires.Before(now) {
			delete(s.affinity, k)
		}
	}
	if len(s.affinity) > maxAffinities {
		for k := range s.affinity {
			delete(s.affinity, k)
			if len(s.affinity) <= maxAffinities {
				break
			}
		}
	}
	if len(s.windows) > maxKeys {
		cut := now.Add(-30 * time.Minute)
		for k, w := range s.windows {
			if len(w.samples) == 0 || w.samples[len(w.samples)-1].at.Before(cut) {
				delete(s.windows, k)
			}
		}
	}
}

// AuthSnapshot is the management-facing view of one auth/model window.
type AuthSnapshot struct {
	AuthID    string  `json:"auth_id"`
	Model     string  `json:"model"`
	Samples   int     `json:"samples"`
	AvgTTFT   int64   `json:"avg_ttft_ms"`
	P95TTFT   int64   `json:"p95_ttft_ms"`
	ErrorRate float64 `json:"error_rate"`
	Degrade   float64 `json:"degrade_rate"`
	Signals   int     `json:"signals"`
}

func (s *Stats) Snapshot() []AuthSnapshot {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuthSnapshot, 0, len(s.windows))
	for key, w := range s.windows {
		authID, model := splitAuthKey(key)
		st := s.summarizeLocked(authID, model, &config.RoutingConfig{MinSamples: 1, Weights: config.Weights{TTFT: 0.6, Error: 0.25, Degrade: 0.15}})
		out = append(out, AuthSnapshot{
			AuthID: authID, Model: model, Samples: st.samples,
			AvgTTFT: msCeil(st.avgTTFT), P95TTFT: msCeil(st.p95TTFT),
			ErrorRate: st.errRate, Degrade: st.degrade, Signals: w.signals,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AuthID == out[j].AuthID {
			return out[i].Model < out[j].Model
		}
		return out[i].AuthID < out[j].AuthID
	})
	return out
}

func msCeil(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}

func candidateUsable(c pluginapi.SchedulerAuthCandidate) bool {
	switch strings.ToLower(strings.TrimSpace(c.Status)) {
	case "disabled", "unavailable", "cooling", "error", "banned", "expired":
		return false
	default:
		return c.ID != ""
	}
}

func requestSession(req pluginapi.SchedulerPickRequest) string {
	for _, key := range []string{"execution_session_id", "canonical_session_id", "session_id"} {
		if v, ok := req.Options.Metadata[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	for _, key := range []string{"X-Session-Id", "Session-Id", "X-Conversation-Id", "Conversation-Id"} {
		if vals, ok := req.Options.Headers[key]; ok && len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			return strings.TrimSpace(vals[0])
		}
	}
	return ""
}

func authKey(authID, model string) string {
	return authID + "\x00" + model
}

func splitAuthKey(key string) (string, string) {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return key, ""
}

func sessionKey(session, model string) string {
	return session + "\x00" + model
}
