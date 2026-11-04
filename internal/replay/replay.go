// Package replay stores a turn's reasoning items server-side and restores them
// on the next request.
//
// A Chat Completions client cannot carry an upstream reasoning item back: the
// wire has nowhere to put it. The plugin therefore leaves a marker in the
// assistant content it returns, stores the items under the marker's reference,
// and expands the marker back into reasoning items when the client sends the
// history again.
package replay

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"strings"
	"time"

	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/Kyresn/relaywarden/internal/protocol"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// MarkerPrefix and MarkerSuffix delimit the replay marker. Both are fixed;
	// the marker is not configurable so two builds never disagree on its shape.
	MarkerPrefix = "<!--rw-replay:"
	MarkerSuffix = "-->"

	// RefLength is the number of characters between the prefix and the suffix.
	RefLength = 25

	// Owner marks an entry as written by this plugin.
	Owner = "relaywarden"

	separator = "\n\n"
)

var refEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// Result reports what one request or response pass did.
type Result struct {
	Restored  int
	Generated int
	Invalid   int
	Foreign   int
}

// Engine applies the replay rules to one body.
type Engine struct {
	cfg   config.ReplayConfig
	store Store
	now   func() time.Time
}

// New returns an engine backed by store. A nil store makes every operation a
// no-op, which is how a disabled feature degrades.
func New(cfg config.ReplayConfig, store Store) *Engine {
	return &Engine{cfg: cfg, store: store, now: time.Now}
}

// NewRef returns a fresh reference. 16 random bytes encoded as base32 yield 26
// characters; the last is dropped to match RefLength.
func NewRef() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ""
	}
	encoded := refEncoding.EncodeToString(raw[:])
	if len(encoded) < RefLength {
		return encoded
	}
	return encoded[:RefLength]
}

// Marker returns the client-visible marker for ref.
func Marker(ref string) string {
	return MarkerPrefix + ref + MarkerSuffix
}

// FindMarkers returns every complete marker in s and s with the markers
// removed. A marker missing its suffix is left alone: it is not provably ours.
func FindMarkers(s string) (refs []string, cleaned string) {
	var out strings.Builder
	rest := s
	for {
		start := strings.Index(rest, MarkerPrefix)
		if start < 0 {
			break
		}
		bodyStart := start + len(MarkerPrefix)
		rel := strings.Index(rest[bodyStart:], MarkerSuffix)
		if rel < 0 {
			break
		}
		ref := rest[bodyStart : bodyStart+rel]
		if len(ref) == RefLength {
			refs = append(refs, ref)
		}
		prefix := rest[:start]
		next := rest[bodyStart+rel+len(MarkerSuffix):]
		// The marker was appended after a separator; drop one so the cleaned
		// text matches what the model wrote.
		if strings.HasSuffix(prefix, separator) {
			prefix = prefix[:len(prefix)-len(separator)]
		} else if strings.HasPrefix(next, separator) {
			next = next[len(separator):]
		}
		out.WriteString(prefix)
		rest = next
	}
	if len(refs) == 0 {
		return nil, s
	}
	out.WriteString(rest)
	return refs, out.String()
}

// NormalizeRequest restores markers in an upstream Responses payload. It is a
// no-op for any other target format.
func (e *Engine) NormalizeRequest(body []byte, toFormat string) ([]byte, Result) {
	res := Result{}
	if e == nil || e.store == nil || !e.cfg.Enabled || !isResponses(toFormat) {
		return body, res
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, res
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(input.Raw), &items); err != nil {
		return body, res
	}

	seen := map[string]bool{}
	out := make([]json.RawMessage, 0, len(items))
	changed := false
	for _, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			out = append(out, item)
			continue
		}
		if !isAssistantMessage(obj) {
			out = append(out, item)
			continue
		}
		var refs []string
		edited, emptied, err := editTextParts(obj, func(s string) (string, bool) {
			found, cleaned := FindMarkers(s)
			if len(found) == 0 {
				return s, false
			}
			refs = append(refs, found...)
			return cleaned, true
		})
		if err != nil || len(refs) == 0 {
			out = append(out, item)
			continue
		}
		changed = true
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			entry, ok := e.store.Get(ref)
			if !ok {
				res.Invalid++
				continue
			}
			if entry.Owner != Owner {
				res.Foreign++
				continue
			}
			for _, raw := range entry.Items {
				out = append(out, raw)
			}
			res.Restored++
		}
		if emptied {
			// The marker was the whole content; the client had no text for this
			// turn, so dropping the shell restores the original shape.
			continue
		}
		out = append(out, edited)
	}
	if !changed {
		return body, res
	}
	encoded, err := marshalNoEscape(out)
	if err != nil {
		return body, res
	}
	patched, err := sjson.SetRawBytes(body, "input", encoded)
	if err != nil {
		return body, res
	}
	return patched, res
}

