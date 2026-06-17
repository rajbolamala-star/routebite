package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dheerajb/routebite/internal/cache"
	"github.com/dheerajb/routebite/internal/geocode"
	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/yelp"
	"github.com/gin-gonic/gin"
)

func TestProviders(t *testing.T) {
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
	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	c.Request = req

	h.Providers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var got Providers
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Restaurants != "mock" || got.Routing != "mock" || got.Geocoding != "mock" {
		t.Fatalf("providers = %+v, want all mock", got)
	}
}

func TestReady_AllDisabledOptionalDependencies(t *testing.T) {
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
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestReady_EnabledDependencyNotReady(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithReadiness(Readiness{
			Postgres: DependencyStatus{Enabled: true, Ready: false, Message: "postgres ping failed"},
			Redis:    DependencyStatus{Enabled: false, Ready: true},
			Ollama:   DependencyStatus{Enabled: false, Ready: true},
		}),
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	var got struct {
		Status       string    `json:"status"`
		Dependencies Readiness `json:"dependencies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != "not_ready" {
		t.Fatalf("status body = %q, want not_ready", got.Status)
	}
	if got.Dependencies.Postgres.Message == "" {
		t.Fatalf("postgres readiness message is empty")
	}
}

func TestGeocode_UsesExternalCacheHit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fakeGeocode := &countingGeocodeClient{}
	shared := &fakeSharedCache{
		values: map[string]any{
			"geocode:kingsport-tn:limit:1": []geocode.Suggestion{
				{Label: "Kingsport, TN", Point: geocode.Point{Lat: 36.5484, Lng: -82.5618}},
			},
		},
	}
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		fakeGeocode,
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithExternalCache(shared, time.Minute),
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/geocode?q=Kingsport%20TN&limit=1", nil)

	h.Geocode(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if fakeGeocode.calls != 0 {
		t.Fatalf("geocode calls = %d, want 0 on cache hit", fakeGeocode.calls)
	}
	var got GeocodeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Label != "Kingsport, TN" {
		t.Fatalf("results = %+v, want cached Kingsport result", got.Results)
	}
}

func TestGeocode_CacheErrorFallsBackToProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fakeGeocode := &countingGeocodeClient{
		results: []geocode.Suggestion{
			{Label: "Nashville, TN", Point: geocode.Point{Lat: 36.1627, Lng: -86.7816}},
		},
	}
	h := NewHandler(
		yelp.NewMock(),
		routing.NewMockEngine(),
		fakeGeocode,
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithExternalCache(&fakeSharedCache{getErr: errors.New("redis unavailable")}, time.Minute),
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/geocode?q=Nashville%20TN&limit=1", nil)

	h.Geocode(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if fakeGeocode.calls != 1 {
		t.Fatalf("geocode calls = %d, want 1 after cache error", fakeGeocode.calls)
	}
}

func TestFetchYelp_UsesExternalCacheHit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	center := routing.Point{Lat: 36.5484, Lng: -82.5618}
	key := restaurantCacheKey("soup", center, 5000, true)
	fakeYelp := &countingYelpClient{}
	shared := &fakeSharedCache{
		values: map[string]any{
			key: []yelp.Business{{ID: "cached-1", Name: "Cached Soup"}},
		},
	}
	h := NewHandler(
		fakeYelp,
		routing.NewMockEngine(),
		geocode.NewMock(),
		cache.New(time.Minute),
		Providers{Restaurants: "mock", Routing: "mock", Geocoding: "mock"},
		WithExternalCache(shared, time.Minute),
	)

	got, err := h.fetchYelp(context.Background(), "soup", center, 5000, true)
	if err != nil {
		t.Fatalf("fetchYelp error: %v", err)
	}
	if fakeYelp.calls != 0 {
		t.Fatalf("yelp calls = %d, want 0 on cache hit", fakeYelp.calls)
	}
	if len(got) != 1 || got[0].Name != "Cached Soup" {
		t.Fatalf("businesses = %+v, want cached business", got)
	}
}

type countingGeocodeClient struct {
	calls   int
	results []geocode.Suggestion
}

func (c *countingGeocodeClient) Search(_ context.Context, _ string, _ int) ([]geocode.Suggestion, error) {
	c.calls++
	if c.results != nil {
		return c.results, nil
	}
	return []geocode.Suggestion{}, nil
}

type countingYelpClient struct {
	calls int
}

func (c *countingYelpClient) Search(context.Context, yelp.SearchParams) ([]yelp.Business, error) {
	c.calls++
	return []yelp.Business{{ID: "provider-1", Name: "Provider Soup"}}, nil
}

type fakeSharedCache struct {
	values map[string]any
	getErr error
	setErr error
	sets   int
}

func (c *fakeSharedCache) Get(_ context.Context, key string, dest any) (bool, error) {
	if c.getErr != nil {
		return false, c.getErr
	}
	if c.values == nil {
		return false, nil
	}
	value, ok := c.values[key]
	if !ok {
		return false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (c *fakeSharedCache) Set(_ context.Context, _ string, _ any, _ time.Duration) error {
	c.sets++
	return c.setErr
}
