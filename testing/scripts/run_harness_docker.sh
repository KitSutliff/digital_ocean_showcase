#!/bin/bash

set -e

echo "=== Package Indexer Test Harness Runner (Docker Mode) ==="

# Build Docker image
echo "Building Docker image..."
make -C ../.. docker-build

echo "Starting package indexer container..."
CONTAINER_ID=$(docker run -d --rm -p 8080:8080 package-indexer)

# Wait for container to be healthy
echo "Waiting for container health check..."
timeout=30
while [ $timeout -gt 0 ]; do
    if docker ps --filter "id=$CONTAINER_ID" --format "{{.Status}}" | grep -q "healthy"; then
        echo "Container is healthy!"
        break
    fi
    sleep 1
    timeout=$((timeout - 1))
done

if [ $timeout -eq 0 ]; then
    echo "❌ Container failed to become healthy"
    docker stop $CONTAINER_ID
    exit 1
fi

# Function to cleanup on exit
cleanup() {
    echo "Stopping container ($CONTAINER_ID)"
    docker stop $CONTAINER_ID 2>/dev/null || true
}
trap cleanup EXIT

# Auto-detect or use environment variable
HARNESS_BIN=${HARNESS_BIN:-"../harness/do-package-tree_$(uname -s | tr '[:upper:]' '[:lower:]')"}
if [ ! -f "$HARNESS_BIN" ]; then
    echo "Error: Harness binary $HARNESS_BIN not found"
    echo "Set HARNESS_BIN environment variable or ensure binary exists"
    exit 1
fi

echo "Running test harness against Docker container: $HARNESS_BIN"
$HARNESS_BIN "$@"

echo "✅ Docker test harness completed successfully!"
