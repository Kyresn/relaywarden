package style

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLevelTables(t *testing.T) {
	for _, level := range CavemanLevels {
		text := CavemanText(level)
		if level == "off" {
			if text != "" {
				t.Fatalf("caveman %q should inject nothing, got %d bytes", level, len(text))
			}
			continue
		}
		if text == "" {
			t.Fatalf("caveman %q has no prompt", level)
		}
		if !strings.Contains(text, "Code blocks, file paths") {
			t.Fatalf("caveman %q lost the shared boundaries", level)
		}
	}
	for _, level := range PonytailLevels {
		text := PonytailText(level)
		if level == "off" {
			if text != "" {
				t.Fatalf("ponytail %q should inject nothing", level)
			}
			continue
		}
		if !strings.Contains(text, "lazy senior developer") {
			t.Fatalf("ponytail %q lost the shared persona", level)
		}
	}
	if got := CavemanText("bogus"); got != "" {
		t.Fatalf("unknown level must not inject, got %q", got)
	}
	if got := CombinedText("off", "off"); got != "" {
		t.Fatalf("both off must yield empty text")
	}
	both := CombinedText("full", "lite")
	if strings.Index(both, "terse caveman") > strings.Index(both, "lazy senior developer") {
		t.Fatalf("caveman must come before ponytail")
	}
}

func TestStripText(t *testing.T) {
	seg := Segment("hello")
	got, changed := StripText("a" + separator + seg + separator + "b")
	if !changed || got != "a"+separator+"b" {
		t.Fatalf("strip = %q, changed=%v", got, changed)
	}
	if _, changed := StripText(MarkerStart + " orphan"); changed {
		t.Fatalf("a lone marker must not be stripped")
	}
}

func TestChatStringContent(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"system","content":"Base."},{"role":"user","content":"hi"}]}`)
	res, err := Apply(body, "openai", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v injected=%v", err, res.Injected)
	}
	var parsed struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(res.Body, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Messages[0].Content; got != "Base."+separator+Segment("PROMPT") {
		t.Fatalf("system content not extended: %q", got)
	}

	back, err := Apply(res.Body, "openai", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v stripped=%v", err, back.Stripped)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore the original bytes:\n got %s\nwant %s", back.Body, body)
	}
}

func TestChatCreatesSystemMessage(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	res, err := Apply(body, "openai", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v injected=%v", err, res.Injected)
	}
	if !strings.Contains(string(res.Body), `"messages":[{"content":"`+MarkerStart) {
		t.Fatalf("system message not prepended: %s", res.Body)
	}
	back, err := Apply(res.Body, "openai", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v", err)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore: got %s want %s", back.Body, body)
	}
}

func TestChatArrayContent(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":[{"type":"text","text":"Base."}]}]}`)
	res, err := Apply(body, "openai", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v", err)
	}
	var parsed struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(res.Body, &parsed); err != nil {
		t.Fatal(err)
	}
	blocks := parsed.Messages[0].Content
	if len(blocks) != 2 || blocks[1].Text != Segment("PROMPT") {
		t.Fatalf("text block not appended: %s", res.Body)
	}
	back, err := Apply(res.Body, "openai", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v", err)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore: got %s want %s", back.Body, body)
	}
}

func TestResponsesInstructions(t *testing.T) {
	body := []byte(`{"model":"m","instructions":"Base.","input":[]}`)
	res, err := Apply(body, "responses", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v", err)
	}
	back, err := Apply(res.Body, "responses", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v", err)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore: got %s want %s", back.Body, body)
	}
}

func TestResponsesInputSystemItem(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"system","content":[{"type":"input_text","text":"Base."}]}]}`)
	res, err := Apply(body, "responses", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v", err)
	}
	if !strings.Contains(string(res.Body), MarkerStart) {
		t.Fatalf("marker missing: %s", res.Body)
	}
	back, err := Apply(res.Body, "responses", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v", err)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore: got %s want %s", back.Body, body)
	}
}

func TestGeminiSystemInstruction(t *testing.T) {
	body := []byte(`{"contents":[],"systemInstruction":{"parts":[{"text":"Base."}]}}`)
	res, err := Apply(body, "gemini", "PROMPT")
	if err != nil || !res.Injected {
		t.Fatalf("inject: %v", err)
	}
	back, err := Apply(res.Body, "gemini", "")
	if err != nil || !back.Stripped {
		t.Fatalf("strip: %v", err)
	}
	if string(back.Body) != string(body) {
		t.Fatalf("strip did not restore: got %s want %s", back.Body, body)
	}
}

func TestSwitchReplacesInsteadOfAccumulating(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":"Base."}]}`)
	first, err := Apply(body, "openai", "ONE")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Apply(first.Body, "openai", "TWO")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second.Body), "ONE") {
		t.Fatalf("old region was not replaced: %s", second.Body)
	}
	if strings.Count(string(second.Body), MarkerStart) != 1 {
		t.Fatalf("expected exactly one region: %s", second.Body)
	}
	again, err := Apply(second.Body, "openai", "TWO")
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Body) != string(second.Body) {
		t.Fatalf("second pass changed the body")
	}
}

func TestUnsupportedFormatUntouched(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":"Base."}]}`)
	res, err := Apply(body, "anthropic", "PROMPT")
	if err != nil || res.Injected || res.Stripped {
		t.Fatalf("unsupported format must be a no-op: %v", err)
	}
	if string(res.Body) != string(body) {
		t.Fatalf("body changed")
	}
}

func TestResponsesInputPartType(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"system","content":[{"type":"input_text","text":"Base."}]}]}`)
	res, err := Apply(body, "responses", "PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Input []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(res.Body, &parsed); err != nil {
		t.Fatal(err)
	}
	parts := parsed.Input[0].Content
	if len(parts) != 2 || parts[1].Type != "input_text" {
		t.Fatalf("Responses input part must be input_text: %s", res.Body)
	}
}

func TestEmptyContentIsNotDropped(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":""},{"role":"user","content":"see <!-- relaywarden-style -->"}]}`)
	res, err := Apply(body, "openai", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Body), `"role":"system"`) {
		t.Fatalf("empty system message was dropped: %s", res.Body)
	}
}

func TestEmptyArrayContentIsNotDropped(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":[]},{"role":"user","content":"see <!-- relaywarden-style -->"}]}`)
	res, err := Apply(body, "openai", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Body), `"role":"system"`) {
		t.Fatalf("empty array message was dropped: %s", res.Body)
	}
}

func TestResponsesStringInput(t *testing.T) {
	body := []byte(`{"input":"hello"}`)
	res, err := Apply(body, "responses", "PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Injected || !strings.Contains(string(res.Body), `"text":"hello"`) {
		t.Fatalf("string input not preserved: %s", res.Body)
	}
	if !strings.Contains(string(res.Body), MarkerStart) {
		t.Fatalf("prompt not injected: %s", res.Body)
	}
}

func TestGeminiNullSystemInstruction(t *testing.T) {
	body := []byte(`{"systemInstruction":null}`)
	res, err := Apply(body, "gemini", "PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Injected || !strings.Contains(string(res.Body), MarkerStart) {
		t.Fatalf("null systemInstruction not handled: %s", res.Body)
	}
}

func TestGeminiStripKeepsSiblingKeys(t *testing.T) {
	body := []byte(`{"systemInstruction":{"role":"system","parts":[{"text":"Base."}]}}`)
	withPrompt, err := Apply(body, "gemini", "PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	back, err := Apply(withPrompt.Body, "gemini", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back.Body), `"role":"system"`) {
		t.Fatalf("sibling key lost: %s", back.Body)
	}
}
