# RouteBite Learning Notes

These notes explain RouteBite from the product idea down to the backend flow.
Use them to understand the project, explain it in interviews, and remember why
each technical decision exists.

## 1. Why RouteBite Exists

RouteBite helps drivers find food along a route without wasting time or using a
phone while driving.

The idea is simple:

> "I am already driving somewhere. What is the best food stop on my way that
> will not delay me too much?"

Most food apps answer, "What restaurants are near me?" Map apps answer, "How do
I get to my destination?" RouteBite combines both questions:

> "What restaurant is good, open, matches what I want, and is convenient for my
> route?"

## 2. Real Problem It Solves

Imagine a person driving from one city to another. They want food, but they do
not want to:

1. Stop navigation.
2. Open a food app.
3. Search restaurants manually.
4. Compare reviews while driving.
5. Check if the stop is actually on the way.
6. Call the restaurant.
7. Start navigation again.

That is slow and unsafe.

RouteBite turns that into one backend request. The response is short,
driver-safe, and easy for a phone or voice assistant to read aloud.

## 3. High-Level Architecture

RouteBite is a Go/Gin backend. The main backend parts are:

- **API handler:** receives HTTP requests and returns JSON.
- **Agent parser:** turns natural language or structured fields into a route
  search plan.
- **Geocoding client:** turns place names into coordinates.
- **Routing engine:** calculates the route between origin and destination.
- **Restaurant provider:** fetches restaurants, either mock data or Yelp.
- **Detour/ranking logic:** decides which restaurants are practical stops.
- **RouteBite Score:** explains why one recommendation is better than another.
- **Request middleware:** adds request IDs and structured logs.
- **Optional history repository:** saves agent searches to PostgreSQL when
  enabled.

Simple architecture diagram:

```text
Client
  |
  | POST /v1/agent/search
  v
Gin API Handler
  |
  v
Agent Parser
  |
  v
Geocoding -> Routing -> Restaurant Search
  |
  v
Detour Ranking + RouteBite Score
  |
  v
best_pick + restaurants + summary
  |
  v
Optional PostgreSQL History Save
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

Flow:

1. Gin receives the HTTP request.
2. Request ID middleware attaches an `X-Request-ID`.
3. The agent handler decodes JSON into `AgentSearchRequest`.
4. The agent service creates an internal `agentPlan`.
5. The service validates start, destination, and food preference.
6. Geocoding turns start and destination into coordinates.
7. The normal route search pipeline runs.
8. Restaurants are ranked by route convenience and quality.
9. Each result gets a RouteBite Score and score breakdown.
10. The highest-scoring result becomes `best_pick`.
11. The API returns a compact JSON response.
12. If persistence is enabled, the request summary is saved to PostgreSQL.

Example response shape:

```json
{
  "summary": "Best option is Saffron Indian Kitchen, about 6 minutes off your route, rated 4.5 stars and currently open.",
  "best_pick": {
    "name": "Saffron Indian Kitchen",
    "rating": 4.5,
    "detour_minutes": 6,
    "open_now": true,
    "routebite_score": 84,
    "score_breakdown": {
      "detour_score": 40,
      "rating_score": 90,
      "open_now_score": 100,
      "preference_match_score": 100,
      "convenience_score": 100
    },
    "reason": "Highest RouteBite Score (84/100): low detour, highly rated, currently open."
  },
  "restaurants": [
    {
      "name": "Saffron Indian Kitchen",
      "rating": 4.5,
      "detour_minutes": 6,
      "open_now": true,
      "routebite_score": 84,
      "score_breakdown": {
        "detour_score": 40,
        "rating_score": 90,
        "open_now_score": 100,
        "preference_match_score": 100,
        "convenience_score": 100
      },
      "reason": "Low detour, highly rated, currently open"
    }
  ]
}
```

## 5. How the Agent Parser Works

The first version of the agent is rule-based. That means it does not call an
LLM yet. It uses simple deterministic parsing.

The parser follows this order:

1. If structured fields exist, use them first.
2. If fields are missing, inspect the natural language `query`.
3. Try to extract:
   - start location
   - destination
   - food preference
   - maximum detour minutes
4. Normalize values, such as changing `"Indian food"` to `"Indian"`.
5. Apply defaults when needed.

Example:

```text
I am driving from Brooklyn to Newark and want pizza under 8 minutes
```

The parser can turn that into:

```text
start = Brooklyn
destination = Newark
preference = pizza
max_detour_minutes = 8
```

Why this is good for the project:

- It is easy to test.
- It is predictable.
- It keeps the backend useful before adding OpenAI or Ollama.
- The parser is behind an interface, so it can be replaced later.

## 6. How Geocoding, Routing, and Restaurant Search Fit In

The agent does not directly search restaurants. It creates a search plan and
then reuses the existing backend pipeline.

The pipeline works like this:

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
Routing engine
  |
  v
Route polyline + base duration
  |
  v
Restaurant search near route
  |
  v
Detour calculation + ranking
```

Geocoding answers:

> "Where is Kingsport, TN on the map?"

Routing answers:

> "What is the route from Kingsport to Nashville?"

Restaurant search answers:

> "What Indian restaurants are near this route?"

Scoring answers:

> "Which of these restaurants is actually worth stopping for?"

## 7. How Detour Scoring Works

Detour scoring asks:

> "How much extra time does this restaurant add to the trip?"

A restaurant can be highly rated but still be a bad driving recommendation if
it is too far off the route.

RouteBite calculates or estimates extra minutes and filters out restaurants that
exceed the user's maximum detour.

Example:

