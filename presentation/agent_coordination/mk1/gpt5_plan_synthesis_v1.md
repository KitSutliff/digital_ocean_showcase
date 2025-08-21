## Scope

This document synthesizes the three v1 project proposals in `multi_agent_communication/` for the DigitalOcean Package Indexer challenge. It highlights where all three agree, where two align and one diverges, and where all three differ. For each deviation, I provide a clear recommendation based purely on technical merit and the brief’s constraints.

Reviewed inputs:
- `claude_project_plan_v1.md`
- `gemini_project_plan_v1.md`
- `gpt5_project_plan_V1.md`

## System and current state (shared understanding)

All three documents agree on the core problem and repository state:
- Build a concurrent TCP server on port 8080 implementing `INDEX`, `REMOVE`, `QUERY` with strict newline-delimited wire protocol and `OK\n`/`FAIL\n`/`ERROR\n` responses.
- Maintain an in-memory dependency graph with forward and reverse relationships, enforcing: index only when all deps are indexed; remove only when no packages depend on the target; query returns existence.
- The repo contains `INSTRUCTIONS.md`, a harness binary set, and a `source.tar.gz` with Go harness sources.

## Areas of clear three-way agreement

- **Language**: Go with standard library only.
- **Concurrency model**: One goroutine per client connection; shared in-memory state protected by a lock.
- **Core data structures**: Maps/sets for dependencies and reverse dependencies.
- **Re-index semantics**: Replacing a package’s dependencies on `INDEX` (not merging previous deps).
- **Removal semantics**: `OK` if not indexed; `FAIL` if there are dependents; otherwise remove and clean up reverse edges.
- **Protocol rigor**: Strict `cmd|pkg|deps\n` framing and responses; malformed lines yield `ERROR\n` but do not crash the server.
- **Testing and delivery**: Provide unit and integration tests; pass the official harness up to concurrency=100; include a Dockerfile and basic build/run docs.

## Two-way alignments with one divergence

1) Line framing method
- **Two**: Use `bufio.Scanner` for line-by-line reads (Claude, Gemini).
- **One**: Use `bufio.Reader.ReadString('\n')` (GPT-5 plan).
- **Recommendation**: Prefer `bufio.Reader.ReadString('\n')`.
  - **Why**: `Scanner` has a token-size cap and implicit tokenization rules; `Reader` is explicit, avoids surprises, and better matches a protocol framed solely by `\n`. Risk of unusually long lines is low but non-zero; explicit reads are safer.

2) Explicit presence set vs implicit presence
- **Two**: Maintain explicit presence (`indexed map`) separate from deps (GPT-5 plan, implied in Claude’s semantics through helper methods).
- **One**: Presence inferred by membership in deps map (Gemini hints at forward map as primary presence source).
- **Recommendation**: Maintain an explicit `indexed` set alongside `deps` and `revDeps`.
  - **Why**: Avoids ambiguity for packages with zero dependencies; simplifies `QUERY` and `REMOVE` logic, keeps invariants straightforward.

3) Locking strategy evolution
- **Two**: Start with single `sync.RWMutex`; consider sharding only if profiling shows contention (Claude, GPT-5 plan).
- **One**: Single `sync.RWMutex` with no forward-looking plan for sharding (Gemini).
- **Recommendation**: Adopt “single lock first, shard if needed” strategy.
  - **Why**: Minimizes complexity and deadlock risk initially; leaves room for performance tuning without overengineering.

4) Input validation strictness
- **Two**: Emphasize structural validation (exactly two `|`, empty deps allowed), minimal name validation (Gemini, GPT-5 plan).
- **One**: Adds additional checks like `isValidPackageName` (Claude).
- **Recommendation**: Enforce only what the spec requires: structure, known commands, and newline termination.
  - **Why**: Over-validation risks false negatives vs. harness expectations; spec does not define allowed character set.

5) Observability surface
- **Two**: Minimal logging and optional development counters; keep surface small (Gemini, GPT-5 plan).
- **One**: Broader ops extras (e.g., Docker Compose, health endpoints) (Claude).
- **Recommendation**: Keep observability minimal for submission; add only what aids debugging without violating the stdlib constraint.
  - **Why**: The harness does not require extended ops features; focus on correctness, robustness, and simplicity.

## Areas of three-way divergence (material differences)

No material three-way divergence exists on the core algorithm or architecture. Minor differences are stylistic or depth of non-functional scope (e.g., timeline estimates, documentation emphasis). All converge on the same correctness model and concurrency approach.

## Recommended unified plan (decision record)

- **Language/runtime**: Go, standard library only.
- **Network**: TCP server on `:8080`, goroutine per connection, per-connection read loop with `bufio.Reader.ReadString('\n')` framing.
- **State**: Three structures under one `sync.RWMutex` initially:
  - `indexed: map[string]bool`
  - `deps: map[string]map[string]struct{}`
  - `revDeps: map[string]map[string]struct{}`
- **INDEX**: With write lock, verify all deps are in `indexed`; replace `deps[pkg]` with new set; update `revDeps` (remove old backrefs not in new set; add new); set `indexed[pkg]=true`; return `OK` if deps satisfied, else `FAIL`.
- **REMOVE**: With write lock, if not `indexed[pkg]` return `OK`; if `revDeps[pkg]` non-empty return `FAIL`; otherwise remove from `indexed`, remove `deps[pkg]`, and delete reverse links.
- **QUERY**: With read lock, return `OK` iff `indexed[pkg]`.
- **Parsing**: Strict structural parsing; exactly two `|`; empty deps allowed; deps split on `,`; trim only trailing `\n`.
- **Error paths**: Any structural or command errors return `ERROR\n` and continue; write errors close connection.
- **Testing**:
  - Unit tests for index transitions, re-index replacement, reverse-deps maintenance, and parsing edge cases.
  - Concurrency tests with mixed operations asserting invariants.
  - Integration tests with real TCP sockets.
  - Harness validation for multiple seeds and concurrency up to 100.
- **Performance**: Ship with single `RWMutex`; only introduce sharded locks if profiling against the harness shows lock contention hotspots.
- **DX/Packaging**: Keep README, Makefile, Dockerfile minimal and oriented to the harness and Ubuntu build constraint.

## Rationale highlights

- Favor explicitness where it reduces ambiguity (explicit `indexed` presence, explicit Reader-based framing).
- Avoid speculative complexity (sharded locks) unless measurements show need.
- Constrain validation to the spec to minimize incompatibilities with the harness.
- Keep the non-functional scope lean to optimize engineering time for correctness and robustness.

## Risks and mitigations

- **Race conditions**: Single coarse `RWMutex`; validate with `-race` and concurrency tests.
- **Deadlocks (if sharded later)**: Enforce deterministic lock ordering by shard id; document clearly.
- **Scanner token limit (if used)**: Avoided by selecting `bufio.Reader`.
- **Protocol drift**: Centralize wire parsing and response mapping; unit test malformed cases extensively.

## Acceptance criteria (for the unified plan)

- All harness tests pass with `--concurrency=100` across multiple seeds; repeated runs are stable.
- Local test suite passes with `go test ./... -race`.
- Docker build and run on Ubuntu latest succeeds; no non-stdlib deps in production code.
- README/Makefile provide a trivial, reproducible run path for reviewers.


