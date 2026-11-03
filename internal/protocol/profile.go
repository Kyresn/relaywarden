package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// Profile is the request-side snapshot used by the guard. It intentionally
// records only what the proxy can observe before translation.
type Profile struct {
	Format          string   `json:"format"`
	Model           string   `json:"model"`
	Stream          bool     `json:"stream"`
	SessionID       string   `json:"session_id,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	ReasoningPath   string   `json:"reasoning_path,omitempty"`
	ThinkingBudget  int64    `json:"thinking_budget,omitempty"`
	ThinkingPath    string   `json:"thinking_path,omitempty"`
	IncludeThoughts bool     `json:"include_thoughts,omitempty"`
	IncludePath     string   `json:"include_path,omitempty"`
	MaxOutputTokens int64    `json:"max_output_tokens,omitempty"`
	MaxTokensPath   string   `json:"max_tokens_path,omitempty"`
	ServiceTier     string   `json:"service_tier,omitempty"`
	ServiceTierPath string   `json:"service_tier_path,omitempty"`
	PromptCacheKey  string   `json:"prompt_cache_key,omitempty"`
	CacheKeyPath    string   `json:"cache_key_path,omitempty"`
	Tools           []string `json:"tools,omitempty"`
	SystemHash      string   `json:"system_hash,omitempty"`
	MessageCount    int      `json:"message_count"`
	ToolOutputCount int      `json:"tool_output_count"`
	LastUserIndex   int      `json:"last_user_index"`
	HasThinking     bool     `json:"has_thinking"`
}

// ExtractProfile builds a profile from a source-format body. modelHint is the
// model carried by the interceptor when the body does not include one.
func ExtractProfile(body []byte, format, modelHint string) Profile {
	p := Profile{Format: normalizeFormat(format), Model: strings.TrimSpace(modelHint), LastUserIndex: -1}
	if len(body) == 0 {
		return p
	}
	switch p.Format {
	case "responses":
		fillResponsesProfile(&p, body)
	case "openai":
		fillChatProfile(&p, body)
	case "gemini":
		fillGeminiProfile(&p, body)
	}
	p.HasThinking = p.ReasoningEffort != "" || p.ThinkingBudget > 0 || p.IncludeThoughts
	return p
}

func fillResponsesProfile(p *Profile, body []byte) {
	if v := gjson.GetBytes(body, "model"); v.Exists() {
		p.Model = v.String()
	}
	p.Stream = gjson.GetBytes(body, "stream").Bool()
	p.ReasoningEffort, p.ReasoningPath = firstStringPath(body, "reasoning.effort", "reasoning_effort")
	p.MaxOutputTokens, p.MaxTokensPath = firstIntPath(body, "max_output_tokens", "max_completion_tokens", "max_tokens")
	p.ServiceTier, p.ServiceTierPath = firstStringPath(body, "service_tier")
	p.PromptCacheKey, p.CacheKeyPath = firstStringPath(body, "prompt_cache_key")
	p.SystemHash = hashText(firstString(body, "instructions"))
	p.Tools = toolNames(body, "tools.#.name", "tools.#.function.name")
	p.MessageCount = len(gjson.GetBytes(body, "input").Array())
	p.ToolOutputCount = len(toolOutputIndicesResponses(gjson.GetBytes(body, "input").Array()))
	p.LastUserIndex = lastUserIndexResponses(gjson.GetBytes(body, "input").Array())
}

func fillChatProfile(p *Profile, body []byte) {
	if v := gjson.GetBytes(body, "model"); v.Exists() {
		p.Model = v.String()
	}
	p.Stream = gjson.GetBytes(body, "stream").Bool()
	p.ReasoningEffort, p.ReasoningPath = firstStringPath(body, "reasoning_effort")
	p.MaxOutputTokens, p.MaxTokensPath = firstIntPath(body, "max_completion_tokens", "max_tokens")
	p.ServiceTier, p.ServiceTierPath = firstStringPath(body, "service_tier")
	p.PromptCacheKey, p.CacheKeyPath = firstStringPath(body, "prompt_cache_key")
	p.Tools = toolNames(body, "tools.#.function.name", "tools.#.name")
	p.MessageCount = len(gjson.GetBytes(body, "messages").Array())
	p.ToolOutputCount = len(toolIndicesChat(gjson.GetBytes(body, "messages").Array()))
	p.LastUserIndex = lastUserIndexChat(gjson.GetBytes(body, "messages").Array())
	var system strings.Builder
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		role := msg.Get("role").String()
		if role == "system" || role == "developer" {
			system.WriteString(msg.Get("content").String())
			system.WriteByte('\n')
		}
	}
	p.SystemHash = hashText(system.String())
}

func fillGeminiProfile(p *Profile, body []byte) {
	p.Stream = gjson.GetBytes(body, "stream").Bool()
	p.ReasoningEffort, p.ReasoningPath = firstStringPath(body, "generationConfig.thinkingConfig.thinkingLevel", "generationConfig.thinking_level")
	p.ThinkingBudget, p.ThinkingPath = firstIntPath(body, "generationConfig.thinkingConfig.thinkingBudget", "generationConfig.thinking_budget")
	p.IncludeThoughts, p.IncludePath = firstBoolPath(body, "generationConfig.thinkingConfig.includeThoughts")
	p.MaxOutputTokens, p.MaxTokensPath = firstIntPath(body, "generationConfig.maxOutputTokens", "generationConfig.max_output_tokens")
	p.ServiceTier, p.ServiceTierPath = firstStringPath(body, "service_tier")
	p.PromptCacheKey, p.CacheKeyPath = firstStringPath(body, "cachedContent")
	p.Tools = toolNames(body, "tools.#.functionDeclarations.#.name")
	p.MessageCount = len(gjson.GetBytes(body, "contents").Array())
	p.LastUserIndex = lastUserTextIndexGeminiRaw(body)
	p.ToolOutputCount = geminiToolCountRaw(body)
	var sys strings.Builder
	for _, part := range gjson.GetBytes(body, "systemInstruction.parts").Array() {
		sys.WriteString(part.Get("text").String())
	}
	p.SystemHash = hashText(sys.String())
}

func firstString(body []byte, paths ...string) string {
	for _, path := range paths {
		if v := gjson.GetBytes(body, path); v.Exists() && v.String() != "" {
			return strings.TrimSpace(v.String())
		}
	}
	return ""
}

func firstStringPath(body []byte, paths ...string) (string, string) {
	for _, path := range paths {
		if v := gjson.GetBytes(body, path); v.Exists() && v.String() != "" {
			return strings.TrimSpace(v.String()), path
		}
	}
	return "", ""
}

func firstIntPath(body []byte, paths ...string) (int64, string) {
	for _, path := range paths {
		if v := gjson.GetBytes(body, path); v.Exists() {
			return v.Int(), path
		}
	}
	return 0, ""
}

func firstBoolPath(body []byte, paths ...string) (bool, string) {
	for _, path := range paths {
		if v := gjson.GetBytes(body, path); v.Exists() {
			return v.Bool(), path
		}
	}
	return false, ""
}

func toolNames(body []byte, paths ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, path := range paths {
		for _, item := range gjson.GetBytes(body, path).Array() {
			name := strings.TrimSpace(item.String())
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

func hashText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func lastUserTextIndexGeminiRaw(body []byte) int {
	contents := gjson.GetBytes(body, "contents")
	if !contents.IsArray() {
		return -1
	}
	last := -1
	for i, content := range contents.Array() {
		if content.Get("role").String() != "user" {
			continue
		}
		hasText := false
		for _, part := range content.Get("parts").Array() {
			if !part.Get("functionResponse").Exists() {
				hasText = true
				break
			}
		}
		if hasText {
			last = i
		}
	}
	return last
}

func geminiToolCountRaw(body []byte) int {
	n := 0
	for _, content := range gjson.GetBytes(body, "contents").Array() {
		for _, part := range content.Get("parts").Array() {
			if part.Get("functionResponse").Exists() {
				n++
			}
		}
	}
	return n
}

// BodyHash is used to key per-request state when only the body is available.
func BodyHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// DecodeJSONObject is a small helper for tests and callers that need the raw
// top-level object without losing unknown fields.
func DecodeJSONObject(body []byte) (map[string]json.RawMessage, error) {
	var out map[string]json.RawMessage
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
