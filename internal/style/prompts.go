// Package style injects the caveman and ponytail behaviour prompts into a
// request's system text.
//
// The prompt texts are copied verbatim from dsh-our-free-model
// (src/token-saver/prompts.js), which in turn ports them from OmniRoute
// (open-sse/rtk/cavemanPrompts.js). Both are MIT. The Caveman project's own
// engine/ is BSL-1.1 and is not used here; only the skill's rule text is.
//
// Every level is assembled from the same shared boundaries: code, paths,
// commands, errors and URLs stay exact; security warnings and irreversible
// actions drop the terse register; the user's language is preserved; the model
// never names the style. A level that dropped one of those boundaries would
// save tokens by making answers wrong or unsafe.
package style

import "github.com/Kyresn/relaywarden/internal/config"

// Levels lists the accepted values for each family. "off" is a real level and
// resolves to "inject nothing"; it is not the empty string, which would append
// a separator and a blank line to the system text.
var (
	CavemanLevels  = config.CavemanLevels
	PonytailLevels = config.PonytailLevels
)

// Shared boundaries every level carries.
const (
	sharedBoundaries  = "Code blocks, file paths, commands, errors, URLs: keep exact. Security warnings, irreversible action confirmations, multi-step ordered sequences: write normal. Resume terse style after."
	sharedExamples    = "Not: \"Sure! I'd be happy to help you with that. The issue you're experiencing is likely caused by...\" Yes: \"Bug in auth middleware. Token expiry check use `<` not `<=`. Fix:\""
	sharedAutoClarity = "Auto-Clarity: drop caveman for security warnings, irreversible actions, multi-step sequences where fragment ambiguity risks misread, or when user repeats a question. Resume after the clear part."
	sharedPersistence = "ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift. Still active if unsure."
	sharedNoAbbrev    = "No invented abbreviations. Standard well-known tech acronyms (DB, API, HTTP, URL, JSON, ID, OS, CPU) OK. Names of code symbols, function names, API names, error strings: keep verbatim."
	sharedLanguage    = "Preserve the user's dominant language. User wrote Vietnamese, reply Vietnamese. User wrote English, reply English. Wenyan/classical-Chinese levels override this language-preservation rule. Code identifiers, error strings, file paths, commands: keep in their original form regardless of language."
	sharedNoSelfRef   = "No self-reference. Do not name or announce the style (no \"caveman mode\", no \"me caveman think\", no \"compressed mode active\"). Just respond."
	sharedNoDecor     = "No decorative emoji. No narrating tool calls (\"I will now search\", \"I used X to find Y\"). No status phrases (\"Sure!\", \"Of course!\", \"I'd be happy to\"). No causal arrow shorthand (\"A -> B -> fails\"). State the thing, the action, the reason. Then next step."
)

// cavemanCommon is the tail every caveman level shares, in order.
var cavemanCommon = []string{
	sharedExamples,
	sharedBoundaries,
	sharedAutoClarity,
	sharedPersistence,
	sharedNoAbbrev,
	sharedLanguage,
	sharedNoSelfRef,
	sharedNoDecor,
}

// ponytailCommon is the tail every ponytail level shares, in order.
var ponytailCommon = []string{
	sharedPonyPersona,
	sharedPonyLadder,
	sharedPonyRules,
	sharedPonyOutput,
	sharedPonyNotLazy,
	sharedPonyPersistence,
}

const (
	sharedPonyPersona     = "You are a lazy senior developer. Lazy means efficient, not careless. The best code is the code never written."
	sharedPonyLadder      = "Before writing code, stop at the first rung that holds: 1) Does this need to exist at all? (YAGNI) 2) Stdlib does it? Use it. 3) Native platform feature covers it? Use it (CSS over JS, DB constraint over app code). 4) Already-installed dependency solves it? Use it; never add a new one for what a few lines can do. 5) Can it be one line? One line. 6) Only then: the minimum code that works."
	sharedPonyRules       = "No unrequested abstractions (no interface with one implementation, no factory for one product, no config for a value that never changes). No boilerplate or scaffolding \"for later\". Deletion over addition. Boring over clever. Fewest files possible; shortest working diff wins. Two stdlib options the same size: take the edge-case-correct one. Mark deliberate simplifications with a `ponytail:` comment naming the ceiling and upgrade path."
	sharedPonyOutput      = "Code first. Then at most three short lines: what was skipped, when to add it. No essays or design notes. Pattern: `[code] → skipped: [X], add when [Y].`"
	sharedPonyNotLazy     = "Never simplify away: input validation at trust boundaries, error handling that prevents data loss, security, accessibility, anything explicitly requested. Non-trivial logic leaves ONE runnable check behind (an assert-based self-check or one small test file; no frameworks). Trivial one-liners need no test."
	sharedPonyPersistence = "ACTIVE EVERY RESPONSE. No drift back to over-building. Still active if unsure."
)

