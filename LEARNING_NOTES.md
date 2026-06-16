# RouteBite Learning Notes

These notes explain RouteBite from the product idea down to the backend design.
Use them to understand the project, talk about it in interviews, and remember
why each feature exists.

## 1. Why RouteBite Exists

RouteBite helps drivers find food along a route without wasting time or using a
phone while driving.

The core question is:

> "I am already driving somewhere. What is the best food stop on my way that
> will not delay me too much?"

Most restaurant apps answer:

> "What restaurants are near me?"

Most map apps answer:

> "How do I get to my destination?"

RouteBite combines both:

> "What restaurant is good, open, matches what I want, and is convenient for my
> route?"

## 2. The Real Problem It Solves

Imagine someone is driving from one city to another and wants to pick up food.
Without RouteBite, they may need to:

1. Stop navigation.
2. Open a food app.
3. Search manually.
4. Compare ratings and reviews.
5. Check whether the restaurant is actually on the route.
6. Call the restaurant.
7. Restart navigation.

That is slow, distracting, and unsafe.

RouteBite turns this into one backend request. The response is compact,
voice-friendly, and useful for a mobile app, CarPlay-style interface, or future
voice assistant.

## 3. High-Level Architecture

RouteBite is a Go/Gin backend. It is built around small components that each
have one job.

Main parts:

- **Gin API handlers:** receive HTTP requests and return JSON responses.
- **Agent parser:** turns natural language or structured input into a route
  search plan.
- **Geocoding client:** turns place names into coordinates.
- **Routing engine:** calculates the route between origin and destination.
- **Restaurant provider:** fetches restaurants from mock data or Yelp.
- **Detour scoring:** calculates how much time a stop adds.
- **RouteBite Score:** gives each recommendation an explainable 0-100 score.
- **best_pick:** chooses the strongest recommendation.
- **Request ID middleware:** adds trace IDs to responses, logs, and errors.
- **Structured logging:** logs request metadata as JSON.
- **Optional PostgreSQL history:** stores successful agent searches.
- **Optional Redis cache:** reduces repeated external API calls.

Architecture diagram:

```text
Client
  |
  | POST /v1/agent/search
  v
Gin Router + Middleware
  |
  | adds X-Request-ID
  v
Agent Handler
  |
  v
Agent Parser
  |
  v
Geocoding -> Routing -> Restaurant Search
  |              |          |
  |              |          +-- optional Redis cache
  |              |
  |              +-- route geometry + duration
  |
  v
Detour Ranking
  |
  v
RouteBite Score + best_pick
  |
  v
JSON Response
  |
  +-- optional PostgreSQL history save
```

## 4. Request Flow for POST /v1/agent/search

Example request:

```json
{
  "query": "I am driving from Kingsport TN to Nashville TN and want Indian food with less than 10 minutes detour",
  "start": "Kingsport, TN",
  "destination": "Nashville, TN",
  "preference": "Indian food",
  "max_detour_minutes": 10
}
```

Step-by-step flow:

1. The client sends `POST /v1/agent/search`.
2. Middleware attaches an `X-Request-ID`.
3. The handler decodes JSON into `AgentSearchRequest`.
4. The agent parser builds an internal `agentPlan`.
5. The service validates start, destination, and preference.
6. Geocoding converts start and destination into coordinates.
7. Routing calculates the base route.
8. Restaurant search finds candidates near the route.
9. Detour logic calculates extra travel time for each stop.
10. Ranking keeps the best practical restaurants.
11. RouteBite Score explains each recommendation.
12. `best_pick` selects the highest-scoring option.
13. The response returns `summary`, `best_pick`, and `restaurants`.
14. If PostgreSQL is enabled, RouteBite saves the request summary.
15. If Redis is enabled, repeated geocoding/restaurant lookups can be cached.

Response shape:

