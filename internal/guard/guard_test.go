package guard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func newGuard(t *testing.T, yaml string) *Guard {
	t.Helper()
	store, err := config.NewStore([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	g := New(store)
	t.Cleanup(g.Close)
	return g
}

func TestReinjectSourceParams(t *testing.T) {
	g := newGuard(t, `
guard:
  enabled: true
  reinject_source_params: true
compression:
  enabled: false
`)
	body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"high"},"max_output_tokens":4096}`)
	req := pluginapi.RequestInterceptRequest{RequestID: "r1", SourceFormat: "openai-response", Model: "gpt-5.5", Body: body}
	g.Before(req)
	after := []byte(`{"model":"gpt-5.5"}`)
	out := g.After(pluginapi.RequestInterceptRequest{RequestID: "r1", SourceFormat: "openai-response", ToFormat: "openai", Model: "gpt-5.5", Body: after})
	if len(out) == 0 {
		t.Fatal("expected restored body")
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["max_output_tokens"] != float64(4096) {
		t.Fatalf("max_output_tokens not restored: %s", out)
	}
	if !strings.Contains(string(out), `"effort":"high"`) {
		t.Fatalf("reasoning not restored: %s", out)
	}
}

func TestModelMismatchAndReasoningStripped(t *testing.T) {
	g := newGuard(t, `
guard:
  enabled: true
  signals:
    model_mismatch: true
    reasoning_stripped: true
compression:
  enabled: false
`)
	body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"high"}}`)
	g.Before(pluginapi.RequestInterceptRequest{RequestID: "r2", SourceFormat: "openai-response", Model: "gpt-5.5", Body: body})
	resp := []byte(`{"model":"gpt-4o-mini","usage":{"output_tokens":20,"output_tokens_details":{"reasoning_tokens":0}},"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}]}`)
	g.ObserveResponse(pluginapi.ResponseInterceptRequest{RequestID: "r2", SourceFormat: "openai-response", Model: "gpt-5.5", Body: resp})
	g.ObserveUsage(pluginapi.UsageRecord{
		RequestID: "r2", Model: "gpt-5.5", ResponseModel: "gpt-4o-mini",
		Detail: pluginapi.UsageDetail{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	})
	sum := g.Summary()
	if sum["model_mismatch"] == 0 {
		t.Fatalf("model_mismatch missing: %+v", sum)
	}
	if sum["reasoning_stripped"] == 0 {
		t.Fatalf("reasoning_stripped missing: %+v", sum)
	}
}

func TestCacheMissStreak(t *testing.T) {
	g := newGuard(t, `
guard:
  enabled: true
  cache_miss_threshold: 3
  prompt_token_threshold: 2048
  signals:
    cache_miss_streak: true
compression:
  enabled: false
`)
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"k1"}`)
	g.Before(pluginapi.RequestInterceptRequest{RequestID: "r3", SourceFormat: "openai-response", Model: "gpt-5.5", Body: body, Metadata: map[string]any{"execution_session_id": "s1"}})
	resp := []byte(`{"model":"gpt-5.5","usage":{"input_tokens":5000,"input_tokens_details":{"cached_tokens":0}}}`)
	for i := 0; i < 3; i++ {
		g.ObserveResponse(pluginapi.ResponseInterceptRequest{RequestID: "r3", SourceFormat: "openai-response", Model: "gpt-5.5", Body: resp})
	}
	if g.Summary()["cache_miss_streak"] == 0 {
		t.Fatalf("cache_miss_streak missing: %+v", g.Summary())
	}
}

func TestModelAliasAndTierMatching(t *testing.T) {
	if !modelsMatch("gpt-5.5", "gpt-5.5-2026-11-01") {
		t.Fatal("dated variant should match")
	}
	if !modelsMatchAny("gpt-4o", "gpt-4o", "mock-gpt") {
		t.Fatal("record model should match")
	}
	if modelsMatch("gpt-4o", "gpt-4o-mini") {
		t.Fatal("mini is a different tier, not a dated variant")
	}
}

func TestHealthyResponseNoSignals(t *testing.T) {
	g := newGuard(t, `
guard:
  enabled: true
compression:
  enabled: false
`)
	body := []byte(`{"model":"gpt-5.5","reasoning":{"effort":"high"},"prompt_cache_key":"k1"}`)
	g.Before(pluginapi.RequestInterceptRequest{RequestID: "r4", SourceFormat: "openai-response", Model: "gpt-5.5", Body: body})
	resp := []byte(`{"model":"gpt-5.5-2026-11-01","usage":{"input_tokens":5000,"output_tokens":100,"output_tokens_details":{"reasoning_tokens":80},"input_tokens_details":{"cached_tokens":4000}},"output":[{"type":"reasoning"},{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)
	g.ObserveResponse(pluginapi.ResponseInterceptRequest{RequestID: "r4", SourceFormat: "openai-response", Model: "gpt-5.5", Body: resp})
	if sum := g.Summary(); len(sum) != 0 {
		t.Fatalf("unexpected signals: %+v", sum)
	}
}
