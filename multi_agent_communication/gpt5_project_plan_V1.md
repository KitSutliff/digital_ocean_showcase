## Executive summary

This document presents a complete plan to implement a robust, production-quality package indexer service that satisfies the DigitalOcean coding challenge. It explains the system, describes the current repository state, defines learning goals, and outlines a precise implementation, testing, and delivery plan suitable for review by a senior platform engineer. No implementation is started here; this is a thorough proposal.

## What the system is

The target system is a concurrent TCP server on port 8080 that maintains a package index and enforces dependency constraints under concurrent client access. The wire protocol is newline-delimited, with messages in the form `command|package|dependencies\n`. The server must respond to `INDEX`, `REMOVE`, and `QUERY` commands with `OK\n`, `FAIL\n`, or `ERROR\n` according to strict rules.

Key excerpts from the provided brief:

```17:27:package_contents/INSTRUCTIONS.md
Messages from clients follow this pattern:

<command>|<package>|<dependencies>\n

Where:
* `<command>` is mandatory, and is either `INDEX`, `REMOVE`, or `QUERY`
* `<package>` is mandatory, the name of the package referred to by the command, e.g. `mysql`, `openssl`, `pkg-config`, `postgresql`, etc.
* `<dependencies>` is optional, and if present it will be a comma-delimited list of packages that need to be present before `<package>` is installed. e.g. `cmake,sphinx-doc,xz`
* The message always ends with the character `\n`
```

```39:44:package_contents/INSTRUCTIONS.md
The response code returned should be as follows:
* For `INDEX` commands, the server returns `OK\n` if the package can be indexed. It returns `FAIL\n` if the package cannot be indexed because some of its dependencies aren't indexed yet and need to be installed first. If a package already exists, then its list of dependencies is updated to the one provided with the latest command.
* For `REMOVE` commands, the server returns `OK\n` if the package could be removed from the index. It returns `FAIL\n` if the package could not be removed from the index because some other indexed package depends on it. It returns `OK\n` if the package wasn't indexed.
* For `QUERY` commands, the server returns `OK\n` if the package is indexed. It returns `FAIL\n` if the package isn't indexed.
* If the server doesn't recognize the command or if there's any problem with the message sent by the client it should return `ERROR\n`.
```

## Current repository state

- **Artifacts present**:
  - `package_contents/INSTRUCTIONS.md`: Full problem statement and constraints.
  - `package_contents/source.tar.gz`: Contains Go source for the provided test harness (`test-suite/*.go`).
  - Platform-specific harness binaries: `do-package-tree_darwin`, `do-package-tree_linux`, `do-package-tree_freebsd`, `do-package-tree_windows`.
  - `package_contents/version`: Challenge version metadata.

- **Harness behavior**:
  - Expects a server listening on TCP port `8080`.
  - Exercises correctness and robustness, including concurrency (random seeds, factor up to 100), malformed inputs, and repeated/contradictory commands.

- **Constraints**:
  - Use only the standard library for production code.
  - Must run and pass harness across different random seeds and concurrency levels.
  - Provide tests, build/run artifacts, and (optionally) a Dockerfile.

## What we are expected to learn/do

- **Demonstrate**: The ability to design and implement a concurrency-safe network service with strict protocol adherence, deterministic state transitions, and robust error handling under load.
- **Design for production**: Clean structure, maintainable code, observability, and clear failure modes without external dependencies beyond the standard library.
- **Validate**: Automated tests (unit + integration) and green runs of the provided harness at high concurrency.

## Success criteria

- **Functional**: All harness tests pass consistently under various random seeds and concurrency up to 100.
- **Correctness**: Strict protocol behavior for `INDEX`, `REMOVE`, `QUERY`, malformed lines, idempotent operations, and dependency constraints.
- **Concurrency**: No data races or deadlocks; predictable behavior under high parallelism.
- **Maintainability**: Clear architecture, readable code, and sufficient tests.
- **Portability**: Builds and runs on latest Ubuntu base image; no non-stdlib dependencies in production code.

## Proposed implementation approach

### Language and rationale

- **Go (standard library only)**: Best fit for network concurrency using `net`, `bufio`, and `sync`. Minimal ceremony, strong standard library, straightforward static binaries, and excellent ergonomics for goroutine-per-connection models.

### High-level architecture

- **Process**: Single binary server, listens on `:8080`, accepts multiple concurrent TCP clients.
- **Concurrency model**: Goroutine per connection; each connection handled via a buffered reader loop, processing newline-delimited lines.
- **State model**: In-memory graph index guarded by locks.
  - `indexed: map[string]bool` — presence in index.
  - `deps: map[string]map[string]struct{}` — forward dependencies per package.
  - `revDeps: map[string]map[string]struct{}` — reverse dependency index.
  - All access protected by a single `sync.RWMutex` initially. Optionally evolve to sharded locks if profiling shows contention.

