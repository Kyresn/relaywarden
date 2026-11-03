// Local development mock for the CPA management API. Not shipped with the plugin.
import { createServer } from "node:http";

let config = {
  enabled: true,
  priority: 10,
  compression: {
    enabled: true,
    mode: "balanced",
    shadow: true,
    tail_only: true,
    tail_items: 3,
    min_bytes: 500,
    max_bytes: 10485760,
    max_chars_per_result: 12000,
    max_lines_per_result: 120,
    dedup_threshold: 3,
    protocols: { openai_chat: true, openai_responses: true, gemini: true },
    filters: {
      git_diff: true, git_status: true, git_log: true, grep: true, find: true, ls: true,
      tree: true, build_output: true, test_output: true, dedup_log: true,
      smart_truncate: true, read_numbered: true, json_minify: true, strip_ansi: true,
    },
  },
  guard: {
    enabled: true, observe_only: true, reinject_source_params: true, inject_prompt_cache_key: false,
    check_model: true, check_reasoning: true, check_cache: true, check_tools: true,
    check_params: true, check_fingerprint: true, check_usage: true, check_cache_reason: true,
    check_truncation: true, check_empty: true, check_ttft: false,
    cache_miss_threshold: 3, prompt_token_threshold: 2048,
    signals: {
      model_mismatch: true, reasoning_stripped: true, source_param_dropped: true,
      cache_miss_streak: true, service_tier_downgraded: true, system_fingerprint_changed: true,
      cache_miss_reason: true, usage_invariant_violation: true, tools_changed: true,
      system_prompt_changed: false, empty_output: true, truncated: true, usage_missing: true,
      ttft_anomaly: false,
    },
    alerts: { log: true, min_severity: "medium", cooldown_seconds: 300, webhook: { enabled: false, url: "" } },
  },
  routing: {
    enabled: true, strategy: "latency_cache", cache_affinity: true, window: 20,
    ttl_seconds: 900, min_samples: 3, affinity_ttl_seconds: 1800,
    weights: { ttft: 0.6, error: 0.25, degrade: 0.15 },
  },
  metrics: { enabled: true, max_signals: 1000 },
  ui: { enabled: true, poll_interval_ms: 2000, show_advanced: false },
};

const now = Date.now();
const signals = [
  { time: new Date(now - 40_000).toISOString(), severity: "high", code: "reasoning_stripped", request_id: "8f3a12d4", model: "gpt-5.5", requested_model: "gpt-5.5", auth_id: "codex-main", provider: "codex", detail: { format: "responses", requested_effort: "high", reasoning_tokens: 0 } },
  { time: new Date(now - 90_000).toISOString(), severity: "critical", code: "model_mismatch", request_id: "b21c90aa", model: "claude-sonnet-5.5", requested_model: "claude-sonnet-5.5", response_model: "claude-haiku-4.5", auth_id: "relay-b", provider: "openai", detail: { expected: "claude-sonnet-5.5", got: "claude-haiku-4.5" } },
  { time: new Date(now - 160_000).toISOString(), severity: "medium", code: "cache_miss_streak", request_id: "11aa77bc", model: "gemini-3-pro", auth_id: "gemini-a", provider: "gemini", detail: { consecutive_misses: 3, cache_key: "relaywarden-4f2a" } },
  { time: new Date(now - 240_000).toISOString(), severity: "high", code: "source_param_dropped", request_id: "99dd01ff", model: "gpt-5.5", auth_id: "codex-main", provider: "codex", detail: { fields: ["reasoning_effort"], format: "openai" } },
  { time: new Date(now - 320_000).toISOString(), severity: "low", code: "truncated", request_id: "5c7b22e1", model: "gemini-3-pro", auth_id: "gemini-a", provider: "gemini", detail: { reason: "MAX_TOKENS" } },
];

