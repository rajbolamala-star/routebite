package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dheerajb/routebite/internal/cache"
	"github.com/dheerajb/routebite/internal/geocode"
	"github.com/dheerajb/routebite/internal/history"
	"github.com/dheerajb/routebite/internal/middleware"
	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/scoring"
	"github.com/dheerajb/routebite/internal/yelp"
	"github.com/gin-gonic/gin"
)

func TestParseAgentRequest_QueryOnly(t *testing.T) {
	got := parseAgentRequest(AgentSearchRequest{
		Query: "I am driving from North Miami Beach to Sunrise and want Indian food with less than 10 minutes detour",
	})

	if got.Start != "North Miami Beach" {
		t.Fatalf("Start = %q, want North Miami Beach", got.Start)
	}
	if got.Destination != "Sunrise" {
		t.Fatalf("Destination = %q, want Sunrise", got.Destination)
	}
	if got.Preference != "Indian" {
		t.Fatalf("Preference = %q, want Indian", got.Preference)
	}
	if got.MaxDetourMinutes != 10 {
		t.Fatalf("MaxDetourMinutes = %d, want 10", got.MaxDetourMinutes)
	}
}

func TestParseAgentRequest_StructuredFieldsWin(t *testing.T) {
	got := parseAgentRequest(AgentSearchRequest{
		Query:            "from Brooklyn to Newark and want pizza under 8 minutes",
		Start:            "North Miami Beach",
		Destination:      "Sunrise",
		Preference:       "Soup Food",
		MaxDetourMinutes: 12,
	})

	if got.Start != "North Miami Beach" || got.Destination != "Sunrise" {
		t.Fatalf("structured route fields were not preserved: %+v", got)
	}
	if got.Preference != "Soup" {
		t.Fatalf("Preference = %q, want Soup", got.Preference)
	}
	if got.MaxDetourMinutes != 12 {
		t.Fatalf("MaxDetourMinutes = %d, want 12", got.MaxDetourMinutes)
	}
}

func TestParseAgentRequest_ForPreference(t *testing.T) {
	got := parseAgentRequest(AgentSearchRequest{
		Query: "driving from Brooklyn to Newark for Indian food under 7 minutes",
	})

	if got.Start != "Brooklyn" {
		t.Fatalf("Start = %q, want Brooklyn", got.Start)
	}
	if got.Destination != "Newark" {
		t.Fatalf("Destination = %q, want Newark", got.Destination)
	}
	if got.Preference != "Indian" {
		t.Fatalf("Preference = %q, want Indian", got.Preference)
	}
	if got.MaxDetourMinutes != 7 {
		t.Fatalf("MaxDetourMinutes = %d, want 7", got.MaxDetourMinutes)
	}
}

func TestParseAgentRequest_DetourDefaultsAndClamp(t *testing.T) {
	defaulted := parseAgentRequest(AgentSearchRequest{Start: "A", Destination: "B", Preference: "coffee"})
	if defaulted.MaxDetourMinutes != defaultAgentMaxDetour {
		t.Fatalf("default detour = %d, want %d", defaulted.MaxDetourMinutes, defaultAgentMaxDetour)
	}

	clamped := parseAgentRequest(AgentSearchRequest{Start: "A", Destination: "B", Preference: "coffee", MaxDetourMinutes: 99})
	if clamped.MaxDetourMinutes != maxAgentDetour {
		t.Fatalf("clamped detour = %d, want %d", clamped.MaxDetourMinutes, maxAgentDetour)
	}
}

