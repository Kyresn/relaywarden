package compress

import (
	"strings"
	"testing"
)

func BenchmarkCompressGrep100KB(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 5000; i++ {
		sb.WriteString("src/main.go:")
		sb.WriteString("123")
		sb.WriteString(":match content here with some detail\n")
	}
	text := sb.String()
	e := New(testCfg())
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := e.Compress(text); !ok {
			b.Fatal("expected compression")
		}
	}
}

func BenchmarkCompressGitDiff100KB(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("diff --git a/a.go b/a.go\n@@ -1,2000 +1,2000 @@\n")
	for i := 0; i < 2000; i++ {
		sb.WriteString("+line " + strings.Repeat("x", 40) + "\n")
	}
	text := sb.String()
	e := New(testCfg())
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := e.Compress(text); !ok {
			b.Fatal("expected compression")
		}
	}
}
