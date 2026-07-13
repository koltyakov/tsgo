.PHONY: build test test-v bench bench-all lint fmt tidy deps-update clean help

# Default target
all: build

# Lint cache configuration (helps in restricted/sandboxed environments)
LINT_HOME ?= /tmp
LINT_GOCACHE ?= /tmp/go-build
LINT_GOLANGCI_CACHE ?= /tmp/golangci-lint-cache
LINT_GOMODCACHE ?= $(shell go env GOMODCACHE)

# Build the library
build:
	go build ./...

# Run all tests
test:
	go test ./...

# Run tests with verbose output
test-v:
	go test -v ./...

# Run tests with coverage
test-cover:
	go test -cover ./...
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run benchmarks (quick)
bench:
	go test -bench=. -benchmem -timeout=5m -run=^$$ ./internal/benchmark/...

# Run benchmarks with count for stability
bench-all:
	go test -bench=. -benchmem -benchtime=1s -count=3 -timeout=10m -run=^$$ ./internal/benchmark/...

# Run specific engine benchmarks
bench-goja:
	go test -bench=GOJA -benchmem -run=^$$ ./internal/benchmark/...

bench-bun:
	go test -bench=Bun -benchmem -run=^$$ ./internal/benchmark/...

# Format code
fmt:
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Tidy dependencies
tidy:
	go mod tidy

# Update dependencies
deps-update:
	go get -u ./...
	go mod tidy

# Run Monaco editor example
monaco:
	@echo "Starting Monaco editor at http://localhost:8080"
	go run ./cmd/monaco

# Run function injection example
functions:
	go run ./cmd/functions

# Statistical benchmark runner (generates markdown)
benchmark-stats:
	go run ./cmd/benchmark -runs=100

# Statistical benchmark runner with output to file
benchmark-stats-md:
	go run ./cmd/benchmark -runs=1000 -output=internal/benchmark/Results.md

# Clean build artifacts
clean:
	rm -f coverage.out coverage.html
	go clean ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  build           - Build the library"
	@echo "  test            - Run all tests"
	@echo "  test-v          - Run tests with verbose output"
	@echo "  test-cover      - Run tests with coverage report"
	@echo "  bench           - Run benchmarks (quick)"
	@echo "  bench-all       - Run benchmarks with multiple iterations"
	@echo "  bench-goja      - Run GOJA benchmarks only"
	@echo "  bench-bun       - Run Bun benchmarks only"
	@echo "  benchmark-stats - Run statistical benchmark (100 runs, markdown output)"
	@echo "  benchmark-stats-md - Run statistical benchmark and save to file"
	@echo "  monaco          - Run Monaco editor example"
	@echo "  functions       - Run function injection example"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  tidy            - Tidy dependencies"
	@echo "  deps-update     - Update dependencies"
	@echo "  clean           - Clean build artifacts"
