export type FilterToggles = {
  git_diff: boolean;
  git_status: boolean;
  git_log: boolean;
  grep: boolean;
  find: boolean;
  ls: boolean;
  tree: boolean;
  build_output: boolean;
  test_output: boolean;
  dedup_log: boolean;
  smart_truncate: boolean;
  read_numbered: boolean;
  json_minify: boolean;
  strip_ansi: boolean;
};

export type SignalToggles = {
  model_mismatch: boolean;
  reasoning_stripped: boolean;
  source_param_dropped: boolean;
  cache_miss_streak: boolean;
  service_tier_downgraded: boolean;
  system_fingerprint_changed: boolean;
  cache_miss_reason: boolean;
  usage_invariant_violation: boolean;
  tools_changed: boolean;
  system_prompt_changed: boolean;
  empty_output: boolean;
  truncated: boolean;
  usage_missing: boolean;
  ttft_anomaly: boolean;
};

export type Config = {
  enabled: boolean;
  priority: number;
  compression: {
    enabled: boolean;
    mode: "safe" | "balanced" | "aggressive";
    shadow: boolean;
    tail_only: boolean;
    tail_items: number;
    min_bytes: number;
    max_bytes: number;
    max_chars_per_result: number;
    max_lines_per_result: number;
    dedup_threshold: number;
    protocols: { openai_chat: boolean; openai_responses: boolean; gemini: boolean };
    filters: FilterToggles;
  };
  guard: {
    enabled: boolean;
    observe_only: boolean;
    reinject_source_params: boolean;
    inject_prompt_cache_key: boolean;
    check_model: boolean;
    check_reasoning: boolean;
    check_cache: boolean;
    check_tools: boolean;
    check_params: boolean;
    check_fingerprint: boolean;
    check_usage: boolean;
    check_cache_reason: boolean;
    check_truncation: boolean;
    check_empty: boolean;
    check_ttft: boolean;
    cache_miss_threshold: number;
    prompt_token_threshold: number;
    signals: SignalToggles;
    alerts: {
      log: boolean;
      min_severity: "low" | "medium" | "high" | "critical";
      cooldown_seconds: number;
      webhook: { enabled: boolean; url: string };
    };
  };
  routing: {
    enabled: boolean;
    strategy: "latency_cache" | "latency_only" | "off";
    cache_affinity: boolean;
    window: number;
    ttl_seconds: number;
    min_samples: number;
    affinity_ttl_seconds: number;
    weights: { ttft: number; error: number; degrade: number };
  };
  metrics: { enabled: boolean; max_signals: number };
  caveman: { level: CavemanLevel };
  ponytail: { level: PonytailLevel };
  ui: { enabled: boolean; poll_interval_ms: number; show_advanced: boolean };
};

export type CavemanLevel = "off" | "lite" | "full" | "ultra" | "wenyan-lite" | "wenyan" | "wenyan-ultra";
export type PonytailLevel = "off" | "lite" | "full" | "ultra";

export type Status = {
  loaded: boolean;
  version: string;
  started_at: string;
  config_revision: number;
  enabled: boolean;
  compression: { enabled: boolean; mode: string; shadow: boolean; tail_only: boolean; saved: number; saved_pct: number; shadow_saved: number };
  guard: { enabled: boolean; observe_only: boolean; signals: Record<string, number> };
  routing: { enabled: boolean; strategy: string; auths: number };
  style: { caveman: string; ponytail: string; injected: number; stripped: number; failed: number };
  ui: { enabled: boolean; poll_interval_ms: number; show_advanced: boolean };
};

export type Counter = {
  requests: number;
  blocks: number;
  bytes_before: number;
  bytes_after: number;
};

export type NamedCounter = Counter & { name: string };

export type CompressionStats = {
  total: Counter;
  shadow: Counter;
  by_filter: NamedCounter[];
  by_model: NamedCounter[];
  by_format: NamedCounter[];
};

export type Signal = {
  time: string;
  severity: "low" | "medium" | "high" | "critical";
  code: string;
  request_id?: string;
  trace_id?: string;
  provider?: string;
  auth_id?: string;
  model?: string;
  requested_model?: string;
  response_model?: string;
  detail?: Record<string, unknown>;
};

export type AuthSnapshot = {
  auth_id: string;
  model: string;
  samples: number;
  avg_ttft_ms: number;
  p95_ttft_ms: number;
  error_rate: number;
  degrade_rate: number;
  signals: number;
};

export type DashboardData = {
  status: Status;
  stats: CompressionStats;
  signals: Signal[];
  auths: AuthSnapshot[];
  config: Config;
};
