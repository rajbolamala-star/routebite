package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/dheerajb/routebite/internal/cache"
	"github.com/dheerajb/routebite/internal/geocode"
	"github.com/dheerajb/routebite/internal/history"
	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/scoring"
	"github.com/dheerajb/routebite/internal/voice"
	"github.com/dheerajb/routebite/internal/yelp"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	searchTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "routebite_search_total",
		Help: "Total /v1/search requests, labeled by outcome.",
	}, []string{"outcome"})

	searchLatencyMs = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "routebite_search_latency_ms",
		Help:    "End-to-end latency for /v1/search in milliseconds.",
		Buckets: []float64{50, 100, 250, 500, 1000, 2000, 5000},
	})

	yelpCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "routebite_yelp_calls_total",
		Help: "Yelp Fusion API calls, by cache outcome.",
	}, []string{"cache"})
)

// Handler is the API layer. All deps are injected for easy testing.
type Handler struct {
	yelp      yelp.Client
	route     routing.Engine
	geocode   geocode.Client
	cache     *cache.TTL
	weights   scoring.Weights
	providers Providers
	agent     agentParser
	history   history.Repository
}

type HandlerOption func(*Handler)

func WithAgentSearchHistory(repo history.Repository) HandlerOption {
	return func(h *Handler) {
		if repo != nil {
			h.history = repo
		}
	}
}

