package compress

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cpa-guard/internal/config"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var oscRE = regexp.MustCompile(`\x1b\][^\x07]*(\x07|\x1b\\)`)

func stripANSI(text string) string {
	if !strings.Contains(text, "\x1b") && !strings.Contains(text, "\r") {
		return text
	}
	text = oscRE.ReplaceAllString(text, "")
	text = ansiRE.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func gitDiff(text string, cfg config.CompressionConfig) string {
	const maxHunkLines = 100
	lines := strings.Split(text, "\n")
	var out []string
	var currentFile string
	added, removed := 0, 0
	inHunk := false
	hunkShown, hunkSkipped := 0, 0
	truncated := false

	flushSkipped := func() {
		if hunkSkipped > 0 {
			out = append(out, fmt.Sprintf("  ... (%d lines truncated)", hunkSkipped))
			truncated = true
			hunkSkipped = 0
		}
	}
	flushSummary := func() {
		if currentFile != "" && (added > 0 || removed > 0) {
			out = append(out, fmt.Sprintf("  +%d -%d", added, removed))
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushSkipped()
			flushSummary()
			parts := strings.Split(line, " b/")
			if len(parts) > 1 {
				currentFile = strings.Join(parts[1:], " b/")
			} else {
				currentFile = "unknown"
			}
			out = append(out, "\n"+currentFile)
			added, removed = 0, 0
			inHunk = false
			hunkShown = 0
		case strings.HasPrefix(line, "@@"):
			flushSkipped()
			inHunk = true
			hunkShown = 0
			out = append(out, "  "+line)
		case inHunk:
			isAdd := strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")
			isDel := strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")
			switch {
			case isAdd:
				added++
				if hunkShown < maxHunkLines {
					out = append(out, "  "+line)
					hunkShown++
				} else {
					hunkSkipped++
				}
			case isDel:
				removed++
				if hunkShown < maxHunkLines {
					out = append(out, "  "+line)
					hunkShown++
				} else {
					hunkSkipped++
				}
			case hunkShown > 0 && hunkShown < maxHunkLines && !strings.HasPrefix(line, `\`):
				out = append(out, "  "+line)
				hunkShown++
			}
		}
		if len(out) >= 500 {
			out = append(out, "\n... (more changes truncated)")
			truncated = true
			break
		}
	}
	flushSkipped()
	flushSummary()
	if truncated {
		out = append(out, "[full diff omitted by cpa-guard]")
	}
	return strings.Join(out, "\n")
}

func gitStatus(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	var out []string
	sectionCount := 0
	section := ""
	flush := func() {
		if sectionCount > 10 {
			out = append(out, fmt.Sprintf("  ... +%d more", sectionCount-10))
		}
		sectionCount = 0
	}
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "On branch ") || strings.HasPrefix(t, "Your branch ") ||
			strings.HasPrefix(t, "Changes not staged") || strings.HasPrefix(t, "Changes to be committed") ||
			strings.HasPrefix(t, "Untracked files:") || strings.HasPrefix(t, "nothing to commit") {
			flush()
			section = t
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(t, "(use \"git") || strings.HasPrefix(t, "no changes added") ||
			strings.HasPrefix(t, "  (") || strings.HasPrefix(t, "nothing added") {
			continue
		}
		if section != "" && (porcelainRE.MatchString(line) || strings.HasPrefix(t, "modified:") ||
			strings.HasPrefix(t, "new file:") || strings.HasPrefix(t, "deleted:") ||
			strings.HasPrefix(t, "renamed:") || strings.HasPrefix(t, "??") || strings.HasPrefix(t, "!!")) {
			sectionCount++
			if sectionCount <= 10 {
				out = append(out, line)
			}
		}
	}
	flush()
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func gitLog(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 200 {
		lines = lines[:200]
		lines = append(lines, "... (older commits truncated)")
	}
	return strings.Join(lines, "\n")
}

func grepOutput(text string, _ config.CompressionConfig) string {
	type fileHits struct {
		name string
		rows [][2]string
	}
	byFile := map[string]*fileHits{}
	total := 0
	for _, line := range strings.Split(text, "\n") {
		m := grepLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		file, num, content := m[1], m[2], strings.TrimSpace(m[3])
		if file == "" || num == "" {
			continue
		}
		total++
		fh := byFile[file]
		if fh == nil {
			fh = &fileHits{name: file}
			byFile[file] = fh
		}
		fh.rows = append(fh.rows, [2]string{num, content})
	}
	if total == 0 || len(byFile) == 0 {
		return ""
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	var b strings.Builder
	fmt.Fprintf(&b, "%d matches in %d files:\n", total, len(files))
	for _, f := range files {
		fh := byFile[f]
		fmt.Fprintf(&b, "[file] %s (%d):\n", fh.name, len(fh.rows))
		show := fh.rows
		if len(show) > 10 {
			show = show[:10]
		}
		for _, row := range show {
			fmt.Fprintf(&b, "  %4s: %s\n", row[0], row[1])
		}
		if len(fh.rows) > 10 {
			fmt.Fprintf(&b, "  +%d\n", len(fh.rows)-10)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func findOutput(text string, _ config.CompressionConfig) string {
	byDir := map[string][]string{}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		dir := "."
		if idx := strings.LastIndexAny(t, `/\`); idx > 0 {
			dir = t[:idx]
		}
		byDir[dir] = append(byDir[dir], t)
	}
	if len(byDir) == 0 {
		return ""
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	if len(dirs) > 20 {
		dirs = dirs[:20]
	}
	var b strings.Builder
	for _, d := range dirs {
		files := byDir[d]
		fmt.Fprintf(&b, "[dir] %s (%d):\n", d, len(files))
		show := files
		if len(show) > 10 {
			show = show[:10]
		}
		for _, f := range show {
			fmt.Fprintf(&b, "  %s\n", f)
		}
		if len(files) > 10 {
			fmt.Fprintf(&b, "  +%d\n", len(files)-10)
		}
	}
	return b.String()
}

var lsNoise = []string{
	"node_modules", ".git", "target", "__pycache__", ".next", "dist", "build",
	".cache", ".turbo", ".vercel", ".pytest_cache", ".mypy_cache", ".tox",
	".venv", "venv", "env", "coverage", ".nyc_output", ".idea", ".vscode",
	".vs", ".eggs",
}

func lsOutput(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	var out []string
	dropped := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if lsTotalRE.MatchString(t) {
			out = append(out, t)
			continue
		}
		lower := strings.ToLower(t)
		noise := false
		for _, n := range lsNoise {
			if strings.HasSuffix(lower, " "+n) || strings.HasSuffix(lower, "/"+n) || lower == n {
				noise = true
				break
			}
		}
		if noise {
			dropped++
			continue
		}
		out = append(out, line)
	}
	if dropped > 0 {
		out = append(out, fmt.Sprintf("... (%d noise directories omitted)", dropped))
	}
	if len(out) > 200 {
		out = out[:200]
		out = append(out, "... (listing truncated)")
	}
	return strings.Join(out, "\n")
}

func treeOutput(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 200 {
		lines = lines[:200]
		lines = append(lines, "... (tree truncated)")
	}
	return strings.Join(lines, "\n")
}

var buildKeepRE = regexp.MustCompile(`(?i)(error|warn|fail|fatal|panic|exception|BUILD (SUCCESS|FAILED)|Tests?:|Test Files|Snapshots?|added \d+ package|Finished|Successfully)`)
var progressRE = regexp.MustCompile(`(?i)^\s*(\[.*\]\s*)?(Compiling|Downloading|Building|Linking|Optimizing|Bundling)\b`)

func buildOutput(text string, cfg config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || progressRE.MatchString(t) {
			continue
		}
		if buildKeepRE.MatchString(t) {
			kept = append(kept, line)
		}
	}
	if len(kept) < 3 {
		return ""
	}
	if len(kept) > 160 {
		kept = append(kept[:120], append([]string{"... (output truncated)"}, kept[len(kept)-40:]...)...)
	}
	return strings.Join(kept, "\n")
}

var testKeepRE = regexp.MustCompile(`(?i)(FAIL|Error|Assertion|Test Files|Tests:|Snapshots?|passed|failed|✗|✕|ok\s)`)

func testOutput(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	var kept []string
	for _, line := range lines {
		if testKeepRE.MatchString(line) {
			kept = append(kept, line)
		}
	}
	if len(kept) < 3 {
		return ""
	}
	if len(kept) > 64 {
		head := kept[:24]
		tail := kept[len(kept)-40:]
		kept = append(head, append([]string{"... (test output truncated)"}, tail...)...)
	}
	return strings.Join(kept, "\n")
}

func dedupLog(text string, cfg config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	threshold := cfg.DedupThreshold
	if threshold < 2 {
		threshold = 3
	}
	var out []string
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		count := j - i
		if count >= threshold && strings.TrimSpace(lines[i]) != "" {
			out = append(out, fmt.Sprintf("%s  [x%d]", lines[i], count))
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	if len(out) >= len(lines) {
		return ""
	}
	if len(out) > 2000 {
		out = append(out[:1200], append([]string{"... (log truncated)"}, out[len(out)-800:]...)...)
	}
	return strings.Join(out, "\n")
}

func smartTruncate(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 250 {
		return ""
	}
	head := lines[:120]
	tail := lines[len(lines)-60:]
	out := append(append([]string{}, head...), fmt.Sprintf("... (%d lines omitted) ...", len(lines)-180))
	out = append(out, tail...)
	return strings.Join(out, "\n")
}

func readNumbered(text string, _ config.CompressionConfig) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 400 {
		return ""
	}
	head := lines[:120]
	tail := lines[len(lines)-60:]
	out := append(append([]string{}, head...), fmt.Sprintf("... (%d lines omitted) ...", len(lines)-180))
	out = append(out, tail...)
	return strings.Join(out, "\n")
}
