package config

import "testing"

func TestDefaultsSurvivePartialYAML(t *testing.T) {
	cfg, err := Parse([]byte(`
enabled: true
compression:
  enabled: false
guard:
  enabled: false
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Compression.Mode != "balanced" || !cfg.Compression.TailOnly {
		t.Fatalf("compression defaults lost: %+v", cfg.Compression)
	}
	if !cfg.Compression.Filters.GitDiff {
		t.Fatal("filter default lost")
	}
	if !cfg.Guard.Signals.ModelMismatch {
		t.Fatal("signal default lost")
	}
	if cfg.Guard.ObserveOnly != true {
		t.Fatal("observe_only must be locked true")
	}
}

func TestWeightsNormalize(t *testing.T) {
	cfg, err := Parse([]byte(`
routing:
  weights: { ttft: 3, error: 1, degrade: 1 }
`))
	if err != nil {
		t.Fatal(err)
	}
	sum := cfg.Routing.Weights.TTFT + cfg.Routing.Weights.Error + cfg.Routing.Weights.Degrade
	if sum < 0.999 || sum > 1.001 {
		t.Fatalf("weights sum = %v", sum)
	}
}

func TestWebhookRequiresURL(t *testing.T) {
	if _, err := Parse([]byte(`
guard:
  alerts:
    webhook:
      enabled: true
      url: "ftp://example.com/hook"
`)); err == nil {
		t.Fatal("expected invalid webhook error")
	}
}

func TestInvalidEnumsRejected(t *testing.T) {
	if _, err := Parse([]byte("compression:\n  mode: turbo\n")); err == nil {
		t.Fatal("expected mode error")
	}
	if _, err := Parse([]byte("routing:\n  strategy: fastest\n")); err == nil {
		t.Fatal("expected strategy error")
	}
	if _, err := Parse([]byte("caveman:\n  level: shouty\n")); err == nil {
		t.Fatal("expected caveman level error")
	}
	if _, err := Parse([]byte("ponytail:\n  level: shouty\n")); err == nil {
		t.Fatal("expected ponytail level error")
	}
	if cfg, err := Parse(nil); err != nil || cfg.Caveman.Level != "off" || cfg.Ponytail.Level != "off" {
		t.Fatalf("style levels must default to off: %+v %v", cfg.Caveman, err)
	}
}
