# RouteBite

> **"Find food along my route, call it in, keep me on the fastest path — all without looking at my screen."**

A Go backend API that answers one question for a driver: *given my origin, destination, and what I'm craving, what should I pick up on the way — and how much will it delay me?*

Built because the existing maps + food apps are unsafe for drivers. RouteBite returns ranked restaurants **along your route**, with detour cost, tap-to-call phone, and a one-line **voice summary** the client can read aloud — so the driver never has to look at the screen.

## Why this exists

Real story: driving from North Miami Beach to Sunrise, wanted to pick up soup on the way. Google Maps showed me restaurants, but I had to:

1. Stop the route
2. Search by name
3. Read tiny menus
4. Switch apps to call
5. Manually re-route

That's five context-switches while driving. Unsafe and slow. RouteBite collapses it into one API call.

## What it does

`POST /v1/search` with coordinates, or `POST /v1/agent/search` with a more
assistant-like request.

Core search:

```json
{
  "origin":      { "lat": 25.929, "lng": -80.169 },
  "destination": { "lat": 26.133, "lng": -80.290 },
  "voice_text":  "find me soup along the way",
  "max_detour_minutes": 10,
  "max_results": 5
}
```

Returns:
- The full route summary (base duration, distance)
- A **ranked list of restaurants** along the route, scored by quality + detour cost + open status
- For each: tap-to-call link, rating, detour minutes, Yelp URL, cuisine, price, distance from route
- A **`voice_summary`** — a single sentence the client can speak aloud

```json
{
  "voice_summary": "Found 3 spots. Top pick is Pho 78 — 4.6 stars, 4 extra minutes. Want to call?"
}
```

Agent search:

```json
{
  "query": "I am driving from Kingsport TN to Nashville TN and want Indian food with less than 10 minutes detour",
  "start": "Kingsport, TN",
  "destination": "Nashville, TN",
  "preference": "Indian food",
  "max_detour_minutes": 10
}
```

Returns a compact, driver-safe recommendation response:

```json
{
  "summary": "Best option is Saffron Indian Kitchen, about 6 minutes off your route, rated 4.5 stars and currently open.",
  "driver_safe_summary": "Saffron Indian Kitchen is the best stop. 6 minute detour. 4.5 stars. Want to call?",
  "match_quality": "strong_match",
  "trip_intent": "food",
  "best_pick": {
    "name": "Saffron Indian Kitchen",
    "rating": 4.5,
    "detour_minutes": 6,
    "open_now": true,
    "address": "123 Main St, Nashville, TN",
    "phone": "+16155551212",
    "tap_to_call": "tel:+16155551212",
    "open_in_maps_url": "https://www.google.com/maps/search/?api=1&query=123+Main+St%2C+Nashville%2C+TN",
    "reason": "Highest RouteBite Score (84/100): low detour, highly rated, currently open.",
    "routebite_score": 84,
    "score_breakdown": {
      "detour_score": 40,
      "rating_score": 90,
      "open_now_score": 100,
      "preference_match_score": 100,
      "convenience_score": 100
    }
  },
  "restaurants": [
    {
      "name": "Saffron Indian Kitchen",
      "rating": 4.5,
      "detour_minutes": 6,
      "open_now": true,
      "address": "123 Main St, Nashville, TN",
      "phone": "+16155551212",
      "tap_to_call": "tel:+16155551212",
      "open_in_maps_url": "https://www.google.com/maps/search/?api=1&query=123+Main+St%2C+Nashville%2C+TN",
      "reason": "Low detour, highly rated, currently open",
      "routebite_score": 84,
      "score_breakdown": {
        "detour_score": 40,
        "rating_score": 90,
        "open_now_score": 100,
        "preference_match_score": 100,
        "convenience_score": 100
      }
    }
  ]
}
```

`match_quality` is `strong_match`, `weak_match`, or `no_match`. If no
restaurant fits the max detour, the agent returns a no-match summary instead of
pretending the result is good. `driver_safe_summary` is short enough for a
mobile or voice assistant to read aloud while driving. `trip_intent` is a simple
classification such as `food`, `soup`, `coffee`, `gas_food`,
`restroom_food`, or `unknown`.

The default agent parser is rule-based by design: it prefers structured fields
when present, parses simple `from ... to ... want ... under N minutes` queries
when needed, then reuses the existing geocoding, routing, Yelp, cache, detour
scoring, and ranking pipeline. An optional Ollama parser can be enabled for
local LLM extraction, but it always falls back to the rule-based parser on
errors, invalid JSON, or missing required fields.

## Architecture

