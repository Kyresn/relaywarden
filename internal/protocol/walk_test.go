package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func stubCompressor(text string) (string, string, bool) {
	if len(text) < 500 {
		return text, "", false
	}
	return "short-" + text[:20], "stub", true
}

func TestResponsesTailOnlyKeepsHistory(t *testing.T) {
	long := strings.Repeat("grep-hit ", 100)
	body := `{"input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"old"}]},` +
		`{"type":"function_call_output","call_id":"1","output":"` + long + `"},` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"new"}]},` +
		`{"type":"function_call_output","call_id":"2","output":"` + long + `"}` +
		`]}`
	res, err := Compress([]byte(body), "openai-response", true, 3, stubCompressor)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(res.Hits))
	}
	if !strings.Contains(string(res.Body), `"call_id":"1","output":"`+long+`"`) {
		t.Fatalf("older tool output was modified")
	}
	if !strings.Contains(string(res.Body), "short-grep-hit") {
		t.Fatalf("newer tool output not compressed")
	}
}

func TestChatTailOnly(t *testing.T) {
	long := strings.Repeat("match ", 120)
	body := `{"messages":[` +
		`{"role":"user","content":"old"},` +
		`{"role":"tool","tool_call_id":"1","content":"` + long + `"},` +
		`{"role":"user","content":"new"},` +
		`{"role":"tool","tool_call_id":"2","content":"` + long + `"}` +
		`]}`
	res, err := Compress([]byte(body), "openai", true, 3, stubCompressor)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(res.Hits))
	}
	if !strings.Contains(string(res.Body), `"tool_call_id":"1","content":"`+long+`"`) {
		t.Fatalf("older tool output was modified")
	}
}

func TestGeminiFunctionResponse(t *testing.T) {
	long := strings.Repeat("log line ", 120)
	body := `{"contents":[` +
		`{"role":"user","parts":[{"text":"run it"}]},` +
		`{"role":"model","parts":[{"functionCall":{"name":"shell","args":{"cmd":"ls"}}}]},` +
		`{"role":"user","parts":[{"functionResponse":{"name":"shell","response":{"output":"` + long + `"}}}]}` +
		`]}`
	res, err := Compress([]byte(body), "gemini", true, 3, stubCompressor)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(res.Hits))
	}
	var root map[string]any
	if err := json.Unmarshal(res.Body, &root); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if !strings.Contains(string(res.Body), "short-log line") {
		t.Fatalf("gemini tool output not compressed")
	}
}

func TestNoToolOutputsReturnsOriginal(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	res, err := Compress(body, "openai-response", true, 3, stubCompressor)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 0 || string(res.Body) != string(body) {
		t.Fatalf("unexpected change")
	}
}
