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

# Run server on specific address
./package-indexer -addr :8080 -quiet

# Run with admin server for observability (optional)
./package-indexer -admin :9090

# Run with custom timeout configurations
./package-indexer -read-timeout 60s -shutdown-timeout 45s

# Test basic functionality
echo "INDEX|test|" | nc localhost 8080  # Returns "OK"
echo "QUERY|test|" | nc localhost 8080  # Returns "OK"
```

## Admin Server (Optional)

The package indexer includes an optional HTTP admin server for production observability:

```bash
# Enable admin server on port 9090
./package-indexer -admin :9090

# Access endpoints
curl http://localhost:9090/healthz    # Health check (readiness/liveness) 
curl http://localhost:9090/metrics   # Runtime metrics (Prometheus format)
curl http://localhost:9090/buildinfo # Build version and Go info (JSON)
curl http://localhost:9090/debug/pprof/ # pprof debugging endpoints
```

### Admin Endpoints

- **`/healthz`** - Health check with actual readiness status and proper HTTP codes
- **`/metrics`** - Prometheus-format metrics (connections, commands, errors, packages, uptime)  
- **`/buildinfo`** - Build information (Go version, module path, settings)
- **`/debug/pprof/`** - Standard Go pprof endpoints for performance analysis

**Key Features:**
- **Structured Logging**: JSON-formatted logs with contextual fields for production analysis
- **Prometheus Integration**: Industry-standard metrics format for monitoring tools
- **Proper Health Checks**: Readiness probes that reflect actual server state

**Note:** Admin server is disabled by default and has zero impact on the main TCP protocol or test harness compatibility.

## Development

### Prerequisites

- Go 1.22+
- Docker (for containerization)
- netcat (for testing)

### Commands

```bash
# Run all tests with race detection
make test

# Run tests with coverage (generates coverage.out, coverage.html)
make test-coverage

# Build binary
make build

# Run server
make run

# Run official test harness (local development)
make harness

# Run test harness against Docker container (production validation)
make harness-docker

# Clean artifacts
make clean

# Development helpers
make fmt          # Format code
make deps         # Tidy dependencies
make test-all     # Run all tests including test-suite
```

### Configuration Options

The server supports several command-line flags for production tuning:

```bash
# Basic configuration
./package-indexer -addr :8080 -quiet

# Timeout configuration (important for production environments)
./package-indexer -read-timeout 60s -shutdown-timeout 45s

# Full production configuration with observability
./package-indexer -addr :8080 -admin :9090 -read-timeout 60s -shutdown-timeout 45s -quiet
```

**Configuration Flags:**
- `-addr`: Server listen address (default `:8080`)
- `-admin`: Admin HTTP server address for observability (disabled if empty)
- `-quiet`: Disable logging for performance testing
- `-read-timeout`: Connection read timeout to prevent slowloris attacks (default `30s`)
- `-shutdown-timeout`: Graceful shutdown timeout (default `30s`)

### Testing

The project supports comprehensive testing at multiple levels:

#### Development Testing (Local)
```bash
# Unit tests
go test ./internal/...

# Integration tests  
go test ./testing/integration

# Race condition testing
go test -race ./...

# Official test harness (local binary)
make harness

# Stress testing with multiple concurrency levels
cd testing/scripts && ./stress_test.sh

# Complete verification (unit tests + integration + harness + stress)
cd testing/scripts && ./final_verification.sh
```

#### Production Testing (Docker)
```bash
# Test against containerized environment
make harness-docker

# Build and test Docker image
make docker-build && make harness-docker

# Cross-platform harness usage (Docker)
cd testing/scripts && HARNESS_BIN=../harness/do-package-tree_linux ./run_harness_docker.sh
```

**Local vs Docker Testing:**
- **Local testing** (`make harness`): Fast iteration, debugging, development workflow
- **Docker testing** (`make harness-docker`): Validates production deployment, health checks, containerization

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
# Run official test harness at maximum concurrency (local)
cd testing/scripts && HARNESS_BIN=../harness/do-package-tree_darwin ./run_harness.sh -concurrency=100 -seed=42

# Run official test harness at maximum concurrency (Docker - production environment)
cd testing/scripts && HARNESS_BIN=../harness/do-package-tree_darwin ./run_harness_docker.sh -concurrency=100 -seed=42

# Run comprehensive stress test (1, 10, 25, 50, 100 concurrent clients with multiple seeds)
cd testing/scripts && ./stress_test.sh

# Chaos engineering - test fault tolerance with malformed messages and random disconnects
cd testing/scripts && ./chaos_test.sh
```

## Production Considerations

- **Security**: Runs as non-root user in Docker
- **Health Checks**: Docker health check via netcat TCP probe (`nc -z localhost 8080`)
- **Testing**: Dual testing approach validates both development and production environments
- **Containerization**: Multi-stage builds with pinned Ubuntu base (ubuntu:24.04)
- **Graceful Shutdown**: Handles SIGTERM/SIGINT signals, closes connections cleanly
- **Monitoring**: Structured JSON logging with connection IDs, client addresses, and contextual fields
- **Resource Usage**: Minimal memory footprint, efficient O(1) operations

## Troubleshooting

### Common Issues

1. **Port in use**: Ensure port 8080 is available
2. **Race conditions**: Run tests with `-race` flag
3. **Memory usage**: Monitor runtime behavior during stress tests

### Debug Mode

Enable structured JSON logging by removing the `-quiet` flag when starting the server. Logs include contextual fields like connection IDs and client addresses for enhanced debugging.

## Project Structure

```
digital_ocean_showcase/
├── app/                     # Core Application
│   └── cmd/server/         # Main entry point
├── internal/               # Core application logic
│   ├── indexer/           # Core dependency graph logic
│   ├── server/            # TCP connection handling
│   └── wire/              # Protocol parsing
├── testing/               # Testing Infrastructure
│   ├── harness/          # Test harness binaries
│   ├── integration/      # End-to-end tests
│   ├── scripts/          # Test automation scripts
│   └── suite/            # Test framework
├── development/           # Development Artifacts
│   ├── agent/            # AI agent planning documents
│   └── operator/         # Project management documents
├── challenge/            # Original Challenge Materials
│   ├── INSTRUCTIONS.md   # Challenge requirements
│   └── source.tar.gz     # Original challenge files
└── [Makefile, Dockerfile, README.md, go.mod]
```

### Directory Guide

- **`app/`**: Core application container with main entry point
- **`internal/`**: Shared internal packages (indexer, server, wire protocol)
- **`testing/`**: All testing-related code and tools
  - `harness/`: Official test harness binaries for all platforms
  - `integration/`: End-to-end integration tests
  - `scripts/`: Test automation and verification scripts
  - `suite/`: Additional test framework components
- **`development/`**: Development artifacts and planning documents
  - `agent/`: AI agent planning documents and implementation proposals  
  - `operator/`: Project management and design decision logs
- **`challenge/`**: Original DigitalOcean challenge materials for reference

## License

Created for the DigitalOcean coding challenge. Not intended for production use.
