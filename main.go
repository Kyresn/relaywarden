package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/Kyresn/relaywarden/internal/compress"
	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/Kyresn/relaywarden/internal/guard"
	"github.com/Kyresn/relaywarden/internal/management"
	"github.com/Kyresn/relaywarden/internal/metrics"
	"github.com/Kyresn/relaywarden/internal/protocol"
	"github.com/Kyresn/relaywarden/internal/routing"
	"github.com/Kyresn/relaywarden/internal/style"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	RequestInterceptor     bool `json:"request_interceptor"`
	RequestLifecyclePlugin bool `json:"request_lifecycle_plugin"`
	ResponseInterceptor    bool `json:"response_interceptor"`
	UsagePlugin            bool `json:"usage_plugin"`
	Scheduler              bool `json:"scheduler"`
	ManagementAPI          bool `json:"management_api"`
}

type pluginState struct {
	mu            sync.Mutex
	cfg           *config.Store
	guard         *guard.Guard
	routing       *routing.Stats
	metrics       *metrics.Store
	started       time.Time
	closed        bool
	schemaVersion uint32
}

var state = &pluginState{started: time.Now(), schemaVersion: 7}

//go:embed web/dist
var webDist embed.FS

var webAssets fs.FS

func init() {
	if sub, err := fs.Sub(webDist, "web/dist"); err == nil {
		webAssets = sub
	}
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required", 0))
		return 1
	}
	var raw []byte
	if request != nil && requestLen > 0 {
		raw = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	out, err := handleMethod(C.GoString(method), raw)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error(), 0))
		return 1
	}
	writeResponse(response, out)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	state.shutdown()
}

func handleMethod(method string, raw []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		var req lifecycleRequest
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, err
			}
		}
		if err := state.configure(req.ConfigYAML, req.SchemaVersion); err != nil {
			return nil, err
		}
		return okEnvelope(registrationFor(state.currentSchema()))
	case pluginabi.MethodPluginQuiesce:
		return okEnvelope(struct{}{})
	case pluginabi.MethodPluginShutdown:
		state.shutdown()
		return okEnvelope(struct{}{})
	case pluginabi.MethodRequestInterceptBefore:
		var req pluginapi.RequestInterceptRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return okEnvelope(state.interceptBefore(req))
	case pluginabi.MethodRequestInterceptAfter:
		var req pluginapi.RequestInterceptRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return okEnvelope(state.interceptAfter(req))
	case pluginabi.MethodResponseInterceptAfter:
		var req pluginapi.ResponseInterceptRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		state.observeResponse(req)
		return okEnvelope(pluginapi.ResponseInterceptResponse{})
	case pluginabi.MethodUsageHandle:
		var rec pluginapi.UsageRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, err
		}
		state.observeUsage(rec)
		return okEnvelope(struct{}{})
	case pluginabi.MethodRequestComplete:
		var completion pluginapi.RequestCompletion
		if err := json.Unmarshal(raw, &completion); err != nil {
			return nil, err
		}
		state.complete(completion)
		return okEnvelope(struct{}{})
	case pluginabi.MethodSchedulerPick:
		var req pluginapi.SchedulerPickRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return okEnvelope(state.pick(req))
	case pluginabi.MethodManagementRegister:
		return okEnvelope(map[string]any{
			"routes":    management.Routes(),
			"resources": management.Resources(webAssets),
		})
	case pluginabi.MethodManagementHandle:
		var req pluginapi.ManagementRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return okEnvelope(management.Handle(req, state.deps()))
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method, 0), nil
	}
}

func (s *pluginState) configure(raw []byte, hostSchema uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	schema := hostSchema
	if schema == 0 || schema > pluginabi.SchemaVersion {
		schema = 7
	}
	s.schemaVersion = schema
	if s.cfg == nil {
		store, err := config.NewStore(raw)
		if err != nil {
			return err
		}
		s.cfg = store
		s.guard = guard.New(store)
		s.routing = routing.New()
		s.metrics = metrics.New()
		s.closed = false
		return nil
	}
	if err := s.cfg.Update(raw); err != nil {
		return err
	}
	if s.closed || s.guard == nil {
		s.guard = guard.New(s.cfg)
	}
	if s.routing == nil {
		s.routing = routing.New()
	}
	if s.metrics == nil {
		s.metrics = metrics.New()
	}
	s.closed = false
	return nil
}

func (s *pluginState) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.guard != nil {
		s.guard.Close()
	}
}

func (s *pluginState) deps() management.Deps {
	s.mu.Lock()
	defer s.mu.Unlock()
	return management.Deps{Config: s.cfg, Guard: s.guard, Routing: s.routing, Metrics: s.metrics, Assets: webAssets, Started: s.started}
}

func (s *pluginState) currentSchema() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.schemaVersion == 0 {
		return 7
	}
	return s.schemaVersion
}

func (s *pluginState) currentConfig() *config.Config {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if cfg == nil {
		return nil
	}
	return cfg.Load()
}

func compressionEnabledFor(cfg *config.Config, format string) bool {
	if cfg == nil || !protocol.Supported(format) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "openai", "chat", "chat-completions":
		return cfg.Compression.Protocols.OpenAIChat
	case "openai-response", "openai_responses", "responses", "codex":
		return cfg.Compression.Protocols.OpenAIResponses
	case "gemini", "google":
		return cfg.Compression.Protocols.Gemini
	default:
		return false
	}
}

