## V2 Synthesis: Step-by-Step Project Guide (based only on V1 syntheses)

Scope and constraint: This guide synthesizes and expands upon the three V1 synthesis documents only (`claude_plan_synthesis_v1.md`, `gemini_plan_synthesis_v1.md`, `gpt5_plan_synthesis_v1.md`). No additional sources or later versions are considered.

### Objective

Deliver a production-quality TCP package indexer that:
- Listens on `:8080` and implements `INDEX`, `REMOVE`, `QUERY`.
- Uses Go standard library only.
- Maintains forward and reverse dependency maps with correct semantics.
- Is concurrency-safe under up to 100 concurrent clients.
- Passes the provided test harness across multiple random seeds and high concurrency.
- Ships with tests, Dockerfile, and minimal run/docs.

### Decision record (from V1 syntheses consensus)

- Language/runtime: Go, standard library only.
- Concurrency model: Goroutine-per-connection; shared state protected by `sync.RWMutex` (single lock initially).
- Data model: Three structures guarded by the lock: `indexed` (presence), `deps` (forward), `revDeps` (reverse).
- Parsing: Strict `cmd|pkg|deps\n` framing; empty deps allowed; errors → `ERROR\n` and continue.
- Responses: `OK\n`, `FAIL\n`, `ERROR\n` only.
- Re-index semantics: Replace dependency list (do not merge), update reverse edges.
- Remove semantics: `OK` if not indexed; `FAIL` if any dependents; otherwise remove and clean up reverse edges.
- Line framing: Prefer `bufio.Reader.ReadString('\n')` over `Scanner` to avoid token limits.
- Validation: Enforce structure and valid commands only (avoid over-validating package names).
- Scalability: Start with single `RWMutex`; shard only if profiling shows contention.
- Observability: Minimal logging; optional counters in development only.

---

## Step-by-step execution plan

Each step lists deliverables and checks so an engineer or LLM can follow deterministically.

### 0) Prerequisites

- Install Go (1.21+ recommended).
- Ensure port 8080 is free.
- Confirm access to the challenge harness binary for your platform.

### 1) Repository bootstrap

- Initialize repo and anonymous authoring.
  - Deliverables:
    - `.git/` initialized
    - `.git/config` with anonymous user settings
  - Actions:
    - `git init`
    - Add under `[user]` section in `.git/config`:
      - `name = "Anonymous"`
      - `email = "anonymous@example.com"`

### 2) Project scaffolding

- Create standard Go module and directories.
  - Deliverables:
    - `go.mod` initialized
    - Directory layout:
      - `cmd/server/` (entrypoint)
      - `internal/wire/` (protocol parsing & responses)
      - `internal/index/` (core state & semantics)
      - `internal/server/` (TCP listener and handlers)
      - `tests/integration/` (integration tests)
      - `Makefile`, `.gitignore`, minimal `README.md`
  - Actions:
    - `go mod init package-indexer`
    - Create directories as listed
    - Seed `Makefile` with `build`, `test`, `run` targets

### 3) Protocol layer (`internal/wire`)

- Implement strict parsing and response formatting.
  - Deliverables:
    - `internal/wire/wire.go` with:
      - `type CommandType string` ("INDEX", "REMOVE", "QUERY")
      - `type Request struct { Cmd CommandType; Package string; Deps []string }`
      - `ParseLine(line string) (Request, error)`
      - `const (
          RespOK = "OK\n"; RespFail = "FAIL\n"; RespError = "ERROR\n"
        )`
    - `internal/wire/wire_test.go` covering:
      - Exactly two `|` separators required
      - Empty deps supported
      - Comma-splitting; ignore trailing commas producing empty items
      - Unknown command → error
      - Preserve everything else per spec (no extra trimming beyond trailing `\n`)
  - Notes:
    - Use `strings.HasSuffix(line, "\n")` check; return error if missing.
    - Use `strings.SplitN(line[:len(line)-1], "|", 3)` to enforce exactly three fields.

### 4) Index layer (`internal/index`)

