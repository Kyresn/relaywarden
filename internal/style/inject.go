package style

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kyresn/relaywarden/internal/protocol"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// The injected region is delimited by a pair of HTML comments. A region is
// removable only when both markers are present: a lone marker is text someone
// else wrote, and cutting to an assumed boundary could eat user content.
const (
	MarkerStart = "<!-- relaywarden-style -->"
	MarkerEnd   = "<!-- /relaywarden-style -->"
	separator   = "\n\n"
)

// Result is the outcome of one request's style pass.
type Result struct {
	Body     []byte
	Injected bool
	Stripped bool
}

// Segment wraps text in the marker pair.
func Segment(text string) string {
	return MarkerStart + separator + text + separator + MarkerEnd
}

// StripText removes every complete marker region from s. It returns the cleaned
// string and whether anything was removed. A lone marker is left alone.
func StripText(s string) (string, bool) {
	changed := false
	for {
		start := strings.Index(s, MarkerStart)
		if start < 0 {
			break
		}
		rel := strings.Index(s[start+len(MarkerStart):], MarkerEnd)
		if rel < 0 {
			break
		}
		to := start + len(MarkerStart) + rel + len(MarkerEnd)
		from := start
		// Remove one joining separator so a stripped body matches the original.
		if strings.HasPrefix(s[to:], separator) {
			to += len(separator)
		} else if from >= len(separator) && strings.HasSuffix(s[:from], separator) {
			from -= len(separator)
		}
		s = s[:from] + s[to:]
		changed = true
	}
	return s, changed
}

// Apply strips any region this plugin injected earlier, then appends text when
// it is non-empty. Stripping always runs, so switching a level replaces the old
// region instead of accumulating one, and turning both levels off restores the
// original content.
//
// Edits go through sjson so untouched fields keep their original bytes and key
// order. Every failure is fail-open: the body comes back untouched and the error
// is reported for the caller's counters only.
func Apply(body []byte, format, text string) (Result, error) {
	res := Result{Body: body}
	if len(body) == 0 {
		return res, nil
	}
	switch protocol.FormatOf(format) {
	case "openai":
		return applyChat(body, text)
	case "responses":
		return applyResponses(body, text)
	case "gemini":
		return applyGemini(body, text)
	default:
		return res, nil
	}
}

func applyChat(body []byte, text string) (Result, error) {
	res := Result{Body: body}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return res, nil
	}
	target := -1
	for i, m := range messages.Array() {
		if isSystemRole(m.Get("role").String()) {
			target = i
			break
		}
	}
	out := body
	if target < 0 {
		if text == "" {
			return res, nil
		}
		item, err := marshalNoEscape(map[string]any{
			"role":    "system",
			"content": Segment(text),
		})
		if err != nil {
			return res, err
		}
		patched, err := prependArray(body, "messages", item)
		if err != nil {
			return res, err
		}
		res.Body = patched
		res.Injected = true
		return res, nil
	}

	base := fmt.Sprintf("messages.%d", target)
	next, injected, stripped, err := editMessage(out, base, text, "text")
	if err != nil {
		return res, err
	}
	res.Body = next
	res.Injected = injected
	res.Stripped = stripped
	return res, nil
}