```json
{
  "summary": "Best option is Saffron Indian Kitchen, about 6 minutes off your route, rated 4.5 stars and currently open.",
  "best_pick": {
    "name": "Saffron Indian Kitchen",
    "rating": 4.5,
    "detour_minutes": 6,
    "open_now": true,
    "address": "123 Main St, Nashville, TN",
    "phone": "+16155551212",
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

Important API detail:

- `best_pick` is the top recommendation.
- `restaurants` still contains the list of top recommendations.
- Existing fields like `name`, `rating`, `detour_minutes`, `open_now`,
  `address`, `phone`, and `reason` remain available.

## 5. How the Agent Parser Works

The current agent parser is rule-based. It does not call OpenAI, Ollama, or any
LLM yet.

It works like this:

1. Prefer structured fields if they exist.
2. If fields are missing, inspect the natural language `query`.
3. Extract what it can:
   - start location
   - destination
   - food preference
   - maximum detour minutes
4. Normalize values.
5. Apply defaults.

Example query:

```text
I am driving from Brooklyn to Newark and want pizza under 8 minutes
```

Parsed plan:

```text
start = Brooklyn
destination = Newark
preference = pizza
max_detour_minutes = 8
```

Why this is a good first version:

- It is deterministic.
- It is easy to test.
- It keeps the project useful without an LLM dependency.
- It is behind an interface, so it can be replaced later.

Interview explanation:

> "I started with a rule-based parser because it is predictable and testable.
> The parser is behind an interface, so an OpenAI or Ollama parser can be added
> later without rewriting the routing and recommendation pipeline."

## 6. How Geocoding, Routing, and Restaurant Search Fit In

The agent does not directly search restaurants. It creates a plan, then reuses
the existing route recommendation pipeline.

```text
Place names
  |
  v
Geocoding
  |
  v
Coordinates
  |
  v
Routing
  |
  v
Route polyline + base duration
  |
  v
Restaurant search near the route
  |
  v
Detour scoring + ranking
```

Each service answers a specific question:

- **Geocoding:** "Where is this place on the map?"
- **Routing:** "What path does the driver take?"
- **Restaurant search:** "What food options exist near that path?"
- **Scoring:** "Which option is actually worth stopping for?"

## 7. How Detour Scoring Works

Detour scoring measures how much extra time a restaurant adds to the trip.

Example:

```text
User max detour: 10 minutes

Restaurant A: 3 minute detour  -> strong candidate
Restaurant B: 9 minute detour  -> possible candidate
Restaurant C: 18 minute detour -> filtered out
```

This makes RouteBite different from a normal restaurant search. A restaurant
can be excellent, but if it is too far off the route, it is not a good driving
recommendation.

## 8. RouteBite Score and best_pick

RouteBite Score is a 0-100 score added to each agent recommendation.

It exists because the API should feel like a decision engine, not just a list.
The backend explains why one recommendation is better than another.

Score parts:

- `detour_score`: rewards low extra drive time.
- `rating_score`: rewards higher restaurant rating.
- `open_now_score`: rewards restaurants that are currently open.
- `preference_match_score`: rewards matching the requested food.
- `convenience_score`: rewards useful pickup info like address and phone.

Formula:

```text
routebite_score =
    detour_score           * 0.35
  + rating_score           * 0.25
  + open_now_score         * 0.15
  + preference_match_score * 0.15
  + convenience_score      * 0.10
