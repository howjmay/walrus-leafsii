# FX Protocol Backend Makefile

.PHONY: build test clean docker-build docker-up docker-down migrate-up migrate-down lint fmt deps help

# Variables
BINARY_DIR=bin
API_BINARY=$(BINARY_DIR)/fx-api
INDEXER_BINARY=$(BINARY_DIR)/fx-indexer
MIGRATE_BINARY=$(BINARY_DIR)/fx-migrate

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Default environment variables for development
export LFS_ENV ?= dev
export LFS_POSTGRES_DSN ?= postgres://fx:fx123@localhost:5432/fx?sslmode=disable
export LFS_REDIS_ADDR ?= localhost:6379
export LFS_SUI_RPC_URL ?= http://localnet:9000
export LFS_SUI_WS_URL ?= wss://localnet:9000 # may not work

## Build commands

build: ## Build all binaries
	@echo "Building binaries..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(API_BINARY) -v ./cmd/api
	$(GOBUILD) -o $(INDEXER_BINARY) -v ./cmd/indexer
	$(GOBUILD) -o $(MIGRATE_BINARY) -v ./cmd/migrate
	@echo "Build complete!"

build-api: ## Build API server binary
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(API_BINARY) -v ./cmd/api

build-indexer: ## Build indexer binary
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(INDEXER_BINARY) -v ./cmd/indexer

build-migrate: ## Build migration tool binary
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(MIGRATE_BINARY) -v ./cmd/migrate

## Run commands

run-api: build-api ## Run API server
	$(API_BINARY)

run-indexer: build-indexer ## Run event indexer
	$(INDEXER_BINARY)

run-migrate: build-migrate ## Run database migrations
	$(MIGRATE_BINARY) up

## Development commands

dev-api: ## Run API server in development mode (with live reload)
	$(GOCMD) run ./cmd/api

dev-indexer: ## Run indexer in development mode
	$(GOCMD) run ./cmd/indexer

dev-migrate: ## Run migrations in development mode
	$(GOCMD) run ./cmd/migrate up

## Test commands

test: ## Run all tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-verbose: ## Run tests with verbose output
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests and show coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-unit: ## Run unit tests only
	$(GOTEST) -short -v ./internal/calc/... ./internal/util/...

test-integration: ## Run integration tests (requires sui binary)
	GOPROXY=direct $(GOTEST) -tags=integration -run TestTransactionsBuildSignSubmit -v ./internal/api

test-integration-all: ## Run all integration tests
	GOPROXY=direct $(GOTEST) -tags=integration -v ./...

test-integration-e2e: ## Run E2E integration tests (requires sui binary)
	GOPROXY=direct $(GOTEST) -tags=integration -run E2E -v ./internal/api

## Quality assurance commands

lint: ## Run linter
	$(GOLINT) run

fmt: ## Format Go code
	$(GOFMT) -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

mod-tidy: ## Tidy go modules
	$(GOMOD) tidy

mod-download: ## Download go modules
	$(GOMOD) download

deps: mod-download ## Download dependencies

## Docker commands

docker-build: ## Build all Docker images
	docker build --target=api -t leafsii-backend-api .
	docker build --target=indexer -t leafsii-backend-indexer .
	docker build --target=migrate -t leafsii-backend-migrate .

docker-up: ## Start all services with Docker Compose
	docker-compose up -d

docker-down: ## Stop all services
	docker-compose down

docker-logs: ## Show logs from all services
	docker-compose logs -f

docker-logs-api: ## Show API logs
	docker-compose logs -f api

docker-logs-indexer: ## Show indexer logs
	docker-compose logs -f indexer

docker-restart-api: ## Restart API service
	docker-compose restart api

docker-restart-indexer: ## Restart indexer service
	docker-compose restart indexer

## Database commands

migrate-up: ## Run database migrations up
	$(GOCMD) run ./cmd/migrate up

migrate-down: ## Run database migrations down
	$(GOCMD) run ./cmd/migrate down

migrate-status: ## Check migration status
	$(GOCMD) run ./cmd/migrate status

## Utility commands

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html

install-tools: ## Install development tools
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest

help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## CI/CD commands

ci-test: deps lint test ## Run CI test pipeline
	@echo "✅ All CI tests passed!"

ci-build: ci-test build ## Run CI build pipeline
	@echo "✅ CI build complete!"

## Performance testing

load-test: ## Run basic load test against API
	@echo "Running load test..."
	@if command -v hey >/dev/null 2>&1; then \
		hey -z 30s -q 10 http://localhost:8080/v1/protocol/state; \
	else \
		echo "Install 'hey' for load testing: go install github.com/rakyll/hey@latest"; \
	fi

## Quick start commands

setup: deps docker-up ## Setup development environment
	@echo "Waiting for services to start..."
	@sleep 10
	@$(MAKE) migrate-up
	@echo "✅ Development environment ready!"

dev: setup ## Start development with all services
	@echo "Starting development servers..."
	@echo "API will be available at http://localhost:8080"
	@echo "Run 'make dev-api' and 'make dev-indexer' in separate terminals"

.DEFAULT_GOAL := help