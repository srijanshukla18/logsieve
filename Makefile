# LogSieve Makefile

# Variables
BINARY_NAME=logsieve
SERVER_BINARY_NAME=server
BUILD_DIR=./dist
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean test coverage lint fmt deps vendor run-server run-cli docker helm install uninstall

# Default target
all: clean fmt lint test build

# Build binaries
build: build-server build-cli

build-server:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(SERVER_BINARY_NAME) ./cmd/server

build-cli:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/logsieve

# Development builds
build-dev:
	$(GOBUILD) -race -o $(BUILD_DIR)/$(SERVER_BINARY_NAME) ./cmd/server
	$(GOBUILD) -race -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/logsieve

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f $(BINARY_NAME) $(SERVER_BINARY_NAME)

# Run tests
test:
	$(GOTEST) -v ./...

test-race:
	$(GOTEST) -race -v ./...

test-integration:
	$(GOTEST) -v -tags=integration ./test/integration/...

# Coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Linting
lint:
	$(GOLINT) run ./...

# Formatting
fmt:
	$(GOFMT) -s -w .

fmt-check:
	@test -z "$(shell $(GOFMT) -l .)" || (echo "Files need formatting:" && $(GOFMT) -l . && exit 1)

# Dependencies
deps:
	$(GOMOD) download
	$(GOMOD) verify

vendor:
	$(GOMOD) vendor

tidy:
	$(GOMOD) tidy

# Development server
run-server:
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVER_BINARY_NAME) ./cmd/server
	./$(BUILD_DIR)/$(SERVER_BINARY_NAME) --config=config/dev.yaml

run-cli:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/logsieve
	./$(BUILD_DIR)/$(BINARY_NAME) --help

# Docker targets
docker-build:
	docker build -t logsieve/sieve:$(VERSION) -f docker/Dockerfile .

docker-build-distroless:
	docker build -t logsieve/sieve:$(VERSION)-distroless -f docker/Dockerfile.distroless .

docker-run:
	docker run -p 8080:8080 -p 9090:9090 logsieve/sieve:$(VERSION)

# Helm targets
helm-lint:
	helm lint ./helm/logsieve

helm-template:
	helm template logsieve ./helm/logsieve --values ./helm/logsieve/values.yaml

helm-package:
	helm package ./helm/logsieve -d $(BUILD_DIR)

# Performance testing
load-test:
	@echo "Running load test..."
	@command -v wrk > /dev/null || (echo "wrk not found. Install with: brew install wrk" && exit 1)
	wrk -t12 -c400 -d30s --script=test/load/basic.lua http://localhost:8080/ingest

profile-memory:
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVER_BINARY_NAME) ./cmd/server
	./$(BUILD_DIR)/$(SERVER_BINARY_NAME) --cpuprofile=cpu.prof --memprofile=mem.prof &
	sleep 10
	pkill -f $(SERVER_BINARY_NAME)
	$(GOCMD) tool pprof mem.prof

profile-cpu:
	$(GOBUILD) -o $(BUILD_DIR)/$(SERVER_BINARY_NAME) ./cmd/server
	./$(BUILD_DIR)/$(SERVER_BINARY_NAME) --cpuprofile=cpu.prof &
	sleep 10
	pkill -f $(SERVER_BINARY_NAME)
	$(GOCMD) tool pprof cpu.prof

# Install/Uninstall (local)
install:
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY_NAME) ./cmd/logsieve
	$(GOBUILD) $(LDFLAGS) -o $(GOPATH)/bin/$(SERVER_BINARY_NAME) ./cmd/server

uninstall:
	rm -f $(GOPATH)/bin/$(BINARY_NAME)
	rm -f $(GOPATH)/bin/$(SERVER_BINARY_NAME)

# Release preparation
release: clean fmt lint test build docker-build helm-package

# Development workflow
dev: clean fmt lint test build-dev

# CI/CD targets
ci: fmt-check lint test-race coverage

# Help
help:
	@echo "Available targets:"
	@echo "  all          - Run full build pipeline (clean, fmt, lint, test, build)"
	@echo "  build        - Build both server and CLI binaries"
	@echo "  build-server - Build server binary only"
	@echo "  build-cli    - Build CLI binary only"
	@echo "  build-dev    - Build with race detection"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run unit tests"
	@echo "  test-race    - Run tests with race detection"
	@echo "  test-integration - Run integration tests"
	@echo "  coverage     - Generate test coverage report"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  deps         - Download dependencies"
	@echo "  vendor       - Vendor dependencies"
	@echo "  tidy         - Tidy go.mod"
	@echo "  run-server   - Run development server"
	@echo "  run-cli      - Run CLI help"
	@echo "  docker-build - Build Docker image"
	@echo "  helm-lint    - Lint Helm chart"
	@echo "  load-test    - Run load test (requires wrk)"
	@echo "  profile-*    - Profile memory/CPU"
	@echo "  install      - Install binaries to GOPATH/bin"
	@echo "  release      - Full release pipeline"
	@echo "  dev          - Development workflow"
	@echo "  ci           - CI pipeline"