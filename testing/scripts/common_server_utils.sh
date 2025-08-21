#!/bin/bash

# Common server utility functions for test scripts
# Provides DRY server lifecycle management across multiple test scripts

# Start the package indexer server for testing
# Usage: start_test_server
start_test_server() {
    echo "Building server..."
    make -C ../.. build

    echo "Starting package indexer server..."
    ../../package-indexer -quiet &
    SERVER_PID=$!

    # Wait for server to start
    sleep 2

    # Export PID for cleanup functions
    export TEST_SERVER_PID=$SERVER_PID
}

# Setup cleanup trap for graceful server shutdown
# Usage: setup_server_cleanup
setup_server_cleanup() {
    cleanup() {
        if [ -n "$TEST_SERVER_PID" ]; then
            echo "Stopping server (PID: $TEST_SERVER_PID)"
            kill $TEST_SERVER_PID 2>/dev/null || true
            wait $TEST_SERVER_PID 2>/dev/null || true
        fi
    }
    trap cleanup EXIT
}

# Auto-detect platform-specific test harness binary
# Usage: get_harness_binary
# Returns: HARNESS_BIN variable set to appropriate binary path
get_harness_binary() {
    local script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    HARNESS_BIN="${HARNESS_BIN:-$script_dir/../harness/do-package-tree_$(uname -s | tr '[:upper:]' '[:lower:]')}"
    
    if [[ ! -f "$HARNESS_BIN" ]]; then
        echo "❌ Test harness binary not found: $HARNESS_BIN"
        echo "   Set HARNESS_BIN environment variable to point to correct binary"
        exit 1
    fi
}

# Complete server setup for test scripts
# Usage: setup_test_environment
setup_test_environment() {
    get_harness_binary
    setup_server_cleanup
    start_test_server
}