func (s *pluginState) interceptBefore(req pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
	cfg := s.currentConfig()
	if cfg == nil {
		return pluginapi.RequestInterceptResponse{}
	}
	body := req.Body
	if cfg.Guard.Enabled && s.guard != nil {
		if injected := s.guard.Before(req); len(injected) > 0 {
			body = injected
		}
	}
	if cfg.Compression.Enabled && compressionEnabledFor(cfg, req.SourceFormat) && s.metrics != nil {
		if cfg.Metrics.Enabled {
			s.metrics.RecordRequest()
		}
		engine := compress.New(cfg.Compression)
		res, err := protocol.Compress(body, req.SourceFormat, cfg.Compression.TailOnly, cfg.Compression.TailItems, engine.Compress)
		if err == nil && len(res.Hits) > 0 {
			if cfg.Metrics.Enabled {
				s.metrics.Record(req.Model, req.SourceFormat, res, cfg.Compression.Shadow)
			}
			if !cfg.Compression.Shadow {
				body = res.Body
			}
		}
	}
	body = s.applyStyle(body, req, cfg)
	if string(body) == string(req.Body) {
		return pluginapi.RequestInterceptResponse{}
	}
	return pluginapi.RequestInterceptResponse{Body: body}
}

// applyStyle strips any region this plugin injected earlier and appends the
// current level's prompt. It runs after compression so the guard's before-profile
// and the compressed tool output are already in place.
func (s *pluginState) applyStyle(body []byte, req pluginapi.RequestInterceptRequest, cfg *config.Config) []byte {
	text := style.CombinedText(cfg.Caveman.Level, cfg.Ponytail.Level)
	if text == "" && !bytes.Contains(body, []byte(style.MarkerStart)) {
		return body
	}
	res, err := style.Apply(body, req.SourceFormat, text)
	if cfg.Metrics.Enabled && s.metrics != nil {
		if err != nil {
			s.metrics.RecordStyle(cfg.Caveman.Level, cfg.Ponytail.Level, false, false, true)
		} else {
			s.metrics.RecordStyle(cfg.Caveman.Level, cfg.Ponytail.Level, res.Injected, res.Stripped, false)
		}
	}
	if err != nil || len(res.Body) == 0 {
		return body
	}
	return res.Body
}

func (s *pluginState) interceptAfter(req pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
	cfg := s.currentConfig()
	if cfg == nil || !cfg.Guard.Enabled || s.guard == nil {
		return pluginapi.RequestInterceptResponse{}
	}
	if body := s.guard.After(req); len(body) > 0 {
		return pluginapi.RequestInterceptResponse{Body: body}
	}
	return pluginapi.RequestInterceptResponse{}
}

func (s *pluginState) observeResponse(req pluginapi.ResponseInterceptRequest) {
	if s.guard != nil {
		s.guard.ObserveResponse(req)
	}
}

func (s *pluginState) observeUsage(rec pluginapi.UsageRecord) {
	cfg := s.currentConfig()
	if s.guard != nil {
		s.guard.ObserveUsage(rec)
	}
	if cfg != nil && s.routing != nil {
		s.routing.Observe(rec, &cfg.Routing)
	}
}

func (s *pluginState) complete(completion pluginapi.RequestCompletion) {
	if s.guard != nil {
		s.guard.Complete(completion.RequestID)
	}
}

func (s *pluginState) pick(req pluginapi.SchedulerPickRequest) pluginapi.SchedulerPickResponse {
	cfg := s.currentConfig()
	if cfg == nil || s.routing == nil {
		return pluginapi.SchedulerPickResponse{Handled: false}
	}
	authID, ok := s.routing.Pick(req, &cfg.Routing)
	if !ok || authID == "" {
		return pluginapi.SchedulerPickResponse{Handled: false}
	}
	return pluginapi.SchedulerPickResponse{AuthID: authID, Handled: true}
}

func registrationFor(schemaVersion uint32) registration {
	return registration{
		SchemaVersion: schemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "RelayWarden",
			Version:          guard.Version,
			Author:           "Kyresn",
			GitHubRepository: "https://github.com/Kyresn/relaywarden",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "enabled", Type: pluginapi.ConfigFieldTypeBoolean, Default: true, Description: "Load the plugin."},
				{Name: "compression", Type: pluginapi.ConfigFieldTypeObject, Description: "Tool-output compression settings and switches."},
				{Name: "guard", Type: pluginapi.ConfigFieldTypeObject, Description: "Fidelity observation and alert settings."},
				{Name: "routing", Type: pluginapi.ConfigFieldTypeObject, Description: "Cache-affinity and latency-aware scheduling."},
				{Name: "caveman", Type: pluginapi.ConfigFieldTypeObject, Description: "Terse-answer behaviour prompt level."},
				{Name: "ponytail", Type: pluginapi.ConfigFieldTypeObject, Description: "Minimal-code behaviour prompt level."},
				{Name: "metrics", Type: pluginapi.ConfigFieldTypeObject, Description: "In-memory counters and signal retention."},
				{Name: "ui", Type: pluginapi.ConfigFieldTypeObject, Description: "Embedded management UI settings."},
			},
		},
		Capabilities: registrationCapability{
			RequestInterceptor:     true,
			RequestLifecyclePlugin: true,
			ResponseInterceptor:    true,
			UsagePlugin:            true,
			Scheduler:              true,
			ManagementAPI:          true,
		},
	}
}

func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string, status int) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message, HTTPStatus: status}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