```text
User max detour: 10 minutes

Restaurant A: 3 minute detour -> good candidate
Restaurant B: 9 minute detour -> possible candidate
Restaurant C: 18 minute detour -> filtered out
```

This makes RouteBite different from a normal restaurant search. The route is
part of the decision.

## 8. How RouteBite Score Works

RouteBite Score is a 0 to 100 score for each agent recommendation.

It exists so the backend can explain its decision, not just return a list.

The score uses five parts:

- `detour_score`: better when the restaurant is closer to the route.
- `rating_score`: better when the restaurant has a higher rating.
- `open_now_score`: better when the restaurant is open.
- `preference_match_score`: better when the restaurant matches the requested
  food.
- `convenience_score`: better when useful pickup info like address and phone is
  available.

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

- Detour matters most because the driver does not want to lose time.
- Rating matters because food quality still matters.
- Open status matters because closed restaurants are useless.
- Preference match matters because the user asked for something specific.
- Convenience matters because calling and navigating should be easy.

## 9. How best_pick Is Selected

`best_pick` is the highest-scoring recommendation.

The API still returns the full `restaurants` list. `best_pick` is just the
single strongest recommendation from that list.

Selection logic:

1. Sort by highest `routebite_score`.
2. If scores tie, prefer lower detour.
3. If detour also ties, prefer higher rating.

This is useful for voice experiences. A phone can say:

> "Best option is Saffron Indian Kitchen, about 6 minutes off your route."

The user does not need to inspect the full list while driving.

## 10. How request_id Tracing and Logging Works

Every request gets a request ID.

If the client sends:

```text
X-Request-ID: demo-123
```

RouteBite keeps it. If the client does not send one, RouteBite generates one.

The response also includes:

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

Structured logs include fields like:

```json
{
  "method": "POST",
  "path": "/v1/agent/search",
  "status": 200,
  "latency_ms": 42,
  "request_id": "demo-123"
}
```

Why this matters:

- A user can report a request ID.
- Logs can be searched by that request ID.
- Debugging production issues becomes much easier.

## 11. How Optional PostgreSQL Persistence Works

Persistence is optional.

By default:

```text
DB_ENABLED=false
```

That means RouteBite runs without a database.

When enabled, RouteBite stores each successful `/v1/agent/search` request in
the `agent_searches` table.

Stored fields include:

- request ID
- original query
- start location
- destination
- preference
- max detour minutes
- result count
- response summary
- created timestamp

This creates a useful history for analytics, debugging, and demoing production
backend design.

## 12. Why Persistence Is Failure-Safe

Persistence should never block the user from getting a recommendation.

If PostgreSQL is disabled, missing, or unavailable, RouteBite still works.

If saving history fails after a successful recommendation:

1. RouteBite logs the error.
2. The API still returns the recommendation.
3. The driver experience is not broken.

This is important because search is the core user value. History is useful, but
it is not critical enough to fail the request.

Interview explanation:

> "I treated persistence as best-effort because the main business function is
> returning route-safe food recommendations. A database failure should not
> prevent the driver from getting a result."

## 13. How Tests Are Structured

RouteBite tests focus on backend behavior, not only happy paths.

Important test areas:

- Agent parser extracts start, destination, preference, and detour.
- Structured fields override parsed query values.
- Missing required fields return errors.
- Error responses include request IDs.
- History persistence uses a mock repository.
- Persistence failure does not break the response.
- RouteBite Score produces expected score components.
- `best_pick` uses the highest RouteBite Score.
- `restaurants` still contains results when `best_pick` exists.

Why mock repositories are used:

- Tests stay fast.
- Tests do not require a real PostgreSQL server.
- The service behavior is tested through interfaces.

Run tests:

```bash
go test ./...
```

## 14. How to Run Locally

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

Run with optional PostgreSQL:

```bash
export DB_ENABLED=true
export DATABASE_URL='postgres://routebite:routebite@localhost:5432/routebite?sslmode=disable'
psql "$DATABASE_URL" -f migrations/001_create_agent_searches.sql
go run ./cmd/server
```

Run without PostgreSQL:

```bash
DB_ENABLED=false go run ./cmd/server
```

## 15. How to Explain This Project in an Interview

Short version:

> "RouteBite is a Go backend that helps drivers find the best food stop along
> their route. It combines geocoding, routing, restaurant search, detour
> ranking, and a rule-based agent endpoint. The API returns a driver-safe
> summary, a best pick, and an explainable RouteBite Score."

More detailed version:

> "The project started from a real problem: food apps and map apps do not work
> well together while driving. RouteBite accepts either structured route fields
> or a natural language request, parses it into a search plan, geocodes the
> route, finds restaurants near the route, calculates detour cost, ranks the
> options, and returns a best recommendation. I added production backend
> patterns like request IDs, structured logging, JSON errors, optional
> PostgreSQL persistence, and tests around the important behavior."

Good technical points to mention:

- The agent parser is behind an interface, so it can later be replaced with
  OpenAI or Ollama.
- Persistence is optional and failure-safe.
- The API keeps response compatibility while adding richer fields.
- RouteBite Score is deterministic and explainable.
- Tests use mocks for external dependencies.
- Request IDs make the service easier to debug in production.

Good product points to mention:

- It solves a real safety and convenience problem.
- It is designed for voice or mobile assistant experiences.
- It chooses the best option, not just a list of nearby restaurants.
- It optimizes for route convenience, not only restaurant quality.

One-sentence closer:

> "RouteBite shows that I can build a practical backend system that connects a
> real user problem to clean API design, external services, scoring logic,
> observability, persistence, and tests."
