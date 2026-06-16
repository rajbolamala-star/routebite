package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dheerajb/routebite/internal/middleware"
)

func TestOllamaAgentParserSuccess(t *testing.T) {
	server := fakeOllamaServer(t, http.StatusOK, ollamaParseResult{
		Start:            "Kingsport, TN",
		Destination:      "Nashville, TN",
		Preference:       "Indian food",
		MaxDetourMinutes: 8,
		TripIntent:       tripIntentFood,
	})
	defer server.Close()

	parser := NewOllamaAgentParser(OllamaParserConfig{
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: time.Second,
	}, ruleBasedAgentParser{})

	ctx := middleware.ContextWithRequestID(context.Background(), "ollama-test")
	got := parser.Parse(ctx, AgentSearchRequest{
		Query: "driving from Kingsport to Nashville and want Indian food under 8 minutes",
	})

	if got.Start != "Kingsport, TN" {
		t.Fatalf("Start = %q, want Kingsport, TN", got.Start)
	}
	if got.Destination != "Nashville, TN" {
		t.Fatalf("Destination = %q, want Nashville, TN", got.Destination)
	}
	if got.Preference != "Indian" {
		t.Fatalf("Preference = %q, want Indian", got.Preference)
	}
	if got.MaxDetourMinutes != 8 {
		t.Fatalf("MaxDetourMinutes = %d, want 8", got.MaxDetourMinutes)
	}
	if got.TripIntent != tripIntentFood {
		t.Fatalf("TripIntent = %q, want %q", got.TripIntent, tripIntentFood)
	}
}

func TestOllamaAgentParserStructuredFieldsWin(t *testing.T) {
	server := fakeOllamaServer(t, http.StatusOK, ollamaParseResult{
		Start:            "Wrong Start",
		Destination:      "Wrong Destination",
		Preference:       "coffee",
		MaxDetourMinutes: 3,
		TripIntent:       tripIntentCoffee,
	})
	defer server.Close()

	parser := NewOllamaAgentParser(OllamaParserConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	}, ruleBasedAgentParser{})

	got := parser.Parse(context.Background(), AgentSearchRequest{
		Query:            "from wrong to wrong and want coffee under 3 minutes",
		Start:            "North Miami Beach",
		Destination:      "Sunrise",
		Preference:       "soup",
		MaxDetourMinutes: 10,
	})

	if got.Start != "North Miami Beach" || got.Destination != "Sunrise" {
		t.Fatalf("structured route fields were not preserved: %+v", got)
	}
	if got.Preference != "soup" {
		t.Fatalf("Preference = %q, want soup", got.Preference)
	}
	if got.MaxDetourMinutes != 10 {
		t.Fatalf("MaxDetourMinutes = %d, want 10", got.MaxDetourMinutes)
	}
	if got.TripIntent != tripIntentCoffee {
		t.Fatalf("TripIntent = %q, want Ollama trip intent", got.TripIntent)
	}
}

func TestOllamaAgentParserFallsBackOnInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{Response: "not-json"})
	}))
	defer server.Close()

	parser := NewOllamaAgentParser(OllamaParserConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	}, ruleBasedAgentParser{})

	got := parser.Parse(context.Background(), AgentSearchRequest{
		Query: "I am driving from Brooklyn to Newark and want pizza under 8 minutes",
	})

	if got.Start != "Brooklyn" || got.Destination != "Newark" || got.Preference != "pizza" || got.MaxDetourMinutes != 8 {
		t.Fatalf("fallback plan = %+v, want rule-based parse", got)
	}
}

func TestOllamaAgentParserFallsBackOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	parser := NewOllamaAgentParser(OllamaParserConfig{
		BaseURL: server.URL,
		Timeout: time.Second,
	}, ruleBasedAgentParser{})

	got := parser.Parse(context.Background(), AgentSearchRequest{
		Query: "I am driving from Brooklyn to Newark and want pizza under 8 minutes",
	})

	if got.Start != "Brooklyn" || got.Destination != "Newark" || got.Preference != "pizza" || got.MaxDetourMinutes != 8 {
		t.Fatalf("fallback plan = %+v, want rule-based parse", got)
	}
}

func fakeOllamaServer(t *testing.T, status int, result ollamaParseResult) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("path = %q, want /api/generate", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(status)
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("marshal result: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{Response: string(raw)})
	}))
}
