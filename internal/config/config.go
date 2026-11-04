package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// Config is the plugin-owned configuration subtree under plugins.configs.relaywarden.
type Config struct {
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	Priority    int               `yaml:"priority" json:"priority"`
	Compression CompressionConfig `yaml:"compression" json:"compression"`
	Guard       GuardConfig       `yaml:"guard" json:"guard"`
	Routing     RoutingConfig     `yaml:"routing" json:"routing"`
	Replay      ReplayConfig      `yaml:"replay" json:"replay"`
	Caveman     StyleConfig       `yaml:"caveman" json:"caveman"`
	Ponytail    StyleConfig       `yaml:"ponytail" json:"ponytail"`
	Metrics     MetricsConfig     `yaml:"metrics" json:"metrics"`
	UI          UIConfig          `yaml:"ui" json:"ui"`
}

type CompressionConfig struct {
	Enabled           bool            `yaml:"enabled" json:"enabled"`
	Mode              string          `yaml:"mode" json:"mode"`
	Shadow            bool            `yaml:"shadow" json:"shadow"`
	TailOnly          bool            `yaml:"tail_only" json:"tail_only"`
	TailItems         int             `yaml:"tail_items" json:"tail_items"`
	MinBytes          int             `yaml:"min_bytes" json:"min_bytes"`
	MaxBytes          int             `yaml:"max_bytes" json:"max_bytes"`
	MaxCharsPerResult int             `yaml:"max_chars_per_result" json:"max_chars_per_result"`
	MaxLinesPerResult int             `yaml:"max_lines_per_result" json:"max_lines_per_result"`
	DedupThreshold    int             `yaml:"dedup_threshold" json:"dedup_threshold"`
	Protocols         ProtocolToggles `yaml:"protocols" json:"protocols"`
	Filters           FilterToggles   `yaml:"filters" json:"filters"`
}

type ProtocolToggles struct {
	OpenAIChat      bool `yaml:"openai_chat" json:"openai_chat"`
	OpenAIResponses bool `yaml:"openai_responses" json:"openai_responses"`
	Gemini          bool `yaml:"gemini" json:"gemini"`
}

type FilterToggles struct {
	GitDiff       bool `yaml:"git_diff" json:"git_diff"`
	GitStatus     bool `yaml:"git_status" json:"git_status"`
	GitLog        bool `yaml:"git_log" json:"git_log"`
	Grep          bool `yaml:"grep" json:"grep"`
	Find          bool `yaml:"find" json:"find"`
	LS            bool `yaml:"ls" json:"ls"`
	Tree          bool `yaml:"tree" json:"tree"`
	BuildOutput   bool `yaml:"build_output" json:"build_output"`
	TestOutput    bool `yaml:"test_output" json:"test_output"`
	DedupLog      bool `yaml:"dedup_log" json:"dedup_log"`
	SmartTruncate bool `yaml:"smart_truncate" json:"smart_truncate"`
	ReadNumbered  bool `yaml:"read_numbered" json:"read_numbered"`
	JSONMinify    bool `yaml:"json_minify" json:"json_minify"`
	StripANSI     bool `yaml:"strip_ansi" json:"strip_ansi"`
}

