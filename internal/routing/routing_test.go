package routing

import (
	"testing"
	"time"

	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func candidates() []pluginapi.SchedulerAuthCandidate {
	return []pluginapi.SchedulerAuthCandidate{
		{ID: "auth-a", Provider: "openai", Status: "active"},
		{ID: "auth-b", Provider: "openai", Status: "active"},
	}
}

func cfg() *config.RoutingConfig {
	c := config.Default().Routing
	c.MinSamples = 1
	c.CacheAffinity = true
	return &c
}

func observe(s *Stats, auth string, ttft time.Duration, session string) {
	s.Observe(pluginapi.UsageRecord{
		RequestID: "r", AuthID: auth, Model: "gpt-5.5", TTFT: ttft, SessionID: session,
		Detail: pluginapi.UsageDetail{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
	}, cfg())
}

func TestPicksFasterAuth(t *testing.T) {
	s := New()
	for i := 0; i < 3; i++ {
		observe(s, "auth-a", 100*time.Millisecond, "")
		observe(s, "auth-b", 800*time.Millisecond, "")
	}
	id, ok := s.Pick(pluginapi.SchedulerPickRequest{Model: "gpt-5.5", Candidates: candidates()}, cfg())
	if !ok || id != "auth-a" {
		t.Fatalf("pick = %q ok=%v, want auth-a", id, ok)
	}
}

func TestColdStartFallsBack(t *testing.T) {
	s := New()
	if id, ok := s.Pick(pluginapi.SchedulerPickRequest{Model: "gpt-5.5", Candidates: candidates()}, cfg()); ok {
		t.Fatalf("cold start picked %q", id)
	}
}

func TestSessionAffinitySticks(t *testing.T) {
	s := New()
	observe(s, "auth-a", 110*time.Millisecond, "s1")
	observe(s, "auth-b", 100*time.Millisecond, "")
	req := pluginapi.SchedulerPickRequest{
		Model: "gpt-5.5", Candidates: candidates(),
		Options: pluginapi.SchedulerOptions{Metadata: map[string]any{"execution_session_id": "s1"}},
	}
	id, ok := s.Pick(req, cfg())
	if !ok || id != "auth-a" {
		t.Fatalf("affinity pick = %q ok=%v, want auth-a", id, ok)
	}
}

func TestDegradationAvoidsAuth(t *testing.T) {
	s := New()
	for i := 0; i < 3; i++ {
		observe(s, "auth-a", 100*time.Millisecond, "")
		observe(s, "auth-b", 120*time.Millisecond, "")
	}
	for i := 0; i < 20; i++ {
		s.NoteSignal("auth-a", "gpt-5.5")
	}
	id, ok := s.Pick(pluginapi.SchedulerPickRequest{Model: "gpt-5.5", Candidates: candidates()}, cfg())
	if !ok || id != "auth-b" {
		t.Fatalf("pick = %q ok=%v, want auth-b", id, ok)
	}
}

func TestUnusableCandidatesSkipped(t *testing.T) {
	s := New()
	observe(s, "auth-a", 100*time.Millisecond, "")
	observe(s, "auth-b", 200*time.Millisecond, "")
	cs := candidates()
	cs[0].Status = "disabled"
	id, ok := s.Pick(pluginapi.SchedulerPickRequest{Model: "gpt-5.5", Candidates: cs}, cfg())
	if !ok || id != "auth-b" {
		t.Fatalf("pick = %q ok=%v, want auth-b", id, ok)
	}
}
