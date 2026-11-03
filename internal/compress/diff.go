package compress

import (
	"fmt"
	"strings"

	"cpa-guard/internal/config"
)

// RepeatDiff replaces a tool output with the difference against an earlier run
// of the same tool call. It is a line-set diff: unchanged lines are omitted
// because the earlier output is still in the request history, while preserved
// error lines are repeated so the safety check in Compress still holds.
//
// It never returns a replacement that is not clearly smaller and it never
// touches text that already carries a cpa-guard marker.
func RepeatDiff(prev, cur string, cfg config.CompressionConfig) (string, bool) {
	if len(cur) < cfg.MinBytes || len(cur) < 64 || len(prev) == 0 {
		return "", false
	}
	if strings.Contains(cur, markerPrefix) || strings.Contains(prev, markerPrefix) {
		return "", false
	}
	const maxLines = 4000
	curLines := strings.Split(cur, "\n")
	prevLines := strings.Split(prev, "\n")
	if len(curLines) > maxLines || len(prevLines) > maxLines {
		return "", false
	}
	if cur == prev {
		out := fmt.Sprintf("%s repeat-diff: identical to the previous run of the same tool call; %d bytes omitted]", markerPrefix, len(cur))
		return out, len(out) < len(cur)
	}

	prevCount := make(map[string]int, len(prevLines))
	for _, line := range prevLines {
		prevCount[line]++
	}
	curCount := make(map[string]int, len(curLines))
	for _, line := range curLines {
		curCount[line]++
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s repeat-diff vs previous run of the same tool call]\n", markerPrefix)
	added, removed := 0, 0
	for _, line := range curLines {
		switch {
		case prevCount[line] > 0:
			prevCount[line]--
			// Unchanged error lines are repeated so the compressed blob keeps
			// at least as many error markers as the original.
			if strings.TrimSpace(line) != "" && errorRE.MatchString(line) {
				b.WriteString("  " + line + "\n")
			}
		case strings.TrimSpace(line) == "":
			// Blank lines carry no information for the model here.
		default:
			if added >= 200 {
				continue
			}
			b.WriteString("+ " + line + "\n")
			added++
		}
	}
	for _, line := range prevLines {
		if curCount[line] > 0 {
			curCount[line]--
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if removed >= 200 {
			break
		}
		b.WriteString("- " + line + "\n")
		removed++
	}
	if added == 0 && removed == 0 {
		return "", false
	}
	fmt.Fprintf(&b, "(%d added, %d removed; unchanged lines omitted, earlier output is above)", added, removed)
	out := b.String()
	if !preservesErrors(cur, out) {
		return "", false
	}
	if len(out) >= len(cur) || len(cur)-len(out) < 32 {
		return "", false
	}
	return out, true
}
