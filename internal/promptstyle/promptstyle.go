// Package promptstyle appends behavioural style prompts to the request's
// system text. It never rewrites tool output, and it never removes or replaces
// text the client already sent.
package promptstyle

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"cpa-guard/internal/config"
)

// Marker makes injection idempotent. A body that already carries it is left
// alone, so a retried or double-intercepted request cannot stack prompts.
const Marker = "[cpa-guard:style]"

// Levels supported by each family, in the order a settings menu should show.
var (
	CavemanLevels  = []string{"off", "lite", "full", "ultra", "wenyan-lite", "wenyan", "wenyan-ultra"}
	PonytailLevels = []string{"off", "lite", "full", "ultra"}
)

const sharedBoundaries = "Code blocks, file paths, commands, errors, URLs: keep exact. Security warnings, irreversible action confirmations, multi-step ordered sequences: write normal. Resume terse style after."

const sharedExamples = `Not: "Sure! I'd be happy to help you with that. The issue you're experiencing is likely caused by..." Yes: "Bug in auth middleware. Token expiry check use ` + "`<`" + ` not ` + "`<=`" + `. Fix:"`

const sharedAutoClarity = "Auto-Clarity: drop caveman for security warnings, irreversible actions, multi-step sequences where fragment ambiguity risks misread, or when user repeats a question. Resume after the clear part."

const sharedPersistence = "ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift. Still active if unsure."

const sharedNoInventedAbbrev = "No invented abbreviations. Standard well-known tech acronyms (DB, API, HTTP, URL, JSON, ID, OS, CPU) OK. Names of code symbols, function names, API names, error strings: keep verbatim."

const sharedPreserveLanguage = "Preserve the user's dominant language. User wrote Vietnamese, reply Vietnamese. User wrote English, reply English. Wenyan/classical-Chinese levels override this language-preservation rule. Code identifiers, error strings, file paths, commands: keep in their original form regardless of language."

const sharedNoSelfReference = `No self-reference. Do not name or announce the style (no "caveman mode", no "me caveman think", no "compressed mode active"). Just respond.`

const sharedNoDecoration = `No decorative emoji. No narrating tool calls ("I will now search", "I used X to find Y"). No status phrases ("Sure!", "Of course!", "I'd be happy to"). No causal arrow shorthand ("A -> B -> fails"). State the thing, the action, the reason. Then next step.`

func cavemanLine(level string) string {
	shared := []string{sharedExamples, sharedBoundaries, sharedAutoClarity, sharedPersistence,
		sharedNoInventedAbbrev, sharedPreserveLanguage, sharedNoSelfReference, sharedNoDecoration}
	switch level {
	case "lite":
		return join("Respond tersely. Keep grammar and full sentences but drop filler, hedging and pleasantries (just/really/basically/sure/of course/I'd be happy to).",
			"Pattern: state the thing, the action, the reason. Then next step.", shared...)
	case "full":
		return join("Respond like terse caveman. All technical substance stay exact, only fluff die.",
			"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries, hedging. Fragments OK. Short synonyms (big not extensive, fix not implement a solution for).",
			"Pattern: [thing] [action] [reason]. [next step].", shared...)
	case "ultra":
		return join("Respond ultra-terse. Maximum compression. Telegraphic.",
			"Strip conjunctions. One word when one word enough.",
			"Pattern: [thing] [action] [reason]. [next step].", shared...)
	case "wenyan-lite":
		return join("Respond semi-classical. Drop filler/hedging but keep grammar structure, classical register.",
			"Use classical Chinese sentence patterns where natural. Keep English for technical terms.", shared...)
	case "wenyan":
		return join("Respond classical Chinese (文言文). Maximum classical terseness. 80-90% character reduction.",
			"Classical sentence patterns, verbs precede objects, subjects often omitted, classical particles (之/乃/為/其).",
			"Keep English for code, commands, function names, API names, error strings.", shared...)
	case "wenyan-ultra":
		return join("Respond extreme classical compression (文言文 ultra). Maximum compression, ultra terse.",
			"Same classical rules as wenyan-full but even more compressed. One classical particle per clause.", shared...)
	default:
		return ""
	}
}