// NormalizeResponse stores this turn's reasoning items and leaves a marker in
// the assistant content. Only non-streaming Responses payloads are handled;
// streaming arrives one event at a time and is a separate ticket.
func (e *Engine) NormalizeResponse(body []byte, fromFormat string, stream bool) ([]byte, Result) {
	res := Result{}
	if e == nil || e.store == nil || !e.cfg.Enabled || !isResponses(fromFormat) || stream || !e.cfg.Nonstream {
		return body, res
	}
	output := gjson.GetBytes(body, "output")
	if !output.IsArray() {
		return body, res
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(output.Raw), &items); err != nil {
		return body, res
	}
	var reasoning []json.RawMessage
	for _, item := range items {
		if gjson.GetBytes(item, "type").String() == "reasoning" {
			reasoning = append(reasoning, item)
		}
	}
	if len(reasoning) == 0 {
		return body, res
	}
	ref := NewRef()
	if ref == "" {
		return body, res
	}
	marker := Marker(ref)

	out := items
	injected := false
	for i, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		if !isAssistantMessage(obj) {
			continue
		}
		next, err := appendTextPart(obj, separator+marker)
		if err != nil {
			continue
		}
		out[i] = next
		injected = true
		break
	}
	if !injected {
		shell, err := marshalNoEscape(map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": marker}},
		})
		if err != nil {
			return body, res
		}
		out = append(out, shell)
	}

	e.store.Put(Entry{
		Ref:        ref,
		Owner:      Owner,
		FromFormat: fromFormat,
		Items:      reasoning,
		CreatedAt:  e.now(),
		ExpiresAt:  e.now().Add(time.Duration(e.cfg.TTLHours) * time.Hour),
		WeakScope:  true,
	})
	encoded, err := marshalNoEscape(out)
	if err != nil {
		return body, res
	}
	patched, err := sjson.SetRawBytes(body, "output", encoded)
	if err != nil {
		return body, res
	}
	res.Generated = 1
	return patched, res
}

func isResponses(format string) bool {
	return protocol.FormatOf(format) == "responses"
}

func isAssistantMessage(obj map[string]json.RawMessage) bool {
	var typ, role string
	if raw, ok := obj["type"]; ok {
		_ = json.Unmarshal(raw, &typ)
	}
	if raw, ok := obj["role"]; ok {
		_ = json.Unmarshal(raw, &role)
	}
	return typ == "message" && role == "assistant"
}

// editTextParts rewrites every text part of a message. fn returns the new text
// and whether it changed. Parts that end up empty are dropped. emptied reports
// that no content is left at all.
func editTextParts(obj map[string]json.RawMessage, fn func(string) (string, bool)) (json.RawMessage, bool, error) {
	raw, ok := obj["content"]
	if !ok {
		return nil, true, nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false, err
		}
		next, changed := fn(s)
		if !changed {
			return nil, false, nil
		}
		if next == "" {
			return nil, true, nil
		}
		obj["content"] = mustRaw(next)
		encoded, err := marshalNoEscape(obj)
		return encoded, false, err
	}
	if !strings.HasPrefix(trimmed, "[") {
		return nil, true, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, false, err
	}
	kept := make([]json.RawMessage, 0, len(parts))
	changed := false
	for _, part := range parts {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(part, &p); err != nil {
			kept = append(kept, part)
			continue
		}
		textRaw, ok := p["text"]
		if !ok {
			kept = append(kept, part)
			continue
		}
		var s string
		if err := json.Unmarshal(textRaw, &s); err != nil {
			kept = append(kept, part)
			continue
		}
		next, cut := fn(s)
		if !cut {
			kept = append(kept, part)
			continue
		}
		changed = true
		if next == "" {
			continue
		}
		p["text"] = mustRaw(next)
		encoded, err := marshalNoEscape(p)
		if err != nil {
			return nil, false, err
		}
		kept = append(kept, encoded)
	}
	if !changed {
		return nil, false, nil
	}
	if len(kept) == 0 {
		return nil, true, nil
	}
	obj["content"] = mustRaw(kept)
	encoded, err := marshalNoEscape(obj)
	return encoded, false, err
}

// appendTextPart appends text to the last text part of a message, creating one
// when the message has none.
func appendTextPart(obj map[string]json.RawMessage, text string) (json.RawMessage, error) {
	raw, ok := obj["content"]
	if !ok {
		obj["content"] = mustRaw(text)
		return marshalNoEscape(obj)
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, `"`) || trimmed == "null" || trimmed == "" {
		obj["content"] = mustRaw(text)
		return marshalNoEscape(obj)
	}
	if !strings.HasPrefix(trimmed, "[") {
		return nil, errUnsupportedContent
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, err
	}
	// Append to the last text part, keeping every other part untouched.
	for i := len(parts) - 1; i >= 0; i-- {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(parts[i], &p); err != nil {
			continue
		}
		textRaw, ok := p["text"]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(textRaw, &s); err != nil {
			continue
		}
		p["text"] = mustRaw(s + text)
		encoded, err := marshalNoEscape(p)
		if err != nil {
			return nil, err
		}
		parts[i] = encoded
		obj["content"] = mustRaw(parts)
		return marshalNoEscape(obj)
	}
	parts = append(parts, mustRaw(map[string]any{"type": "output_text", "text": text}))
	obj["content"] = mustRaw(parts)
	return marshalNoEscape(obj)
}

type replayError string

func (e replayError) Error() string { return string(e) }

const errUnsupportedContent = replayError("replay: unsupported content shape")

func mustRaw(v any) json.RawMessage {
	b, err := marshalNoEscape(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
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
