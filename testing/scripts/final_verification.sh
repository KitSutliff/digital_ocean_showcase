#!/bin/bash

set -e

echo "VERIFICATION: Final Verification Script for Package Indexer"
echo "================================================="

# Clean previous builds
echo "CLEAN: Cleaning previous builds..."
make -C ../.. clean

# Run all tests with race detection
echo "TESTING: Running unit tests with race detection..."
pushd ../.. && go test -race ./internal/... && popd

echo "TESTING: Running integration tests..."
pushd ../.. && go test -race ./testing/integration/... && popd

echo "COVERAGE: Running tests with coverage..."
pushd ../.. && go test -cover ./... && popd

# Build the binary
echo "BUILD: Building server binary..."
make -C ../.. build

# Test basic functionality
echo "CONNECTIVITY: Testing basic connectivity..."
../../package-indexer -quiet &
SERVER_PID=$!
# Wait for readiness
timeout=30
while ! nc -z 127.0.0.1 8080 >/dev/null 2>&1; do
    sleep 1
    timeout=$((timeout - 1))
    if [ $timeout -le 0 ]; then
        echo "ERROR: Server did not become ready in time"
        kill $SERVER_PID 2>/dev/null || true
        exit 1
    fi
done

# Basic functional test
echo "INDEX|test|" | nc localhost 8080 | grep -q "OK" || (echo "ERROR: Basic functionality test failed"; kill $SERVER_PID; exit 1)
echo "QUERY|test|" | nc localhost 8080 | grep -q "OK" || (echo "ERROR: Query test failed"; kill $SERVER_PID; exit 1)
echo "REMOVE|test|" | nc localhost 8080 | grep -q "OK" || (echo "ERROR: Remove test failed"; kill $SERVER_PID; exit 1)

# Stop test server
kill $SERVER_PID
sleep 1

echo "SUCCESS: Basic functionality verified!"

# Run official test harness
echo "HARNESS: Running official test harness..."
./run_harness.sh

# Run stress tests
echo "STRESS: Running stress tests..."
./stress_test.sh

echo "SUCCESS: All verification tests passed!"
echo "READY: Project is ready for submission!"

# Generate project statistics
echo ""
echo "STATS: Project Statistics:"
echo "====================="
pushd ../.. >/dev/null
echo "Go files: $(find . -name '*.go' | wc -l)"
echo "Total lines of code: $(find . -name '*.go' -exec wc -l {} + | awk '{s+=$1} END {print s}')"
echo "Test files: $(find . -name '*_test.go' | wc -l)"
echo "Test coverage: $(go test -cover ./... 2>/dev/null | grep -E 'coverage: [0-9.]+%' | tail -1 | awk '{print $2}')"
popd >/dev/null