```
   ┌─────────────┐
   │  Client     │  (mobile, CarPlay, Alexa — not built here)
   └──────┬──────┘
          │ POST /v1/search
          ▼
   ┌─────────────────────────────────────────────┐
   │            RouteBite API (Go)               │
   │                                             │
   │   Voice Parser  ─▶  Intent (cuisine,radius) │
   │                                             │
   │   Route Engine  ─▶  base route + bounds     │
   │           │                                 │
   │           ▼                                 │
   │   Yelp Client   ─▶  candidates along route  │
   │           │                                 │
   │           ▼                                 │
   │   Detour Scorer ─▶  ranked results +        │
   │                     voice_summary           │
   └─────────────────────────────────────────────┘
```

## Scoring formula

Core restaurant ranking:

```
score = rating_normalized       * 0.4
      + review_count_normalized * 0.2
      + convenience(extra_min)  * 0.3
      + (open_now ? 0.1 : 0)
```

Agent responses also include a **RouteBite Score** from 0 to 100. This is a
simple decision score for the final recommendation layer:

```
routebite_score =
    detour_score           * 0.35
  + rating_score           * 0.25
  + open_now_score         * 0.15
  + preference_match_score * 0.15
  + convenience_score      * 0.10
```

`best_pick` is the highest RouteBite Score after tie-breaking by lower detour
and then higher rating. This mimics the tradeoff a driver makes mentally: how
good is it, how closely does it match what I asked for, and how much does it
cost me to stop?

## Tech stack

- **Language:** Go 1.21
- **HTTP:** Gin
- **Web app:** Next.js, React, TypeScript
- **Restaurant data:** Yelp Fusion (free tier — 5000 calls/day)
- **Routing:** OSRM public demo for MVP (swap for Mapbox / Google Routes for production)
- **Agent parser:** rule-based by default, optional Ollama local LLM parser
- **Cache:** In-memory TTL cache plus optional Redis shared cache
- **Observability:** Prometheus metrics
- **Deploy:** Docker, docker-compose, Kubernetes, Fly.io-ready

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/search` | Find restaurants along a route |
| `POST` | `/v1/agent/search` | Agent-style natural language or structured route search |
| `GET`  | `/v1/geocode` | Resolve typed places into coordinates |
| `GET`  | `/v1/providers` | Show active data providers |
| `GET`  | `/v1/health` | Health check |
| `GET`  | `/v1/metrics` | Prometheus metrics |

## Request tracing and logs

Every request gets an `X-Request-ID` response header. If a caller sends an
`X-Request-ID` header, RouteBite preserves it; otherwise the middleware
generates one. Error responses include the same `request_id` so logs and client
reports can be correlated.

Example error shape:

```json
{
  "error": {
    "message": "agent needs a start location",
    "request_id": "client-request-123"
  }
}
```

The request logger emits one JSON line per request with method, path, status,
latency, client IP, and request ID:

```json
{
  "timestamp": "2026-06-16T00:00:00Z",
  "method": "POST",
  "path": "/v1/agent/search",
  "status": 200,
  "latency_ms": 42,
  "client_ip": "127.0.0.1",
  "request_id": "client-request-123"
}
```

## Optional PostgreSQL persistence

RouteBite can store `/v1/agent/search` history in PostgreSQL for demos,
analytics, and backend portfolio depth. This is disabled by default, so the app
still runs normally without a database.

Enable it only when you have a database ready:

```bash
export DB_ENABLED=true
export DATABASE_URL='postgres://routebite:routebite@localhost:5432/routebite?sslmode=disable'
psql "$DATABASE_URL" -f migrations/001_create_agent_searches.sql
go run ./cmd/server
```

Each successful agent search records the request ID, query, normalized route
fields, preference, max detour, result count, and driver-safe summary. If the
save fails, RouteBite logs the error and still returns the recommendation.

Run without PostgreSQL:

```bash
DB_ENABLED=false go run ./cmd/server
```

## Optional Redis caching

RouteBite can use Redis as a shared cache for repeated geocoding and restaurant
searches. This reduces repeated calls to external providers like Nominatim and
Yelp, improves response speed, and keeps local/mock mode working the same way.

Redis is disabled by default:

```bash
REDIS_ENABLED=false go run ./cmd/server
```

Run Redis locally:

```bash
docker run --rm -p 6379:6379 redis:7
```

Enable Redis caching:

```bash
export REDIS_ENABLED=true
export REDIS_ADDR=localhost:6379
export REDIS_DB=0
export CACHE_TTL_MINUTES=15
go run ./cmd/server
```

Optional password:

```bash
export REDIS_PASSWORD=your_redis_password
```

Cached data uses stable, non-secret keys such as `geocode:kingsport-tn:limit:1`
and `restaurants:soup:25.9290:-80.1690:radius:5000:open:true`. Cache failures
are failure-safe: RouteBite logs `cache_error`, calls the real provider, and
continues serving the API response.

## Optional Ollama agent parser

RouteBite can use a local Ollama model to extract route-agent fields from the
natural language query:

- `start`
- `destination`
- `preference`
- `max_detour_minutes`
- `trip_intent`

It is disabled by default:

```bash
OLLAMA_ENABLED=false go run ./cmd/server
```

Run Ollama locally:

```bash
ollama serve
ollama pull llama3.2:3b
```

Enable the Ollama parser:

```bash
export OLLAMA_ENABLED=true
export OLLAMA_BASE_URL=http://localhost:11434
export OLLAMA_MODEL=llama3.2:3b
export OLLAMA_TIMEOUT_SECONDS=5
go run ./cmd/server
```

Fallback behavior is intentional: if Ollama is unavailable, times out, returns
invalid JSON, or misses required fields, RouteBite logs the event and uses the
rule-based parser. The recommendation endpoint should still work even when the
local LLM is down.

## Getting started

```bash
# 1. Set your Yelp API key (free at https://docs.developer.yelp.com)
export YELP_API_KEY=your_yelp_fusion_api_key

