# Architecture

## Runtime shape

RelayWarden is a Go `-buildmode=c-shared` plugin loaded by CLIProxyAPI v8. The
host resolves `cliproxy_plugin_init`, then exchanges JSON envelopes over a single
`cliproxyPluginCall` entry point. Registration returns the lower of the host's
advertised schema and `pluginabi.SchemaVersion`, with 7 as the fallback, so the
same DLL loads on older v8 builds. The plugin declares six capabilities:

- `request_interceptor`
- `request_lifecycle_plugin`
- `response_interceptor`
- `usage_plugin`
- `scheduler`
- `management_api`

It does not register an executor, a model provider, a thinking applier, or a
stream chunk interceptor.

## Request path

```text
client request
  -> model router / provider resolution
  -> request.intercept_before      source JSON (openai / openai-response / codex / gemini)
       guard.Before                 profile + optional prompt_cache_key
       protocol.Compress            tail-only tool-output walk
  -> auth selection                scheduler.pick
  -> request.intercept_after       still source JSON; ToFormat is set
       guard.After                  restore fields lost from the source body
  -> executor translation
       native translator -> normalizers -> thinking apply -> payload rules
  -> upstream
  -> response.intercept_after      non-stream response only
       guard.ObserveResponse        model, reasoning, cache, fingerprint, truncation
  -> usage.handle                  guard.ObserveUsage + routing.Observe
  -> request.complete              guard.Complete
```

Important: `request.intercept_after` does not see the provider payload. The
provider translation happens inside the executor after the hook. Provider-level
enforcement would require a `request_normalizer`; v1 does not register one
because it has no request ID or headers and cannot correlate per-request intent.

## State

- `config.Store`: immutable `*Config` snapshot in `atomic.Pointer`; revision
  counter increments on reconfigure. Handlers read without locking.
- `guard.Guard`: bounded profile map keyed by `RequestID`, signal ring buffer,
  counters, cache-miss streak map, fingerprint map, TTFT baselines, optional
  webhook queue.
- `routing.Stats`: per `authID + model` rolling windows, session affinity map,
  degradation counters.
- `metrics.Store`: compression counters by filter/model/format, plus shadow
  counters.

All maps are bounded and TTL-cleaned. Host calls may be concurrent; every
handler is thread-safe.

## Failure isolation

- Every optional path fails open: parse error, filter panic, encode error, or a
  larger result returns the original body.
- Plugin panics are recovered by the host and fuse the plugin until reload; code
  still uses defensive recovery inside filters.
- Webhook delivery is asynchronous, bounded (256), and drops on overflow.
- `guard.observe_only` is forced true in config normalization.

## Configuration

The host serializes `plugins.configs.relaywarden` as YAML, base64-encodes it into
the `config_yaml` field, and sends it on `plugin.register` and
`plugin.reconfigure`. The plugin parses it into `config.Config`, applies
defaults for omitted fields, clamps ranges, validates enums and webhook URLs, and
swaps the snapshot.

The management UI writes switches through CPA v8:

```text
PATCH /v8/management/config/plugins/configs/relaywarden
```

CPA persists `config.yaml` and triggers `plugin.reconfigure`. The UI polls
`/v0/management/relaywarden/status` until `config_revision` changes.

## Management UI

- Static React/Vite build embedded with `go:embed web/dist`.
- Served as a CPA resource route at `/v0/resource/plugins/relaywarden/ui/`.
- The resource page contains no data. The browser asks for the management key
  and keeps it in `sessionStorage`; every API call sends it as a Bearer token.
- CSP: `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'
  data:; connect-src 'self'; frame-ancestors 'self'; base-uri 'none';
  form-action 'self'`. `'self'` is required because the CPA management panel
  embeds this page in a same-origin iframe; cross-origin framing stays blocked.
- No external fonts, analytics, or CDN requests.

## Performance notes

- Request/response bodies cross the C ABI as base64 JSON. A 1 MB body expands by
  roughly 33% for each interceptor call. The plugin avoids extra full-body
  passes and does not intercept stream chunks.
- Compression is O(n) over the body and only walks tail items. Filters are
  string operations with bounded output.
- Scheduler and usage handlers are O(1) with small maps.

## Source map

```text
main.go                       C ABI exports, method dispatch, registration
internal/config               config schema, defaults, validation, snapshot
internal/protocol             source-format walkers and request profiles
internal/compress             filters, safety checks, markers
internal/guard                profiles, detection, signals, webhook
internal/routing              latency windows, affinity, scheduler pick
internal/metrics              compression counters
internal/management           management routes and static UI serving
web/                          React + Vite management UI
```
