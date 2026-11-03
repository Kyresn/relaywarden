// Package protocol walks the client request shapes CLIProxyAPI accepts and
// applies a text compressor to tool outputs only.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Compressor receives one tool-output text blob. It returns the replacement,
// the filter name that matched, and whether a replacement should be applied.
// Implementations must never return an empty or larger replacement.
type Compressor func(text string) (out string, filter string, ok bool)

// Hit records one successful compression.
type Hit struct {
	Shape  string `json:"shape"`
	Filter string `json:"filter"`
	Before int    `json:"before"`
	After  int    `json:"after"`
}

// Result is the outcome of one request walk.
type Result struct {
	Body   []byte
	Hits   []Hit
	Before int
	After  int
}

// Supported reports whether the format is handled by this package.
func Supported(format string) bool {
	switch normalizeFormat(format) {
	case "openai", "responses", "gemini":
		return true
	default:
		return false
	}
}

func normalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "openai", "chat", "chat-completions":
		return "openai"
	case "openai-response", "openai_responses", "responses", "codex":
		return "responses"
	case "gemini", "google":
		return "gemini"
	default:
		return ""
	}
}

// Compress rewrites tool outputs in body. It returns the original body when the
// format is unsupported, the walk fails, or nothing was compressed.
func Compress(body []byte, format string, tailOnly bool, tailItems int, fn Compressor) (Result, error) {
	res := Result{Body: body}
	if len(body) == 0 || fn == nil {
		return res, nil
	}
	switch normalizeFormat(format) {
	case "responses":
		return compressResponses(body, tailOnly, tailItems, fn)
	case "openai":
		return compressChat(body, tailOnly, tailItems, fn)
	case "gemini":
		return compressGemini(body, tailOnly, tailItems, fn)
	default:
		return res, nil
	}
}

func compressResponses(body []byte, tailOnly bool, tailItems int, fn Compressor) (Result, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return Result{Body: body}, nil
	}
	items := input.Array()
	lastUser := lastUserIndexResponses(items)
	start := 0
	if tailOnly {
		start = lastUser + 1
	}
	toolIndices := toolOutputIndicesResponses(items)
	if tailOnly && len(toolIndices) > 0 {
		// If the request has no user turn (continuation), keep the last N tool outputs.
		if lastUser < 0 {
			start = toolIndices[max(0, len(toolIndices)-tailItems)]
		}
	}
	out := body
	res := Result{Body: body}
	for i := start; i < len(items); i++ {
		item := items[i]
		typ := item.Get("type").String()
		if typ != "function_call_output" && typ != "custom_tool_call_output" && typ != "local_shell_call_output" {
			continue
		}
		output := item.Get("output")
		if output.IsArray() {
			for k, part := range output.Array() {
				text := part.Get("text")
				if !text.Exists() || text.String() == "" {
					continue
				}
				next, filter, ok := fn(text.String())
				if !ok {
					continue
				}
				path := fmt.Sprintf("input.%d.output.%d.text", i, k)
				patched, err := sjson.SetBytes(out, path, next)
				if err != nil {
					return Result{Body: body}, err
				}
				out = patched
				res.Hits = append(res.Hits, Hit{Shape: "openai-responses-array", Filter: filter, Before: len(text.String()), After: len(next)})
			}
			continue
		}
		if !output.Exists() || output.String() == "" {
			continue
		}
		next, filter, ok := fn(output.String())
		if !ok {
			continue
		}
		path := fmt.Sprintf("input.%d.output", i)
		patched, err := sjson.SetBytes(out, path, next)
		if err != nil {
			return Result{Body: body}, err
		}
		out = patched
		res.Hits = append(res.Hits, Hit{Shape: "openai-responses-string", Filter: filter, Before: len(output.String()), After: len(next)})
	}
	return finish(out, body, res), nil
}

