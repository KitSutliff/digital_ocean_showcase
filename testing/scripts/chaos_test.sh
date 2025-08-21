#!/bin/bash

# Chaos Engineering Test Script
# Tests fault tolerance with intentional broken messages and random disconnects

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🧪 Chaos Engineering Test Suite"
echo "================================"
echo "Testing fault tolerance and error handling"

# Build the server
echo "Building server..."
make -C ../.. build

echo "Starting package indexer server..."
../../package-indexer -quiet &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Function to cleanup on exit
cleanup() {
    echo "Stopping server (PID: $SERVER_PID)"
    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true
}
trap cleanup EXIT

# Auto-detect or use environment variable
HARNESS_BIN="${HARNESS_BIN:-$SCRIPT_DIR/../harness/do-package-tree_$(uname -s | tr '[:upper:]' '[:lower:]')}"

if [[ ! -f "$HARNESS_BIN" ]]; then
    echo "❌ Test harness binary not found: $HARNESS_BIN"
    echo "   Set HARNESS_BIN environment variable to point to correct binary"
    exit 1
fi

echo "Running chaos engineering tests..."
echo ""

# Test 1: Moderate chaos (10% failure rate)
echo "🔥 Test 1: Moderate Chaos (10% failure rate, 20 concurrent clients)"
$HARNESS_BIN -concurrency=20 -unluckiness=10 -debug -seed=42
echo "✅ Moderate chaos test passed!"
echo ""

# Test 2: High chaos (15% failure rate) 
echo "🔥 Test 2: High Chaos (15% failure rate, 25 concurrent clients)"
$HARNESS_BIN -concurrency=25 -unluckiness=15 -debug -seed=99
echo "✅ High chaos test passed!"
echo ""

# Test 3: Extreme chaos (20% failure rate)
echo "🔥 Test 3: Extreme Chaos (20% failure rate, 30 concurrent clients)"
$HARNESS_BIN -concurrency=30 -unluckiness=20 -debug -seed=123
echo "✅ Extreme chaos test passed!"
echo ""

echo "🎉 All chaos engineering tests completed successfully!"
echo ""
echo "📋 What was tested:"
echo "   ✅ Malformed protocol messages (invalid separators, special characters)"
echo "   ✅ Unknown commands (LIZARD, BLINDEX, REMOVES, etc.)"
echo "   ✅ Truncated messages and connection drops"
echo "   ✅ Concurrent chaos with up to 30 clients"
echo "   ✅ Server stability under 20% failure injection"
echo ""
echo "💪 Your server demonstrates production-grade fault tolerance!"
