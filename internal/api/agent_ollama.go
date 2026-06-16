package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dheerajb/routebite/internal/middleware"
)

type OllamaParserConfig struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

type ollamaAgentParser struct {
	fallback agentParser
	hc       *http.Client
	baseURL  string
	model    string
}

func NewOllamaAgentParser(config OllamaParserConfig, fallback agentParser) agentParser {
	if fallback == nil {
		fallback = ruleBasedAgentParser{}
	}
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Model == "" {
		config.Model = "llama3.2:3b"
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	return &ollamaAgentParser{
		fallback: fallback,
		hc:       &http.Client{Timeout: config.Timeout},
		baseURL:  strings.TrimRight(config.BaseURL, "/"),
		model:    config.Model,
	}
}

func (p *ollamaAgentParser) Parse(ctx context.Context, req AgentSearchRequest) agentPlan {
	base := p.fallback.Parse(ctx, req)
	p.log(ctx, "ollama_parser_enabled", nil)

	if strings.TrimSpace(req.Query) == "" {
		p.log(ctx, "ollama_parser_fallback", map[string]any{"reason": "empty_query"})
		return base
	}

	parsed, err := p.parseWithOllama(ctx, req)
	if err != nil {
		p.log(ctx, "ollama_parser_error", map[string]any{"error": err.Error()})
		p.log(ctx, "ollama_parser_fallback", map[string]any{"reason": "ollama_error"})
		return base
	}
	if parsed.Start == "" || parsed.Destination == "" || parsed.Preference == "" {
		p.log(ctx, "ollama_parser_fallback", map[string]any{"reason": "missing_required_fields"})
		return base
	}

	merged := mergeOllamaPlan(base, parsed, req)
	if merged.Start == "" || merged.Destination == "" || merged.Preference == "" {
		p.log(ctx, "ollama_parser_fallback", map[string]any{"reason": "missing_required_fields"})
		return base
	}

	p.log(ctx, "ollama_parser_success", nil)
	return merged
}

func (p *ollamaAgentParser) parseWithOllama(ctx context.Context, req AgentSearchRequest) (agentPlan, error) {
	body := ollamaGenerateRequest{
		Model:  p.model,
		Stream: false,
		Format: "json",
		Prompt: ollamaPrompt(req),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return agentPlan{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(raw))
	if err != nil {
		return agentPlan{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.hc.Do(httpReq)
	if err != nil {
		return agentPlan{}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return agentPlan{}, fmt.Errorf("ollama returned %s", resp.Status)
	}

	var out ollamaGenerateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return agentPlan{}, fmt.Errorf("decode ollama response: %w", err)
	}
	if strings.TrimSpace(out.Response) == "" {
		return agentPlan{}, fmt.Errorf("empty ollama response")
	}

	var parsed ollamaParseResult
	if err := json.Unmarshal([]byte(extractJSONObject(out.Response)), &parsed); err != nil {
		return agentPlan{}, fmt.Errorf("decode ollama JSON: %w", err)
	}
	return parsed.toPlan(), nil
}

func (p *ollamaAgentParser) log(ctx context.Context, event string, fields map[string]any) {
	entry := map[string]any{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"event":      event,
		"request_id": middleware.RequestIDFromContext(ctx),
	}
	for k, v := range fields {
		entry[k] = v
	}
	raw, _ := json.Marshal(entry)
	log.Println(string(raw))
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

type ollamaParseResult struct {
	Start            string `json:"start"`
	Destination      string `json:"destination"`
	Preference       string `json:"preference"`
	MaxDetourMinutes int    `json:"max_detour_minutes"`
	TripIntent       string `json:"trip_intent"`
}

func (r ollamaParseResult) toPlan() agentPlan {
	return agentPlan{
		Start:            cleanPlace(r.Start),
		Destination:      cleanPlace(r.Destination),
		Preference:       normalizePreference(r.Preference),
		MaxDetourMinutes: normalizeDetour(r.MaxDetourMinutes),
		OpenNowOnly:      true,
		TripIntent:       normalizeTripIntent(r.TripIntent),
	}
}

func mergeOllamaPlan(base agentPlan, parsed agentPlan, req AgentSearchRequest) agentPlan {
	merged := base
	if strings.TrimSpace(req.Start) == "" && parsed.Start != "" {
		merged.Start = parsed.Start
	}
	if strings.TrimSpace(req.Destination) == "" && parsed.Destination != "" {
		merged.Destination = parsed.Destination
	}
	if strings.TrimSpace(req.Preference) == "" && parsed.Preference != "" {
		merged.Preference = parsed.Preference
	}
	if req.MaxDetourMinutes <= 0 && parsed.MaxDetourMinutes > 0 {
		merged.MaxDetourMinutes = parsed.MaxDetourMinutes
	}
	if parsed.TripIntent != tripIntentUnknown {
		merged.TripIntent = parsed.TripIntent
	}
	merged.MaxDetourMinutes = normalizeDetour(merged.MaxDetourMinutes)
	merged.Preference = normalizePreference(merged.Preference)
	merged.TripIntent = normalizeTripIntent(merged.TripIntent)
	return merged
}

func ollamaPrompt(req AgentSearchRequest) string {
	return fmt.Sprintf(`Extract route-food assistant fields from the user request.
Return only JSON with this exact shape:
{"start":"","destination":"","preference":"","max_detour_minutes":0,"trip_intent":"unknown"}

Allowed trip_intent values: food, soup, coffee, gas_food, restroom_food, unknown.
Use empty strings or 0 when unknown. Do not invent locations.

Structured fields, if provided, are:
start=%q
destination=%q
preference=%q
max_detour_minutes=%d

User request:
%q`, req.Start, req.Destination, req.Preference, req.MaxDetourMinutes, req.Query)
}

func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end >= start {
		return s[start : end+1]
	}
	return s
}
