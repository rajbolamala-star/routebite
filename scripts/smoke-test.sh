#!/usr/bin/env bash
# End-to-end smoke test. Works fully offline with mock Yelp + mock routing.
# Set YELP_API_KEY for real Yelp data; leave USE_MOCK_ROUTING=true on first run.

set -e
BASE="${BASE:-http://localhost:8080}"

echo "=== Health ==="
curl -sS "$BASE/health" | jq .

echo
echo "=== Search: find me soup along the way ==="
curl -sS -X POST "$BASE/v1/search" \
  -H "Content-Type: application/json" \
  -d @scripts/example_request.json | jq '{
    route_summary,
    result_count: (.results | length),
    voice_summary,
    top_3: (.results | .[0:3] | map({name, rating, extra_minutes, score, call_link}))
  }'

echo
echo "=== Metrics ==="
curl -sS "$BASE/metrics" | grep -E "routebite_" | head -10