# 2. Run
go run ./cmd/server

# 3. Try it
curl -X POST http://localhost:8080/v1/search \
  -H "Content-Type: application/json" \
  -d @scripts/example_request.json
```

Try the agent endpoint:

```bash
curl -X POST http://localhost:8080/v1/agent/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "I am driving from North Miami Beach to Sunrise and want soup with less than 10 minutes detour",
    "start": "North Miami Beach",
    "destination": "Sunrise",
    "preference": "soup",
    "max_detour_minutes": 10
  }'
```

Or via Docker:

```bash
make docker-up
./scripts/smoke-test.sh
```

## Web app

RouteBite has a mobile-first Next.js app in `web/`. Run the API and web app in
two terminals during development.

Terminal 1, start the Go API with mock providers:

```bash
make run
```

Terminal 2, install and start the web app:

```bash
make web-install
make web-dev
```

Open:

```text
http://localhost:3000/
```

The Next.js app proxies `/v1/*` requests to the Go API at `localhost:8080`.
Local `make run` uses mock routing, mock Yelp, and mock geocoding so the full
flow works without external API keys. Use `make run-geocode` when you want
real typed-address lookup with mock food/routing. Use `make run-yelp` after
setting `YELP_API_KEY` when you want live Yelp restaurants with stable mock
routing/geocoding. `make run-real` uses OSRM and Nominatim geocoding, and uses
Yelp when `YELP_API_KEY` is set.

For a deployed frontend, set `ROUTEBITE_API_BASE` to the public backend URL.
The production Next.js rewrite will forward `/v1/*` to that API.

To get live Yelp results:

1. Create a Yelp Fusion app at https://docs.developer.yelp.com/docs/fusion-authentication
2. Copy `.env.example` to `.env` and set `YELP_API_KEY`, or export it:

```bash
export YELP_API_KEY=your_yelp_fusion_api_key
make run-yelp
```

The web app will show `Mock restaurants` or `Live Yelp` based on the backend's
active provider.

Useful web checks:

```bash
make web-lint
make web-typecheck
make web-build
```

The app lets you pick a sample route, enter or speak a food request, review
ranked pickup options, call the restaurant, open navigation, and hear the
one-line voice summary. Address autocomplete, real route entry, and production
mobile packaging are next milestones.

## Deploy

### Backend on Fly.io

The Go API is Fly-ready through `Dockerfile` and `fly.toml`.

```bash
fly secrets set YELP_API_KEY=your_yelp_fusion_api_key
fly deploy
fly status
```

Health and provider checks:

```text
https://routebite.fly.dev/v1/health
https://routebite.fly.dev/v1/providers
```

`/v1/providers` should show `restaurants: "yelp"` when the Yelp secret is set.

### Frontend on Vercel

Create a Vercel project from this GitHub repo and use:

```text
Root Directory: web
Build Command: npm run build
Output Directory: .next
Environment Variable: ROUTEBITE_API_BASE=https://routebite.fly.dev
```

The frontend keeps calling `/v1/*`; Next.js rewrites those requests to the Fly
backend using `ROUTEBITE_API_BASE`.

## What this project demonstrates

- Real-world problem → focused API design
- 3rd-party API integration with rate-limit protection (Yelp)
- External routing service integration (OSRM)
- Geo math: bounding box, distance-to-route
- Via-route detour calculation for restaurant stops
- Ranking/scoring algorithm with tunable weights
- Voice-friendly response generation
- Production patterns: caching, structured logging, Prometheus metrics, graceful shutdown
- Docker + Kubernetes deploy
- Unit-tested core (scoring, voice parsing, geo)

## License

MIT
