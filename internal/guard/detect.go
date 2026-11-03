package guard

import (
	"math"
	"strings"

	"github.com/Kyresn/relaywarden/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

func responseModel(body []byte) string {
	for _, path := range []string{"model", "modelVersion", "response.model"} {
		if v := gjson.GetBytes(body, path); v.Exists() && strings.TrimSpace(v.String()) != "" {
			return strings.TrimSpace(v.String())
		}
	}
	return ""
}

func modelsMatch(requested, response string) bool {
	req := normalizeModel(requested)
	resp := normalizeModel(response)
	if req == "" || resp == "" {
		return true
	}
	if req == resp {
		return true
	}
	if stripDate(req) == stripDate(resp) {
		return true
	}
	// Do not treat arbitrary suffixes as the same model: "-mini", "-flash" and
	// "-pro" are different tiers, not dated variants. Date suffixes are handled
	// above.
	return false
}

func modelsMatchAny(response string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if modelsMatch(candidate, response) {
			return true
		}
	}
	return false
}

func normalizeModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	m = strings.ReplaceAll(m, "_", "-")
	return m
}

func stripDate(model string) string {
	// Strip common dated suffixes: -2026-11-01, -20261101, -v2026-11-01.
	for i := len(model) - 1; i >= 0; i-- {
		if model[i] != '-' {
			continue
		}
		suffix := model[i+1:]
		if len(suffix) == 10 && suffix[4] == '-' && suffix[7] == '-' {
			return model[:i]
		}
		if len(suffix) == 8 && allDigits(suffix) {
			return model[:i]
		}
	}
	return model
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func responseHasReasoning(body []byte, format string) bool {
	switch format {
	case "responses":
		if gjson.GetBytes(body, "usage.output_tokens_details.reasoning_tokens").Int() > 0 {
			return true
		}
		for _, item := range gjson.GetBytes(body, "output").Array() {
			if item.Get("type").String() == "reasoning" {
				return true
			}
		}
	case "openai":
		if gjson.GetBytes(body, "usage.completion_tokens_details.reasoning_tokens").Int() > 0 {
			return true
		}
		for _, choice := range gjson.GetBytes(body, "choices").Array() {
			if choice.Get("message.reasoning_content").Exists() || choice.Get("message.reasoning").Exists() {
				return true
			}
		}
	case "gemini":
		if gjson.GetBytes(body, "usageMetadata.thoughtsTokenCount").Int() > 0 {
			return true
		}
		for _, cand := range gjson.GetBytes(body, "candidates").Array() {
			for _, part := range cand.Get("content.parts").Array() {
				if part.Get("thought").Bool() {
					return true
				}
			}
		}
	}
	return false
}

func responseCacheTokens(body []byte, format string) (cached, prompt int64) {
	switch format {
	case "responses":
		return gjson.GetBytes(body, "usage.input_tokens_details.cached_tokens").Int(),
			gjson.GetBytes(body, "usage.input_tokens").Int()
	case "openai":
		return gjson.GetBytes(body, "usage.prompt_tokens_details.cached_tokens").Int(),
			gjson.GetBytes(body, "usage.prompt_tokens").Int()
	case "gemini":
		return gjson.GetBytes(body, "usageMetadata.cachedContentTokenCount").Int(),
			gjson.GetBytes(body, "usageMetadata.promptTokenCount").Int()
	}
	return 0, 0
}

func cacheMissReason(body []byte) string {
	for _, path := range []string{
		"prompt_cache_diagnostics.cache_miss.reason",
		"diagnostics.cache_miss_reason.type",
		"usage.prompt_cache_diagnostics.cache_miss.reason",
	} {
		if v := gjson.GetBytes(body, path); v.Exists() && strings.TrimSpace(v.String()) != "" {
			return strings.ToLower(strings.TrimSpace(v.String()))
		}
	}
	return ""
}

func truncationReason(body []byte, format string) string {
	switch format {
	case "responses":
		status := gjson.GetBytes(body, "status").String()
		reason := gjson.GetBytes(body, "incomplete_details.reason").String()
		if status == "incomplete" && reason != "" {
			return reason
		}
	case "openai":
		for _, choice := range gjson.GetBytes(body, "choices").Array() {
			fr := choice.Get("finish_reason").String()
			if fr == "length" || fr == "max_tokens" {
				return fr
			}
		}
	case "gemini":
		for _, cand := range gjson.GetBytes(body, "candidates").Array() {
			fr := strings.ToUpper(cand.Get("finishReason").String())
			if fr == "MAX_TOKENS" || fr == "LENGTH" {
				return fr
			}
		}
	}
	return ""
}

func responseEmpty(body []byte, format string) bool {
	switch format {
	case "responses":
		out := gjson.GetBytes(body, "output")
		if out.IsArray() {
			if len(out.Array()) == 0 {
				return true
			}
			for _, item := range out.Array() {
				if item.Get("content").Exists() || item.Get("type").String() == "function_call" || item.Get("type").String() == "custom_tool_call" || item.Get("type").String() == "reasoning" {
					return false
				}
			}
		}
		return gjson.GetBytes(body, "usage.output_tokens").Int() == 0 && !gjson.GetBytes(body, "error").Exists()
	case "openai":
		choices := gjson.GetBytes(body, "choices").Array()
		if len(choices) == 0 {
			return true
		}
		for _, choice := range choices {
			msg := choice.Get("message")
			if msg.Get("content").String() != "" || msg.Get("tool_calls").Exists() || msg.Get("function_call").Exists() {
				return false
			}
		}
		return true
	case "gemini":
		cands := gjson.GetBytes(body, "candidates").Array()
		if len(cands) == 0 {
			return true
		}
		for _, cand := range cands {
			if len(cand.Get("content.parts").Array()) > 0 {
				return false
			}
		}
		return true
	}
	return false
}

func usageInvariant(rec pluginapi.UsageRecord) string {
	d := rec.Detail
	if d.CachedTokens > d.InputTokens && d.InputTokens > 0 {
		return "cached_gt_input"
	}
	if d.ReasoningTokens > d.OutputTokens && d.OutputTokens > 0 {
		return "reasoning_gt_output"
	}
	if d.TotalTokens > 0 && d.InputTokens+d.OutputTokens > 0 {
		want := d.InputTokens + d.OutputTokens
		tolerance := int64(math.Max(10, float64(want)*0.2))
		diff := d.TotalTokens - want
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			return "total_mismatch"
		}
	}
	return ""
}