func compressChat(body []byte, tailOnly bool, tailItems int, fn Compressor) (Result, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return Result{Body: body}, nil
	}
	items := messages.Array()
	lastUser := lastUserIndexChat(items)
	start := 0
	if tailOnly {
		start = lastUser + 1
	}
	toolIndices := toolIndicesChat(items)
	if tailOnly && len(toolIndices) > 0 && lastUser < 0 {
		start = toolIndices[max(0, len(toolIndices)-tailItems)]
	}
	out := body
	res := Result{Body: body}
	for i := start; i < len(items); i++ {
		item := items[i]
		if item.Get("role").String() != "tool" {
			continue
		}
		content := item.Get("content")
		if content.IsArray() {
			for k, part := range content.Array() {
				text := part.Get("text")
				if !text.Exists() || text.String() == "" {
					continue
				}
				next, filter, ok := fn(text.String())
				if !ok {
					continue
				}
				path := fmt.Sprintf("messages.%d.content.%d.text", i, k)
				patched, err := sjson.SetBytes(out, path, next)
				if err != nil {
					return Result{Body: body}, err
				}
				out = patched
				res.Hits = append(res.Hits, Hit{Shape: "openai-tool-array", Filter: filter, Before: len(text.String()), After: len(next)})
			}
			continue
		}
		if !content.Exists() || content.String() == "" {
			continue
		}
		next, filter, ok := fn(content.String())
		if !ok {
			continue
		}
		path := fmt.Sprintf("messages.%d.content", i)
		patched, err := sjson.SetBytes(out, path, next)
		if err != nil {
			return Result{Body: body}, err
		}
		out = patched
		res.Hits = append(res.Hits, Hit{Shape: "openai-tool", Filter: filter, Before: len(content.String()), After: len(next)})
	}
	return finish(out, body, res), nil
}

func compressGemini(body []byte, tailOnly bool, tailItems int, fn Compressor) (Result, error) {
	// Gemini's functionResponse.response is an arbitrary JSON object. Rebuilding
	// the contents array is simpler and safer than guessing every sjson path.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return Result{Body: body}, nil
	}
	var contents []json.RawMessage
	if raw, ok := root["contents"]; ok {
		if err := json.Unmarshal(raw, &contents); err != nil {
			return Result{Body: body}, nil
		}
	} else {
		return Result{Body: body}, nil
	}
	lastUser := lastUserTextIndexGemini(contents)
	start := 0
	if tailOnly {
		start = lastUser + 1
		if lastUser < 0 {
			toolIndices := geminiToolIndices(contents)
			if len(toolIndices) > 0 {
				start = toolIndices[max(0, len(toolIndices)-tailItems)]
			}
		}
	}
	modified := false
	res := Result{Body: body}
	for i := start; i < len(contents); i++ {
		var content map[string]json.RawMessage
		if err := json.Unmarshal(contents[i], &content); err != nil {
			continue
		}
		rawParts, ok := content["parts"]
		if !ok {
			continue
		}
		var parts []json.RawMessage
		if err := json.Unmarshal(rawParts, &parts); err != nil {
			continue
		}
		changedParts := false
		for j, rawPart := range parts {
			var part map[string]json.RawMessage
			if err := json.Unmarshal(rawPart, &part); err != nil {
				continue
			}
			rawResp, ok := part["functionResponse"]
			if !ok {
				continue
			}
			var fr map[string]json.RawMessage
			if err := json.Unmarshal(rawResp, &fr); err != nil {
				continue
			}
			rawResponse, ok := fr["response"]
			if !ok {
				continue
			}
			next, hits, changed := compressJSONValue(rawResponse, "gemini-function-response", fn)
			if !changed {
				continue
			}
			fr["response"] = next
			nextFR, err := json.Marshal(fr)
			if err != nil {
				continue
			}
			part["functionResponse"] = nextFR
			nextPart, err := json.Marshal(part)
			if err != nil {
				continue
			}
			parts[j] = nextPart
			changedParts = true
			res.Hits = append(res.Hits, hits...)
		}
		if !changedParts {
			continue
		}
		nextParts, err := json.Marshal(parts)
		if err != nil {
			continue
		}
		content["parts"] = nextParts
		nextContent, err := json.Marshal(content)
		if err != nil {
			continue
		}
		contents[i] = nextContent
		modified = true
	}
	if !modified {
		return Result{Body: body}, nil
	}
	nextContents, err := json.Marshal(contents)
	if err != nil {
		return Result{Body: body}, nil
	}
	root["contents"] = nextContents
	out, err := marshalNoEscape(root)
	if err != nil {
		return Result{Body: body}, nil
	}
	return finish(out, body, res), nil
}

