# V3 Final Synthesis: A Step-by-Step Implementation Guide

## 1. Objective

This document is the definitive V3 synthesis for implementing the DigitalOcean Package Indexer. It starts with the comprehensive and code-specific V2 plan from Claude and refines it with the clear, phased structure from the Gemini V2 plan. The result is a single, actionable guide that is both **easy to follow** and will produce the **best possible implementation**.

Follow these steps sequentially to build a production-quality, concurrent TCP server in Go that will pass all requirements of the challenge.

---

## Phase 0: Project Bootstrap

**Goal:** Establish a clean, professional project structure and initialize all necessary tooling.

**Tasks:**

1.  **Create Project Directories:**
    ```bash
    mkdir -p package-indexer/cmd/server
    mkdir -p package-indexer/internal/indexer
    mkdir -p package-indexer/internal/wire
    mkdir -p package-indexer/internal/server
    mkdir -p package-indexer/tests/integration
    mkdir -p package-indexer/scripts
    cd package-indexer
    ```

2.  **Initialize Go Module:**
    ```bash
    go mod init package-indexer
    ```

3.  **Initialize Git and Configure for Anonymity:**
    ```bash
    git init
    git config user.name "Anonymous"
    git config user.email "anonymous@example.com"
    ```

4.  **Create `.gitignore`:**
    ```bash
    cat > .gitignore << 'EOF'
    # Binaries
    server
    package-indexer
    *.exe

    # Test artifacts
    *.test
    *.out

    # OS-specific
    .DS_Store
    EOF
    ```

---

## Phase 1: Core Logic and Protocol Implementation

**Goal:** Build the logical core of the application. This phase involves no networking; it is focused entirely on data structures, algorithms, and pure business logic.

**Tasks:**

1.  **Implement the Indexer (`internal/indexer/indexer.go`):**
    *   This file contains the thread-safe dependency graph. Copy the complete, production-ready code below.

    ```go
    package indexer

    import (
    	"sync"
    )

    // Indexer manages the package dependency graph with thread-safe operations.
    type Indexer struct {
    	mu           sync.RWMutex
    	dependencies map[string]map[string]struct{} // Package -> its dependencies
    	dependents   map[string]map[string]struct{} // Package -> packages that depend on it
    }

    // New creates a new, empty package indexer.
    func New() *Indexer {
    	return &Indexer{
    		dependencies: make(map[string]map[string]struct{}),
    		dependents:   make(map[string]map[string]struct{}),
    	}
    }

    // Index adds or updates a package with its dependencies.
    // It returns true if the operation is successful, false otherwise.
    func (idx *Indexer) Index(pkg string, deps []string) bool {
    	idx.mu.Lock()
    	defer idx.mu.Unlock()

    	for _, dep := range deps {
    		if _, ok := idx.dependencies[dep]; !ok {
    			return false // Dependency not found
    		}
    	}

    	// Remove old dependencies' reverse links
    	if oldDeps, ok := idx.dependencies[pkg]; ok {
    		for dep := range oldDeps {
    			delete(idx.dependents[dep], pkg)
    		}
    	}

    	// Add new dependencies and reverse links
    	newDeps := make(map[string]struct{})
    	for _, dep := range deps {
    		newDeps[dep] = struct{}{}
    		if _, ok := idx.dependents[dep]; !ok {
    			idx.dependents[dep] = make(map[string]struct{})
    		}
    		idx.dependents[dep][pkg] = struct{}{}
    	}
    	idx.dependencies[pkg] = newDeps

    	return true
    }

    // Remove deletes a package from the index.
    // It returns true if the operation is successful, false otherwise.
    func (idx *Indexer) Remove(pkg string) bool {
    	idx.mu.Lock()
    	defer idx.mu.Unlock()

    	if _, ok := idx.dependencies[pkg]; !ok {
    		return true // Package doesn't exist, which is a success for removal
    	}

    	if len(idx.dependents[pkg]) > 0 {
    		return false // Package has dependents
    	}

    	// Remove reverse links from its dependencies
    	for dep := range idx.dependencies[pkg] {
    		delete(idx.dependents[dep], pkg)
    	}

    	delete(idx.dependencies, pkg)
    	delete(idx.dependents, pkg)

    	return true
    }

    // Query checks if a package exists in the index.
    func (idx *Indexer) Query(pkg string) bool {
    	idx.mu.RLock()
    	defer idx.mu.RUnlock()
    	_, ok := idx.dependencies[pkg]
    	return ok
    }
    ```

2.  **Implement the Wire Protocol Parser (`internal/wire/protocol.go`):**
    *   This file handles parsing raw client messages. Copy the code below.

    ```go
    package wire

    import (
    	"errors"
    	"strings"
    )

    // Parse extracts the command, package, and dependencies from a raw message.
    func Parse(msg string) (cmd, pkg string, deps []string, err error) {
    	msg = strings.TrimSpace(msg)
    	parts := strings.Split(msg, "|")

    	if len(parts) < 2 || len(parts) > 3 {
    		return "", "", nil, errors.New("invalid message format")
    	}

    	cmd = parts[0]
    	pkg = parts[1]

    	if cmd != "INDEX" && cmd != "REMOVE" && cmd != "QUERY" {
    		return "", "", nil, errors.New("invalid command")
    	}
    	
    	if pkg == "" {
    	    return "", "", nil, errors.New("package name cannot be empty")
    	}

    	if len(parts) == 3 && parts[2] != "" {
    		deps = strings.Split(parts[2], ",")
    	}

    	return cmd, pkg, deps, nil
    }
    ```