```

Why these weights make sense:

- Detour is most important because drivers care about time.
- Rating matters because quality still matters.
- Open status matters because closed restaurants are not useful.
- Preference match matters because the user asked for a specific food.
- Convenience matters because pickup should be easy.

`best_pick` selection:

1. Choose the highest `routebite_score`.
2. If tied, prefer lower detour.
3. If still tied, prefer higher rating.

Interview explanation:

> "I added RouteBite Score so the API can explain its recommendation. Instead
> of just returning restaurants, the backend gives each option a deterministic
> score and picks the best one."

## 9. Request ID Tracing and Structured Logging

Every request gets a request ID.

If the client sends:

```text
X-Request-ID: demo-123
```

RouteBite preserves it. If the client does not send one, RouteBite generates
one.

The response includes:

```text
X-Request-ID: demo-123
```

Error responses include the same ID:

```json
{
  "error": {
    "message": "agent needs a start location",
    "request_id": "demo-123"
  }
}
```

Structured request logs include:

```json
{
  "timestamp": "2026-06-16T00:00:00Z",
  "method": "POST",
  "path": "/v1/agent/search",
  "status": 200,
  "latency_ms": 42,
  "client_ip": "127.0.0.1",
  "request_id": "demo-123"
}
```

Cache logs also include request ID when available:

```json
{
  "event": "cache_hit",
  "source": "geocode",
  "cache_key": "geocode:kingsport-tn:limit:1",
  "request_id": "demo-123"
}
```

Why this matters:

- A user can report a request ID.
- Logs can be searched by that request ID.
- Debugging production issues becomes much easier.

## 10. Optional PostgreSQL Agent Search History

PostgreSQL persistence is optional.

Default:

```text
DB_ENABLED=false
```

When enabled, RouteBite stores successful `/v1/agent/search` requests in the
`agent_searches` table.

Stored fields:

- request ID
- query
- start location
- destination
- preference
- max detour minutes
- result count
- summary
- created timestamp

Why this is useful:

- It shows production-style persistence.
- It creates a searchable history of agent decisions.
- It can support future analytics.
- It helps debug real user requests.

Run with PostgreSQL:

```bash
export DB_ENABLED=true
export DATABASE_URL='postgres://routebite:routebite@localhost:5432/routebite?sslmode=disable'
psql "$DATABASE_URL" -f migrations/001_create_agent_searches.sql
go run ./cmd/server
```

## 11. Optional Redis Caching

Redis caching is also optional.

Default:

```text
REDIS_ENABLED=false
```

When enabled, Redis caches repeated external/API-style results:

- geocoding results
- restaurant/Yelp search results

This helps because a user may repeat the same search, refresh the app, or adjust
small settings. Caching avoids repeated external calls and can make responses
faster.

Important environment variables:

```text
REDIS_ENABLED=true
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0
CACHE_TTL_MINUTES=15
```

Run Redis locally:

```bash
docker run --rm -p 6379:6379 redis:7
```

Run RouteBite with Redis:

```bash
export REDIS_ENABLED=true
export REDIS_ADDR=localhost:6379
export CACHE_TTL_MINUTES=15
go run ./cmd/server
```

Example cache keys:

```text
geocode:kingsport-tn:limit:1
restaurants:soup:25.9290:-80.1690:radius:5000:open:true
```

Good cache key rules used here:

- normalize strings to lowercase
- include important inputs
- avoid secrets
- keep keys readable

Interview explanation:

> "I added Redis as an optional shared cache. If Redis is disabled or fails,
> the API still calls the real provider. This reduces repeated external API
> calls without making Redis required for local development."

## 12. Failure-Safe Design Decisions

RouteBite is designed so optional infrastructure does not break the main
recommendation flow.

Failure-safe choices:

- If PostgreSQL is disabled, the app still runs.
- If PostgreSQL save fails, RouteBite logs the error and still returns the
  recommendation.
- If Redis is disabled, the app still runs.
- If Redis read fails, RouteBite logs `cache_error` and calls the real service.
- If Redis write fails, RouteBite logs `cache_error` and continues.
- If a request is invalid, the API returns a JSON error with `request_id`.

Why this matters:

> The core user value is returning a safe food recommendation. Persistence and
> caching are useful, but they should not prevent the driver from getting an
> answer.

This is a production mindset: separate critical path from nice-to-have
infrastructure.

## 13. How Tests Are Structured

RouteBite tests focus on behavior.

Important test areas:

- agent parser extracts route and preference fields
- structured fields win over parsed query text
- missing fields return proper errors
- error responses include request ID
- request ID middleware adds IDs to headers and context
- PostgreSQL persistence uses a mock repository
- persistence failure does not break the response
- Redis cache behavior uses fake caches, not a real Redis server
- cache read failure falls back to the real provider
- restaurant cache hits avoid provider calls
- RouteBite Score produces expected score components
- `best_pick` uses the highest RouteBite Score
- `restaurants` still contains results when `best_pick` exists

Why mocks/fakes are important:

- tests stay fast
- tests run without real Yelp, Redis, or PostgreSQL
- behavior is tested through interfaces
- failures are easy to simulate

Run tests:

```bash
go test ./...
```

## 14. What Each Major Folder/File Does

Useful repo map:

```text
cmd/server/main.go
  Starts the API server, reads environment variables, wires dependencies.

internal/api/
  HTTP handlers, request/response models, agent service, errors, tests.

internal/api/agent_handler.go
  Handles POST /v1/agent/search.

internal/api/agent_service.go
  Parses agent requests, runs search, applies RouteBite Score, picks best_pick.

internal/api/agent_models.go
  Defines agent request and response JSON shapes.

internal/cache/
  In-memory TTL cache plus optional Redis JSON cache interface.

internal/geocode/
  Mock and Nominatim geocoding clients.

internal/routing/
  Mock and OSRM routing engines, route geometry helpers.

internal/yelp/
  Yelp Fusion client and mock restaurant provider.

internal/scoring/
  Core restaurant ranking and voice summary logic.

internal/middleware/
  Request ID middleware and structured request logger.

internal/history/
  Optional PostgreSQL repository for agent search history.

migrations/
  SQL migration for the agent_searches table.

README.md
  Main project documentation and setup instructions.

LEARNING_NOTES.md
  Interview and learning guide for understanding the project.

web/
  Frontend app. This backend work does not change it.
```

## 15. How to Run Locally

Mock mode, no external APIs:

```bash
make run
```

Run the web app:

```bash
make web-dev
```

Try the agent endpoint:

```bash
curl -X POST http://localhost:8080/v1/agent/search \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: demo-agent-123" \
  -d '{
    "query": "I am driving from North Miami Beach to Sunrise and want soup under 10 minutes",
    "start": "North Miami Beach",
    "destination": "Sunrise",
    "preference": "soup",
    "max_detour_minutes": 10
  }'