// cavemanLeads is the one line each level swaps; the shared tail follows it.
var cavemanLeads = map[string][]string{
	"lite": {
		"Respond tersely. Keep grammar and full sentences but drop filler, hedging and pleasantries (just/really/basically/sure/of course/I'd be happy to).",
		"Pattern: state the thing, the action, the reason. Then next step.",
	},
	"full": {
		"Respond like terse caveman. All technical substance stay exact, only fluff die.",
		"Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries, hedging. Fragments OK. Short synonyms (big not extensive, fix not implement a solution for).",
		"Pattern: [thing] [action] [reason]. [next step].",
	},
	"ultra": {
		"Respond ultra-terse. Maximum compression. Telegraphic.",
		"Strip conjunctions. One word when one word enough.",
		"Pattern: [thing] [action] [reason]. [next step].",
	},
	"wenyan-lite": {
		"Respond semi-classical. Drop filler/hedging but keep grammar structure, classical register.",
		"Use classical Chinese sentence patterns where natural. Keep English for technical terms.",
	},
	"wenyan": {
		"Respond classical Chinese (文言文). Maximum classical terseness. 80-90% character reduction.",
		"Classical sentence patterns, verbs precede objects, subjects often omitted, classical particles (之/乃/為/其).",
		"Keep English for code, commands, function names, API names, error strings.",
	},
	"wenyan-ultra": {
		"Respond extreme classical compression (文言文 ultra). Maximum compression, ultra terse.",
		"Same classical rules as wenyan-full but even more compressed. One classical particle per clause.",
	},
}

// ponytailLeads is the one line each level swaps; the shared tail follows it.
var ponytailLeads = map[string]string{
	"lite":  "Lite: build what's asked, but name the lazier alternative in one line. User picks.",
	"full":  "Full: the ladder enforced. Stdlib and native first. Shortest diff, shortest explanation.",
	"ultra": "Ultra: YAGNI extremist. Deletion before addition. Ship the one-liner and challenge the rest of the requirement in the same response.",
}

// CavemanText returns the prompt for one caveman level, or "" when the level is
// "off" or unknown. The empty string is the "inject nothing" signal.
func CavemanText(level string) string {
	lead, ok := cavemanLeads[level]
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(lead)+len(cavemanCommon))
	lines = append(lines, lead...)
	lines = append(lines, cavemanCommon...)
	return join(lines)
}

// PonytailText returns the prompt for one ponytail level, or "" when the level
// is "off" or unknown.
func PonytailText(level string) string {
	lead, ok := ponytailLeads[level]
	if !ok {
		return ""
	}
	lines := make([]string, 0, 1+len(ponytailCommon))
	lines = append(lines, lead)
	lines = append(lines, ponytailCommon...)
	return join(lines)
}

// CombinedText returns the text to inject for a pair of levels. caveman comes
// first (it shapes the answer), ponytail second (it shapes the work). Either may
// be "off". Both off yields "".
func CombinedText(caveman, ponytail string) string {
	a := CavemanText(caveman)
	b := PonytailText(ponytail)
	switch {
	case a == "" && b == "":
		return ""
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n\n" + b
	}
}

// ValidCavemanLevel reports whether level is an accepted caveman level.
func ValidCavemanLevel(level string) bool { return contains(CavemanLevels, level) }

// ValidPonytailLevel reports whether level is an accepted ponytail level.
func ValidPonytailLevel(level string) bool { return contains(PonytailLevels, level) }

func join(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += " "
		}
		out += line
	}
	return out
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
