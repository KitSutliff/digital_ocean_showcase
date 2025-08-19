#!/bin/bash

set -e

echo "🚀 Final Verification Script for Package Indexer"
echo "================================================="

# Clean previous builds
echo "🧹 Cleaning previous builds..."
make clean

# Run all tests with race detection
echo "🧪 Running unit tests with race detection..."
go test -race ./internal/...

echo "🧪 Running integration tests..."
go test -race ./tests/integration/...

echo "📊 Running tests with coverage..."
go test -cover ./...

# Build the binary
echo "🔨 Building server binary..."
make build

# Test basic functionality
echo "🔌 Testing basic connectivity..."
./package-indexer -quiet &
SERVER_PID=$!
sleep 2

# Basic functional test
echo "INDEX|test|" | nc localhost 8080 | grep -q "OK" || (echo "❌ Basic functionality test failed"; kill $SERVER_PID; exit 1)
echo "QUERY|test|" | nc localhost 8080 | grep -q "OK" || (echo "❌ Query test failed"; kill $SERVER_PID; exit 1)
echo "REMOVE|test|" | nc localhost 8080 | grep -q "OK" || (echo "❌ Remove test failed"; kill $SERVER_PID; exit 1)

# Stop test server
kill $SERVER_PID
sleep 1

echo "✅ Basic functionality verified!"

# Run official test harness
echo "🎯 Running official test harness..."
./scripts/run_harness.sh

# Run stress tests
echo "💪 Running stress tests..."
./scripts/stress_test.sh

echo "✅ All verification tests passed!"
echo "📦 Project is ready for submission!"

# Generate project statistics
echo ""
echo "📈 Project Statistics:"
echo "====================="
echo "Go files: $(find . -name '*.go' | wc -l)"
echo "Total lines of code: $(find . -name '*.go' -exec wc -l {} + | tail -1 | awk '{print $1}')"
echo "Test files: $(find . -name '*_test.go' | wc -l)"
echo "Test coverage: $(go test -cover ./... 2>/dev/null | grep -E 'coverage: [0-9.]+%' | tail -1 | awk '{print $2}')"
