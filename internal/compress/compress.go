// Package compress implements the tool-output compressors used by the request
// interceptor. Every path is fail-open: an unrecognised, tiny, huge, or
// malformed blob is returned unchanged.
package compress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"cpa-guard/internal/config"
)

const markerPrefix = "[cpa-guard:"

// Engine applies the configured filters to one tool-output blob.
type Engine struct {
	cfg config.CompressionConfig
}

func New(cfg config.CompressionConfig) *Engine {
	return &Engine{cfg: cfg}
}

// Compress returns the replacement text, the filter name, and whether it
// should replace the original.
func (e *Engine) Compress(text string) (string, string, bool) {
	if e == nil {
		return text, "", false
	}
	if len(text) < e.cfg.MinBytes || len(text) > e.cfg.MaxBytes {
		return text, "", false
	}
	if strings.Contains(text, markerPrefix) {
		return text, "", false
	}

	cleaned := text
	if e.cfg.Filters.StripANSI {
		cleaned = stripANSI(cleaned)
	}
	if cleaned == "" {
		return text, "", false
	}

	name, fn := e.detect(cleaned)
	if fn == nil || !modeAllows(e.cfg.Mode, name) {
		return text, "", false
	}
	out := safeApply(fn, cleaned, e.cfg)
	if out == "" || len(out) >= len(cleaned) {
		return text, "", false
	}
	if !preservesErrors(cleaned, out) {
		return text, "", false
	}
	out = truncateLines(out, e.cfg.MaxLinesPerResult)
	if len(out) > e.cfg.MaxCharsPerResult {
		out = headTailChars(out, e.cfg.MaxCharsPerResult)
	}
	marker := fmt.Sprintf("\n%s compressed %d -> %d bytes, filter=%s; full output omitted]", markerPrefix, len(text), len(out), name)
	out += marker
	if len(out) >= len(text) {
		return text, "", false
	}
	return out, name, true
}

type filterFunc func(string, config.CompressionConfig) string

func modeAllows(mode, filter string) bool {
	if mode != "safe" {
		return true
	}
	switch filter {
	case "json-minify", "dedup-log", "strip-ansi":
		return true
	default:
		return false
	}
}

func safeApply(fn filterFunc, text string, cfg config.CompressionConfig) (out string) {
	defer func() {
		if recover() != nil {
			out = ""
		}
	}()
	return fn(text, cfg)
}

func (e *Engine) detect(text string) (string, filterFunc) {
	head := text
	if len(head) > 1024 {
		head = head[:1024]
	}
	switch {
	case gitLogRE.MatchString(head):
		if !e.cfg.Filters.GitLog {
			return "", nil
		}
		return "git-log", gitLog
	case gitDiffRE.MatchString(head) || gitHunkRE.MatchString(head):
		if !e.cfg.Filters.GitDiff {
			return "", nil
		}
		return "git-diff", gitDiff
	case gitStatusRE.MatchString(head):
		if !e.cfg.Filters.GitStatus {
			return "", nil
		}
		return "git-status", gitStatus
	case testRE.MatchString(head):
		if !e.cfg.Filters.TestOutput {
			return "", nil
		}
		return "test-output", testOutput
	case buildRE.MatchString(head):
		if !e.cfg.Filters.BuildOutput {
			return "", nil
		}
		return "build-output", buildOutput
	}
	if isMostlyPorcelain(head) {
		if !e.cfg.Filters.GitStatus {
			return "", nil
		}
		return "git-status", gitStatus
	}
	if looksLikeGrep(text) {
		if !e.cfg.Filters.Grep {
			return "", nil
		}
		return "grep", grepOutput
	}
	if looksLikeFind(text) {
		if !e.cfg.Filters.Find {
			return "", nil
		}
		return "find", findOutput
	}
	if treeRE.MatchString(head) {
		if !e.cfg.Filters.Tree {
			return "", nil
		}
		return "tree", treeOutput
	}
	if looksLikeLS(head) {
		if !e.cfg.Filters.LS {
			return "", nil
		}
		return "ls", lsOutput
	}
	if looksLikeNumbered(text) {
		if !e.cfg.Filters.ReadNumbered {
			return "", nil
		}
		return "read-numbered", readNumbered
	}
	if countLines(text) >= 250 {
		if !e.cfg.Filters.SmartTruncate {
			return "", nil
		}
		return "smart-truncate", smartTruncate
	}
	if hasDuplicateLines(text) {
		if !e.cfg.Filters.DedupLog {
			return "", nil
		}
		return "dedup-log", dedupLog
	}
	if looksLikeJSON(text) {
		if !e.cfg.Filters.JSONMinify {
			return "", nil
		}
		return "json-minify", jsonMinify
	}
	return "", nil
}

