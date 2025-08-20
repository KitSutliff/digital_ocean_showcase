.PHONY: all build test run clean docker-build docker-run harness

# Build the server binary
build:
	go build -o package-indexer ./app/cmd/server

# Run all tests with race detection (excludes test-suite)
test:
	go test -race -covermode=atomic -coverprofile=coverage.out ./internal/... ./app/cmd/... ./testing/integration/...

# Run tests with coverage (excludes test-suite)
test-coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./internal/... ./app/cmd/... ./testing/integration/...
	go tool cover -func=coverage.out | tee coverage.txt
	go tool cover -html=coverage.out -o coverage.html

# Run all tests including test-suite (when explicitly needed)
test-all:
	go test -race ./internal/... ./app/cmd/... ./testing/integration/... ./testing/suite/...

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

# Run test harness (local development)
harness:
	cd testing/scripts && ./run_harness.sh

# Run test harness against Docker container (production validation)
harness-docker:
	cd testing/scripts && ./run_harness_docker.sh

# Development helpers
fmt:
	go fmt ./...

deps:
	go mod tidy
