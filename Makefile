.PHONY: help build test run run-real web-install web-dev web-build web-lint web-typecheck docker-up docker-down deploy clean

help:
	@echo "RouteBite — available targets:"
	@echo "  build        - Build the binary"
	@echo "  test         - Run unit tests"
	@echo "  run          - Run locally (mock providers, no API key required)"
	@echo "  run-real     - Run with real Yelp + OSRM (requires YELP_API_KEY)"
	@echo "  web-install  - Install Next.js app dependencies"
	@echo "  web-dev      - Run the Next.js app at localhost:3000"
	@echo "  web-build    - Build the Next.js app"
	@echo "  web-lint     - Lint the Next.js app"
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

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

web-lint:
	cd web && npm run lint

web-typecheck:
	cd web && npm run typecheck

docker-up:
	cd deploy/docker && docker-compose up --build

docker-down:
	cd deploy/docker && docker-compose down

deploy:
	fly deploy

clean:
	rm -rf bin/ web/.next web/out