const sharedPersona = "You are a lazy senior developer. Lazy means efficient, not careless. The best code is the code never written."

const sharedLadder = "Before writing code, stop at the first rung that holds: 1) Does this need to exist at all? (YAGNI) 2) Stdlib does it? Use it. 3) Native platform feature covers it? Use it (CSS over JS, DB constraint over app code). 4) Already-installed dependency solves it? Use it; never add a new one for what a few lines can do. 5) Can it be one line? One line. 6) Only then: the minimum code that works."

const sharedRules = `No unrequested abstractions (no interface with one implementation, no factory for one product, no config for a value that never changes). No boilerplate or scaffolding "for later". Deletion over addition. Boring over clever. Fewest files possible; shortest working diff wins. Two stdlib options the same size: take the edge-case-correct one. Mark deliberate simplifications with a ` + "`ponytail:`" + ` comment naming the ceiling and upgrade path.`

const sharedOutput = "Code first. Then at most three short lines: what was skipped, when to add it. No essays or design notes. Pattern: [code] then skipped: [X], add when [Y]. If adding the skipped thing becomes necessary, do it then."

const sharedNotLazy = "Never simplify away: input validation at trust boundaries, error handling that prevents data loss, security, accessibility, anything explicitly requested. Non-trivial logic leaves ONE runnable check behind (an assert-based self-check or one small test file; no frameworks). Trivial one-liners need no test."

func ponytailLine(level string) string {
	shared := []string{sharedLadder, sharedRules, sharedOutput, sharedNotLazy,
		"ACTIVE EVERY RESPONSE. No drift back to over-building. Still active if unsure."}
	switch level {
	case "lite":
		return join(sharedPersona, "Lite: build what's asked, but name the lazier alternative in one line. User picks.", shared...)
	case "full":
		return join(sharedPersona, "Full: the ladder enforced. Stdlib and native first. Shortest diff, shortest explanation.", shared...)
	case "ultra":
		return join(sharedPersona, "Ultra: YAGNI extremist. Deletion before addition. Ship the one-liner and challenge the rest of the requirement in the same response.", shared...)
	default:
		return ""
	}
}

func join(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(p)
	}
	return b.String()
}

// Segment returns the text to append for the configured levels. Order matters:
// caveman shapes the answer, ponytail shapes the work.
func Segment(cfg config.PromptStyleConfig) string {
	if !cfg.Enabled {
		return ""
	}
	var parts []string
	if s := cavemanLine(cfg.Caveman); s != "" {
		parts = append(parts, s)
	}
	if s := ponytailLine(cfg.Ponytail); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return Marker + " " + strings.Join(parts, " ")
}

// Apply appends the configured segment to the request's system text. It returns
// the original body when the format is unsupported, the family is off, the
// marker is already present, or the rewrite fails.
func Apply(body []byte, format string, cfg config.PromptStyleConfig) ([]byte, bool) {
	segment := Segment(cfg)
	if segment == "" || len(body) == 0 || !protocolEnabled(cfg, format) {
		return body, false
	}
	switch normalizeFormat(format) {
	case "openai":
		return injectChat(body, segment)
	case "responses":
		return injectResponses(body, segment)
	case "gemini":
		return injectGemini(body, segment)
	default:
		return body, false
	}
}

