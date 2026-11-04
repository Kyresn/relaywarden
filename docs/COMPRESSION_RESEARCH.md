# Compression research

Status: design research, not RelayWarden measurements. Numbers below are upstream
claims from dsh and OmniRoute unless marked otherwise. RelayWarden must produce its
own numbers before changing defaults.

## Sources read

- dsh `src/token-saver/rtk/*` and `src/token-saver/prompts.js`
  - `constants.js`: `RAW_CAP=10 MiB`, `MIN_COMPRESS_SIZE=500`, detect window
    1024 chars, hunk cap 100 lines, grep cap 10/file, find cap 10/dir + 20 dirs,
    status cap 10/section, smart truncate head 120 + tail 60, numbered dump
    ratio 0.7.
  - `index.js`: only tool output is touched; error results are skipped; a
    replacement must be non-empty and strictly smaller; every filter failure
    passes the original through.
- OmniRoute `docs/compression/*` and `open-sse/services/compression/*`
  - RTK: 56 filters, command/output detection, ANSI stripping, dedup threshold
    3, `maxLinesPerResult=120`, `maxCharsPerResult=12000`, preserved failures,
    warnings, summaries and tail context.
  - Caveman: prose filler removal, ~46% input compression claim.
  - Stacked RTK -> Caveman: 78–95% "eligible" range, not every request.
  - Cache-aware strategy: caching providers skip system-prompt rewriting and
    use deterministic-only compression; prompt-cache affinity is resolved by
    explicit `prompt_cache_key` or a prefix hash.

## Method matrix

| Method | Best input | Claimed saving | Risk | RelayWarden v1 |
|---|---|---|---|---|
| RTK command-aware filters | git, grep, find, ls, build/test logs | 60–90% on matched output | removes context the model needed | yes |
| ANSI/progress cleanup | any terminal output | 1–15% | almost none | yes, pre-pass |
| Duplicate-line collapse | repetitive logs | 10–90% | loses repetition count unless marked | yes, `[xN]` |
| JSON minify | structured tool output | 5–30% | none if valid JSON | yes |
| Head/tail truncation | huge dumps | 50–95% | loses middle | yes, with explicit marker |
| Caveman / ponytail behaviour prompts | system prompt, appended segment | output tokens; not yet measured | changes answer style | yes, default off |
| LLMLingua-2 pruning | long prose | medium | ONNX runtime, latency | no |
| CCR / session dedup | repeated large blocks | depends on repetition | needs retrieval contract | no |
| Semantic summarization | any | high | model call, latency, quality risk | no |

## Recommended default

`mode: balanced`, `shadow: true`, `tail_only: true`, all 14 tool-output filters
on, prose slimming off. The operator runs shadow mode on real traffic, reviews
the per-filter savings, then turns shadow off.

## Safety rules

1. Never touch system/instructions/tools/user/assistant text; only tool output. The one exception is the style module: it appends a marker-delimited instruction segment to the system text and never rewrites text already there.
2. Skip error results (`is_error`, `status: "error"`).
3. Skip blobs under 500 bytes or over 10 MiB.
4. Replacement must be non-empty and strictly smaller.
5. Error/failure lines in the compressed output must not be fewer than in the
   original; otherwise send the original.
6. Add a marker naming the original size, compressed size, filter, and that the
   full output was omitted.
7. Be idempotent: text containing the marker is not compressed again.
8. Fail open on every parse, filter, or encode error.

## Cache interaction

Prompt caching is prefix-based. Compressing history invalidates the prefix and
costs more than it saves. `tail_only` compresses only tool output after the last
user turn; system, tools, and earlier turns stay byte-stable. Compression output
is deterministic, so a retry sends the same bytes.

## Benchmark protocol

1. Capture 20–50 sanitized requests per protocol (OpenAI Chat, Responses/Codex,
   Gemini) with tool outputs.
2. Record bytes by content type, tool, and message position.
3. Run each filter in shadow mode; record bytes before/after, filter hit, time.
4. Replay a sample of compressed outputs and ask the model the same follow-up
   question; compare answers with the uncompressed control.
5. Promote a filter to default only if it saves at least 10% on its target
   content and produces no answer regression in the replay sample.
6. Benchmark 1 KB / 100 KB / 1 MB bodies; target p99 plugin overhead under 5 ms
   for 100 KB on the operator's machine.

No RelayWarden benchmark result exists yet. Do not quote upstream numbers as CPA
Guard performance.

## Local benchmark (this machine)

`go test ./internal/compress -run '^$' -bench . -benchtime=2s` on a Ryzen 7
9800X3D, Windows amd64, Go 1.27:

| Benchmark | Time per 100 KB |
|---|---|
| `BenchmarkCompressGrep100KB` | ~2.4 ms |
| `BenchmarkCompressGitDiff100KB` | ~0.06 ms |

These are filter-only measurements, not end-to-end CPA overhead. The C ABI body
base64 encoding and host JSON marshal are measured separately during integration.