func compressJSONValue(raw json.RawMessage, shape string, fn Compressor) (json.RawMessage, []Hit, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return raw, nil, false
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return raw, nil, false
		}
		next, filter, ok := fn(text)
		if !ok {
			return raw, nil, false
		}
		encoded, err := marshalNoEscape(next)
		if err != nil {
			return raw, nil, false
		}
		return encoded, []Hit{{Shape: shape, Filter: filter, Before: len(text), After: len(next)}}, true
	}
	if strings.HasPrefix(trimmed, "{") {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return raw, nil, false
		}
		var hits []Hit
		changed := false
		for k, v := range obj {
			next, h, ok := compressJSONValue(v, shape, fn)
			if !ok {
				continue
			}
			obj[k] = next
			hits = append(hits, h...)
			changed = true
		}
		if !changed {
			return raw, nil, false
		}
		encoded, err := marshalNoEscape(obj)
		if err != nil {
			return raw, nil, false
		}
		return encoded, hits, true
	}
	if strings.HasPrefix(trimmed, "[") {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return raw, nil, false
		}
		var hits []Hit
		changed := false
		for i, v := range arr {
			next, h, ok := compressJSONValue(v, shape, fn)
			if !ok {
				continue
			}
			arr[i] = next
			hits = append(hits, h...)
			changed = true
		}
		if !changed {
			return raw, nil, false
		}
		encoded, err := marshalNoEscape(arr)
		if err != nil {
			return raw, nil, false
		}
		return encoded, hits, true
	}
	return raw, nil, false
}

func finish(out, original []byte, res Result) Result {
	if len(res.Hits) == 0 || string(out) == string(original) {
		return Result{Body: original}
	}
	res.Body = out
	for _, h := range res.Hits {
		res.Before += h.Before
		res.After += h.After
	}
	return res
}

func lastUserIndexResponses(items []gjson.Result) int {
	last := -1
	for i, item := range items {
		typ := item.Get("type").String()
		role := item.Get("role").String()
		if role == "user" && (typ == "" || typ == "message") {
			last = i
		}
	}
	return last
}

func lastUserIndexChat(items []gjson.Result) int {
	last := -1
	for i, item := range items {
		if item.Get("role").String() == "user" {
			last = i
		}
	}
	return last
}

func lastUserTextIndexGemini(items []json.RawMessage) int {
	last := -1
	for i, raw := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		role := "user"
		if r, ok := obj["role"]; ok {
			_ = json.Unmarshal(r, &role)
		}
		if role != "user" {
			continue
		}
		parts, ok := geminiParts(obj)
		if !ok {
			continue
		}
		for _, part := range parts {
			var p map[string]json.RawMessage
			if err := json.Unmarshal(part, &p); err != nil {
				continue
			}
			if _, isTool := p["functionResponse"]; !isTool {
				last = i
				break
			}
		}
	}
	return last
}

func geminiToolIndices(items []json.RawMessage) []int {
	var out []int
	for i, raw := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		parts, ok := geminiParts(obj)
		if !ok {
			continue
		}
		for _, part := range parts {
			var p map[string]json.RawMessage
			if err := json.Unmarshal(part, &p); err != nil {
				continue
			}
			if _, isTool := p["functionResponse"]; isTool {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

func geminiParts(obj map[string]json.RawMessage) ([]json.RawMessage, bool) {
	raw, ok := obj["parts"]
	if !ok {
		return nil, false
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, false
	}
	return parts, true
}

func toolOutputIndicesResponses(items []gjson.Result) []int {
	var out []int
	for i, item := range items {
		switch item.Get("type").String() {
		case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
			out = append(out, i)
		}
	}
	return out
}

func toolIndicesChat(items []gjson.Result) []int {
	var out []int
	for i, item := range items {
		if item.Get("role").String() == "tool" {
			out = append(out, i)
		}
	}
	return out
}

func marshalNoEscape(v any) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(b.String(), "\n")), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
