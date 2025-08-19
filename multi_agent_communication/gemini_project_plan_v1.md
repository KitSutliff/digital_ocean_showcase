# Project Plan: Concurrent Package Indexer

## 1. System Overview

Based on the coding challenge instructions, the system to be built is a TCP server that functions as a package indexer. Its primary role is to maintain a consistent index of software packages and their dependencies.

### Core Functionality:

- **Package Management**: The server tracks packages and their dependency relationships. It ensures that a package can only be indexed if all its dependencies are already present in the index. Conversely, a package cannot be removed if other packages depend on it.
- **Concurrent Client Handling**: The server must be capable of handling multiple simultaneous client connections. Clients can connect and disconnect at any time and may send commands concurrently.
- **Robust Communication Protocol**: The server communicates with clients over TCP on port 8080. It uses a simple text-based protocol (`<command>|<package>|<dependencies>\n`) and must gracefully handle malformed or invalid messages.
- **Stateful Logic**: The server maintains the state of the package index in memory. This state is shared across all client connections and must be managed in a thread-safe manner.

### Key Operations:

- **`INDEX`**: Adds or updates a package in the index. This operation is conditional on the existence of its dependencies.
- **`REMOVE`**: Removes a package from the index. This operation is conditional on the package not being a dependency for any other indexed package.
- **`QUERY`**: Checks for the existence of a package in the index.

## 2. Current State Analysis

The project is currently in its initial state. No source code for the server has been written. The provided materials consist of:

- **`INSTRUCTIONS.md`**: The detailed specification for the coding challenge.
- **Test Harness Executables**: Platform-specific binaries (`do-package-tree_*`) designed to test the correctness and robustness of the server implementation.
- **Test Harness Source Code**: A `source.tar.gz` archive containing the Go source code for the test harness. This can be used for a deeper understanding of the test cases.
- **`version`**: A file indicating the version of the challenge.

The task is to build the package indexer server from the ground up, adhering to the specifications, and ensuring it passes the provided test harness.

## 3. Project Goals and Success Criteria

### Primary Goal

The primary goal is to design, implement, and test a production-ready, concurrent package indexer server that satisfies all the requirements outlined in the `INSTRUCTIONS.md` document.

### Success Criteria

The project will be considered successful when the following criteria are met:

1.  **Full Functionality**: The server correctly implements the `INDEX`, `REMOVE`, and `QUERY` commands according to the specified logic.
2.  **Test Harness Compliance**: The implementation successfully passes all tests in the provided `do-package-tree` test harness, specifically with varying random seeds and a concurrency factor of up to 100.
3.  **Production-Ready Code**: The codebase is clean, well-structured, maintainable, and appropriately documented. It reflects best practices for writing production-grade software.
4.  **Concurrency and Robustness**: The server is stable under high concurrency and is resilient to badly behaved clients and malformed messages.
5.  **Comprehensive Testing**: The solution includes a suite of automated unit and integration tests that cover the core logic and server functionality.
6.  **Dependency Constraints**: The production code relies exclusively on the chosen language's standard library, as per the instructions.
7.  **Deployment Readiness**: The project includes a `Dockerfile` to demonstrate that it can be built and run in a standard Ubuntu environment.
8.  **Anonymized Version Control**: All development is tracked in a Git repository with anonymized commit history.

## 4. Proposed Solution and Architecture

I propose to implement the solution in **Go (Golang)**. This choice is motivated by its excellent support for concurrency (goroutines and channels), a strong standard library for networking, high performance, and its compilation to a single static binary, which simplifies deployment. Go is also mentioned as a language used at DigitalOcean, making it a fitting choice.

### High-Level Architecture

The system will be composed of several distinct components, each with a clear responsibility:

![Architecture Diagram](https://mermaid.ink/svg/eyJjb2RlIjoiZ3JhcGggVERcbiAgICBzdWJncmFwaCBDbGllbnRzXG4gICAgICAgIENsaWVudDEoKENsaWVudCAxKSlcbiAgICAgICAgQ2xpZW50MihcIihDbGllbnQgMikgXCIpXG4gICAgICAgIENsaWVudE4oXCIoQ2xpZW50IE4pIFwiKVxuICAgIGVuZFxuXG4gICAgc3ViZ3JhcGggUGFja2FnZSBJbmRleGVyIFNlcnZlciAoR28gQXBwbGljYXRpb24pXG4gICAgICAgIFRDUFNlcnZlcigoVENQIFNlcnZlcikpXG4gICAgICAgIGNvbm5lY3Rpb25IYW5kbGVyc3t7Q29ubmVjdGlvbiBIYW5kbGVyc319XG4gICAgICAgIFJlcXVlc3RQYXJzZXIoKFJlcXVlc3QgUGFyc2VyKSlcbiAgICAgICAgQ29tbWFuZFByb2Nlc3NvcihbQ29tbWFuZCBQcm9jZXNzb3JdKVxuICAgICAgICBJbmRleERhdGFTdG9yZVsoSW5kZXggRGF0YSBTdG9yZSBdKVxuXG4gICAgICAgIFRDUFNlcnZlciAtLT58QWNjZXB0cyBjb25uZWN0aW9uc3wgY29ubmVjdGlvbkhhbmRsZXJzXG4gICAgICAgIGNvbm5lY3Rpb25IYW5kbGVycyAtLT58Rm9yd2FyZHMgcmF3IG1lc3NhZ2V8IFJlcXVlc3RQYXJzZXJcbiAgICAgICAgUmVxdWVzdFBhcnNlciAtLT58U2VuZHMgcGFyc2VkIGNvbW1hbmR8IENvbW1hbmRQcm9jZXNzb3JcbiAgICAgICAgQ29tbWFuZFByb2Nlc3NvciA8LT4-fFJlYWRzL1dyaXRlcyBwYWNrYWdlIGRhdGF8IEluZGV4RGF0YVN0b3JlXG4gICAgZW5kXG5cbiAgICBDbGllbnQxIC0tPiB8VENQIGNvbm5lY3Rpb258IFRDUFNlcnZlclxuICAgIENsaWVudDIgLS0-fFRDUCBuZXR3b3JrIHByb3RvY29sfCBUQ1BTZXJ2ZXJcbiAgICBDbGllbnROIC0tPiB8VENQIGNvbm5lY3Rpb258IFRDUFNlcnZlclxuIiwibWVybWFpZCI6eyJ0aGVtZSI6ImRlZmF1bHQifSwidXBkYXRlRWRpdG9yIjpmYWxzZX0)

1.  **TCP Server**: The entry point of the application. It will listen for incoming TCP connections on port 8080. Upon accepting a new connection, it will spawn a new goroutine to handle that client independently.
2.  **Connection Handler**: Each client connection will be managed by a dedicated goroutine. This handler will read incoming data from the client, pass it to the parser, send the processed command to the engine, and write the response back to the client. It will use `bufio.Scanner` for efficient and simple line-by-line reading.
3.  **Request Parser**: A pure function responsible for parsing the raw string message from a client. It will validate the message format, extract the command, package name, and dependencies, and return a structured command object or an error if the message is malformed.
4.  **Command Processor / Engine**: This is the core logic of the application. It will receive the parsed command objects and orchestrate the operations by interacting with the data store.
5.  **Index Data Store**: An in-memory data structure responsible for storing the state of the package index. **Crucially, this component will be thread-safe** to prevent race conditions from concurrent read/write operations.

### Data Structure Design

To efficiently manage package relationships, I will use a graph-like structure implemented with standard Go maps.

-   **`packages` (`map[string]map[string]struct{}`)**: A map where the key is the package name (e.g., `"cloog"`) and the value is a `set` (implemented as `map[string]struct{}`) of its dependencies (e.g., `{"gmp": {}, "isl": {}}`). Using a set for dependencies allows for O(1) lookups.
-   **`reverseDependencies` (`map[string]map[string]struct{}`)**: A map that stores the reverse relationships. The key is a package name, and the value is a set of packages that depend on it. This structure is essential for implementing the `REMOVE` command efficiently, allowing for an O(1) check to see if any other packages depend on a given package.
-   **Concurrency Control**: A single `sync.RWMutex` will be used to protect both data structures.
    -   `QUERY` operations will use a read lock (`RLock()`), allowing multiple queries to run concurrently.
    -   `INDEX` and `REMOVE` operations will use a write lock (`Lock()`), ensuring exclusive access and maintaining data consistency.

This entire state will be encapsulated within an `Indexer` struct, which will expose the thread-safe `Index`, `Remove`, and `Query` methods.

## 5. Detailed Implementation Plan

The project will be developed incrementally, with testing at each stage.

### Step 1: Project Setup & Scaffolding

-   **Goal**: Initialize the project environment.
-   **Tasks**:
    1.  Initialize a Git repository: `git init`.
    2.  **Crucially**, configure Git for anonymous commits *before the first commit* by creating a `.git/config` file with the specified user details.
    3.  Initialize a Go module: `go mod init package-indexer`.
    4.  Create the initial directory structure:
        ```
        /
        ├── cmd/
        │   └── server/
        │       └── main.go         // Main application entry point
        ├── internal/
        │   ├── indexer/            // Core data store and logic
        │   │   ├── indexer.go
        │   │   └── indexer_test.go
        │   └── server/             // TCP server and connection handling
        │       ├── server.go
        │       └── server_test.go
        ├── .gitignore
        ├── Dockerfile
        └── Makefile                // For build/test/run automation
        ```

### Step 2: Core Indexer Logic & Unit Tests

-   **Goal**: Implement the stateful, thread-safe indexer logic.
-   **Tasks**:
    1.  In `internal/indexer/indexer.go`, define the `Indexer` struct containing the package maps and the `sync.RWMutex`.
    2.  Implement the `NewIndexer()` constructor function.
    3.  Implement the public methods:
        -   `Index(pkg string, deps []string) error`
        -   `Remove(pkg string) error`
        -   `Query(pkg string) bool`
    4.  In `internal/indexer/indexer_test.go`, write comprehensive unit tests for these methods. The tests must cover all success and failure cases described in the instructions (e.g., indexing with missing dependencies, removing a package that is a dependency, etc.). Test concurrency by spawning multiple goroutines that access the indexer simultaneously.

### Step 3: TCP Server & Protocol Handling

-   **Goal**: Implement the networking layer that handles client communication.
-   **Tasks**:
    1.  In `internal/server/server.go`, create a `Server` struct that holds a reference to the `Indexer`.
    2.  Implement `Server.Start()` which will create a TCP listener on `0.0.0.0:8080`.
    3.  Implement the main accept loop. For each new `net.Conn`, it will spawn a `handleConnection` goroutine.
    4.  Implement `handleConnection(conn net.Conn)`. This function will:
        -   Use `bufio.NewScanner(conn)` to read messages line by line.
        -   For each message, call a `parseMessage` function.
        -   Based on the parsed command, call the appropriate method on the `Indexer`.
        -   Translate the results (`true`, `false`, `error`) from the `Indexer` into the required protocol responses (`OK\n`, `FAIL\n`, `ERROR\n`) and write them back to the connection.
        -   Ensure the connection is properly closed on exit.
    5.  In `cmd/server/main.go`, instantiate the `Indexer` and the `Server`, and call `Server.Start()`.

### Step 4: Integration Testing & Test Harness Validation

-   **Goal**: Ensure all components work together correctly and pass the official tests.
-   **Tasks**:
    1.  Write a basic integration test in `internal/server/server_test.go` that starts the server, connects a client, sends a series of commands, and asserts the responses are correct.
    2.  Run the server (`go run ./cmd/server`).
    3.  In a separate terminal, run the provided `./do-package-tree_darwin` test harness against the live server.
    4.  Debug and refine the implementation until the test harness passes consistently.
    5.  Run the harness with increased concurrency (`./do-package-tree_darwin -c 100`) to validate the thread-safety of the implementation under load.

### Step 5: Containerization & Finalization

-   **Goal**: Prepare the project for submission and deployment.
-   **Tasks**:
    1.  Create a multi-stage `Dockerfile`.
        -   The first stage will use a Go builder image to compile the application into a static binary.
        -   The second stage will use a minimal base image (like `ubuntu:latest` as requested, or a smaller one like `gcr.io/distroless/static-debian11`) and copy only the compiled binary into it. This creates a small, secure production image.
    2.  Create a `Makefile` with simple targets:
        -   `build`: Compiles the Go binary.
        -   `test`: Runs all unit and integration tests.
        -   `run`: Builds and runs the server.
        -   `docker-build`: Builds the Docker image.
        -   `docker-run`: Runs the application inside a Docker container.
    3.  Write a final `README.md` file explaining the project design, and providing clear instructions on how to build, test, and run the application using both Go commands and the Makefile/Docker.
    4.  Review the entire project to ensure it meets all "Must Have" and "Should Have" requirements, including code quality and anonymization.
