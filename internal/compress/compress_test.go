package compress

import (
	"strings"
	"testing"

	"cpa-guard/internal/config"
)

func testCfg() config.CompressionConfig {
	cfg := config.Default().Compression
	cfg.Shadow = false
	return cfg
}

func TestGitDiffCompressesAndKeepsChangedFiles(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/internal/api/server.go b/internal/api/server.go\n")
	b.WriteString("index 111..222 100644\n--- a/internal/api/server.go\n+++ b/internal/api/server.go\n")
	b.WriteString("@@ -1,200 +1,220 @@\n")
	b.WriteString("+error: something failed in handler\n")
	for i := 0; i < 180; i++ {
		b.WriteString("+added line with some detail " + strings.Repeat("x", 40) + "\n")
	}
	text := b.String()
	e := New(testCfg())
	out, filter, ok := e.Compress(text)
	if !ok {
		t.Fatalf("expected compression")
	}
	if filter != "git-diff" {
		t.Fatalf("filter = %q", filter)
	}
	if len(out) >= len(text) {
		t.Fatalf("compressed %d >= original %d", len(out), len(text))
	}
	if !strings.Contains(out, "internal/api/server.go") {
		t.Fatalf("file path lost")
	}
	if !strings.Contains(out, "error: something failed") {
		t.Fatalf("error line lost")
	}
	if !strings.Contains(out, "[cpa-guard:") {
		t.Fatalf("marker missing")
	}
}

func TestGrepGroupsByFile(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("src/main.go:")
		b.WriteString(itoa(10 + i))
		b.WriteString(":match content here\n")
	}
	text := b.String()
	e := New(testCfg())
	out, filter, ok := e.Compress(text)
	if !ok || filter != "grep" {
		t.Fatalf("expected grep, got %q ok=%v", filter, ok)
	}
	if !strings.Contains(out, "40 matches in 1 files") {
		t.Fatalf("summary missing: %s", out[:min(len(out), 200)])
	}
	if !strings.Contains(out, "+30") {
		t.Fatalf("overflow marker missing")
	}
}

func TestTinyAndErrorOnlyInputsPassThrough(t *testing.T) {
	e := New(testCfg())
	if out, _, ok := e.Compress("short"); ok || out != "short" {
		t.Fatalf("tiny input changed")
	}
	text := "error: a\n" + strings.Repeat("x", 600) + "\nerror: b\n"
	out, _, ok := e.Compress(text)
	if ok && !strings.Contains(out, "error: a") {
		t.Fatalf("error line lost")
	}
}

func TestSafeModeSkipsToolParsers(t *testing.T) {
	cfg := testCfg()
	cfg.Mode = "safe"
	var b strings.Builder
	b.WriteString("diff --git a/a.go b/a.go\n@@ -1 +1 @@\n")
	for i := 0; i < 80; i++ {
		b.WriteString("+line " + strings.Repeat("x", 30) + "\n")
	}
	e := New(cfg)
	if _, filter, ok := e.Compress(b.String()); ok && filter == "git-diff" {
		t.Fatal("safe mode must not run git-diff")
	}
}

func TestJSONMinify(t *testing.T) {
	text := "{\n  \"a\": 1,\n  \"b\": [1, 2, 3]\n}\n" + strings.Repeat(" ", 600)
	e := New(testCfg())
	out, filter, ok := e.Compress(text)
	if !ok || filter != "json-minify" {
		t.Fatalf("filter = %q ok=%v", filter, ok)
	}
	if strings.Contains(out, "\n  \"a\"") {
		t.Fatalf("whitespace not compacted")
	}
}

func TestDisabledMatchingFilterDoesNotFallThrough(t *testing.T) {
	cfg := testCfg()
	cfg.Filters.Grep = false
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("src/main.go:" + itoa(i) + ":match content here\n")
	}
	e := New(cfg)
	if _, filter, ok := e.Compress(b.String()); ok {
		t.Fatalf("disabled grep fell through to %q", filter)
	}
}

func TestIdempotent(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("src/main.go:" + itoa(i) + ":match content here\n")
	}
	e := New(testCfg())
	out, _, ok := e.Compress(b.String())
	if !ok {
		t.Fatalf("expected compression")
	}
	again, _, ok2 := e.Compress(out)
	if ok2 {
		t.Fatalf("second compression should be skipped")
	}
	if again != out {
		t.Fatalf("idempotence violated")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