type GuardConfig struct {
	Enabled              bool          `yaml:"enabled" json:"enabled"`
	ObserveOnly          bool          `yaml:"observe_only" json:"observe_only"`
	ReinjectSourceParams bool          `yaml:"reinject_source_params" json:"reinject_source_params"`
	InjectPromptCacheKey bool          `yaml:"inject_prompt_cache_key" json:"inject_prompt_cache_key"`
	CheckModel           bool          `yaml:"check_model" json:"check_model"`
	CheckReasoning       bool          `yaml:"check_reasoning" json:"check_reasoning"`
	CheckCache           bool          `yaml:"check_cache" json:"check_cache"`
	CheckTools           bool          `yaml:"check_tools" json:"check_tools"`
	CheckParams          bool          `yaml:"check_params" json:"check_params"`
	CheckFingerprint     bool          `yaml:"check_fingerprint" json:"check_fingerprint"`
	CheckUsage           bool          `yaml:"check_usage" json:"check_usage"`
	CheckCacheReason     bool          `yaml:"check_cache_reason" json:"check_cache_reason"`
	CheckTruncation      bool          `yaml:"check_truncation" json:"check_truncation"`
	CheckEmpty           bool          `yaml:"check_empty" json:"check_empty"`
	CheckTTFT            bool          `yaml:"check_ttft" json:"check_ttft"`
	CacheMissThreshold   int           `yaml:"cache_miss_threshold" json:"cache_miss_threshold"`
	PromptTokenThreshold int           `yaml:"prompt_token_threshold" json:"prompt_token_threshold"`
	Signals              SignalToggles `yaml:"signals" json:"signals"`
	Alerts               AlertsConfig  `yaml:"alerts" json:"alerts"`
}

type SignalToggles struct {
	ModelMismatch            bool `yaml:"model_mismatch" json:"model_mismatch"`
	ReasoningStripped        bool `yaml:"reasoning_stripped" json:"reasoning_stripped"`
	SourceParamDropped       bool `yaml:"source_param_dropped" json:"source_param_dropped"`
	CacheMissStreak          bool `yaml:"cache_miss_streak" json:"cache_miss_streak"`
	ServiceTierDowngraded    bool `yaml:"service_tier_downgraded" json:"service_tier_downgraded"`
	SystemFingerprintChanged bool `yaml:"system_fingerprint_changed" json:"system_fingerprint_changed"`
	CacheMissReason          bool `yaml:"cache_miss_reason" json:"cache_miss_reason"`
	UsageInvariantViolation  bool `yaml:"usage_invariant_violation" json:"usage_invariant_violation"`
	ToolsChanged             bool `yaml:"tools_changed" json:"tools_changed"`
	SystemPromptChanged      bool `yaml:"system_prompt_changed" json:"system_prompt_changed"`
	EmptyOutput              bool `yaml:"empty_output" json:"empty_output"`
	Truncated                bool `yaml:"truncated" json:"truncated"`
	UsageMissing             bool `yaml:"usage_missing" json:"usage_missing"`
	TTFTAnomaly              bool `yaml:"ttft_anomaly" json:"ttft_anomaly"`
}

type AlertsConfig struct {
	Log             bool          `yaml:"log" json:"log"`
	MinSeverity     string        `yaml:"min_severity" json:"min_severity"`
	CooldownSeconds int           `yaml:"cooldown_seconds" json:"cooldown_seconds"`
	Webhook         WebhookConfig `yaml:"webhook" json:"webhook"`
}

type WebhookConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	URL     string `yaml:"url" json:"url"`
}

type RoutingConfig struct {
	Enabled            bool    `yaml:"enabled" json:"enabled"`
	Strategy           string  `yaml:"strategy" json:"strategy"`
	CacheAffinity      bool    `yaml:"cache_affinity" json:"cache_affinity"`
	Window             int     `yaml:"window" json:"window"`
	TTLSeconds         int     `yaml:"ttl_seconds" json:"ttl_seconds"`
	MinSamples         int     `yaml:"min_samples" json:"min_samples"`
	AffinityTTLSeconds int     `yaml:"affinity_ttl_seconds" json:"affinity_ttl_seconds"`
	Weights            Weights `yaml:"weights" json:"weights"`
}

type Weights struct {
	TTFT    float64 `yaml:"ttft" json:"ttft"`
	Error   float64 `yaml:"error" json:"error"`
	Degrade float64 `yaml:"degrade" json:"degrade"`
}

