.PHONY: build clean test run help

# Binary name
BINARY=qbrtool

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY) ./cmd/qbrtool

## build-debug: Build with debug symbols
build-debug:
	$(GOBUILD) -o bin/$(BINARY) ./cmd/qbrtool

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf bin/

## test: Run tests
test:
	$(GOTEST) -v -race ./...

## test-coverage: Run tests with coverage
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## deps: Download dependencies
deps:
	$(GOMOD) download

## tidy: Tidy dependencies
tidy:
	$(GOMOD) tidy

## vet: Run go vet
vet:
	$(GOCMD) vet ./...

## lint: Run linters (requires golangci-lint)
lint:
	golangci-lint run

## install: Install the binary to $GOPATH/bin
install:
	$(GOCMD) install ./cmd/qbrtool

## run: Run the binary (use ARGS to pass arguments)
run: build
	./bin/$(BINARY) $(ARGS)

## help: Show this help
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/ /'

# Example usage targets
## export-q3: Export Q3-2024 items (example)
export-q3: build
	./bin/$(BINARY) export --quarter Q3-2024 --include-archived -f q3-2024.json

## analyze-all: Run all analyzers on exported data (example)
analyze-all: build
	./bin/$(BINARY) analyze -i q3-2024.json --analysis all -f analysis.json