- Implement thread-safe dependency index with exact semantics.
  - Deliverables:
    - `internal/index/index.go` with:
      - `type Index struct { mu sync.RWMutex; indexed map[string]bool; deps map[string]map[string]struct{}; revDeps map[string]map[string]struct{} }`
      - `func New() *Index`
      - `func (i *Index) IndexPackage(pkg string, deps []string) bool` (true→OK; false→FAIL)
      - `func (i *Index) RemovePackage(pkg string) (ok bool)` (true→OK; false→FAIL)
      - `func (i *Index) QueryPackage(pkg string) bool` (true→OK; false→FAIL)
    - `internal/index/index_test.go` with tests for:
      - Index blocked by missing dependency → FAIL
      - Re-index replaces deps; reverse edges updated
      - Remove non-existent → OK
      - Remove blocked by dependents → FAIL
      - Query reflects `indexed` truth
      - Concurrency smoke test: mixed ops via goroutines maintain invariants
  - Algorithmic rules (inside single write-locked critical sections for mutating ops):
    - Index:
      - Check all `deps` are present in `indexed`; if any missing, return false (do not mutate state)
      - Upsert `indexed[pkg] = true`
      - Replace forward deps set; compute delta to update `revDeps`
      - Return true
    - Remove:
      - If `!indexed[pkg]` → return true
      - If `len(revDeps[pkg]) > 0` → return false
      - For each `d` in `deps[pkg]`, delete backref `revDeps[d][pkg]`
      - Delete `indexed[pkg]`, `deps[pkg]`, and `revDeps[pkg]` (cleanup empty maps as needed)
      - Return true
    - Query: read lock, return `indexed[pkg]`

### 5) Server layer (`internal/server`)

- Implement TCP listener and per-connection handler.
  - Deliverables:
    - `internal/server/server.go` with:
      - `func Serve(listener net.Listener, idx *index.Index) error`
      - `func handleConn(conn net.Conn, idx *index.Index)`
    - Behavior:
      - Accept loop spawns goroutine per connection
      - In `handleConn`, create `bufio.NewReader(conn)`
      - Loop: `line, err := reader.ReadString('\n')`; on EOF/err → close connection
      - Parse via `wire.ParseLine`; on error → write `RespError` and continue
      - Dispatch to index based on command; map bools to `RespOK`/`RespFail`
      - On write error → close connection
  - Tests:
    - Unit tests for handler logic with in-memory `net.Pipe()` or loopback
    - Verify malformed lines return `ERROR\n` and connection stays open

### 6) Entrypoint (`cmd/server/main.go`)

- Wire everything together with minimal flags.
  - Deliverables:
    - `cmd/server/main.go` that:
      - Parses `-addr` (default `:8080`) and `-log-level` (optional)
      - Creates `net.Listen("tcp", addr)`
      - Instantiates `index.New()` and calls `server.Serve(...)`

### 7) Integration testing (`tests/integration`)

- Validate end-to-end behavior over real TCP.
  - Deliverables:
    - `tests/integration/basic_test.go` covering happy paths and error cases:
      - INDEX with satisfied deps → OK
      - INDEX with missing deps → FAIL
      - QUERY presence and absence
      - REMOVE not indexed → OK; REMOVE with dependents → FAIL; otherwise OK
      - Malformed messages → ERROR, subsequent valid requests still processed
    - `tests/integration/concurrency_test.go`:
      - Launch N goroutines issuing mixed commands; ensure invariants hold and no deadlocks/timeouts
  - Run with `go test ./... -race`.

### 8) Harness validation

- Execute the official harness at increasing concurrency.
  - Deliverables:
    - Documented commands to run harness against a server running on `:8080`
    - Notes on using multiple random seeds
  - Actions:
    - Start server (`make run`)
    - In another terminal, run platform harness binary without flags; then with higher concurrency (e.g., `-concurrency=100` if available) and different `-seed` values
  - Acceptance:
    - Consistent "All tests passed!" across seeds and concurrency=100

### 9) Packaging and DX

