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
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
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

	// maxRefsPerRequest and maxRestoredBytes bound how far one request may
	// inflate. A client can collect many refs and send them all at once; without
	// a bound the upstream body would be limited only by the whole store.
	maxRefsPerRequest = 64
	maxRestoredBytes  = 8 << 20
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

// FindMarkers returns the valid references in s, how many malformed markers were
// removed, and s with every marker removed.
//
// A malformed marker is only removed when its suffix sits close behind the
// prefix. Scanning for the next "-->" without a bound would let one dangling
// prefix swallow a later valid marker and every character in between.
func FindMarkers(s string) (refs []string, invalid int, cleaned string) {
	if !strings.Contains(s, MarkerPrefix) {
		return nil, 0, s
	}
	var out strings.Builder
	rest := s
	removed := false
	for {
		start := strings.Index(rest, MarkerPrefix)
		if start < 0 {
			break
		}
		bodyStart := start + len(MarkerPrefix)
		if bodyStart+RefLength+len(MarkerSuffix) <= len(rest) {
			candidate := rest[bodyStart : bodyStart+RefLength]
			if isRef(candidate) && strings.HasPrefix(rest[bodyStart+RefLength:], MarkerSuffix) {
				refs = append(refs, candidate)
				removed = true
				prefix, next := cutRegion(rest, start, bodyStart+RefLength+len(MarkerSuffix))
				out.WriteString(prefix)
				rest = next
				continue
			}
		}
		// A malformed marker must terminate within a short window; otherwise the
		// prefix is treated as ordinary text and scanning resumes after it.
		if rel := strings.Index(rest[bodyStart:], MarkerSuffix); rel >= 0 && rel <= RefLength*2 {
			invalid++
			removed = true
			prefix, next := cutRegion(rest, start, bodyStart+rel+len(MarkerSuffix))
			out.WriteString(prefix)
			rest = next
			continue
		}
		out.WriteString(rest[:bodyStart])
		rest = rest[bodyStart:]
	}
	if !removed {
		return nil, 0, s
	}
	out.WriteString(rest)
	return refs, invalid, out.String()
}

// cutRegion returns the text before start and after end, dropping one joining
// separator so the cleaned text matches what the model wrote.
func cutRegion(s string, start, end int) (string, string) {
	prefix := s[:start]
	next := s[end:]
	if strings.HasSuffix(prefix, separator) {
		prefix = prefix[:len(prefix)-len(separator)]
	} else if strings.HasPrefix(next, separator) {
		next = next[len(separator):]
	}
	return prefix, next
}

func isRef(s string) bool {
	if len(s) != RefLength {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '2' && r <= '7':
		default:
			return false
		}
	}
	return true
}

// ConversationKey hashes the parts of a provider request that stay stable
// across turns of one conversation: the instructions and the first input item.
// It binds an entry to the conversation that produced it, so a ref echoed from
// somewhere else resolves to Foreign instead of replaying.
func ConversationKey(providerBody []byte) string {
	if len(providerBody) == 0 {
		return ""
	}
	first := gjson.GetBytes(providerBody, "input.0")
	if !first.Exists() {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(gjson.GetBytes(providerBody, "instructions").String()))
	h.Write([]byte{0})
	h.Write([]byte(first.Raw))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// NormalizeRequest restores markers in an upstream Responses payload. It is a
// no-op for any other target format.
func (e *Engine) NormalizeRequest(body []byte, clientFormat, toFormat, model string) ([]byte, Result) {
	res := Result{}
	if e == nil || e.store == nil || !e.cfg.Enabled || !isResponses(toFormat) {
		return body, res
	}
	if protocol.FormatOf(clientFormat) != "openai" {
		// Native Responses and Claude clients carry reasoning themselves; the
		// host already replays for Claude. Only the Chat wire needs us.
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

	convKey := ConversationKey(body)
	seen := map[string]bool{}
	out := make([]json.RawMessage, 0, len(items))
	changed := false
	restoredBytes := 0
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
		changedText := false
		edited, emptied, err := editTextParts(obj, func(s string) (string, bool) {
			found, bad, cleaned := FindMarkers(s)
			if bad > 0 {
				res.Invalid += bad
			}
			if cleaned == s {
				return s, false
			}
			changedText = true
			refs = append(refs, found...)
			return cleaned, true
		})
		if err != nil || !changedText {
			out = append(out, item)
			continue
		}
		changed = true
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			if len(seen) > maxRefsPerRequest {
				res.Invalid++
				continue
			}
			entry, ok := e.store.Get(ref)
			if !ok {
				res.Invalid++
				continue
			}
			if entry.Owner != Owner || entry.FromFormat != toFormat || !sameModel(entry.Model, model) || (entry.ConvKey != "" && convKey != "" && entry.ConvKey != convKey) {
				res.Foreign++
				continue
			}
			if restoredBytes >= maxRestoredBytes {
				res.Invalid++
				continue
			}
			for _, raw := range entry.Items {
				if restoredBytes+len(raw) > maxRestoredBytes {
					break
				}
				restoredBytes += len(raw)
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
func (e *Engine) NormalizeResponse(body, providerRequest []byte, fromFormat, clientFormat, model string, stream bool) ([]byte, Result) {
	res := Result{}
	if e == nil || e.store == nil || !e.cfg.Enabled || !isResponses(fromFormat) || stream || !e.cfg.Nonstream {
		return body, res
	}
	if protocol.FormatOf(clientFormat) != "openai" {
		return body, res
	}
	output := gjson.GetBytes(body, "output")
	outputPath := "output"
	if !output.IsArray() {
		// The Codex executor hands the terminal response.completed event to the
		// response normalizer, so the array sits under response.output.
		output = gjson.GetBytes(body, "response.output")
		outputPath = "response.output"
	}
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
	// Use the last assistant message: the marker must be the final segment the
	// client sees, and a turn may carry commentary before the final answer.
	for i := len(items) - 1; i >= 0; i-- {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(items[i], &obj); err != nil {
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
		Model:      model,
		ConvKey:    ConversationKey(providerRequest),
		Items:      reasoning,
		CreatedAt:  e.now(),
		ExpiresAt:  e.now().Add(time.Duration(e.cfg.TTLHours) * time.Hour),
		WeakScope:  true,
	})
	encoded, err := marshalNoEscape(out)
	if err != nil {
		return body, res
	}
	patched, err := sjson.SetRawBytes(body, outputPath, encoded)
	if err != nil {
		return body, res
	}
	res.Generated = 1
	return patched, res
}

func isResponses(format string) bool {
	return protocol.FormatOf(format) == "responses"
}

// sameModel compares model names after dropping a thinking suffix such as
// "gpt-5.5(high)"; the two hooks receive the name from different layers.
func sameModel(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	return baseModel(a) == baseModel(b)
}

func baseModel(s string) string {
	if i := strings.IndexAny(s, "(:"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(strings.ToLower(s))
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
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		obj["content"] = mustRaw(s + text)
		return marshalNoEscape(obj)
	}
	if trimmed == "null" || trimmed == "" {
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
