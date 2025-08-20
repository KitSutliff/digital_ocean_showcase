#!/bin/bash

set -e

echo "=== Package Indexer Stress Test ==="

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
HARNESS_BIN=${HARNESS_BIN:-"../harness/do-package-tree_$(uname -s | tr '[:upper:]' '[:lower:]')"}
if [ ! -f "$HARNESS_BIN" ]; then
    echo "Error: Harness binary $HARNESS_BIN not found"
    echo "Set HARNESS_BIN environment variable or ensure binary exists"
    exit 1
fi

echo "Running stress tests..."

# Test with increasing concurrency levels
for concurrency in 1 10 25 50 100; do
    echo "🧪 Testing with concurrency level: $concurrency"
    
    # Test with multiple random seeds for robustness
    for seed in 42 12345 98765; do
        echo "   Seed: $seed"
        $HARNESS_BIN -concurrency=$concurrency -seed=$seed
        if [ $? -ne 0 ]; then
            echo "❌ FAILED: concurrency=$concurrency, seed=$seed"
            exit 1
        fi
    done
    echo "   ✅ All seeds passed for concurrency $concurrency"
done

echo "🎉 All stress tests passed!"