### Command semantics

- **INDEX pkg|deps**:
  - Parse dependency list (`[]string`). Empty allowed.
  - Acquire write lock.
  - If any dependency is not currently indexed → respond `FAIL\n`.
  - Upsert `indexed[pkg]=true`.
  - Replace dependency set for `pkg` with the provided list (not a merge). Update `revDeps` accordingly: remove old backrefs not in new set; add new backrefs.
  - Respond `OK\n`.

- **REMOVE pkg**:
  - Acquire write lock.
  - If `!indexed[pkg]` → `OK\n`.
  - If `revDeps[pkg]` non-empty → `FAIL\n`.
  - Else, delete `indexed[pkg]`, drop `deps[pkg]`, and remove all backrefs in `revDeps` for this package; clear `revDeps[pkg]`.
  - Respond `OK\n`.

- **QUERY pkg**:
  - Acquire read lock.
  - If `indexed[pkg]` → `OK\n`, else `FAIL\n`.

- **ERROR cases**:
  - Any line not matching `cmd|pkg|deps\n` with exactly two `|` separators.
  - Unknown command string.
  - Non-UTF-8 or otherwise invalid decode.

### Wire format and parsing

- Use `bufio.Reader.ReadString('\n')` per connection to obtain lines.
- Validate exactly three fields split by `|`. The third field may be empty; if non-empty, split on comma; ignore empty segments from trailing commas.
- Trim only the trailing `\n`; do not trim other whitespace unless specified by brief.
- Respond immediately per line; do not batch.
- Never assume one read equals one message; always rely on newline framing.

### Error handling and resilience

- Per malformed line: write `ERROR\n` and continue reading subsequent lines on the same connection.
- Per write error: log at warning level and close the connection.
- Do not crash on bad inputs; treat client behavior as untrusted but non-malicious.

### Observability

- Minimal structured logging to stdout/stderr using `log` package with timestamps.
- Log connection lifecycle at debug level (optional flag), protocol errors, and internal errors.
- Optionally expose lightweight internal counters via expvar if permitted by the standard library constraint (development only, not required by harness).

### Performance considerations

- Start with a single global `RWMutex`. With 100 concurrent clients, expected contention is acceptable for the small critical sections (check-then-update, simple map ops).
- If profiling indicates contention, introduce sharding by first byte of package name with consistent lock ordering to avoid deadlocks.

### Data lifecycle and memory

- The in-memory index is unbounded for the lifetime of the process (scope of the challenge). No persistence required.
- Use `map[string]struct{}` for sets to minimize allocations and lookups.

## Detailed plan of execution (step-by-step)

### Phase 0 — Bootstrap

1. Initialize a new Go module (e.g., `module idxserver`).
2. Create directories: `cmd/server`, `internal/index`, `internal/wire`, `internal/server`, `tests/integration`.
3. Add `.gitignore` and anonymized `.git/config` as per brief, and a lightweight `Makefile` with `build`, `run`, `test` targets.

### Phase 1 — Core domain and parsing

1. Implement `internal/wire`:
   - Types: `Command` enum-like, `Request{Cmd, Package, Deps []string}`.
   - `ParseLine(string) (Request, error)` enforcing the strict `cmd|pkg|deps` format.
   - `FormatResponse(enum) string` returning `OK\n`, `FAIL\n`, `ERROR\n`.
   - Unit tests for valid/invalid cases, including edge cases: missing fields, extra separators, empty deps, trailing commas, unknown commands.
2. Implement `internal/index`:
   - Structure `Index` with `indexed`, `deps`, `revDeps`, `mu sync.RWMutex`.
   - Methods: `IndexPackage(pkg string, deps []string) (ok bool)`, `RemovePackage(pkg string) (ok bool, blocked bool)`, `QueryPackage(pkg string) bool`.
   - Semantics exactly as specified; method return values mapped to `OK`/`FAIL`.
   - Unit tests covering: re-index with replacement, index blocked by missing deps, remove blocked by reverse deps, remove non-existent returns OK, query semantics.

### Phase 2 — TCP server

1. Implement `internal/server`:
   - `Serve(listener net.Listener, idx *index.Index)` accept loop.
   - `handleConn(conn net.Conn)` with `bufio.Reader`, line loop → parse → dispatch to `index` → write response.
   - Robust handling for partial reads, malformed lines (return `ERROR\n` and continue), and write errors (close connection).
2. `cmd/server/main.go`:
   - Parse flags (e.g., `-addr :8080`, `-log-level info`).
   - Create listener, instantiate `Index`, run `Serve`.
