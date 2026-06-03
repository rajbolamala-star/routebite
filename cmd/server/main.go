package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dheerajb/routebite/internal/api"
	"github.com/dheerajb/routebite/internal/cache"
	"github.com/dheerajb/routebite/internal/middleware"
	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/yelp"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port := getEnv("PORT", "8080")
	yelpKey := os.Getenv("YELP_API_KEY")
	useMockRouting := os.Getenv("USE_MOCK_ROUTING") == "true"

	// Yelp: real client if key present, mock otherwise.
	var yelpClient yelp.Client
	if yelpKey == "" {
		log.Println("YELP_API_KEY not set - using mock Yelp client")
		yelpClient = yelp.NewMock()
	} else {
		yelpClient = yelp.New(yelpKey)
	}

	// Routing: OSRM public for real, mock as fallback / for tests.
	var routeEngine routing.Engine
	if useMockRouting {
		log.Println("USE_MOCK_ROUTING=true - using mock route engine")
		routeEngine = routing.NewMockEngine()
	} else {
		routeEngine = routing.NewOSRM()
	}

	// 5-minute cache for identical Yelp queries.
	c := cache.New(5 * time.Minute)
	go purgeLoop(c)

	h := api.NewHandler(yelpClient, routeEngine, c)

	gin.SetMode(getEnv("GIN_MODE", "release"))
	r := gin.New()
	r.Use(gin.Recovery(), middleware.StructuredLogger())

	v1 := r.Group("/v1")
	{
		v1.POST("/search", h.Search)
		v1.GET("/health", h.Health)
	}
	r.GET("/health", h.Health) // also at root for load balancers
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	go func() {
		log.Printf("routebite listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown — same pattern as the rest of your Go projects.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// purgeLoop periodically drops expired cache entries so memory doesn't grow
// unbounded on a long-running process.
func purgeLoop(c *cache.TTL) {
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for range t.C {
		c.PurgeExpired()
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
