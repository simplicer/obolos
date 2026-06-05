.PHONY: build run test test-integration test-compose test-short test-cover clean fmt lint help image-build stack-up stack-down migrate-up

# Project
MODULE     := codeberg.org/simplicer/metering
BINARY     := bin/metering
VERSION    := $(shell cat VERSION)
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS    := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Go
GO      ?= go
GOFLAGS := -trimpath

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY) ./cmd/server

run: build ## Build and run the server
	./$(BINARY)

test: ## Run all tests
	$(GO) test -v -race -count=1 ./...

test-short: ## Run tests without race detection
	$(GO) test -count=1 ./...

test-integration: ## Run integration tests against a Postgres container (needs Docker)
	./bin/run-integration-tests

test-compose: ## Run the full test suite (unit + integration) inside containers
	docker compose -f infra/compose.test.yaml up --exit-code-from tests; \
	rc=$$?; docker compose -f infra/compose.test.yaml down >/dev/null 2>&1 || true; exit $$rc

test-cover: ## Run tests with coverage report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -f coverage.out coverage.html

fmt: ## Format code
	$(GO) fmt ./...

lint: ## Vet the code
	$(GO) vet ./...

image-build: ## Build the container image
	docker build -t metering:$(VERSION) -f infra/image/Containerfile .

stack-up: ## Start the service stack (service + database)
	docker compose -f infra/compose.yaml up -d

stack-down: ## Stop the service stack
	docker compose -f infra/compose.yaml down

migrate-up: ## Manually apply migrations via psql (the server also auto-migrates on startup)
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	@for f in migrations/*.sql; do echo ">> $$f"; psql "$(DATABASE_URL)" -f "$$f"; done
