package api

import (
	"encoding/json"
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