3. Basic integration test: start server on random port, send a few known-good and malformed lines, assert responses.

### Phase 3 — Harness integration and robustness

1. Run the provided platform harness against the server on `:8080`.
2. Iterate on any discovered mismatches (timing, error paths, idempotency nuances) until consistent green.
3. Validate across multiple seeds and concurrency levels (up to 100).

### Phase 4 — Developer experience and packaging

1. Add `README.md` describing build, run, test, and design rationale.
2. Add `Dockerfile` based on latest Ubuntu image to build and run the server (multi-stage: build in `golang:1.x`, copy binary into `ubuntu:latest`).
3. Provide `scripts/run_harness.sh` to launch server and run harness deterministically.
4. Ensure no PII in git metadata and README.

### Phase 5 — Testing strategy (beyond harness)

- **Unit tests**:
  - Wire parsing edge cases.
  - Index transitions: add, replace, remove, reverse-deps maintenance, idempotency.
- **Concurrency tests**:
  - Fuzz-style goroutine workers issuing mixed commands; assert invariants (no package with missing deps when indexed; no package removed while depended upon).
- **Integration tests**:
  - Real TCP connections, message framing validation, malformed inputs, duplicate commands.
- **Determinism**:
  - Where appropriate, isolate state via fresh `Index` per test; avoid shared globals.

## Design decisions and rationale

- **Single `RWMutex` first**: Simplicity and safety trump micro-optimizations; critical sections are small and contention is acceptable at 100 clients.
- **Replace, don’t merge, dependencies on re-INDEX**: Matches spec explicitly; simplifies reverse-index maintenance.
- **Strict line framing**: Newline as the only frame boundary prevents ambiguity and makes partial read handling deterministic.
- **Immediate per-line response**: Keeps client state simple and avoids batching complexity.
- **Standard library only**: Complies with constraints, reduces supply chain risk, eases review.

## Risks and mitigations

- **Race conditions**: Use coarse-grained locking; validate with `-race` and concurrency tests.
- **Deadlocks with sharded locks**: If sharding is introduced, enforce deterministic lock ordering by shard index; document and test.
- **Protocol edge cases**: Centralize parsing and response mapping; exhaustive unit tests for malformed inputs.
- **Throughput under spikes**: Backed by cheap goroutines; if needed, add listener backpressure (connection limit) and bounded per-connection read buffer.

## Operational considerations

- **Config**: Address/port via flags/env; sane defaults to `:8080` as required.
- **Logging**: Human-readable logs with timestamps; optional debug flag to reduce noise under harness.
- **Metrics**: Optional expvar counters (requests, errors) during development; not required for submission.

## Deliverables

- Source code (server and tests) using only the standard library.
- Unit, concurrency, and integration tests with clear coverage of edge cases.
- `README.md` with design rationale and run instructions.
- `Dockerfile` and `Makefile` for reproducible builds.
- Script to run the harness locally against the server.

## Acceptance tests (definition of done)

- Harness outputs:
  - Shows correctness tests passed.
  - Shows robustness tests passed.
  - Reports "All tests passed!" as per brief when run with multiple seeds and `--concurrency=100`.
- Local test suite is green with `go test ./... -race`.
- Docker image builds and the binary runs on Ubuntu latest.

## Execution timeline (indicative)

- Day 0.5: Parse/wire + unit tests.
- Day 0.5: Index implementation + unit tests.
- Day 0.5: TCP server + basic integration tests.
- Day 0.5: Harness hardening; resolve edge cases under load.
- Day 0.5: Packaging, docs, Dockerfile, scripts, final verification.

## Alternatives considered

- **Rust**: Excellent safety and performance; standard library sufficient; longer implementation time for equivalent ergonomics.
- **Python**: Doable with `socket` + threads; more careful framing and synchronization, performance headroom lower; increased risk under high concurrency with only stdlib.

## Assumptions

- No persistence is required; in-memory index suffices for harness objectives.
- Package names are ASCII/UTF-8 without needing normalization; treat inputs as opaque strings.
- No requirement to detect or forbid dependency cycles proactively; the spec only requires that dependencies be already indexed to allow `INDEX`.

## Out of scope

- Authentication, authorization, TLS.
- Persistence or recovery.
- Advanced observability (metrics backends, tracing).

## Appendix — Provided artifacts reference

- Challenge version:

```1:1:package_contents/version
v1.0 - Tue Jan 23 15:32:15 UTC 2024
```

- Harness source archive contents (partial): `test-suite/client.go`, `test-suite/main.go`, `test-suite/wire_format.go`, etc., packaged in `source.tar.gz`.