var (
	gitLogRE    = regexp.MustCompile(`(?m)^[*|/\\ ]*commit [0-9a-f]{7,40}$`)
	gitDiffRE   = regexp.MustCompile(`(?m)^diff --git `)
	gitHunkRE   = regexp.MustCompile(`(?m)^@@ `)
	gitStatusRE = regexp.MustCompile(`(?m)^(On branch |nothing to commit|Changes (not |to be )|Untracked files:)`)
	porcelainRE = regexp.MustCompile(`^[ MADRCU?!][ MADRCU?!] \S`)
	buildRE     = regexp.MustCompile(`(?im)^(npm (warn|error|ERR!)|yarn (warn|error)|\s*Compiling\s+\S+|\s*Downloading\s+\S+|added \d+ package|\[ERROR\]|BUILD (SUCCESS|FAILED)|\s*Finished\s+|Successfully (installed|built)|ERROR:)`)
	testRE      = regexp.MustCompile(`(?im)^(FAIL|PASS|Test Files|Tests:|Snapshots?:|.*AssertionError)`)
	treeRE      = regexp.MustCompile(`[├└]──|│  `)
	lsTotalRE   = regexp.MustCompile(`(?m)^total \d+$`)
	lsRowRE     = regexp.MustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	grepLineRE  = regexp.MustCompile(`^(.+?):(\d+):(.*)$`)
	numberedRE  = regexp.MustCompile(`^\s*\d+\|`)
	errorRE     = regexp.MustCompile(`(?i)\b(error|fatal|panic|fail(ed|ure)?|exception)\b`)
)

func isMostlyPorcelain(head string) bool {
	lines := nonEmptyLines(head)
	if len(lines) < 3 {
		return false
	}
	hits := 0
	for _, l := range lines {
		if porcelainRE.MatchString(l) {
			hits++
		}
	}
	return float64(hits)/float64(len(lines)) >= 0.6
}

func looksLikeGrep(text string) bool {
	// Detection only needs the head; the filter validates the complete output
	// when it parses every line.
	head := text
	if len(head) > 2048 {
		head = head[:2048]
	}
	lines := nonEmptyLines(head)
	if len(lines) < 3 {
		return false
	}
	parsed := 0
	sample := lines
	if len(sample) > 5 {
		sample = sample[:5]
	}
	for _, l := range sample {
		if m := grepLineRE.FindStringSubmatch(l); m != nil && m[2] != "" {
			parsed++
		}
	}
	return parsed >= 3
}

func looksLikeFind(text string) bool {
	lines := nonEmptyLines(text)
	if len(lines) < 3 {
		return false
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.Contains(t, ":") && !isWindowsPath(t) {
			return false
		}
		if !(strings.HasPrefix(t, ".") || strings.HasPrefix(t, "/") || strings.HasPrefix(t, `\`) || strings.Contains(t, "/") || strings.Contains(t, `\`)) {
			return false
		}
	}
	return true
}

func isWindowsPath(s string) bool {
	return len(s) >= 3 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

func looksLikeLS(head string) bool {
	if lsTotalRE.MatchString(head) {
		return true
	}
	return len(lsRowRE.FindAllString(head, 3)) >= 3
}

func looksLikeNumbered(text string) bool {
	lines := strings.Split(text, "\n")
	if len(lines) < 250 {
		return false
	}
	nonEmpty, hits := 0, 0
	for i, l := range lines {
		if i >= 100 {
			break
		}
		if l == "" {
			continue
		}
		nonEmpty++
		if numberedRE.MatchString(l) {
			hits++
		}
	}
	return nonEmpty >= 5 && float64(hits)/float64(nonEmpty) >= 0.7
}

func looksLikeJSON(text string) bool {
	t := strings.TrimSpace(text)
	return (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && json.Valid([]byte(t))
}

func hasDuplicateLines(text string) bool {
	lines := strings.Split(text, "\n")
	seen := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" && lines[i] == lines[i-1] {
			seen++
			if seen >= 2 {
				return true
			}
		}
	}
	return false
}

func preservesErrors(original, compressed string) bool {
	if !hasErrorKeyword(original) {
		return true
	}
	want := len(errorRE.FindAllString(original, -1))
	got := len(errorRE.FindAllString(compressed, -1))
	return got >= want
}

func hasErrorKeyword(text string) bool {
	for _, word := range []string{"error", "Error", "ERROR", "fatal", "Fatal", "panic", "Panic", "fail", "Fail", "FAIL", "exception", "Exception"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func truncateLines(text string, max int) string {
	if max <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= max {
		return text
	}
	return strings.Join(lines[:max], "\n")
}

func headTailChars(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	head := max * 2 / 3
	tail := max - head
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	return text[:head] + "\n... [middle omitted] ...\n" + text[len(text)-tail:]
}

func jsonMinify(text string, _ config.CompressionConfig) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(text)); err != nil {
		return ""
	}
	return buf.String()
}
