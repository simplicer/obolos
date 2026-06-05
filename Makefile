.PHONY: build run test clean fmt lint help docker-build docker-up docker-down

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

docker-build: ## Build Docker image
	docker build -t metering:$(VERSION) -f infra/docker/Dockerfile .

docker-up: ## Start Docker container
	cd infra && docker compose up -d

docker-down: ## Stop Docker container
	cd infra && docker compose down

migrate-up: ## Run database migrations
	$(GO) run github.com/jackc/pgx/v5/pgxpool@latest || true
	@echo "Run migrations manually: psql -f migrations/001_initial_schema.sql"
