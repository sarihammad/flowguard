.PHONY: help build test run clean docker-build docker-run docker-stop lint fmt

# Default target
help: ## Show this help message
	@echo "Rate Limiting Gateway - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build the application
build: ## Build the application
	@echo "Building rate limiting gateway..."
	go build -o bin/gateway cmd/main.go

# Run tests
test: ## Run all tests
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run the application locally
run: ## Run the application locally
	@echo "Starting rate limiting gateway..."
	go run cmd/main.go

# Clean build artifacts
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Build Docker image
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t rate-limiting-gateway .

# Run with Docker Compose
docker-run: ## Start all services with Docker Compose
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

# Stop Docker Compose services
docker-stop: ## Stop Docker Compose services
	@echo "Stopping Docker Compose services..."
	docker-compose down

# View Docker Compose logs
docker-logs: ## View Docker Compose logs
	docker-compose logs -f

# Lint code
lint: ## Lint Go code
	@echo "Linting code..."
	golangci-lint run

# Format code
fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...

# Install dependencies
deps: ## Install Go dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Generate go.sum
go-sum: ## Generate go.sum file
	@echo "Generating go.sum..."
	go mod tidy

# Run integration tests
test-integration: ## Run integration tests (requires Redis)
	@echo "Running integration tests..."
	go test -v ./tests/

# Start Redis for development
redis: ## Start Redis for development
	@echo "Starting Redis..."
	docker run -d --name redis-dev -p 6379:6379 redis:7-alpine

# Stop Redis
redis-stop: ## Stop Redis
	@echo "Stopping Redis..."
	docker stop redis-dev || true
	docker rm redis-dev || true

# Development setup
dev-setup: ## Setup development environment
	@echo "Setting up development environment..."
	@if [ ! -f .env ]; then cp config.env .env; fi
	@echo "Development environment ready!"

# Production build
build-prod: ## Build production binary
	@echo "Building production binary..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o bin/gateway cmd/main.go

# Run with specific config
run-with-config: ## Run with custom config file
	@echo "Running with custom config..."
	@if [ -z "$(CONFIG)" ]; then echo "Usage: make run-with-config CONFIG=path/to/config.env"; exit 1; fi
	@cp $(CONFIG) .env
	go run cmd/main.go

# Health check
health: ## Check service health
	@echo "Checking service health..."
	@curl -f http://localhost:8080/health || echo "Service not running"

# Load test
load-test: ## Run load test (requires hey tool)
	@echo "Running load test..."
	@if ! command -v hey &> /dev/null; then echo "Install hey tool first: go install github.com/rakyll/hey@latest"; exit 1; fi
	hey -n 1000 -c 10 -H "X-API-Key: test-key" http://localhost:8080/proxy/get

# Benchmark
benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	go test -bench=. ./...

# Security scan
security: ## Run security scan
	@echo "Running security scan..."
	gosec ./...

# Update dependencies
update-deps: ## Update Go dependencies
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

# Show project info
info: ## Show project information
	@echo "Rate Limiting Gateway"
	@echo "===================="
	@echo "Go version: $(shell go version)"
	@echo "Git commit: $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
	@echo "Build time: $(shell date)"
	@echo ""
	@echo "Available endpoints:"
	@echo "  - Health: http://localhost:8080/health"
	@echo "  - Metrics: http://localhost:8080/metrics"
	@echo "  - Rate limit info: http://localhost:8080/api/rate-limit-info"
	@echo "  - Proxy: http://localhost:8080/proxy/*" 