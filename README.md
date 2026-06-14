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

`POST /v1/search` with:

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

```
score = rating_normalized       * 0.4
      + review_count_normalized * 0.2
      + convenience(extra_min)  * 0.3
      + (open_now ? 0.1 : 0)
```

Mimics the tradeoff a driver makes mentally: how good is it, vs how much does it cost me to stop?

## Tech stack

- **Language:** Go 1.21
- **HTTP:** Gin
- **Web app:** Next.js, React, TypeScript
- **Restaurant data:** Yelp Fusion (free tier — 5000 calls/day)
- **Routing:** OSRM public demo for MVP (swap for Mapbox / Google Routes for production)
- **Voice parser:** keyword + intent extractor (pluggable for OpenAI / Whisper later)
- **Cache:** In-memory TTL cache (Postgres-ready interface for later persistence)
- **Observability:** Prometheus metrics
- **Deploy:** Docker, docker-compose, Kubernetes, Fly.io-ready

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/search` | Find restaurants along a route |
| `GET`  | `/v1/geocode` | Resolve typed places into coordinates |
| `GET`  | `/v1/providers` | Show active data providers |
| `GET`  | `/v1/health` | Health check |
| `GET`  | `/v1/metrics` | Prometheus metrics |

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

## What this project demonstrates

- Real-world problem → focused API design
- 3rd-party API integration with rate-limit protection (Yelp)
- External routing service integration (OSRM)
- Geo math: bounding box, distance-to-route
- Ranking/scoring algorithm with tunable weights
- Voice-friendly response generation
- Production patterns: caching, structured logging, Prometheus metrics, graceful shutdown
- Docker + Kubernetes deploy
- Unit-tested core (scoring, voice parsing, geo)

## License

MIT
