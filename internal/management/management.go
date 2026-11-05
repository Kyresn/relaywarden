// Package management exposes the plugin's read-only management API.
package management

import (
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Kyresn/relaywarden/internal/compress"
	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/Kyresn/relaywarden/internal/guard"
	"github.com/Kyresn/relaywarden/internal/metrics"
	"github.com/Kyresn/relaywarden/internal/routing"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type Deps struct {
	Config  *config.Store
	Guard   *guard.Guard
	Routing *routing.Stats
	Metrics *metrics.Store
	Assets  fs.FS
	Started time.Time
}

// Routes returns the management routes. Menu is intentionally empty: a GET
// route with Menu becomes an unauthenticated resource route in CPA.
func Routes() []pluginapi.ManagementRoute {
	return []pluginapi.ManagementRoute{
		{Method: http.MethodGet, Path: "/relaywarden/status", Description: "Plugin status and effective switches"},
		{Method: http.MethodGet, Path: "/relaywarden/config", Description: "Current plugin configuration"},
		{Method: http.MethodGet, Path: "/relaywarden/compression/stats", Description: "Compression counters"},
		{Method: http.MethodPost, Path: "/relaywarden/compression/preview", Description: "Preview compression for one text blob"},
		{Method: http.MethodGet, Path: "/relaywarden/signals", Description: "Recent degradation signals"},
		{Method: http.MethodGet, Path: "/relaywarden/signals/summary", Description: "Degradation counters by code"},
		{Method: http.MethodGet, Path: "/relaywarden/usage", Description: "Recent usage records for diagnostics"},
		{Method: http.MethodGet, Path: "/relaywarden/routing", Description: "Per-auth latency and health windows"},
	}
}

// Resources returns the static UI routes. CPA resource routes are exact paths,
// so each hashed asset gets its own route.
func Resources(assets fs.FS) []pluginapi.ResourceRoute {
	out := []pluginapi.ResourceRoute{{
		Path:        "/ui",
		Menu:        "RelayWarden",
		Description: "Compression, fidelity alerts, and latency routing for this gateway.",
	}}
	if assets == nil {
		return out
	}
	matches, err := fs.Glob(assets, "assets/*")
	if err != nil {
		return out
	}
	for _, name := range matches {
		out = append(out, pluginapi.ResourceRoute{Path: "/ui/" + name})
	}
	return out
}

func Handle(req pluginapi.ManagementRequest, deps Deps) pluginapi.ManagementResponse {
	if strings.Contains(req.Path, "/resource/plugins/") {
		return serveResource(req, deps)
	}
	path := normalizePath(req.Path)
	switch {
	case req.Method == http.MethodGet && path == "/relaywarden/status":
		return jsonResponse(status(deps))
	case req.Method == http.MethodGet && path == "/relaywarden/config":
		return jsonResponse(deps.Config.Load())
	case req.Method == http.MethodGet && path == "/relaywarden/compression/stats":
		return jsonResponse(deps.Metrics.Snapshot())
	case req.Method == http.MethodPost && path == "/relaywarden/compression/preview":
		return preview(req, deps)
	case req.Method == http.MethodGet && path == "/relaywarden/signals":
		limit := 100
		if raw := req.Query.Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		return jsonResponse(map[string]any{"signals": deps.Guard.Signals(limit)})
	case req.Method == http.MethodGet && path == "/relaywarden/signals/summary":
		return jsonResponse(map[string]any{"counters": deps.Guard.Summary()})
	case req.Method == http.MethodGet && path == "/relaywarden/usage":
		return jsonResponse(map[string]any{"records": deps.Guard.RecentUsage(20)})
	case req.Method == http.MethodGet && path == "/relaywarden/routing":
		return jsonResponse(map[string]any{"auths": deps.Routing.Snapshot()})
	default:
		return errorResponse(http.StatusNotFound, "not_found", "unknown relaywarden management path")
	}
}

func status(deps Deps) map[string]any {
	cfg := deps.Config.Load()
	if cfg == nil {
		return map[string]any{"loaded": false}
	}
	comp := deps.Metrics.Snapshot()
	return map[string]any{
		"loaded":          true,
		"version":         guard.Version,
		"started_at":      deps.Started.UTC(),
		"config_revision": deps.Config.Revision(),
		"enabled":         cfg.Enabled,
		"compression": map[string]any{
			"enabled":      cfg.Compression.Enabled,
			"mode":         cfg.Compression.Mode,
			"shadow":       cfg.Compression.Shadow,
			"tail_only":    cfg.Compression.TailOnly,
			"saved":        comp.Total.SavedBytes(),
			"saved_pct":    comp.Total.SavingsPercent(),
			"shadow_saved": comp.Shadow.SavedBytes(),
		},
		"guard": map[string]any{
			"enabled":      cfg.Guard.Enabled,
			"observe_only": cfg.Guard.ObserveOnly,
			"signals":      deps.Guard.Summary(),
		},
		"routing": map[string]any{
			"enabled":  cfg.Routing.Enabled,
			"strategy": cfg.Routing.Strategy,
			"auths":    len(deps.Routing.Snapshot()),
		},
		"style": map[string]any{
			"caveman":  cfg.Caveman.Level,
			"ponytail": cfg.Ponytail.Level,
			"injected": comp.Style.Injected,
			"stripped": comp.Style.Stripped,
			"failed":   comp.Style.Failed,
		},
		"ui": map[string]any{
			"enabled":          cfg.UI.Enabled,
			"poll_interval_ms": cfg.UI.PollIntervalMS,
			"show_advanced":    cfg.UI.ShowAdvanced,
		},
	}
}

func preview(req pluginapi.ManagementRequest, deps Deps) pluginapi.ManagementResponse {
	var body struct {
		Text   string `json:"text"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return errorResponse(http.StatusBadRequest, "invalid_json", err.Error())
	}
	cfg := deps.Config.Load()
	if cfg == nil || !cfg.Compression.Enabled {
		return jsonResponse(map[string]any{"enabled": false, "output": body.Text})
	}
	e := compress.New(cfg.Compression)
	out, filter, ok := e.Compress(body.Text)
	if !ok {
		out = body.Text
	}
	return jsonResponse(map[string]any{
		"enabled": true,
		"shadow":  cfg.Compression.Shadow,
		"filter":  filter,
		"before":  len(body.Text),
		"after":   len(out),
		"saved":   len(body.Text) - len(out),
		"output":  out,
	})
}

func serveResource(req pluginapi.ManagementRequest, deps Deps) pluginapi.ManagementResponse {
	if deps.Assets == nil {
		return errorResponse(http.StatusServiceUnavailable, "ui_unavailable", "management UI assets are not embedded")
	}
	p := req.Path
	idx := strings.Index(p, "/relaywarden")
	if idx < 0 {
		return errorResponse(http.StatusNotFound, "not_found", "unknown relaywarden resource path")
	}
	rel := p[idx+len("/relaywarden"):]
	rel = strings.TrimPrefix(rel, "/ui")
	if rel == "" || rel == "/" {
		rel = "/index.html"
	}
	clean := path.Clean("/" + rel)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || strings.HasPrefix(clean, "../") {
		return errorResponse(http.StatusNotFound, "not_found", "invalid resource path")
	}
	data, err := fs.ReadFile(deps.Assets, clean)
	if err != nil {
		return errorResponse(http.StatusNotFound, "not_found", "resource not found")
	}
	contentType := mime.TypeByExtension(path.Ext(clean))
	switch path.Ext(clean) {
	case ".js":
		contentType = "text/javascript; charset=utf-8"
	case ".css":
		contentType = "text/css; charset=utf-8"
	case ".html":
		contentType = "text/html; charset=utf-8"
	case ".svg":
		contentType = "image/svg+xml"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers := http.Header{
		"Content-Type":           []string{contentType},
		"X-Content-Type-Options": []string{"nosniff"},
		"Referrer-Policy":        []string{"no-referrer"},
	}
	if path.Ext(clean) == ".html" {
		headers.Set("Cache-Control", "no-store")
		headers.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'self'")
	} else {
		headers.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: headers, Body: data}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if i := strings.Index(path, "/relaywarden/"); i >= 0 {
		path = path[i:]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

func jsonResponse(v any) pluginapi.ManagementResponse {
	raw, err := json.Marshal(v)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "encode_error", err.Error())
	}
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}, "Cache-Control": []string{"no-store"}},
		Body:       raw,
	}
}

func errorResponse(status int, code, message string) pluginapi.ManagementResponse {
	raw, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       raw,
	}
}