func protocolEnabled(cfg config.PromptStyleConfig, format string) bool {
	switch normalizeFormat(format) {
	case "openai":
		return cfg.Protocols.OpenAIChat
	case "responses":
		return cfg.Protocols.OpenAIResponses
	case "gemini":
		return cfg.Protocols.Gemini
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

func injectChat(body []byte, segment string) ([]byte, bool) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body, false
	}
	for i, message := range messages.Array() {
		if message.Get("role").String() != "system" {
			continue
		}
		if hasMarker(message.Get("content")) {
			return body, false
		}
		content := message.Get("content")
		if content.Type == gjson.String {
			out, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", i), content.String()+"\n\n"+segment)
			if err != nil {
				return body, false
			}
			return out, true
		}
		if content.IsArray() {
			part := map[string]any{"type": "text", "text": segment}
			out, err := appendJSONArray(body, fmt.Sprintf("messages.%d.content", i), part)
			if err != nil {
				return body, false
			}
			return out, true
		}
		return body, false
	}
	out, err := prependJSONArray(body, "messages", map[string]any{"role": "system", "content": segment})
	if err != nil {
		return body, false
	}
	return out, true
}

func injectResponses(body []byte, segment string) ([]byte, bool) {
	instructions := gjson.GetBytes(body, "instructions")
	if instructions.Exists() {
		if instructions.Type != gjson.String || strings.Contains(instructions.String(), Marker) {
			return body, false
		}
		out, err := sjson.SetBytes(body, "instructions", instructions.String()+"\n\n"+segment)
		if err != nil {
			return body, false
		}
		return out, true
	}
	out, err := sjson.SetBytes(body, "instructions", segment)
	if err != nil {
		return body, false
	}
	return out, true
}

func injectGemini(body []byte, segment string) ([]byte, bool) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false
	}
	key := "systemInstruction"
	raw, ok := root[key]
	if !ok {
		key = "system_instruction"
		raw, ok = root[key]
	}
	var instruction map[string]json.RawMessage
	if ok {
		if err := json.Unmarshal(raw, &instruction); err != nil {
			return body, false
		}
	}
	if instruction == nil {
		instruction = map[string]json.RawMessage{}
	}
	var parts []json.RawMessage
	if rawParts, exists := instruction["parts"]; exists {
		if err := json.Unmarshal(rawParts, &parts); err != nil {
			return body, false
		}
	}
	for _, part := range parts {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(part, &obj); err != nil {
			continue
		}
		var text string
		if err := json.Unmarshal(obj["text"], &text); err == nil && strings.Contains(text, Marker) {
			return body, false
		}
	}
	nextPart, err := marshalNoEscape(map[string]any{"text": segment})
	if err != nil {
		return body, false
	}
	parts = append(parts, nextPart)
	instruction["parts"], err = json.Marshal(parts)
	if err != nil {
		return body, false
	}
	nextInstruction, err := marshalNoEscape(instruction)
	if err != nil {
		return body, false
	}
	root[key] = nextInstruction
	out, err := marshalNoEscape(root)
	if err != nil {
		return body, false
	}
	return out, true
}

func hasMarker(content gjson.Result) bool {
	if content.Type == gjson.String {
		return strings.Contains(content.String(), Marker)
	}
	if content.IsArray() {
		for _, part := range content.Array() {
			if strings.Contains(part.Get("text").String(), Marker) {
				return true
			}
		}
	}
	return strings.Contains(content.Raw, Marker)
}

func prependJSONArray(body []byte, path string, item any) ([]byte, error) {
	raw, err := marshalNoEscape(item)
	if err != nil {
		return body, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(gjson.GetBytes(body, path).Raw), &items); err != nil {
		return body, err
	}
	items = append([]json.RawMessage{raw}, items...)
	encoded, err := marshalNoEscape(items)
	if err != nil {
		return body, err
	}
	return sjson.SetRawBytes(body, path, encoded)
}

func appendJSONArray(body []byte, path string, item any) ([]byte, error) {
	raw, err := marshalNoEscape(item)
	if err != nil {
		return body, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(gjson.GetBytes(body, path).Raw), &items); err != nil {
		return body, err
	}
	items = append(items, raw)
	encoded, err := marshalNoEscape(items)
	if err != nil {
		return body, err
	}
	return sjson.SetRawBytes(body, path, encoded)
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
