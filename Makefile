# PandaFuzz Makefile

.PHONY: all build test clean docker help

# Variables
MASTER_BINARY := pandafuzz-master
BOT_BINARY := pandafuzz-bot
DOCKER_IMAGE := pandafuzz
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

# Default target
all: build

## build: Build both master and bot binaries
build: build-master build-bot

## build-master: Build the master binary
build-master:
	@echo "Building master..."
	@go build $(LDFLAGS) -o $(MASTER_BINARY) ./cmd/master

## build-bot: Build the bot binary
build-bot:
	@echo "Building bot..."
	@go build $(LDFLAGS) -o $(BOT_BINARY) ./cmd/bot

## build-web: Build the web UI
build-web:
	@echo "Building web UI..."
	@cd web && npm install && npm run build

## build-all: Build everything including web UI
build-all: build-master build-bot build-web

## web-dev: Run web UI in development mode
web-dev:
	@echo "Starting web UI in development mode..."
	@cd web && npm start

## test: Run all tests
test:
	@echo "Running tests..."
	@./run_tests.sh

## test-unit: Run unit tests only
test-unit:
	@echo "Running unit tests..."
	@go test ./tests/unit/... -v

## test-integration: Run integration tests only
test-integration:
	@echo "Running integration tests..."
	@go test ./tests/integration/... -v

## test-short: Run tests in short mode (skip long tests)
test-short:
	@echo "Running tests (short mode)..."
	@./run_tests.sh -s

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	@./run_tests.sh -c

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	@go test ./tests/integration -bench=. -benchmem

## lint: Run linters
lint:
	@echo "Running linters..."
	@golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

## docker: Build Docker images
docker:
	@echo "Building Docker images..."
	@docker build --target master -t $(DOCKER_IMAGE)-master:$(VERSION) .
	@docker build --target bot -t $(DOCKER_IMAGE)-bot:$(VERSION) .

## docker-compose: Start services with docker-compose
docker-compose:
	@echo "Starting services..."
	@docker-compose up -d

## docker-compose-down: Stop services
docker-compose-down:
	@echo "Stopping services..."
	@docker-compose down

## docker-compose-logs: View logs
docker-compose-logs:
	@docker-compose logs -f

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(MASTER_BINARY) $(BOT_BINARY)
	@rm -f coverage.out coverage.html
	@rm -rf data/ work/ logs/
	@rm -rf web/build/ web/node_modules/

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download

## mod-tidy: Tidy go modules
mod-tidy:
	@echo "Tidying modules..."
	@go mod tidy

## install: Install binaries to GOPATH/bin
install:
	@echo "Installing..."
	@go install $(LDFLAGS) ./cmd/master
	@go install $(LDFLAGS) ./cmd/bot

## run-master: Run master locally
run-master: build-master
	@echo "Running master..."
	@./$(MASTER_BINARY) -config master.yaml

## run-master-with-ui: Run master with web UI
run-master-with-ui: build-master build-web
	@echo "Running master with web UI..."
	@./$(MASTER_BINARY) -config master.yaml

## run-bot: Run bot locally
run-bot: build-bot
	@echo "Running bot..."
	@./$(BOT_BINARY) -config bot.yaml

## generate: Generate code (if any)
generate:
	@echo "Generating code..."
	@go generate ./...

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test

## ci: Run CI checks
ci: deps check

## help: Show this help message
help:
	@echo "PandaFuzz Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /' | sort

# Define version target
version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"