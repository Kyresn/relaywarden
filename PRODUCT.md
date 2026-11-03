# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Go C ABI dynamic-library plugin for CLIProxyAPI v8, with a React + TypeScript + Vite management UI embedded through `go:embed` and served from `/v0/resource/plugins/cpa-guard/`.

## Users

A solo developer running CLIProxyAPI on a local Windows machine. They use Codex, Gemini CLI, and OpenAI-compatible clients, often through relays or multiple upstream accounts. They work at the same desk as the gateway, need to understand why a session feels slower or less capable, and want to change protections without editing YAML by hand.

## Product Purpose

CPA Guard sits inside CLIProxyAPI's request path and makes three things visible and controllable: how many tokens tool-output compression saves, whether a relay or protocol conversion is degrading the model response, and which upstream account is fastest for the current session. Success means the operator can answer "is this request being degraded, how much did we save, and which account should serve the next turn" in seconds, then flip the relevant feature on or off.

## Positioning

Not another gateway. CPA Guard is a plugin-level signal desk for an existing gateway: compression, quality-fidelity observation, and latency-aware account selection share one configuration surface and one event stream. Every feature is independently switchable, and the UI never hides whether a protection is active, shadowing, or off.

## Operating Context

- CLIProxyAPI v8.0.51 on Windows, loaded as `plugins/cpa-guard.dll`.
- Clients: Codex / OpenAI Responses, Gemini CLI / Gemini API, and generic OpenAI-compatible clients.
- Upstreams: native provider accounts and third-party relays.
- The management UI runs in the same browser origin as CPA, behind the CPA management key.
- The operator is usually alone, on a local network, and expects changes to persist in `config.yaml`.

## Capabilities and Constraints

- Tool-output compression before the request leaves the gateway, with shadow mode and per-filter switches.
- Passive fidelity checks for model identity, reasoning/thinking presence, prompt-cache behavior, tool preservation, usage invariants, and truncation.
- Observe-only degradation handling in v1: record, aggregate, and alert; do not reroute or retry automatically.
- Cache-affinity plus latency-aware auth selection through the CPA scheduler hook.
- Every feature has a switch; every switch is readable and writable from the management UI through the CPA v8 config API.
- The UI is offline-capable: no external fonts, analytics, or CDN assets. The management key lives in `sessionStorage`, never on disk. UI preferences (language and the last config change) live in `localStorage`; they contain no secrets.
- The plugin must fail open. A compression or observation error must never fail a user request.
- v1 does not cover Claude/Anthropic client protocols, prompt-style compression, streaming per-chunk integrity checks, automatic fallback, or hedging.

## Brand Commitments

Working name: CPA Guard. Voice: plain, precise, operational. Controls name their effect; errors name the problem and the recovery. No hype, no mascot, no gamification.

## Evidence on Hand

No production traffic corpus, benchmarks, or customer evidence exists yet. The compression research document and benchmark fixtures are the first evidence to produce; savings numbers from dsh and OmniRoute are upstream claims, not CPA Guard results. Do not present them as measured CPA Guard performance.

## Product Principles

1. Observe before optimizing. Shadow mode and measurement precede any default that changes what the model reads.
2. Fail open, never fail a turn. Compression and inspection are optional; the request is not.
3. Every protection is a switch. If the operator cannot see it and turn it off, it is not finished.
4. Preserve the cacheable prefix. Compression happens at the tail; history and system context stay byte-stable.
5. Separate evidence from inference. A signal is logged with its confidence and never dressed as proof of model substitution.

## Accessibility & Inclusion

Keyboard-operable switches and tables, visible focus rings, text labels alongside color-coded states, contrast at or above 4.5:1 for body text, and reduced-motion support. The UI must remain usable at 390px width and at 200% zoom.
