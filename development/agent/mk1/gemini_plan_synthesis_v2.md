# V2 Synthesized Project Guide: Concurrent Package Indexer

## 1. Objective

This document provides a comprehensive, step-by-step guide for implementing the DigitalOcean Package Indexer. It is a V2 synthesis derived from the analysis of three separate V1 synthesis plans, combining the optimal strategies for project structure, core logic, testing, and production readiness into a single, actionable roadmap.

The goal is to produce a production-quality, concurrent TCP server in Go that passes the provided test harness under high load. This guide is designed to be followed sequentially.

---

## Phase 0: Project Bootstrap

**Goal:** Establish a clean, professional project structure and initialize all necessary tooling.

**Tasks:**

1.  **Initialize Git Repository:**
    ```bash
    git init
    ```

2.  **Configure Anonymous Commits:** **This must be done before the first commit.** Create/edit the `.git/config` file to include:
    ```ini
    [user]
        name = "Anonymous"
        email = "anonymous@example.com"
    ```

3.  **Create Project Structure:** Based on the superior, more modular structure proposed in the V1 analyses, create the following directories:
    ```
    .
    ├── cmd/
    │   └── server/
    ├── internal/
    │   ├── index/
    │   ├── server/
    │   └── wire/
    ├── scripts/
    └── tests/
        └── integration/
    ```

4.  **Initialize Go Module:**
    ```bash
    go mod init package-indexer
    ```

5.  **Create `.gitignore`:** Add a `.gitignore` file with standard Go and OS exclusions, including the compiled binary:
    ```gitignore
    # Binaries
    server
    
    # OS-specific
    .DS_Store
    *.exe
    *.out
    ```

6.  **Create Initial `Makefile`:** Create a `Makefile` with placeholder targets. This will be expanded later.
    ```makefile
    .PHONY: all build run test clean

    build:
        @echo "Building..."
        go build -o server ./cmd/server

    run: build
        @echo "Running server..."
        ./server

    test:
        @echo "Running tests..."
        go test ./... -race
    ```

---

## Phase 1: Core Domain Logic & Protocol

**Goal:** Implement the "brains" of the application and the protocol parsing logic. These components should have no knowledge of networking.

**Tasks:**

1.  **Define Wire Protocol (`internal/wire/protocol.go`):**
    -   Define the `Request` struct and `Command` type.
    -   Implement `Parse(message string) (*Request, error)`. This function takes a raw string and returns a structured request or an error.
    -   **Unit Test (`internal/wire/protocol_test.go`):** Write thorough tests for the parser, covering:
        -   Valid `INDEX`, `REMOVE`, `QUERY` commands.
        -   Commands with and without dependencies.
        -   Malformed messages (too few/many pipes, unknown commands).
        -   Edge cases like empty package names or dependencies.

2.  **Implement Thread-Safe Index (`internal/index/index.go`):**
    -   Define the `Index` struct containing the `sync.RWMutex`, `dependencies` map, and `dependents` map.
    -   Implement the public methods:
        -   `Index(pkg string, deps []string) error`
        -   `Remove(pkg string) error`
        -   `Query(pkg string) bool`
    -   **Unit Test (`internal/index/index_test.go`):**
        -   Test all success and failure paths for each method.
        -   Crucially, write a concurrency test that spawns many goroutines to call the methods concurrently.
        -   Run all tests with the `-race` flag to detect race conditions (`go test -race ./...`).

---

## Phase 2: TCP Server Implementation

**Goal:** Expose the core logic over a TCP socket, handling concurrent client connections gracefully.

**Tasks:**

1.  **Implement the Server (`internal/server/server.go`):**
    -   Create a `Server` struct that holds an instance of the `*index.Index`.
    -   Implement a `Start()` method that begins listening for TCP connections on port `8080`.
    -   The `Start` method should contain the main `for` loop that accepts new connections. For each accepted `net.Conn`, it must spawn a new goroutine running a `handleConnection` method.