- Provide minimal, reviewer-friendly build/run tooling.
  - Deliverables:
    - `Makefile` with: `build` (go build), `test` (go test -race), `run` (go run ./cmd/server), `harness` (optional wrapper to run local platform harness)
    - `Dockerfile` (multi-stage): build in `golang:1.x`, run in `ubuntu:latest` with the static binary
    - `README.md` documenting quickstart, architecture, and decisions (concise)

### 10) Performance posture and evolution

- Keep the initial implementation simple; scale only if needed.
  - Deliverables:
    - Clear note in README about potential future lock sharding by package shard (first byte or hash), with strict lock ordering if introduced
    - Optional development-only counters (e.g., via `expvar`) disabled by default

### 11) Final acceptance checklist

- Functional:
  - INDEX/REMOVE/QUERY semantics match spec exactly
  - Re-index replaces deps and updates reverse edges
  - Malformed lines produce `ERROR\n` without killing the connection
- Concurrency:
  - No races under `go test -race`
  - Harness passes at concurrency=100 across multiple seeds
- Packaging:
  - Docker image builds and server runs on Ubuntu latest
- Documentation:
  - README explains build/run/test and core decisions
  - No PII in repo or commit metadata

---

## Reference implementation skeletons (for guidance)

These signatures reflect the agreed design from V1 syntheses; they are not full implementations.

```go
// internal/wire/wire.go
type CommandType string
const (
    CmdIndex CommandType = "INDEX"
    CmdRemove CommandType = "REMOVE"
    CmdQuery CommandType = "QUERY"
    RespOK    = "OK\n"
    RespFail  = "FAIL\n"
    RespError = "ERROR\n"
)

type Request struct {
    Cmd     CommandType
    Package string
    Deps    []string
}

func ParseLine(line string) (Request, error) { /* per spec */ }
```

```go
// internal/index/index.go
type Index struct {
    mu      sync.RWMutex
    indexed map[string]bool
    deps    map[string]map[string]struct{}
    revDeps map[string]map[string]struct{}
}

func New() *Index { /* init maps */ }
func (i *Index) IndexPackage(pkg string, deps []string) bool { /* see rules */ }
func (i *Index) RemovePackage(pkg string) bool { /* see rules */ }
func (i *Index) QueryPackage(pkg string) bool { /* read lock */ }
```

```go
// internal/server/server.go
func Serve(l net.Listener, idx *index.Index) error {
    for {
        c, err := l.Accept()
        if err != nil { return err }
        go handleConn(c, idx)
    }
}

func handleConn(c net.Conn, idx *index.Index) {
    defer c.Close()
    r := bufio.NewReader(c)
    for {
        line, err := r.ReadString('\n')
        if err != nil { return }
        req, err := wire.ParseLine(line)
        if err != nil { _, _ = c.Write([]byte(wire.RespError)); continue }
        switch req.Cmd {
        case wire.CmdIndex:
            if idx.IndexPackage(req.Package, req.Deps) { c.Write([]byte(wire.RespOK)) } else { c.Write([]byte(wire.RespFail)) }
        case wire.CmdRemove:
            if idx.RemovePackage(req.Package) { c.Write([]byte(wire.RespOK)) } else { c.Write([]byte(wire.RespFail)) }
        case wire.CmdQuery:
            if idx.QueryPackage(req.Package) { c.Write([]byte(wire.RespOK)) } else { c.Write([]byte(wire.RespFail)) }
        default:
            c.Write([]byte(wire.RespError))
        }
    }
}
```

---

## Milestones and timeline (indicative)

- Day 0.5: Steps 1–3 (repo, scaffolding, wire) with unit tests
- Day 0.5: Step 4 (index) with unit + concurrency tests
- Day 0.5: Steps 5–6 (server + main) with basic integration tests
- Day 0.5: Step 7–8 (integration + harness hardening)
- Day 0.5: Step 9–11 (packaging, docs, acceptance)

This step-by-step guide is fully aligned with the V1 syntheses and is sufficient for a competent engineer or LLM to implement the solution end-to-end without ambiguity.


