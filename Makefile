.PHONY: all build test run clean docker-build docker-run harness

# Build the server binary
build:
	go build -o package-indexer ./cmd/server

# Run all tests with race detection (excludes test-suite)
test:
	go test -race -covermode=atomic -coverprofile=coverage.out ./internal/... ./cmd/... ./tests/...

# Run tests with coverage (excludes test-suite)
test-coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./internal/... ./cmd/... ./tests/...
	go tool cover -func=coverage.out | tee coverage.txt
	go tool cover -html=coverage.out -o coverage.html

# Run all tests including test-suite (when explicitly needed)
test-all:
	go test -race ./internal/... ./cmd/... ./tests/... ./test-suite/...

# Run the server
run: build
	./package-indexer

# Clean build artifacts
clean:
	rm -f package-indexer
	go clean ./...

# Build Docker image
docker-build:
	docker build -t package-indexer .

# Run in Docker
docker-run: docker-build
	docker run -p 8080:8080 package-indexer

# Run test harness
harness:
	./scripts/run_harness.sh

# Development helpers
fmt:
	go fmt ./...

deps:
	go mod tidy
