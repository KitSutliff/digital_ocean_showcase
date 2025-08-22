#!/bin/bash

set -e

echo "=== Package Indexer Test Harness Runner ==="

# Source common server utilities (build, readiness, cleanup, harness detection)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common_server_utils.sh"

# Start server and setup cleanup + harness detection
setup_test_environment

echo "Running test harness: $HARNESS_BIN"
$HARNESS_BIN "$@"

echo "SUCCESS: Test harness completed successfully!"
