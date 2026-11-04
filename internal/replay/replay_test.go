package replay

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kyresn/relaywarden/internal/config"
)

func enabledConfig() config.ReplayConfig {
	return config.ReplayConfig{
		Enabled:   true,
		Backend:   "memory",
		TTLHours:  24,
		MaxBytes:  512 << 20,
		Stream:    true,
		Nonstream: true,
	}
}

func TestFindMarkers(t *testing.T) {
	ref := NewRef()
	if len(ref) != RefLength {
		t.Fatalf("ref length = %d, want %d", len(ref), RefLength)
	}
	refs, cleaned := FindMarkers("hello\n\n" + Marker(ref))
	if len(refs) != 1 || refs[0] != ref {
		t.Fatalf("refs = %v", refs)
	}
	if cleaned != "hello" {
		t.Fatalf("cleaned = %q", cleaned)
	}
	if _, cleaned := FindMarkers("orphan " + MarkerPrefix + ref); cleaned != "orphan "+MarkerPrefix+ref {
		t.Fatalf("a lone marker must be left alone")
	}
}

func TestCaptureAndRestore(t *testing.T) {
	store := NewMemory()
	engine := New(enabledConfig(), store)

	resp := []byte(`{"output":[{"type":"reasoning","encrypted_content":"abc"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`)
	patched, res := engine.NormalizeResponse(resp, "openai-response", false)
	if res.Generated != 1 {
		t.Fatalf("generated = %d", res.Generated)
	}
	if !strings.Contains(string(patched), MarkerPrefix) {
		t.Fatalf("marker missing: %s", patched)
	}

	refs, _ := FindMarkers(gjsonString(patched, "output.1.content.0.text"))
	if len(refs) != 1 {
		t.Fatalf("marker not in assistant content: %s", patched)
	}

	req := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"go"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi\n\n` + Marker(refs[0]) + `"}]}]}`)
	restored, res := engine.NormalizeRequest(req, "openai-response")
	if res.Restored != 1 {
		t.Fatalf("restored = %d", res.Restored)
	}
	if strings.Contains(string(restored), MarkerPrefix) {
		t.Fatalf("marker survived restore: %s", restored)
	}
	if !strings.Contains(string(restored), `"encrypted_content":"abc"`) {
		t.Fatalf("reasoning item missing: %s", restored)
	}
	var parsed struct {
		Input []struct {
			Type string `json:"type"`
		} `json:"input"`
	}
	if err := json.Unmarshal(restored, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Input) != 3 || parsed.Input[1].Type != "reasoning" {
		t.Fatalf("reasoning item is not before the assistant message: %s", restored)
	}

	again, res := engine.NormalizeRequest(restored, "openai-response")
	if res.Restored != 0 || string(again) != string(restored) {
		t.Fatalf("second pass changed the body")
	}
}

func TestInvalidRefIsDropped(t *testing.T) {
	engine := New(enabledConfig(), NewMemory())
	req := []byte(`{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi ` + Marker("AAAAAAAAAAAAAAAAAAAAAAAAA") + `"}]}]}`)
	restored, res := engine.NormalizeRequest(req, "openai-response")
	if res.Invalid != 1 {
		t.Fatalf("invalid = %d", res.Invalid)
	}
	if strings.Contains(string(restored), MarkerPrefix) {
		t.Fatalf("invalid marker not removed: %s", restored)
	}
	if !strings.Contains(string(restored), `"hi "`) {
		t.Fatalf("assistant text was not preserved: %s", restored)
	}
}

func TestDisabledAndWrongFormatAreNoOps(t *testing.T) {
	cfg := enabledConfig()
	cfg.Enabled = false
	engine := New(cfg, NewMemory())
	body := []byte(`{"output":[{"type":"reasoning","encrypted_content":"abc"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`)
	out, res := engine.NormalizeResponse(body, "openai-response", false)
	if res.Generated != 0 || string(out) != string(body) {
		t.Fatalf("disabled engine changed the body")
	}
	on := New(enabledConfig(), NewMemory())
	out, res = on.NormalizeResponse(body, "openai", false)
	if res.Generated != 0 || string(out) != string(body) {
		t.Fatalf("non-responses format changed the body")
	}
}