2.  **Implement Connection Handling:**
    -   The `handleConnection(conn net.Conn)` method is the core of the server. It must:
        -   Ensure the connection is closed using `defer conn.Close()`.
        -   Create a `bufio.NewReader(conn)` to read data.
        -   Loop indefinitely, reading messages until a newline character using `reader.ReadString('\n')`. This is more robust than `Scanner`.
        -   Pass the received message to the `wire.Parse` function.
        -   If parsing fails, write `ERROR\n` back to the client.
        -   If parsing succeeds, call the appropriate method on the `index.Index`.
        -   Translate the result from the index into the correct `OK\n` or `FAIL\n` response and write it to the client.

3.  **Create the Main Entrypoint (`cmd/server/main.go`):**
    -   This file should be minimal.
    -   Instantiate the `index.New()` and `server.New(indexer)`.
    -   Call `server.Start()` and log any fatal errors.

---

## Phase 3: Integration and Harness Validation

**Goal:** Verify that all components work together correctly and pass the official test suite under stress.

**Tasks:**

1.  **Write Integration Tests (`tests/integration/main_test.go`):**
    -   Create a test that starts a real server on a random, available port.
    -   In the test, connect a TCP client to the server.
    -   Send a sequence of valid and invalid commands and assert that the responses are correct. This verifies the entire stack is working.

2.  **Create Harness Script (`scripts/run_harness.sh`):**
    -   This script simplifies testing.
    -   It should first build the server, then run it in the background, and finally execute the platform-specific test harness.
    -   Make the script executable: `chmod +x scripts/run_harness.sh`.
    ```bash
    #!/bin/bash
    echo "Building server..."
    go build -o server ./cmd/server

    echo "Starting server in background..."
    ./server &
    SERVER_PID=$!

    # Allow server to start
    sleep 1

    echo "Running test harness..."
    ./do-package-tree_darwin

    echo "Killing server..."
    kill $SERVER_PID
    ```

3.  **Pass the Harness:**
    -   Run the script and debug any issues until the harness passes with default settings.
    -   **Key Acceptance Criteria:** Run the harness with high concurrency and multiple random seeds to prove robustness. The final implementation **must** pass this command consistently:
    ```bash
    ./do-package-tree_darwin -c=100
    ```

---

## Phase 4: Production Readiness & Finalization

**Goal:** Package the application with a `Dockerfile` and `README`, making it easy for others to build and run.

**Tasks:**

1.  **Write a Multi-Stage `Dockerfile`:**
    ```dockerfile
    # ---- Builder Stage ----
    FROM golang:1.19-alpine AS builder
    WORKDIR /app
    COPY go.mod ./
    COPY . .
    RUN go build -o /server ./cmd/server

    # ---- Final Stage ----
    FROM ubuntu:latest
    COPY --from=builder /server /server
    EXPOSE 8080
    CMD ["/server"]
    ```

2.  **Finalize `Makefile`:** Add targets for Docker commands.
    ```makefile
    # ... existing targets ...

    docker-build:
        docker build -t package-indexer .

    docker-run:
        docker run -p 8080:8080 package-indexer
    ```

3.  **Write `README.md`:**
    -   Provide a brief overview of the project and its architecture.
    -   Include clear, simple instructions on how to:
        -   Build the binary (`make build`).
        -   Run the server (`make run`).
        -   Run the tests (`make test`).
        -   Build and run with Docker (`make docker-build`, `make docker-run`).

4.  **Final Review:**
    -   Perform a final pass over the code to ensure it is clean, commented where necessary, and free of any personally identifiable information.
    -   Confirm all deliverables are present.

---

## Appendix: Definition of Done Checklist

-   [ ] All harness tests pass with `--concurrency=100` across multiple random seeds.
-   [ ] Local test suite (`unit` and `integration`) passes with `go test ./... -race`.
-   [ ] The Docker image builds and runs successfully on a standard Docker host.
-   [ ] The `README.md` and `Makefile` provide a clear, reproducible path for building and running the project and its tests.
-   [ ] No non-standard library dependencies are included in the production binary.
-   [ ] Git history is anonymized.
