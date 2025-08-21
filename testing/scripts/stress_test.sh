#!/bin/bash

set -e

# Source common server utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common_server_utils.sh"

echo "=== Package Indexer Stress Test ==="

# Setup test environment (server, cleanup, harness detection)
setup_test_environment

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
