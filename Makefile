.PHONY: all build test run clean docker-build docker-run harness

# Build the server binary
build:
	go build -o package-indexer ./cmd/server

# Run all tests with race detection
test:
	go test -race ./...

# Run tests with coverage
test-coverage:
	go test -cover ./...

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
