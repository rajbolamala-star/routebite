package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dheerajb/routebite/internal/api"
	"github.com/dheerajb/routebite/internal/cache"
	"github.com/dheerajb/routebite/internal/geocode"
	"github.com/dheerajb/routebite/internal/history"
	"github.com/dheerajb/routebite/internal/middleware"
	"github.com/dheerajb/routebite/internal/routing"
	"github.com/dheerajb/routebite/internal/yelp"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	port := getEnv("PORT", "8080")
	yelpKey := os.Getenv("YELP_API_KEY")
	useMockRouting := os.Getenv("USE_MOCK_ROUTING") == "true"
	useMockGeocoding := os.Getenv("USE_MOCK_GEOCODING") == "true"

	// Yelp: real client if key present, mock otherwise.
	var yelpClient yelp.Client
	restaurantProvider := "yelp"
	if yelpKey == "" {
		restaurantProvider = "mock"
		log.Println("restaurant provider: mock (YELP_API_KEY not set)")
		yelpClient = yelp.NewMock()
	} else {
		log.Println("restaurant provider: yelp")
		yelpClient = yelp.New(yelpKey)
	}

	// Routing: OSRM public for real, mock as fallback / for tests.
	var routeEngine routing.Engine
	routingProvider := "osrm"
	if useMockRouting {
		routingProvider = "mock"
		log.Println("routing provider: mock (USE_MOCK_ROUTING=true)")
		routeEngine = routing.NewMockEngine()
	} else {
		log.Println("routing provider: osrm")
		routeEngine = routing.NewOSRM()
	}

	var geocodeClient geocode.Client
	geocodingProvider := "nominatim"
	if useMockGeocoding {
		geocodingProvider = "mock"
		log.Println("geocoding provider: mock (USE_MOCK_GEOCODING=true)")
		geocodeClient = geocode.NewMock()
	} else {
		log.Println("geocoding provider: nominatim")
		geocodeClient = geocode.NewNominatim()
	}

	cacheTTL := readCacheTTL()
	// Local in-memory cache for repeated identical queries inside this process.
	c := cache.New(cacheTTL)
	go purgeLoop(c)

	historyRepo, closeHistoryRepo := openHistoryRepository()
	defer closeHistoryRepo()

	externalCache, closeExternalCache := openExternalCache()
	defer closeExternalCache()
	agentParserOption := openAgentParserOption()

	h := api.NewHandler(yelpClient, routeEngine, geocodeClient, c, api.Providers{
		Restaurants: restaurantProvider,
		Routing:     routingProvider,
		Geocoding:   geocodingProvider,
	}, api.WithAgentSearchHistory(historyRepo), api.WithExternalCache(externalCache, cacheTTL), agentParserOption)

	gin.SetMode(getEnv("GIN_MODE", "release"))
	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestID(), middleware.StructuredLogger())

	if _, err := os.Stat("./web/out/index.html"); err == nil {
		r.Static("/_next", "./web/out/_next")
		r.StaticFile("/", "./web/out/index.html")
	}

	v1 := r.Group("/v1")
	{
		v1.POST("/search", h.Search)
		v1.POST("/agent/search", h.AgentSearch)
		v1.GET("/geocode", h.Geocode)
		v1.GET("/providers", h.Providers)
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

func readCacheTTL() time.Duration {
	minutes, err := strconv.Atoi(getEnv("CACHE_TTL_MINUTES", "15"))
	if err != nil || minutes <= 0 {
		minutes = 15
	}
	return time.Duration(minutes) * time.Minute
}

func openHistoryRepository() (history.Repository, func()) {
	if os.Getenv("DB_ENABLED") != "true" {
		log.Println("agent search persistence: disabled (DB_ENABLED=false)")
		return history.NoopRepository{}, func() {}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Println("agent search persistence: disabled (DATABASE_URL not set)")
		return history.NoopRepository{}, func() {}
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Printf("agent search persistence: disabled (open failed: %v)", err)
		return history.NoopRepository{}, func() {}
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("agent search persistence: disabled (ping failed: %v)", err)
		_ = db.Close()
		return history.NoopRepository{}, func() {}
	}

	log.Println("agent search persistence: postgres")
	return history.NewPostgresRepository(db), func() {
		_ = db.Close()
	}
}

func openExternalCache() (cache.Cache, func()) {
	if os.Getenv("REDIS_ENABLED") != "true" {
		log.Println("redis cache: disabled (REDIS_ENABLED=false)")
		return cache.NoopCache{}, func() {}
	}

	db, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil || db < 0 {
		db = 0
	}
	client := redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("redis cache: disabled (ping failed: %v)", err)
		_ = client.Close()
		return cache.NoopCache{}, func() {}
	}

	log.Println("redis cache: enabled")
	return cache.NewRedisCache(client), func() {
		_ = client.Close()
	}
}

func openAgentParserOption() api.HandlerOption {
	if os.Getenv("OLLAMA_ENABLED") != "true" {
		log.Println("agent parser: rule-based (OLLAMA_ENABLED=false)")
		return func(*api.Handler) {}
	}

	timeoutSeconds, err := strconv.Atoi(getEnv("OLLAMA_TIMEOUT_SECONDS", "5"))
	if err != nil || timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}

	log.Printf("agent parser: ollama enabled model=%s base_url=%s",
		getEnv("OLLAMA_MODEL", "llama3.2:3b"),
		getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
	)
	return api.WithAgentParser(api.NewOllamaAgentParser(api.OllamaParserConfig{
		BaseURL: getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
		Model:   getEnv("OLLAMA_MODEL", "llama3.2:3b"),
		Timeout: time.Duration(timeoutSeconds) * time.Second,
	}, nil))
}
