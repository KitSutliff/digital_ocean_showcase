## V3 Synthesis: Definitive Step-by-Step Guide (derived only from V2 syntheses)

Scope: This guide is synthesized exclusively from the three V2 syntheses (`claude_plan_synthesis_v2.md`, `gemini_plan_synthesis_v2.md`, `gpt5_plan_synthesis_v2.md`). It integrates the best ideas into the clearest, implementation-ready plan. No other sources were used.

### Executive summary

- Build a Go (stdlib-only) TCP server on `:8080` that implements `INDEX`, `REMOVE`, `QUERY` with strict wire protocol and concurrency safety for 100 clients.
- State is an in-memory dependency graph with forward and reverse indices plus an explicit presence set; operations are atomic via a single `sync.RWMutex` (shard later only if measured contention).
- Parsing uses newline framing with `bufio.Reader.ReadString('\n')`; validation follows the spec exactly to avoid false negatives. Deliver with unit/integration tests, a minimal Dockerfile, Makefile, and harness scripts.

### Decision record (consensus + best-of choices)

- Language/runtime: Go, standard library only.
- Concurrency: Goroutine-per-connection; shared state protected by a single `sync.RWMutex` initially.
- Data model: `indexed: map[string]bool`, `deps: map[string]map[string]struct{}`, `revDeps: map[string]map[string]struct{}`.
- Parsing: Strict structure `cmd|pkg|deps\n`; dependencies comma-separated; empty deps allowed.
- Responses: Only `OK\n`, `FAIL\n`, `ERROR\n`.
- Re-index: Replace dependency list; update reverse edges accordingly.
- Remove: `OK` if not indexed; `FAIL` if dependents exist; else remove and clean reverse edges.
- Framing: Prefer `bufio.Reader.ReadString('\n')` over `Scanner` to avoid token limits.
- Validation: Enforce structure and command names only; do not over-validate package names.
- Evolution: Consider sharded locks only if profiling shows lock contention.
- Observability: Minimal logging; optional counters for local debugging only.

---

## Step-by-step execution plan

Each step includes concrete deliverables and acceptance checks to ensure deterministic progress.

### 0) Prerequisites

- Go 1.21+ installed; Docker available for packaging.
- Port 8080 available.
- Access to the platform harness binary for your OS.

### 1) Repository bootstrap

- Initialize Git and anonymous authoring before first commit.
- Deliverables:
  - `.git/` initialized
  - `.git/config` with:
    - `[user] name = "Anonymous"`
    - `[user] email = "anonymous@example.com"`

### 2) Project scaffolding

- Initialize module and create directories.
- Deliverables:
  - `go.mod` (e.g., `go mod init package-indexer`)
  - Directory layout:
    - `cmd/server/` (entrypoint)
    - `internal/wire/` (protocol parsing & response formatting)
    - `internal/index/` (core state & semantics)
    - `internal/server/` (TCP listener and handlers)
    - `tests/integration/` (integration tests)
    - `scripts/` (harness/stress/verification helpers)
    - `Makefile`, `.gitignore`, minimal `README.md`
  - Makefile with `build`, `test`, `run` targets.

### 3) Protocol layer (`internal/wire`)

- Implement strict, spec-accurate parsing and response formatting.
- Deliverables:
  - `wire.go` with:
    - `type CommandType string` (INDEX, REMOVE, QUERY)
    - `type Request struct { Cmd CommandType; Package string; Deps []string }`
    - `ParseLine(line string) (Request, error)` — requires trailing `\n`, exactly two `|`, allows empty deps, splits on `,` and ignores empty segments from trailing comma.
    - Response constants: `RespOK = "OK\n"`, `RespFail = "FAIL\n"`, `RespError = "ERROR\n"`.
  - `wire_test.go` covering valid/invalid structure, unknown commands, with and without dependencies, and malformed lines.
- Acceptance:
  - `go test -race ./internal/wire` passes.

### 4) Index layer (`internal/index`)

- Implement thread-safe dependency index with exact semantics and reverse-edge maintenance.
- Deliverables:
  - `index.go` with:
    - `type Index struct { mu sync.RWMutex; indexed map[string]bool; deps map[string]map[string]struct{}; revDeps map[string]map[string]struct{} }`
    - `func New() *Index`
    - `func (i *Index) IndexPackage(pkg string, deps []string) bool`
    - `func (i *Index) RemovePackage(pkg string) bool`
    - `func (i *Index) QueryPackage(pkg string) bool`
  - `index_test.go` covering:
    - Index blocked by missing deps → FAIL (no mutation)
    - Re-index replaces deps; reverse deps updated (old backrefs removed, new added)
    - Remove not indexed → OK
    - Remove blocked by dependents → FAIL
    - Query reflects `indexed`
    - Concurrency smoke test with mixed ops keeps invariants
- Acceptance:
  - `go test -race ./internal/index` passes.

### 5) Server layer (`internal/server`)

- Implement TCP listener and per-connection line loop.
- Deliverables:
  - `server.go` with:
    - `Serve(l net.Listener, idx *index.Index) error` (accept loop; goroutine per connection)
    - `handleConn(c net.Conn, idx *index.Index)` using `bufio.Reader.ReadString('\n')`, parse → dispatch → write response; on parse error: write `ERROR\n` and continue; on write error: close.
  - Unit tests for handler logic using `net.Pipe()` or loopback.
- Acceptance:
  - Basic server handler tests pass under `go test -race`.

