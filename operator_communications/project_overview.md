# DigitalOcean Package Indexer Challenge - Plain Talk Overview

## What This Project Is

This is DigitalOcean's coding test. They want you to build a **package manager server** - think of it like a librarian who keeps track of which books depend on other books.

## The Four Main Parts

### 1. **The Test Harness Executables** (`do-package-tree_*`)
These are like **grade checkers**. There's one for each operating system:
- `do-package-tree_darwin` (Mac - 8.1MB)
- `do-package-tree_linux` (8.2MB)
- `do-package-tree_freebsd` (8.2MB) 
- `do-package-tree_windows` (8.3MB)

Each one will hammer your server with thousands of requests to see if it breaks. They test both correctness (does it work?) and toughness (does it survive 100 angry clients at once?).

### 2. **The Source Code** (`source.tar.gz`)
This contains the **actual code** that builds those test programs. It's written in Go and includes:
- Real package data from Homebrew (1000+ packages with real dependencies)
- The exact testing logic that will grade your work
- Think of it as the answer key - you can peek to understand exactly how you'll be graded

**What's inside:**
```
test-suite/client.go
test-suite/client_test.go  
test-suite/main.go
test-suite/packages.go
test-suite/packages_test.go
test-suite/test_run.go
test-suite/test_run_test.go
test-suite/wire_format.go
test-suite/wire_format_test.go
```

### 3. **The Instructions** (`INSTRUCTIONS.md`)
The rulebook. Build a TCP server on port 8080 that speaks a simple protocol:
- `INDEX|package|dependencies\n` - "Add this package"
- `REMOVE|package|\n` - "Delete this package" 
- `QUERY|package|\n` - "Is this package here?"

**The catch**: 
- You can only add a package if ALL its dependencies are already there
- You can only remove a package if NOTHING depends on it
- Respond with exactly: `OK\n`, `FAIL\n`, or `ERROR\n`

### 4. **The AI Planning Documents** (`multi_agent_communication/`)
Someone already had three different AIs analyze this project:
- **Claude plan** (374 lines) - Very thorough, includes architecture diagrams
- **Gemini plan** (159 lines) - Clean step-by-step approach
- **GPT-5 plan** (265 lines) - Detailed with executive summary

All three recommend Go as the implementation language.

## The Version
`v1.0 - Tue Jan 23 15:32:15 UTC 2024` - This test is from January 2024.

## What's Missing (The Work Needed)

**The big thing**: There's **no actual server code**. These folders are empty:
- `src/` - Empty
- `scripts/` - Empty  
- `tests/` - Empty

You need to build everything from scratch.

**The goal**: Make the test harness print "All tests passed!" when you run it.

## Potential Land Mines

### 1. **Concurrency Nightmares**
The test will hit you with 100 simultaneous clients. One wrong move with shared data and you get race conditions that are hard to debug.

### 2. **Protocol Pickiness** 
The exact format matters:
- `OK\n` not `ok\n` or `OK` 
- Must handle malformed messages gracefully
- Every response needs that newline

### 3. **Dependency Logic Traps**
The rules are strict:
- If package A needs B, and you try to add A before B exists → `FAIL`
- If you try to remove B while A still needs it → `FAIL`
- Updating a package's dependencies replaces the old list entirely

### 4. **Standard Library Only**
You can't use any fancy packages. Just what comes with your chosen language (Go recommended).

### 5. **The Test is Sneaky**
- Sends broken messages on purpose to see if your server crashes
- Uses real-world Homebrew package data with complex dependency chains
- Tests with random seeds to catch timing bugs
- Connects/disconnects clients randomly

### 6. **Empty Folders Suspicion**
The `src/`, `scripts/`, and `tests/` folders being empty might indicate:
- Incomplete package extraction
- This is a truly green-field project
- Missing some expected starter code

## What the Test Harness Does

Based on the AI analysis of the source code:

1. **Cleanup Phase**: Tries to remove all packages (handles previous failed runs)
2. **Brute-Force Indexing**: Repeatedly tries to add packages until dependencies work out
3. **Verification**: Queries all packages expecting `OK`
4. **Removal Phase**: Removes packages in dependency-safe order  
5. **Final Check**: Queries expecting `FAIL` responses

It also throws in "unluckiness" - random broken messages and disconnections.

## Recommended Next Steps

1. **Extract and examine** the `source.tar.gz` to understand the test strategy
2. **Pick Go** as implementation language (all AI plans recommend it)
3. **Start simple**: Core dependency graph logic first
4. **Build up**: Add TCP server, protocol handling, concurrency
5. **Test early**: Run the harness frequently during development

## Success Definition

When you run `./do-package-tree_darwin`, you should see:
```
================
All tests passed!
================
```

That means your server correctly handles the package dependency logic under high load without crashing or corrupting data.

## The Bottom Line

You're building a **dependency-aware package index** that can handle serious concurrent load. The test harness is your judge and jury. The AI plans are solid roadmaps, but you still need to write every line of code.

The challenge tests real-world skills: network programming, concurrency safety, protocol design, and handling misbehaving clients. It's a solid test of production engineering capabilities.