const status = {
  loaded: true, version: "0.1.0", started_at: new Date(now - 3_600_000).toISOString(),
  config_revision: 12, enabled: true,
  compression: { enabled: true, mode: "balanced", shadow: true, tail_only: true, saved: 4_812_000, saved_pct: 38.2, shadow_saved: 1_200_000 },
  guard: { enabled: true, observe_only: true, signals: { reasoning_stripped: 3, model_mismatch: 1, cache_miss_streak: 2, source_param_dropped: 1, truncated: 1 } },
  routing: { enabled: true, strategy: "latency_cache", auths: 3 },
  ui: { enabled: true, poll_interval_ms: 2000, show_advanced: false },
};

const stats = {
  total: { requests: 1284, blocks: 2310, bytes_before: 12_600_000, bytes_after: 7_788_000 },
  shadow: { requests: 0, blocks: 0, bytes_before: 0, bytes_after: 0 },
  by_filter: [
    { name: "git-diff", requests: 0, blocks: 1204, bytes_before: 7_200_000, bytes_after: 2_736_000 },
    { name: "grep", requests: 0, blocks: 892, bytes_before: 3_100_000, bytes_after: 2_139_000 },
    { name: "build-output", requests: 0, blocks: 311, bytes_before: 1_800_000, bytes_after: 828_000 },
    { name: "read-numbered", requests: 0, blocks: 44, bytes_before: 500_000, bytes_after: 440_000 },
  ],
  by_model: [],
  by_format: [],
};

const auths = [
  { auth_id: "codex-main", model: "gpt-5.5", samples: 42, avg_ttft_ms: 412, p95_ttft_ms: 980, error_rate: 0.008, degrade_rate: 0.02, signals: 2 },
  { auth_id: "gemini-a", model: "gemini-3-pro", samples: 31, avg_ttft_ms: 688, p95_ttft_ms: 1420, error_rate: 0.014, degrade_rate: 0.06, signals: 3 },
  { auth_id: "relay-b", model: "claude-sonnet-5.5", samples: 18, avg_ttft_ms: 1100, p95_ttft_ms: 2600, error_rate: 0.062, degrade_rate: 0.22, signals: 4 },
];

function json(res, code, body) {
  const raw = JSON.stringify(body);
  res.writeHead(code, { "content-type": "application/json", "cache-control": "no-store" });
  res.end(raw);
}

createServer((req, res) => {
  const url = new URL(req.url ?? "/", "http://127.0.0.1:8317");
  const p = url.pathname;
  if (p === "/v0/management/relaywarden/status") return json(res, 200, status);
  if (p === "/v0/management/relaywarden/compression/stats") return json(res, 200, stats);
  if (p === "/v0/management/relaywarden/signals") return json(res, 200, { signals });
  if (p === "/v0/management/relaywarden/signals/summary") return json(res, 200, { counters: status.guard.signals });
  if (p === "/v0/management/relaywarden/routing") return json(res, 200, { auths });
  if (p === "/v0/management/relaywarden/compression/preview") {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const parsed = JSON.parse(body || "{}");
      const text = String(parsed.text ?? "");
      const out = text.length > 120 ? text.slice(0, 80) + "\n... [middle omitted] ...\n" + text.slice(-40) + "\n[relaywarden: compressed; filter=grep; full output omitted]" : text;
      json(res, 200, { enabled: true, shadow: config.compression.shadow, filter: text.length > 120 ? "grep" : "", before: text.length, after: out.length, saved: text.length - out.length, output: out });
    });
    return;
  }
  if (p === "/v8/management/config/plugins/configs/relaywarden") {
    if (req.method === "GET") return json(res, 200, config);
    if (req.method === "PATCH") {
      let body = "";
      req.on("data", (c) => (body += c));
      req.on("end", () => {
        config = merge(config, JSON.parse(body || "{}"));
        status.config_revision += 1;
        json(res, 200, config);
      });
      return;
    }
  }
  json(res, 404, { error: { code: "not_found", message: p } });
}).listen(8317, "127.0.0.1", () => console.log("mock CPA management API on http://127.0.0.1:8317"));

function merge(a, b) {
  if (Array.isArray(b)) return b;
  if (b && typeof b === "object") {
    const out = { ...(a && typeof a === "object" ? a : {}) };
    for (const [k, v] of Object.entries(b)) out[k] = merge(out[k], v);
    return out;
  }
  return b;
}