func TestToolCallOnlyTurn(t *testing.T) {
	store := NewMemory()
	engine := New(enabledConfig(), store)
	resp := []byte(`{"output":[{"type":"reasoning","encrypted_content":"abc"},{"type":"function_call","name":"run","arguments":"{}"}]}`)
	patched, res := engine.NormalizeResponse(resp, "openai-response", false)
	if res.Generated != 1 {
		t.Fatalf("generated = %d", res.Generated)
	}
	var parsed struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(patched, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Output) != 3 || parsed.Output[2].Type != "message" {
		t.Fatalf("marker shell not appended: %s", patched)
	}
	refs, _ := FindMarkers(parsed.Output[2].Content[0].Text)
	if len(refs) != 1 {
		t.Fatalf("shell has no marker: %s", patched)
	}

	req := []byte(`{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + Marker(refs[0]) + `"}]},{"type":"function_call","name":"run","arguments":"{}"}]}`)
	restored, res := engine.NormalizeRequest(req, "openai-response")
	if res.Restored != 1 {
		t.Fatalf("restored = %d", res.Restored)
	}
	if strings.Contains(string(restored), `"role":"assistant"`) {
		t.Fatalf("empty marker shell was not dropped: %s", restored)
	}
	if !strings.Contains(string(restored), `"type":"reasoning"`) {
		t.Fatalf("reasoning item missing: %s", restored)
	}
}

func TestStreamingIsSkipped(t *testing.T) {
	engine := New(enabledConfig(), NewMemory())
	body := []byte(`{"output":[{"type":"reasoning","encrypted_content":"abc"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}]}`)
	out, res := engine.NormalizeResponse(body, "openai-response", true)
	if res.Generated != 0 || string(out) != string(body) {
		t.Fatalf("streaming must be skipped in this ticket")
	}
}

func TestMemoryStoreExpires(t *testing.T) {
	store := NewMemory()
	store.Put(Entry{Ref: "r1", ExpiresAt: store.now().Add(-1)})
	if _, ok := store.Get("r1"); ok {
		t.Fatal("expired entry must not be returned")
	}
}

func TestMultiPartContentIsPreserved(t *testing.T) {
	engine := New(enabledConfig(), NewMemory())
	resp := []byte(`{"output":[{"type":"reasoning","encrypted_content":"abc"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first"},{"type":"output_text","text":"second"}]}]}`)
	patched, res := engine.NormalizeResponse(resp, "openai-response", false)
	if res.Generated != 1 {
		t.Fatalf("generated = %d", res.Generated)
	}
	var parsed struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(patched, &parsed); err != nil {
		t.Fatal(err)
	}
	parts := parsed.Output[1].Content
	if len(parts) != 2 {
		t.Fatalf("part count = %d, want 2: %s", len(parts), patched)
	}
	if parts[0].Text != "first" {
		t.Fatalf("first part was rewritten: %q", parts[0].Text)
	}
	refs, _ := FindMarkers(parts[1].Text)
	if len(refs) != 1 {
		t.Fatalf("marker not on the last part: %q", parts[1].Text)
	}

	req := []byte(`{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first"},{"type":"output_text","text":"second\n\n` + Marker(refs[0]) + `"}]}]}`)
	restored, res := engine.NormalizeRequest(req, "openai-response")
	if res.Restored != 1 {
		t.Fatalf("restored = %d", res.Restored)
	}
	if !strings.Contains(string(restored), `"text":"first"`) {
		t.Fatalf("first part lost on restore: %s", restored)
	}
	if strings.Contains(string(restored), MarkerPrefix) {
		t.Fatalf("marker survived: %s", restored)
	}
}

func gjsonString(body []byte, path string) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	cur := v
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			idx := 0
			for _, c := range part {
				idx = idx*10 + int(c-'0')
			}
			if idx < len(node) {
				cur = node[idx]
			}
		}
	}
	s, _ := cur.(string)
	return s
}