func tierRank(tier string) int {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "priority":
		return 3
	case "default", "standard":
		return 2
	case "auto":
		return 1
	case "flex", "batch", "economy":
		return 0
	default:
		return -1
	}
}

func severityForReasoning(model string) string {
	m := normalizeModel(model)
	for _, family := range []string{"gpt-5", "o3", "o4", "gemini-3", "deepseek", "kimi", "claude", "grok-4"} {
		if strings.Contains(m, family) {
			return SeverityHigh
		}
	}
	return SeverityMedium
}

type ttftStat struct {
	mean    float64
	outMean float64
	samples int
}

func (g *Guard) noteTTFT(cfg *config.Config, rec pluginapi.UsageRecord) {
	key := rec.AuthID + "|" + rec.Model
	g.mu.Lock()
	if g.ttft == nil {
		g.ttft = make(map[string]*ttftStat)
	}
	st := g.ttft[key]
	if st == nil {
		st = &ttftStat{}
		g.ttft[key] = st
	}
	anomaly := st.samples >= 5 && st.mean > 0 &&
		float64(rec.TTFT) < st.mean*0.2 &&
		st.outMean > 0 && float64(rec.Detail.OutputTokens) < st.outMean*0.3
	if st.samples == 0 {
		st.mean = float64(rec.TTFT)
		st.outMean = float64(rec.Detail.OutputTokens)
	} else {
		st.mean = st.mean*0.8 + float64(rec.TTFT)*0.2
		st.outMean = st.outMean*0.8 + float64(rec.Detail.OutputTokens)*0.2
	}
	st.samples++
	g.mu.Unlock()
	if anomaly {
		g.emit(Signal{
			Severity: SeverityLow, Code: "ttft_anomaly",
			RequestID: rec.RequestID, Provider: rec.Provider, AuthID: rec.AuthID, Model: rec.Model,
			Detail: map[string]any{"ttft_ms": rec.TTFT.Milliseconds(), "baseline_ms": int64(st.mean)},
		})
	}
}