3.  **Write Unit Tests for Core Logic:**
    *   Create `internal/indexer/indexer_test.go` and `internal/wire/protocol_test.go`.
    *   Add comprehensive tests covering all success, failure, and edge cases. **Crucially, include a concurrency test for the indexer that uses multiple goroutines.**

4.  **Verify Phase 1:**
    ```bash
    # Run tests with the -race flag to detect concurrency issues
    go test -race ./internal/...
    ```
    *   **Acceptance Criteria:** All tests must pass, especially the race detection.

---

## Phase 2: TCP Server Implementation

**Goal:** Expose the core logic over a TCP socket and handle client connections.

**Tasks:**

1.  **Implement the Server (`internal/server/server.go`):**
    *   This file contains the TCP listener and connection handling logic.

    ```go
    package server

    import (
    	"bufio"
    	"io"
    	"log"
    	"net"
    	"package-indexer/internal/indexer"
    	"package-indexer/internal/wire"
    )

    // Server manages the TCP listener and handles connections.
    type Server struct {
    	addr  string
    	index *indexer.Indexer
    }

    // New creates a new Server.
    func New(addr string) *Server {
    	return &Server{
    		addr:  addr,
    		index: indexer.New(),
    	}
    }

    // Start listens for and handles incoming TCP connections.
    func (s *Server) Start() error {
    	listener, err := net.Listen("tcp", s.addr)
    	if err != nil {
    		return err
    	}
    	defer listener.Close()
    	log.Printf("Server listening on %s", s.addr)

    	for {
    		conn, err := listener.Accept()
    		if err != nil {
    			log.Printf("Error accepting connection: %v", err)
    			continue
    		}
    		go s.handleConnection(conn)
    	}
    }

    func (s *Server) handleConnection(conn net.Conn) {
    	defer conn.Close()
    	reader := bufio.NewReader(conn)

    	for {
    		msg, err := reader.ReadString('\n')
    		if err != nil {
    			if err != io.EOF {
    				log.Printf("Error reading from connection: %v", err)
    			}
    			return
    		}

    		cmd, pkg, deps, err := wire.Parse(msg)
    		if err != nil {
    			conn.Write([]byte("ERROR\n"))
    			continue
    		}

    		var ok bool
    		switch cmd {
    		case "INDEX":
    			ok = s.index.Index(pkg, deps)
    		case "REMOVE":
    			ok = s.index.Remove(pkg)
    		case "QUERY":
    			ok = s.index.Query(pkg)
    		}

    		if ok {
    			conn.Write([]byte("OK\n"))
    		} else {
    			conn.Write([]byte("FAIL\n"))
    		}
    	}
    }
    ```

2.  **Create the Main Entrypoint (`cmd/server/main.go`):**
    ```go
    package main

    import (
    	"log"
    	"package-indexer/internal/server"
    )

    func main() {
    	srv := server.New(":8080")
    	if err := srv.Start(); err != nil {
    		log.Fatalf("Server failed to start: %v", err)
    	}
    }
    ```

---

## Phase 3: Integration and Harness Validation

**Goal:** Verify the complete application against the official test suite under high stress.

**Tasks:**

1.  **Write an Integration Test (`tests/integration/server_test.go`):**
    *   Create a test that starts the server on a random port, connects a real TCP client, sends a sequence of commands, and asserts the responses.

2.  **Create a Harness Script (`scripts/run_harness.sh`):**
    ```bash
    #!/bin/bash
    set -e
    
    echo "Building server..."
    go build -o server ./cmd/server

    echo "Starting server in background..."
    ./server &
    SERVER_PID=$!

    # Gracefully kill the server on script exit
    trap 'kill $SERVER_PID' EXIT

    # Allow server time to start
    sleep 1

    echo "Running test harness..."
    # Use the harness for your specific OS
    ./do-package-tree_darwin "$@"

    echo "Harness finished."
    ```
    *   Make the script executable: `chmod +x scripts/run_harness.sh`.

3.  **Pass the Harness:**
    *   Run the script with default settings: `./scripts/run_harness.sh`
    *   **Key Acceptance Criteria:** Run the script with high concurrency and multiple random seeds to prove robustness. The final implementation **must** pass this command consistently:
        ```bash
        ./scripts/run_harness.sh -c=100
        ```

---

## Phase 4: Production Readiness

**Goal:** Package the application with a `Dockerfile` and `README`.

**Tasks:**

1.  **Write a Multi-Stage `Dockerfile`:**
    ```dockerfile
    # ---- Builder Stage ----
    FROM golang:1.21-alpine AS builder
    WORKDIR /app
    COPY go.mod go.sum ./
    RUN go mod download
    COPY . .
    # Build a static, CGO-disabled binary for maximum portability
    RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o /server ./cmd/server

    # ---- Final Stage ----
    FROM ubuntu:latest
    # Create a non-root user for security
    RUN useradd -ms /bin/bash -r appuser
    WORKDIR /home/appuser
    
    COPY --from=builder /server /usr/local/bin/server
    
    # Run as the non-root user
    USER appuser
    EXPOSE 8080
    CMD ["server"]
    ```

2.  **Write a `README.md`:**
    *   Provide a brief overview of the project and its architecture.
    *   Include clear instructions on how to build, run the tests, and run the server using both `go` commands and Docker.

3.  **Final Review:**
    *   Perform a final pass over all code and documentation to ensure clarity, correctness, and removal of any personal information.

---

## Appendix: Definition of Done Checklist

-   [ ] All unit tests pass with the `-race` flag.
-   [ ] All integration tests pass.
-   [ ] The official test harness passes with `-c=100` and multiple random seeds.
-   [ ] The Docker image builds and the container runs successfully.
-   [ ] `README.md` is complete and accurate.
-   [ ] Git history is anonymized.
-   [ ] The project contains no external dependencies.
