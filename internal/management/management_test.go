package management

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/Kyresn/relaywarden/internal/guard"
	"github.com/Kyresn/relaywarden/internal/metrics"
	"github.com/Kyresn/relaywarden/internal/routing"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func deps(t *testing.T) Deps {
	t.Helper()
	store, err := config.NewStore([]byte(`
enabled: true
compression:
  enabled: true
  shadow: false
  min_bytes: 100
`))
	if err != nil {
		t.Fatal(err)
	}
	return Deps{
		Config: store, Guard: guard.New(store), Routing: routing.New(), Metrics: metrics.New(),
		Assets: fstest.MapFS{
			"index.html":    {Data: []byte("<html>ok</html>")},
			"assets/app.js": {Data: []byte("console.log('ok')")},
		},
		Started: time.Now(),
	}
}

func TestStatusAndPreview(t *testing.T) {
	d := deps(t)
	defer d.Guard.Close()
	resp := Handle(pluginapi.ManagementRequest{Method: "GET", Path: "/relaywarden/status"}, d)
	if resp.StatusCode != 200 || len(resp.Body) == 0 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var status map[string]any
	if err := json.Unmarshal(resp.Body, &status); err != nil {
		t.Fatal(err)
	}
	if status["loaded"] != true {
		t.Fatalf("loaded = %v", status["loaded"])
	}

	body, _ := json.Marshal(map[string]string{"text": "src/main.go:1:match\nsrc/main.go:2:match\n", "format": "openai"})
	resp = Handle(pluginapi.ManagementRequest{Method: "POST", Path: "/relaywarden/compression/preview", Body: body}, d)
	if resp.StatusCode != 200 {
		t.Fatalf("preview = %d: %s", resp.StatusCode, resp.Body)
	}
}

func TestResourceServingAndCSP(t *testing.T) {
	d := deps(t)
	defer d.Guard.Close()
	resp := Handle(pluginapi.ManagementRequest{Method: "GET", Path: "/v0/resource/plugins/relaywarden/ui"}, d)
	if resp.StatusCode != 200 || string(resp.Body) != "<html>ok</html>" {
		t.Fatalf("index = %d %s", resp.StatusCode, resp.Body)
	}
	if resp.Headers.Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
	// The management panel embeds this document in an iframe from the same
	// origin, so the CSP must not forbid that framing.
	csp := resp.Headers.Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Fatalf("CSP does not allow the management panel iframe: %q", csp)
	}
	if strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP forbids the management panel iframe: %q", csp)
	}
	resp = Handle(pluginapi.ManagementRequest{Method: "GET", Path: "/v0/resource/plugins/relaywarden/ui/assets/app.js"}, d)
	if resp.StatusCode != 200 || resp.Headers.Get("Cache-Control") == "" {
		t.Fatalf("asset = %d", resp.StatusCode)
	}
	if resp.Headers.Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("content type = %q", resp.Headers.Get("Content-Type"))
	}
}

func TestUnknownPath(t *testing.T) {
	d := deps(t)
	defer d.Guard.Close()
	resp := Handle(pluginapi.ManagementRequest{Method: "GET", Path: "/relaywarden/nope"}, d)
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