// editMessage rewrites one message-shaped object (role + content). base is the
// sjson path to that object.
func editMessage(body []byte, base, text, partType string) ([]byte, bool, bool, error) {
	out := body
	contentPath := base + ".content"
	content := gjson.GetBytes(out, contentPath)
	injected, stripped := false, false

	switch {
	case content.IsArray():
		parts := content.Array()
		for j := len(parts) - 1; j >= 0; j-- {
			t := parts[j].Get("text")
			if !t.Exists() || t.Type != gjson.String {
				continue
			}
			cleaned, cut := StripText(t.String())
			if !cut {
				continue
			}
			stripped = true
			if cleaned == "" {
				patched, err := sjson.DeleteBytes(out, fmt.Sprintf("%s.%d", contentPath, j))
				if err != nil {
					return body, false, false, err
				}
				out = patched
				continue
			}
			patched, err := setString(out, fmt.Sprintf("%s.%d.text", contentPath, j), cleaned)
			if err != nil {
				return body, false, false, err
			}
			out = patched
		}
		if text != "" {
			block, err := marshalNoEscape(map[string]any{"type": partType, "text": Segment(text)})
			if err != nil {
				return body, false, false, err
			}
			patched, err := sjson.SetRawBytes(out, contentPath+".-1", block)
			if err != nil {
				return body, false, false, err
			}
			out = patched
			injected = true
		} else if stripped && len(gjson.GetBytes(out, contentPath).Array()) == 0 {
			// The region was the whole message: drop the shell so the body
			// matches the one before injection.
			patched, err := sjson.DeleteBytes(out, base)
			if err != nil {
				return body, false, false, err
			}
			return patched, false, stripped, nil
		}
	case content.Type == gjson.String:
		cleaned, cut := StripText(content.String())
		stripped = cut
		next := cleaned
		if text != "" {
			if next == "" {
				next = Segment(text)
			} else {
				next += separator + Segment(text)
			}
			injected = true
		}
		if next == "" && stripped {
			patched, err := sjson.DeleteBytes(out, base)
			if err != nil {
				return body, false, false, err
			}
			return patched, false, stripped, nil
		}
		if next != content.String() {
			patched, err := setString(out, contentPath, next)
			if err != nil {
				return body, false, false, err
			}
			out = patched
		}
	default:
		if text != "" {
			patched, err := setString(out, contentPath, Segment(text))
			if err != nil {
				return body, false, false, err
			}
			out = patched
			injected = true
		}
	}
	return out, injected, stripped, nil
}

func applyResponses(body []byte, text string) (Result, error) {
	res := Result{Body: body}
	instructions := gjson.GetBytes(body, "instructions")
	if instructions.Exists() && instructions.Type == gjson.String {
		cleaned, stripped := StripText(instructions.String())
		next := cleaned
		if text != "" {
			if next == "" {
				next = Segment(text)
			} else {
				next += separator + Segment(text)
			}
		}
		res.Stripped = stripped
		res.Injected = text != ""
		if next == instructions.String() {
			return res, nil
		}
		if next == "" && stripped {
			patched, err := sjson.DeleteBytes(body, "instructions")
			if err != nil {
				return res, err
			}
			res.Body = patched
			return res, nil
		}
		patched, err := setString(body, "instructions", next)
		if err != nil {
			return res, err
		}
		res.Body = patched
		return res, nil
	}

	input := gjson.GetBytes(body, "input")
	if input.Type == gjson.String && text != "" {
		// The Responses API accepts a bare string as the whole input; expand it
		// into a message so the system item can be added without losing it.
		converted, err := marshalNoEscape([]any{map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": input.String(),
			}},
		}})
		if err != nil {
			return res, err
		}
		patched, err := sjson.SetRawBytes(body, "input", converted)
		if err != nil {
			return res, err
		}
		body = patched
		res.Body = patched
		input = gjson.GetBytes(body, "input")
	}
	if !input.IsArray() {
		return res, nil
	}
	target := -1
	for i, item := range input.Array() {
		if isResponsesSystem(item) {
			target = i
			break
		}
	}
	out := body
	if target < 0 {
		if text == "" {
			return res, nil
		}
		item, err := marshalNoEscape(map[string]any{
			"type": "message",
			"role": "system",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": Segment(text),
			}},
		})
		if err != nil {
			return res, err
		}
		patched, err := prependArray(body, "input", item)
		if err != nil {
			return res, err
		}
		res.Body = patched
		res.Injected = true
		return res, nil
	}

	next, injected, stripped, err := editMessage(out, fmt.Sprintf("input.%d", target), text, "input_text")
	if err != nil {
		return res, err
	}
	res.Body = next
	res.Injected = injected
	res.Stripped = stripped
	return res, nil
}