// CavemanLevels and PonytailLevels are the accepted behaviour-prompt levels.
// "off" is a real level and means "inject nothing"; it is not the empty string.
var (
	CavemanLevels  = []string{"off", "lite", "full", "ultra", "wenyan-lite", "wenyan", "wenyan-ultra"}
	PonytailLevels = []string{"off", "lite", "full", "ultra"}
)

// ReplayConfig holds the reasoning-replay settings.
type ReplayConfig struct {
	Enabled             bool           `yaml:"enabled" json:"enabled"`
	Backend             string         `yaml:"backend" json:"backend"`
	AllowMemoryFallback bool           `yaml:"allow_memory_fallback" json:"allow_memory_fallback"`
	Postgres            PostgresConfig `yaml:"postgres" json:"postgres"`
	TTLHours            int            `yaml:"ttl_hours" json:"ttl_hours"`
	MaxBytes            int64          `yaml:"max_bytes" json:"max_bytes"`
	Stream              bool           `yaml:"stream" json:"stream"`
	Nonstream           bool           `yaml:"nonstream" json:"nonstream"`
}

// PostgresConfig points at the plugin's own database.
type PostgresConfig struct {
	DSN string `yaml:"dsn" json:"dsn"`
}

// StyleConfig holds one behaviour-prompt family. Level is the switch: "off"
// means inject nothing, so there is no separate enabled flag to contradict it.
type StyleConfig struct {
	Level string `yaml:"level" json:"level"`
}

type MetricsConfig struct {
	Enabled    bool `yaml:"enabled" json:"enabled"`
	MaxSignals int  `yaml:"max_signals" json:"max_signals"`
}

type UIConfig struct {
	Enabled        bool `yaml:"enabled" json:"enabled"`
	PollIntervalMS int  `yaml:"poll_interval_ms" json:"poll_interval_ms"`
	ShowAdvanced   bool `yaml:"show_advanced" json:"show_advanced"`
}

// Default returns the shipped defaults. Compression starts in shadow mode so a
// fresh install measures before it rewrites anything.
func Default() Config {
	return Config{
		Enabled:  true,
		Priority: 10,
		Compression: CompressionConfig{
			Enabled:           true,
			Mode:              "balanced",
			Shadow:            true,
			TailOnly:          true,
			TailItems:         3,
			MinBytes:          500,
			MaxBytes:          10 * 1024 * 1024,
			MaxCharsPerResult: 12000,
			MaxLinesPerResult: 120,
			DedupThreshold:    3,
			Protocols: ProtocolToggles{
				OpenAIChat:      true,
				OpenAIResponses: true,
				Gemini:          true,
			},
			Filters: FilterToggles{
				GitDiff:       true,
				GitStatus:     true,
				GitLog:        true,
				Grep:          true,
				Find:          true,
				LS:            true,
				Tree:          true,
				BuildOutput:   true,
				TestOutput:    true,
				DedupLog:      true,
				SmartTruncate: true,
				ReadNumbered:  true,
				JSONMinify:    true,
				StripANSI:     true,
			},
		},
		Guard: GuardConfig{
			Enabled:              true,
			ObserveOnly:          true,
			ReinjectSourceParams: true,
			CheckModel:           true,
			CheckReasoning:       true,
			CheckCache:           true,
			CheckTools:           true,
			CheckParams:          true,
			CheckFingerprint:     true,
			CheckUsage:           true,
			CheckCacheReason:     true,
			CheckTruncation:      true,
			CheckEmpty:           true,
			CacheMissThreshold:   3,
			PromptTokenThreshold: 2048,
			Signals: SignalToggles{
				ModelMismatch:            true,
				ReasoningStripped:        true,
				SourceParamDropped:       true,
				CacheMissStreak:          true,
				ServiceTierDowngraded:    true,
				SystemFingerprintChanged: true,
				CacheMissReason:          true,
				UsageInvariantViolation:  true,
				ToolsChanged:             true,
				EmptyOutput:              true,
				Truncated:                true,
				UsageMissing:             true,
			},
			Alerts: AlertsConfig{
				Log:             true,
				MinSeverity:     "medium",
				CooldownSeconds: 300,
			},
		},
		Routing: RoutingConfig{
			Enabled:            true,
			Strategy:           "latency_cache",
			CacheAffinity:      true,
			Window:             20,
			TTLSeconds:         900,
			MinSamples:         3,
			AffinityTTLSeconds: 1800,
			Weights:            Weights{TTFT: 0.60, Error: 0.25, Degrade: 0.15},
		},
		Metrics:  MetricsConfig{Enabled: true, MaxSignals: 1000},
		UI:       UIConfig{Enabled: true, PollIntervalMS: 2000},
		Caveman:  StyleConfig{Level: "off"},
		Ponytail: StyleConfig{Level: "off"},
		Replay: ReplayConfig{
			Backend:             "memory",
			AllowMemoryFallback: true,
			TTLHours:            24,
			MaxBytes:            512 << 20,
			Stream:              true,
			Nonstream:           true,
		},
	}
}