```

Run without optional infrastructure:

```bash
DB_ENABLED=false REDIS_ENABLED=false go run ./cmd/server
```

Run with PostgreSQL:

```bash
export DB_ENABLED=true
export DATABASE_URL='postgres://routebite:routebite@localhost:5432/routebite?sslmode=disable'
psql "$DATABASE_URL" -f migrations/001_create_agent_searches.sql
go run ./cmd/server
```

Run with Redis:

```bash
docker run --rm -p 6379:6379 redis:7

export REDIS_ENABLED=true
export REDIS_ADDR=localhost:6379
export CACHE_TTL_MINUTES=15
go run ./cmd/server
```

## 16. How to Explain the Full Request Flow in an Interview

Short version:

> "RouteBite receives a natural language or structured route request, parses it
> into a search plan, geocodes the route, gets routing data, searches
> restaurants near the route, calculates detour cost, scores recommendations,
> returns a best pick, logs the request with a request ID, optionally caches
> repeated external results in Redis, and optionally stores the agent search in
> PostgreSQL."

Detailed version:

> "The API starts in Gin. Middleware adds a request ID so logs and errors can
> be correlated. The agent handler validates the JSON request and passes it to
> the agent service. The rule-based parser prefers structured fields but can
> parse simple natural language like 'from X to Y and want pizza under 10
> minutes.' Then the service geocodes the locations, calculates a route, fetches
> restaurants near the route, filters by max detour, and ranks candidates. The
> agent layer adds RouteBite Score, chooses best_pick, and returns a
> voice-friendly summary. Optional Redis caching reduces repeated geocoding and
> restaurant calls. Optional PostgreSQL history saves successful searches, but
> failures do not break the main response."

Strong technical points:

- interface-based parser design
- mockable external dependencies
- optional Redis and PostgreSQL
- failure-safe infrastructure
- request ID tracing
- structured JSON logs
- deterministic scoring
- tests for edge cases and failure paths

## 17. What I Learned From Building This Project

Important lessons:

- A good backend starts with a real user problem, not just technology.
- It is better to build a simple working agent first, then make it smarter.
- Interfaces make future upgrades easier.
- External APIs should be wrapped behind small clients.
- Optional infrastructure should not break the core user experience.
- Request IDs are simple but very useful for debugging.
- Caching is not only about speed; it also protects rate limits.
- Tests should cover failure paths, not just happy paths.
- A score is more useful when it can be explained.
- Production-style code is often about clean boundaries and graceful failure.

How this helps in interviews:

> "I can explain not only what I built, but why each part exists and how I
> handled reliability, observability, external services, and testability."

## 18. Future Upgrades

Good next steps:

### Ollama or OpenAI Parser

Replace or augment the rule-based parser with an LLM-backed parser.

Important idea:

- Keep the existing `agentParser` interface.
- Add a new implementation.
- Fall back to rule-based parsing if the LLM fails.

### Auth

Add user accounts or API keys.

Possible use cases:

- save user preferences
- protect public API usage
- track usage by user

### Deployment

Improve production deployment.

Possible steps:

- Fly.io backend deployment
- managed Postgres
- managed Redis
- health checks
- environment-specific configs
- CI tests before deploy

### Frontend Polish

Improve the user-facing app once backend behavior is strong.

Possible upgrades:

- cleaner result cards
- better mobile layout
- loading and error states
- map visualization
- voice readout
- better address autocomplete

### Better Ranking

Improve recommendation quality.

Possible upgrades:

- user preferences
- cuisine embeddings
- price filtering
- restaurant hours confidence
- more precise route detour calculations

## 19. Interview Closing Pitch

One-sentence version:

> "RouteBite is a production-style Go backend that helps drivers find the best
> food stop along their route using geocoding, routing, restaurant search,
> detour ranking, explainable scoring, request tracing, optional Redis caching,
> and optional PostgreSQL history."

Longer version:

> "I built RouteBite around a real safety and convenience problem. The backend
> accepts natural language or structured route requests, turns them into a
> search plan, combines route data with restaurant data, calculates detour cost,
> and returns a best recommendation with an explainable RouteBite Score. I also
> added production backend patterns: request IDs, structured logs, JSON errors,
> optional Redis caching, optional PostgreSQL history, and tests using mocks and
> fakes. The result is not just a demo endpoint, but a backend system with clean
> boundaries and realistic reliability decisions."