func NewHandler(y yelp.Client, r routing.Engine, g geocode.Client, c *cache.TTL, providers Providers, opts ...HandlerOption) *Handler {
	h := &Handler{
		yelp:      y,
		route:     r,
		geocode:   g,
		cache:     c,
		weights:   scoring.Default,
		providers: providers,
		agent:     ruleBasedAgentParser{},
		history:   history.NoopRepository{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Search handles POST /v1/search.
func (h *Handler) Search(c *gin.Context) {
	start := time.Now()
	defer func() {
		searchLatencyMs.Observe(float64(time.Since(start).Milliseconds()))
	}()

	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		searchTotal.WithLabelValues("bad_request").Inc()
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, searchErr := h.runSearch(c.Request.Context(), req)
	if searchErr != nil {
		searchTotal.WithLabelValues(searchErr.outcome).Inc()
		writeError(c, searchErr.status, searchErr.message)
		return
	}

	searchTotal.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, resp)
}

type apiSearchError struct {
	outcome string
	status  int
	message string
}

func (h *Handler) runSearch(ctx context.Context, req SearchRequest) (SearchResponse, *apiSearchError) {
	applyDefaults(&req)

	// Resolve cuisine intent from voice_text if not provided directly.
	cuisine := req.Cuisine
	if cuisine == "" && req.VoiceText != "" {
		intent := voice.Parse(req.VoiceText)
		cuisine = intent.Cuisine
		// Voice can override open_now via "any time".
		if !intent.OpenNowOnly {
			req.OpenNowOnly = false
		}
	}

	originPoint := routing.Point{Lat: req.Origin.Lat, Lng: req.Origin.Lng}
	destinationPoint := routing.Point{Lat: req.Destination.Lat, Lng: req.Destination.Lng}

	// 1) Route.
	route, err := h.route.Route(ctx, originPoint, destinationPoint)
	if err != nil {
		return SearchResponse{}, &apiSearchError{
			outcome: "route_error",
			status:  http.StatusBadGateway,
			message: "routing failed: " + err.Error(),
		}
	}

	// 2) Yelp candidates around route midpoint (with cache).
	center := routing.MidpointOf(route.Polyline)
	radius := routing.BoundingRadiusM(route.Polyline)
	businesses, err := h.fetchYelp(ctx, cuisine, center, radius, req.OpenNowOnly)
	if err != nil {
		return SearchResponse{}, &apiSearchError{
			outcome: "yelp_error",
			status:  http.StatusBadGateway,
			message: "restaurant search failed: " + err.Error(),
		}
	}

	// 3) Score + detour math.
	detours := h.preciseDetours(ctx, businesses, originPoint, destinationPoint, route.DurationSec)
	results := scoring.RankWithDetours(businesses, route.Polyline, detours, req.MaxDetourMinutes, req.MaxResults, h.weights)

	// 4) Build response.
	return SearchResponse{
		RouteSummary: RouteSummary{
			BaseDurationMin: route.DurationSec / 60.0,
			DistanceKm:      route.DistanceM / 1000.0,
		},
		Results:      results,
		VoiceSummary: scoring.VoiceSummary(results, cuisine),
	}, nil
}

func (h *Handler) preciseDetours(
	ctx context.Context,
	businesses []yelp.Business,
	origin routing.Point,
	destination routing.Point,
	baseDurationSec float64,
) map[string]int {
	detours := make(map[string]int, len(businesses))
	for _, b := range businesses {
		if b.IsClosed {
			continue
		}
		stop := routing.Point{Lat: b.Coordinates.Latitude, Lng: b.Coordinates.Longitude}
		if stop.Lat == 0 && stop.Lng == 0 {
			continue
		}

		via, err := routing.RouteVia(ctx, h.route, origin, stop, destination)
		if err != nil {
			continue
		}
		extraMin := int(math.Round((via.DurationSec - baseDurationSec) / 60.0))
		if extraMin < 0 {
			extraMin = 0
		}
		detours[scoring.DetourKey(b)] = extraMin
	}
	return detours
}

// Geocode handles GET /v1/geocode?q=place and returns address suggestions.
func (h *Handler) Geocode(c *gin.Context) {
	q := c.Query("q")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	if q == "" {
		c.JSON(http.StatusOK, GeocodeResponse{Results: []geocode.Suggestion{}})
		return
	}

	results, err := h.geocode.Search(c.Request.Context(), q, limit)
	if err != nil {
		writeError(c, http.StatusBadGateway, "geocoding failed: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, GeocodeResponse{Results: results})
}

// Providers handles GET /v1/providers.
func (h *Handler) Providers(c *gin.Context) {
	c.JSON(http.StatusOK, h.providers)
}

// fetchYelp wraps the Yelp call with a content-addressed cache so identical
// searches don't burn quota.
func (h *Handler) fetchYelp(ctx context.Context, term string, center routing.Point, radius int, openNow bool) ([]yelp.Business, error) {
	key := cacheKey(term, center, radius, openNow)

	if raw, hit := h.cache.Get(key); hit {
		yelpCalls.WithLabelValues("hit").Inc()
		var out []yelp.Business
		if err := json.Unmarshal(raw, &out); err == nil {
			return out, nil
		}
		// fall through on decode failure
	}

	yelpCalls.WithLabelValues("miss").Inc()
	bs, err := h.yelp.Search(ctx, yelp.SearchParams{
		Term:    term,
		Lat:     center.Lat,
		Lng:     center.Lng,
		RadiusM: radius,
		Limit:   25,
		OpenNow: openNow,
	})
	if err != nil {
		return nil, err
	}

	if raw, err := json.Marshal(bs); err == nil {
		h.cache.Set(key, raw)
	}
	return bs, nil
}

func cacheKey(term string, center routing.Point, radius int, openNow bool) string {
	h := sha256.New()
	_, _ = h.Write([]byte(term))
	_, _ = h.Write([]byte{byte(int(center.Lat * 100))})
	_, _ = h.Write([]byte{byte(int(center.Lng * 100))})
	_, _ = h.Write([]byte{byte(radius / 100)})
	if openNow {
		_, _ = h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func applyDefaults(r *SearchRequest) {
	if r.MaxDetourMinutes <= 0 {
		r.MaxDetourMinutes = 10
	}
	if r.MaxResults <= 0 {
		r.MaxResults = 5
	}
	// open_now defaults to true if no voice override is detected
	if !r.OpenNowOnly && r.VoiceText == "" {
		r.OpenNowOnly = true
	}
}

func badRequest(message string) *apiSearchError {
	return &apiSearchError{outcome: "bad_request", status: http.StatusBadRequest, message: message}
}

func badGateway(prefix string, err error) *apiSearchError {
	return &apiSearchError{outcome: "upstream_error", status: http.StatusBadGateway, message: fmt.Sprintf("%s: %v", prefix, err)}
}

// Health is a basic readiness check.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
}