### 6) Entrypoint (`cmd/server/main.go`)

- Minimal main wiring with flags.
- Deliverables:
  - Parse `-addr` (default `:8080`), optional `-log-level`.
  - Create listener, instantiate `index.New()`, call `server.Serve(...)`.
- Acceptance:
  - `make run` starts server; basic manual `nc` tests return expected responses.

### 7) Integration testing (`tests/integration`)

- End-to-end validation over real TCP.
- Deliverables:
  - `basic_test.go`: happy paths and error cases (INDEX with/without deps, QUERY present/absent, REMOVE OK/FAIL, malformed → ERROR then continue).
  - `concurrency_test.go`: N concurrent clients issuing mixed commands; no deadlocks/timeouts; invariants maintained.
- Acceptance:
  - `go test -race ./tests/integration` passes locally.

### 8) Harness validation (`scripts/run_harness.sh`)

- Automate running the official harness against the local server.
- Deliverables:
  - `scripts/run_harness.sh` to build, run server in background, execute harness, and clean up.
- Acceptance:
  - Harness passes with default settings; then with higher concurrency (e.g., `-concurrency=100`) and multiple seeds.

### 9) Stress and verification (`scripts/stress_test.sh`, `scripts/final_verification.sh`)

- Optional but recommended scripts derived from the strongest V2 material.
- Deliverables:
  - `scripts/stress_test.sh`: loops over concurrency levels and seeds, failing on first error.
  - `scripts/final_verification.sh`: one-button clean → test (unit/integration/race/coverage) → build → Docker build → smoke test → harness → stress → final race check.
- Acceptance:
  - Both scripts run cleanly on a developer workstation.

### 10) Packaging and developer experience

- Provide minimal yet complete packaging.
- Deliverables:
  - `Dockerfile` (multi-stage): build in `golang:1.x` and run in `ubuntu:latest`; run as non-root; expose 8080; optional healthcheck.
  - `Makefile` extended with targets: `build`, `test` (`-race`), `run`, `docker-build`, `docker-run`, `fmt`, `clean`.
  - `README.md` with quickstart, protocol summary, architecture sketch, test and harness instructions, and a note on future lock sharding.
- Acceptance:
  - `docker build` and `docker run -p 8080:8080` work; manual `nc` returns expected responses.

### 11) Performance posture and evolution

- Start simple; optimize if and only if measured.
- Notes:
  - Profile only after harness is green; if lock contention is evident, introduce sharded locks by package shard with strict lock ordering.
  - Keep per-connection work small; avoid long critical sections.

### 12) Final acceptance checklist

- Functional: `INDEX`/`REMOVE`/`QUERY` match spec; re-index replaces deps; malformed lines return `ERROR\n` without dropping connection.
- Concurrency: `go test -race` clean; harness passes at `--concurrency=100` across multiple seeds.
- Packaging: Docker image builds and runs on Ubuntu latest; no non-stdlib prod deps.
- Documentation: README complete; Makefile targets present; no PII in repo or commit metadata.

---

## Appendices (ready-to-adapt snippets)

### A. Harness runner (scripts/run_harness.sh)

```bash
#!/usr/bin/env bash
set -euo pipefail

go build -o server ./cmd/server
./server &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true; wait $SERVER_PID 2>/dev/null || true' EXIT

sleep 1
./do-package-tree_darwin
```

### B. Stress test (scripts/stress_test.sh)

```bash
#!/usr/bin/env bash
set -euo pipefail

make build
./server &
SERVER_PID=$!
trap 'kill $SERVER_PID 2>/dev/null || true; wait $SERVER_PID 2>/dev/null || true' EXIT

sleep 2
for c in 1 10 25 50 100; do
  for seed in 42 12345 98765; do
    echo "concurrency=$c seed=$seed"
    ./do-package-tree_darwin -concurrency=$c -seed=$seed
  done
done
```

### C. Final verification (scripts/final_verification.sh)

```bash
#!/usr/bin/env bash
set -euo pipefail

make clean || true
go test -race ./internal/...
go test -race ./tests/integration/...
go test -cover ./...

make build
docker build -t package-indexer .

docker run -d --name indexer-test -p 8081:8080 package-indexer
trap 'docker rm -f indexer-test >/dev/null 2>&1 || true' EXIT
sleep 2

printf 'INDEX|smoke|\n' | nc localhost 8081 | grep -q OK

./scripts/run_harness.sh
./scripts/stress_test.sh

go test -race -timeout=60s ./tests/integration
```

### D. Minimal Dockerfile

```dockerfile
# Builder
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

# Runner (Ubuntu as requested)
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/server /app/server
RUN useradd -r -s /bin/false indexer && chown indexer:indexer /app/server
USER indexer
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD nc -z localhost 8080 || exit 1
CMD ["/app/server"]
```

### E. Common pitfalls and fixes

- Returning `FAIL` instead of `OK` when removing a non-indexed package → ensure idempotent remove returns `OK`.
- Forgetting to remove old reverse edges on re-index → compute deltas and update `revDeps` accordingly.
- Treating a single TCP read as message-aligned → always rely on `\n` framing via `ReadString`.
- Over-validating names → stick to structural checks per spec; unknown commands → `ERROR\n`.
- Coarse locks inside long operations → keep critical sections tight to reduce contention.

This V3 guide is tightly scoped, implementation-ready, and integrates the most reliable tactics from the V2 syntheses while remaining faithful to the challenge specification.


