package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func reset() {
	state = &pluginState{started: time.Now()}
}

func register(t *testing.T, yaml string) {
	t.Helper()
	reset()
	raw, err := json.Marshal(map[string]any{"config_yaml": []byte(yaml), "schema_version": 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handleMethod("plugin.register", raw); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func call(t *testing.T, method string, req any, out any) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	env, err := handleMethod(method, raw)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var e envelope
	if err := json.Unmarshal(env, &e); err != nil {
		t.Fatal(err)
	}
	if !e.OK {
		t.Fatalf("%s failed: %+v", method, e.Error)
	}
	if out != nil {
		if err := json.Unmarshal(e.Result, out); err != nil {
			t.Fatalf("%s result: %v", method, err)
		}
	}
}

func TestRegisterAndCompress(t *testing.T) {
	register(t, `
enabled: true
compression:
  enabled: true
  shadow: false
  min_bytes: 100
guard:
  enabled: true
routing:
  enabled: true
`)
	var grep strings.Builder
	for i := 0; i < 40; i++ {
		grep.WriteString("src/main.go:")
		grep.WriteString("1")
		grep.WriteString(":match content here\n")
	}
	body := `{"model":"gpt-5.5","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"find it"}]},` +
		`{"type":"function_call_output","call_id":"c1","output":` + quote(grep.String()) + `}` +
		`]}`
	var out pluginapi.RequestInterceptResponse
	call(t, "request.intercept_before", pluginapi.RequestInterceptRequest{
		RequestID: "r1", SourceFormat: "openai-response", Model: "gpt-5.5", Body: []byte(body),
	}, &out)
	if len(out.Body) == 0 {
		t.Fatalf("expected compressed body")
	}
	if !strings.Contains(string(out.Body), "[cpa-guard:") {
		t.Fatalf("marker missing: %s", out.Body)
	}
}

func TestModelMismatchSignal(t *testing.T) {
	register(t, `
enabled: true
guard:
  enabled: true
compression:
  enabled: false
`)
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	call(t, "request.intercept_before", pluginapi.RequestInterceptRequest{
		RequestID: "r2", SourceFormat: "openai-response", Model: "gpt-5.5", Body: body,
	}, nil)
	resp := []byte(`{"model":"gpt-4o-mini","usage":{"input_tokens":10,"output_tokens":2},"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}]}`)
	call(t, "response.intercept_after", pluginapi.ResponseInterceptRequest{
		RequestID: "r2", SourceFormat: "openai-response", Model: "gpt-5.5", Body: resp,
	}, nil)
	call(t, "usage.handle", pluginapi.UsageRecord{
		RequestID: "r2", Model: "gpt-5.5", ResponseModel: "gpt-4o-mini",
		Detail: pluginapi.UsageDetail{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
	}, nil)
	if state.guard == nil {
		t.Fatal("guard missing")
	}
	summary := state.guard.Summary()
	if summary["model_mismatch"] == 0 {
		t.Fatalf("model_mismatch not recorded: %+v", summary)
	}
}

func TestSchedulerPicksFasterAuth(t *testing.T) {
	register(t, `
enabled: true
routing:
  enabled: true
  strategy: latency_cache
  min_samples: 1
  weights: { ttft: 0.6, error: 0.25, degrade: 0.15 }
`)
	for i := 0; i < 4; i++ {
		call(t, "usage.handle", pluginapi.UsageRecord{
			RequestID: "u-a", AuthID: "auth-a", Model: "gpt-5.5", TTFT: 100 * time.Millisecond,
			Detail: pluginapi.UsageDetail{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
		}, nil)
		call(t, "usage.handle", pluginapi.UsageRecord{
			RequestID: "u-b", AuthID: "auth-b", Model: "gpt-5.5", TTFT: 800 * time.Millisecond,
			Detail: pluginapi.UsageDetail{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
		}, nil)
	}
	var pick pluginapi.SchedulerPickResponse
	call(t, "scheduler.pick", pluginapi.SchedulerPickRequest{
		Model: "gpt-5.5",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "auth-a", Provider: "openai", Status: "active"},
			{ID: "auth-b", Provider: "openai", Status: "active"},
		},
	}, &pick)
	if !pick.Handled || pick.AuthID != "auth-a" {
		t.Fatalf("pick = %+v, want auth-a", pick)
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
