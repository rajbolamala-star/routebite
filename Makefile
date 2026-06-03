.PHONY: help build test run docker-up docker-down deploy clean

help:
	@echo "RouteBite — available targets:"
	@echo "  build        - Build the binary"
	@echo "  test         - Run unit tests"
	@echo "  run          - Run locally (mock providers, no API key required)"
	@echo "  run-real     - Run with real Yelp + OSRM (requires YELP_API_KEY)"
	@echo "  docker-up    - Run via docker-compose"
	@echo "  docker-down  - Stop docker-compose"
	@echo "  deploy       - Deploy to Fly.io"

build:
	go build -o bin/routebite ./cmd/server

test:
	go test ./... -race -cover

run: build
	USE_MOCK_ROUTING=true ./bin/routebite

run-real: build
	USE_MOCK_ROUTING=false ./bin/routebite

docker-up:
	cd deploy/docker && docker-compose up --build

docker-down:
	cd deploy/docker && docker-compose down

deploy:
	fly deploy

clean:
	rm -rf bin/
