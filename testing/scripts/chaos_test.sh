#!/bin/bash

# Chaos Engineering Test Script
# Tests fault tolerance with intentional broken messages and random disconnects

set -e

# Source common server utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common_server_utils.sh"

echo "CHAOS: Chaos Engineering Test Suite"
echo "================================"
echo "Testing fault tolerance and error handling"

# Setup test environment (server, cleanup, harness detection)
setup_test_environment

echo "Running chaos engineering tests..."
echo ""

# Test 1: Moderate chaos (10% failure rate)
echo "🔥 Test 1: Moderate Chaos (10% failure rate, 20 concurrent clients)"
$HARNESS_BIN -concurrency=20 -unluckiness=10 -debug -seed=42
echo "PASS: Moderate chaos test passed!"
echo ""

# Test 2: High chaos (15% failure rate) 
echo "🔥 Test 2: High Chaos (15% failure rate, 25 concurrent clients)"
$HARNESS_BIN -concurrency=25 -unluckiness=15 -debug -seed=99
echo "PASS: High chaos test passed!"
echo ""

# Test 3: Extreme chaos (20% failure rate)
echo "🔥 Test 3: Extreme Chaos (20% failure rate, 30 concurrent clients)"
$HARNESS_BIN -concurrency=30 -unluckiness=20 -debug -seed=123
echo "PASS: Extreme chaos test passed!"
echo ""

echo "SUCCESS: All chaos engineering tests completed successfully!"
echo ""
echo "SUMMARY: What was tested:"
echo "   PASS: Malformed protocol messages (invalid separators, special characters)"
echo "   PASS: Unknown commands (LIZARD, BLINDEX, REMOVES, etc.)"
echo "   PASS: Truncated messages and connection drops"
echo "   PASS: Concurrent chaos with up to 30 clients"
echo "   PASS: Server stability under 20% failure injection"
echo ""
echo "EXCELLENT: Your server demonstrates production-grade fault tolerance!"
