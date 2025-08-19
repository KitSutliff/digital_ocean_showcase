# Package Indexer Server

A high-performance, concurrent TCP server that maintains a package dependency index, built for the DigitalOcean coding challenge.

## Overview

This server implements a stateful dependency graph that enforces strict constraints:
- Packages can only be indexed if all their dependencies are already present
- Packages can only be removed if no other packages depend on them
- All operations are thread-safe and handle 100+ concurrent clients

## Protocol

The server communicates via TCP on port 8080 using a line-oriented protocol:

```
<command>|<package>|<dependencies>\n
```

### Commands

- `INDEX|package|dep1,dep2`: Add/update package with dependencies
- `REMOVE|package|`: Remove package from index  
- `QUERY|package|`: Check if package is indexed

### Responses

- `OK\n`: Operation succeeded
- `FAIL\n`: Operation failed due to business logic
- `ERROR\n`: Malformed request or invalid command

## Quick Start

### Using Docker (Recommended)

```bash
# Build and run
make docker-build
make docker-run

# Or use docker directly
docker build -t package-indexer .
docker run -p 8080:8080 package-indexer
```

### Manual Build

```bash
# Build
make build

# Run
make run

# Run server in quiet mode (for performance testing)
./package-indexer -quiet

# Test basic functionality
echo "INDEX|test|" | nc localhost 8080  # Returns "OK"
echo "QUERY|test|" | nc localhost 8080  # Returns "OK"
```

## Development

### Prerequisites

- Go 1.19+
- Docker (for containerization)
- netcat (for testing)

### Commands

```bash
# Run all tests with race detection
make test

# Run tests with coverage
make test-coverage

# Build binary
make build

# Run server
make run

# Run official test harness
make harness

# Clean artifacts
make clean
```

### Testing

```bash
# Unit tests
go test ./internal/...

# Integration tests
go test ./tests/integration

# Race condition testing
go test -race ./...

# Stress testing
./scripts/stress_test.sh

# Cross-platform harness usage
HARNESS_BIN=./do-package-tree_linux ./scripts/run_harness.sh
```

## Architecture

### Core Components

- **Wire Protocol**: Command parsing and response formatting
- **Indexer**: Thread-safe dependency graph management  
- **Server**: TCP connection handling and request routing

### Data Structures

- **Forward Dependencies**: `map[string]StringSet` - package → dependencies
- **Reverse Dependencies**: `map[string]StringSet` - package → dependents
- **Indexed Packages**: `StringSet` - currently indexed packages

### Concurrency Model

- Goroutine per client connection
- Shared state protected by `sync.RWMutex`
- Read operations (QUERY) use read locks for concurrency
- Write operations (INDEX/REMOVE) use write locks for safety

## Performance

Designed to handle:
- 100+ concurrent clients
- Complex dependency graphs (1000+ packages)
- High-frequency operations with minimal latency

### Benchmarks

```bash
# Run official test harness at maximum concurrency
HARNESS_BIN=./do-package-tree_darwin ./scripts/run_harness.sh -concurrency=100 -seed=42
```

## Production Considerations

- **Security**: Runs as non-root user in Docker
- **Health Checks**: Docker health check via TCP port connectivity probe
- **Monitoring**: Basic logging with connection lifecycle events
- **Resource Usage**: Minimal memory footprint, efficient O(1) operations

## Troubleshooting

### Common Issues

1. **Port in use**: Ensure port 8080 is available
2. **Race conditions**: Run tests with `-race` flag
3. **Memory usage**: Monitor runtime behavior during stress tests

### Debug Mode

Enable verbose logging by removing the `-quiet` flag when starting the server.

## Project Structure

```
package-indexer/
├── cmd/server/              # Main entry point
├── internal/
│   ├── indexer/            # Core dependency graph logic
│   ├── wire/               # Protocol parsing  
│   └── server/             # TCP connection handling
├── tests/integration/       # End-to-end tests
├── scripts/                # Build and test automation
└── [Makefile, Dockerfile, README.md]
```

## License

Created for the DigitalOcean coding challenge. Not intended for production use.
