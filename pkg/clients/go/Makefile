# Makefile for PandaFuzz Go Client SDK

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint
BINARY_NAME=pandafuzz-client
PKG=github.com/ethpandaops/pandafuzz/clients/go

# Build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# LDFLAGS
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build clean test test-race test-verbose coverage lint fmt vet tidy deps examples help

# Default target
all: test lint build

# Build the client SDK
build:
	@echo "Building Go client SDK..."
	$(GOBUILD) -v $(LDFLAGS) ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	$(GOTEST) -race -v ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) -v -count=1 ./...

# Generate test coverage report
coverage:
	@echo "Generating test coverage report..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Generate client code from OpenAPI spec
generate:
	@echo "Generating client code from OpenAPI specification..."
	~/go/bin/oapi-codegen -config oapi-codegen.yaml ../../api/v1/openapi/pandafuzz.yaml

# Update OpenAPI specification and regenerate
update-spec:
	@echo "Updating OpenAPI specification and regenerating client..."
	cp ../../api/v1/openapi/pandafuzz.yaml ./openapi-spec.yaml
	$(MAKE) generate

# Build and run examples
examples: build
	@echo "Building examples..."
	cd examples && $(GOBUILD) -o basic_operations basic_operations.go
	cd examples && $(GOBUILD) -o sse_events sse_events.go
	cd examples && $(GOBUILD) -o batch_operations batch_operations.go
	cd examples && $(GOBUILD) -o error_handling error_handling.go

# Run basic operations example
example-basic:
	@echo "Running basic operations example..."
	cd examples && $(GOBUILD) -o basic_operations basic_operations.go && ./basic_operations

# Run SSE events example
example-sse:
	@echo "Running SSE events example..."
	cd examples && $(GOBUILD) -o sse_events sse_events.go && ./sse_events

# Run batch operations example
example-batch:
	@echo "Running batch operations example..."
	cd examples && $(GOBUILD) -o batch_operations batch_operations.go && ./batch_operations

# Run error handling example
example-errors:
	@echo "Running error handling example..."
	cd examples && $(GOBUILD) -o error_handling error_handling.go && ./error_handling

# Clean example binaries
clean-examples:
	@echo "Cleaning example binaries..."
	rm -f examples/basic_operations examples/sse_events examples/batch_operations examples/error_handling

# Install development tools
install-tools:
	@echo "Installing development tools..."
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOGET) github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

# Check for security vulnerabilities
security:
	@echo "Checking for security vulnerabilities..."
	$(GOCMD) list -json -m all | nancy sleuth

# Benchmark tests
benchmark:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Check module compatibility
mod-check:
	@echo "Checking module compatibility..."
	$(GOMOD) graph | grep -E "$(PKG)" | head -10

# Print module information
mod-info:
	@echo "Module information:"
	@echo "Package: $(PKG)"
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	$(GOCMD) version
	$(GOMOD) graph | grep -c "$(PKG)" || echo "0 dependencies"

# Full CI pipeline
ci: tidy fmt vet lint test-race coverage

# Development workflow
dev: tidy fmt vet test

# Pre-commit hook
pre-commit: fmt vet lint test

# Release preparation
release-prep: clean tidy fmt vet lint test-race coverage examples

# Docker build (if needed)
docker-build:
	@echo "Building Docker image for examples..."
	docker build -t pandafuzz-go-client-examples .

# Help target
help:
	@echo "Available targets:"
	@echo "  all          - Run tests, lint, and build"
	@echo "  build        - Build the client SDK"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  test-race    - Run tests with race detection"
	@echo "  test-verbose - Run tests with verbose output"
	@echo "  coverage     - Generate test coverage report"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  tidy         - Tidy dependencies"
	@echo "  deps         - Download dependencies"
	@echo "  verify       - Verify dependencies"
	@echo "  generate     - Generate client code from OpenAPI spec"
	@echo "  update-spec  - Update OpenAPI spec and regenerate"
	@echo "  examples     - Build examples"
	@echo "  example-*    - Run specific examples"
	@echo "  clean-examples - Clean example binaries"
	@echo "  install-tools - Install development tools"
	@echo "  security     - Check for security vulnerabilities"
	@echo "  benchmark    - Run benchmarks"
	@echo "  mod-check    - Check module compatibility"
	@echo "  mod-info     - Print module information"
	@echo "  ci           - Full CI pipeline"
	@echo "  dev          - Development workflow"
	@echo "  pre-commit   - Pre-commit hook"
	@echo "  release-prep - Release preparation"
	@echo "  help         - Show this help message"