func applyGemini(body []byte, text string) (Result, error) {
	res := Result{Body: body}
	if si := gjson.GetBytes(body, "systemInstruction"); !si.Exists() || !si.IsObject() {
		if text == "" {
			return res, nil
		}
		obj, err := marshalNoEscape(map[string]any{
			"parts": []any{map[string]any{"text": Segment(text)}},
		})
		if err != nil {
			return res, err
		}
		patched, err := sjson.SetRawBytes(body, "systemInstruction", obj)
		if err != nil {
			return res, err
		}
		res.Body = patched
		res.Injected = true
		return res, nil
	}

	parts := gjson.GetBytes(body, "systemInstruction.parts")
	if !parts.IsArray() {
		if text == "" {
			return res, nil
		}
		block, err := marshalNoEscape(map[string]any{"text": Segment(text)})
		if err != nil {
			return res, err
		}
		patched, err := sjson.SetRawBytes(body, "systemInstruction.parts", append(append([]byte("["), block...), ']'))
		if err != nil {
			return res, err
		}
		res.Body = patched
		res.Injected = true
		return res, nil
	}
	out := body
	arr := parts.Array()
	for j := len(arr) - 1; j >= 0; j-- {
		t := arr[j].Get("text")
		if !t.Exists() || t.Type != gjson.String {
			continue
		}
		cleaned, cut := StripText(t.String())
		if !cut {
			continue
		}
		res.Stripped = true
		if cleaned == "" {
			patched, err := sjson.DeleteBytes(out, fmt.Sprintf("systemInstruction.parts.%d", j))
			if err != nil {
				return res, err
			}
			out = patched
			continue
		}
		patched, err := setString(out, fmt.Sprintf("systemInstruction.parts.%d.text", j), cleaned)
		if err != nil {
			return res, err
		}
		out = patched
	}
	if text == "" && len(gjson.GetBytes(out, "systemInstruction.parts").Array()) == 0 {
		var obj map[string]json.RawMessage
		_ = json.Unmarshal([]byte(gjson.GetBytes(out, "systemInstruction").Raw), &obj)
		if len(obj) <= 1 {
			patched, err := sjson.DeleteBytes(out, "systemInstruction")
			if err != nil {
				return res, err
			}
			res.Body = patched
			return res, nil
		}
		patched, err := sjson.SetRawBytes(out, "systemInstruction.parts", []byte("[]"))
		if err != nil {
			return res, err
		}
		res.Body = patched
		return res, nil
	}
	if text != "" {
		block, err := marshalNoEscape(map[string]any{"text": Segment(text)})
		if err != nil {
			return res, err
		}
		patched, err := sjson.SetRawBytes(out, "systemInstruction.parts.-1", block)
		if err != nil {
			return res, err
		}
		out = patched
		res.Injected = true
	}
	res.Body = out
	return res, nil
}

// prependArray inserts item at the front of the array at path. sjson can append
// but not prepend, so the array's raw bytes are rebuilt and nothing else in the
// body is touched.
func prependArray(body []byte, path string, item []byte) ([]byte, error) {
	raw := gjson.GetBytes(body, path).Raw
	inner := strings.TrimSpace(raw)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	inner = strings.TrimSpace(inner)
	var rebuilt []byte
	if inner == "" {
		rebuilt = append(append([]byte("["), item...), ']')
	} else {
		rebuilt = append(append(append([]byte("["), item...), ','), []byte(inner)...)
		rebuilt = append(rebuilt, ']')
	}
	return sjson.SetRawBytes(body, path, rebuilt)
}

func isSystemRole(role string) bool {
	return role == "system" || role == "developer"
}

func isResponsesSystem(item gjson.Result) bool {
	if !isSystemRole(item.Get("role").String()) {
		return false
	}
	typ := item.Get("type").String()
	return typ == "" || typ == "message"
}

// setString writes s as a JSON string at path without HTML-escaping, so the
// marker's angle brackets stay readable in the outgoing body.
func setString(body []byte, path, s string) ([]byte, error) {
	raw, err := marshalNoEscape(s)
	if err != nil {
		return body, err
	}
	return sjson.SetRawBytes(body, path, raw)
}

func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