// Parse decodes a plugin config YAML subtree, applies defaults for omitted
// fields, and validates the result.
func Parse(raw []byte) (Config, error) {
	cfg := Default()
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse relaywarden config: %w", err)
		}
	}
	if err := cfg.Normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Normalize clamps numeric ranges and validates enums/URLs.
func (c *Config) Normalize() error {
	c.Compression.Mode = strings.ToLower(strings.TrimSpace(c.Compression.Mode))
	switch c.Compression.Mode {
	case "", "safe", "balanced", "aggressive":
		if c.Compression.Mode == "" {
			c.Compression.Mode = "balanced"
		}
	default:
		return fmt.Errorf("compression.mode %q is not safe|balanced|aggressive", c.Compression.Mode)
	}
	clampInt(&c.Compression.TailItems, 1, 20)
	clampInt(&c.Compression.MinBytes, 0, 10*1024*1024)
	clampInt(&c.Compression.MaxBytes, c.Compression.MinBytes, 64*1024*1024)
	clampInt(&c.Compression.MaxCharsPerResult, 256, 1024*1024)
	clampInt(&c.Compression.MaxLinesPerResult, 10, 5000)
	clampInt(&c.Compression.DedupThreshold, 2, 100)

	c.Guard.ObserveOnly = true
	c.Guard.Alerts.MinSeverity = strings.ToLower(strings.TrimSpace(c.Guard.Alerts.MinSeverity))
	switch c.Guard.Alerts.MinSeverity {
	case "", "low", "medium", "high", "critical":
		if c.Guard.Alerts.MinSeverity == "" {
			c.Guard.Alerts.MinSeverity = "medium"
		}
	default:
		return fmt.Errorf("guard.alerts.min_severity %q is not low|medium|high|critical", c.Guard.Alerts.MinSeverity)
	}
	clampInt(&c.Guard.CacheMissThreshold, 2, 100)
	clampInt(&c.Guard.PromptTokenThreshold, 256, 10_000_000)
	clampInt(&c.Guard.Alerts.CooldownSeconds, 0, 86400)
	if c.Guard.Alerts.Webhook.Enabled {
		u, err := url.Parse(strings.TrimSpace(c.Guard.Alerts.Webhook.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("guard.alerts.webhook.url must be an absolute http(s) URL when enabled")
		}
		c.Guard.Alerts.Webhook.URL = strings.TrimSpace(c.Guard.Alerts.Webhook.URL)
	}

	c.Routing.Strategy = strings.ToLower(strings.TrimSpace(c.Routing.Strategy))
	switch c.Routing.Strategy {
	case "", "latency_cache", "latency_only", "off":
		if c.Routing.Strategy == "" {
			c.Routing.Strategy = "latency_cache"
		}
	default:
		return fmt.Errorf("routing.strategy %q is not latency_cache|latency_only|off", c.Routing.Strategy)
	}
	clampInt(&c.Routing.Window, 3, 200)
	clampInt(&c.Routing.TTLSeconds, 60, 86400)
	clampInt(&c.Routing.MinSamples, 1, 100)
	clampInt(&c.Routing.AffinityTTLSeconds, 60, 86400*7)
	c.Routing.Weights = normalizeWeights(c.Routing.Weights)

	c.Replay.Backend = strings.ToLower(strings.TrimSpace(c.Replay.Backend))
	if c.Replay.Backend == "" {
		c.Replay.Backend = "memory"
	}
	switch c.Replay.Backend {
	case "memory", "postgres":
	default:
		return fmt.Errorf("replay.backend %q is not memory|postgres", c.Replay.Backend)
	}
	if c.Replay.Enabled && !c.Replay.Stream && !c.Replay.Nonstream {
		return errors.New("replay: stream and nonstream are both off, so replay would never run")
	}
	if c.Replay.Backend == "postgres" && strings.TrimSpace(c.Replay.Postgres.DSN) == "" {
		if !c.Replay.AllowMemoryFallback {
			return errors.New("replay.postgres.dsn is required when backend is postgres and memory fallback is off")
		}
		c.Replay.Backend = "memory"
	}
	clampInt(&c.Replay.TTLHours, 1, 24*30)
	if c.Replay.MaxBytes < 1<<20 {
		c.Replay.MaxBytes = 1 << 20
	}
	if c.Replay.MaxBytes > 64<<30 {
		c.Replay.MaxBytes = 64 << 30
	}

	c.Caveman.Level = strings.ToLower(strings.TrimSpace(c.Caveman.Level))
	if c.Caveman.Level == "" {
		c.Caveman.Level = "off"
	}
	if !containsString(CavemanLevels, c.Caveman.Level) {
		return fmt.Errorf("caveman.level %q is not %s", c.Caveman.Level, strings.Join(CavemanLevels, "|"))
	}
	c.Ponytail.Level = strings.ToLower(strings.TrimSpace(c.Ponytail.Level))
	if c.Ponytail.Level == "" {
		c.Ponytail.Level = "off"
	}
	if !containsString(PonytailLevels, c.Ponytail.Level) {
		return fmt.Errorf("ponytail.level %q is not %s", c.Ponytail.Level, strings.Join(PonytailLevels, "|"))
	}

	clampInt(&c.Metrics.MaxSignals, 10, 100000)
	clampInt(&c.UI.PollIntervalMS, 500, 60000)
	return nil
}

func normalizeWeights(w Weights) Weights {
	if w.TTFT < 0 {
		w.TTFT = 0
	}
	if w.Error < 0 {
		w.Error = 0
	}
	if w.Degrade < 0 {
		w.Degrade = 0
	}
	sum := w.TTFT + w.Error + w.Degrade
	if sum <= 0 {
		return Weights{TTFT: 0.60, Error: 0.25, Degrade: 0.15}
	}
	return Weights{TTFT: w.TTFT / sum, Error: w.Error / sum, Degrade: w.Degrade / sum}
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func clampInt(v *int, min, max int) {
	if *v < min {
		*v = min
	}
	if *v > max {
		*v = max
	}
}

// Store holds an immutable config snapshot. Handlers read it without locking.
type Store struct {
	cur atomic.Pointer[Config]
	rev atomic.Uint64
}

func NewStore(raw []byte) (*Store, error) {
	cfg, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	s := &Store{}
	s.cur.Store(&cfg)
	s.rev.Store(1)
	return s, nil
}

func (s *Store) Load() *Config {
	if s == nil {
		return nil
	}
	return s.cur.Load()
}

func (s *Store) Revision() uint64 {
	if s == nil {
		return 0
	}
	return s.rev.Load()
}

func (s *Store) Update(raw []byte) error {
	cfg, err := Parse(raw)
	if err != nil {
		return err
	}
	s.cur.Store(&cfg)
	s.rev.Add(1)
	return nil
}