func TestAgentSearch_StructuredFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
	)

	body := bytes.NewBufferString(`{
		"start": "North Miami Beach",
		"destination": "Sunrise",
		"preference": "soup",
		"max_detour_minutes": 10
	}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agent/search", body)
	c.Request.Header.Set("Content-Type", "application/json")

	h.AgentSearch(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var got AgentSearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Summary == "" {
		t.Fatal("Summary is empty")
	}
	if len(got.Restaurants) == 0 || len(got.Restaurants) > agentResultLimit {
		t.Fatalf("restaurants len = %d, want 1..%d", len(got.Restaurants), agentResultLimit)
	}
	if got.Restaurants[0].Reason == "" {
		t.Fatal("first restaurant reason is empty")
	}
	if got.Restaurants[0].RouteBiteScore <= 0 {
		t.Fatalf("RouteBiteScore = %d, want positive", got.Restaurants[0].RouteBiteScore)
	}
	if got.BestPick == nil {
		t.Fatal("BestPick is nil")
	}
	if len(got.Restaurants) == 0 {
		t.Fatal("restaurants is empty when best_pick exists")
	}
	if got.BestPick.Name != got.Restaurants[0].Name {
		t.Fatalf("best pick = %q, first restaurant = %q", got.BestPick.Name, got.Restaurants[0].Name)
	}
	if got.BestPick.RouteBiteScore != got.Restaurants[0].RouteBiteScore {
		t.Fatalf("best pick score = %d, first restaurant score = %d", got.BestPick.RouteBiteScore, got.Restaurants[0].RouteBiteScore)
	}
	if got.Restaurants[0].ScoreBreakdown == (ScoreBreakdown{}) {
		t.Fatal("first restaurant score_breakdown is empty")
	}
}

func TestAgentSearch_MissingRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agent/search", bytes.NewBufferString(`{"preference":"soup"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.AgentSearch(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAgentSearch_ErrorIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
	)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/v1/agent/search", h.AgentSearch)

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/search", bytes.NewBufferString(`{"preference":"soup"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.RequestIDHeader, "test-request-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var got ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.RequestID != "test-request-id" {
		t.Fatalf("request_id = %q, want test-request-id", got.Error.RequestID)
	}
}

func TestAgentSearch_SavesHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingHistoryRepo{}
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithAgentSearchHistory(repo),
	)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/v1/agent/search", h.AgentSearch)

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/search", bytes.NewBufferString(`{
		"query": "I am driving from North Miami Beach to Sunrise and want soup under 10 minutes",
		"start": "North Miami Beach",
		"destination": "Sunrise",
		"preference": "soup",
		"max_detour_minutes": 10
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.RequestIDHeader, "history-request-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(repo.records) != 1 {
		t.Fatalf("saved records = %d, want 1", len(repo.records))
	}
	got := repo.records[0]
	if got.RequestID != "history-request-id" {
		t.Fatalf("RequestID = %q, want history-request-id", got.RequestID)
	}
	if got.Query == "" || got.StartLocation != "North Miami Beach" || got.Destination != "Sunrise" || got.Preference != "soup" {
		t.Fatalf("saved record has wrong request fields: %+v", got)
	}
	if got.MaxDetourMinutes != 10 {
		t.Fatalf("MaxDetourMinutes = %d, want 10", got.MaxDetourMinutes)
	}
	if got.ResultCount == 0 || got.Summary == "" {
		t.Fatalf("saved response fields are incomplete: %+v", got)
	}
}

func TestAgentSearch_HistoryFailureDoesNotBreakResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithAgentSearchHistory(&recordingHistoryRepo{err: errors.New("database unavailable")}),
	)

	body := bytes.NewBufferString(`{
		"start": "North Miami Beach",
		"destination": "Sunrise",
		"preference": "soup",
		"max_detour_minutes": 10
	}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agent/search", body)
	c.Request.Header.Set("Content-Type", "application/json")

	h.AgentSearch(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestRouteBiteScoreBreakdown(t *testing.T) {
	result := scoring.Restaurant{
		Name:         "Saffron Indian Kitchen",
		Rating:       4.5,
		ExtraMinutes: 2,
		IsOpenNow:    true,
		Address:      "123 Main St",
		Phone:        "+16155551212",
		Cuisine:      []string{"Indian"},
	}

	got := routeBiteScoreBreakdown(result, "Indian food", 10)
	if got.DetourScore != 80 {
		t.Fatalf("DetourScore = %d, want 80", got.DetourScore)
	}
	if got.RatingScore != 90 {
		t.Fatalf("RatingScore = %d, want 90", got.RatingScore)
	}
	if got.OpenNowScore != 100 {
		t.Fatalf("OpenNowScore = %d, want 100", got.OpenNowScore)
	}
	if got.PreferenceMatchScore != 100 {
		t.Fatalf("PreferenceMatchScore = %d, want 100", got.PreferenceMatchScore)
	}
	if got.ConvenienceScore != 100 {
		t.Fatalf("ConvenienceScore = %d, want 100", got.ConvenienceScore)
	}

	if score := routeBiteScore(got); score != 91 {
		t.Fatalf("RouteBiteScore = %d, want 91", score)
	}
}

func TestBestPickUsesHighestRouteBiteScore(t *testing.T) {
	restaurants := []AgentRestaurant{
		{
			Name:           "Far But Famous",
			Rating:         4.9,
			DetourMinutes:  9,
			OpenNow:        true,
			RouteBiteScore: 72,
		},
		{
			Name:           "Close Curry",
			Rating:         4.4,
			DetourMinutes:  3,
			OpenNow:        true,
			RouteBiteScore: 90,
		},
	}

	sortAgentRestaurants(restaurants)
	got := bestAgentPick(restaurants)
	if got == nil {
		t.Fatal("best pick is nil")
	}
	if got.Name != "Close Curry" {
		t.Fatalf("best pick = %q, want Close Curry", got.Name)
	}
	if !strings.Contains(got.Reason, "Highest RouteBite Score (90/100)") {
		t.Fatalf("reason = %q, want RouteBite Score explanation", got.Reason)
	}
}

type recordingHistoryRepo struct {
	records []history.AgentSearch
	err     error
}

func (r *recordingHistoryRepo) SaveAgentSearch(_ context.Context, search history.AgentSearch) error {
	if r.err != nil {
		return r.err
	}
	r.records = append(r.records, search)
	return nil
}
