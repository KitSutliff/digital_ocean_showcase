package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"package-indexer/internal/wire"
)

// waitFor waits until the predicate returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, pred func() bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for condition after %v", timeout)
}

func TestHandleConnection_ProcessAndShutdown(t *testing.T) {
	s := NewServer(":0")
	s.ctx, s.cancel = context.WithCancel(context.Background())

	client, server := net.Pipe()
	defer client.Close()

	// Account for wg.Done() in handleConnection
	s.wg.Add(1)
	go s.handleConnection(server)

	// Send a valid command and expect OK
	if _, err := client.Write([]byte("INDEX|pkg|\n")); err != nil {
		t.Fatalf("failed to write command: %v", err)
	}

	resp, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if resp != wire.OK.String() {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Trigger graceful shutdown and ensure goroutine exits
	s.cancel()
	_ = client.Close()
	waitFor(t, 2*time.Second, func() bool {
		c := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(c)
		}()
		select {
		case <-c:
			return true
		case <-time.After(20 * time.Millisecond):
			return false
		}
	})
}

func TestStartWithContext_GracefulShutdown(t *testing.T) {
	s := NewServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.StartWithContext(ctx) }()

	// Wait for the server to be ready
	<-s.ready
	addr := s.listener.Addr().String()

	// Connect a client to ensure at least one Accept succeeds
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("QUERY|nope|\n")); err != nil {
		t.Fatalf("failed to write query: %v", err)
	}
	if resp, err := bufio.NewReader(conn).ReadString('\n'); err != nil {
		t.Fatalf("failed to read response: %v", err)
	} else if resp != wire.FAIL.String() {
		t.Fatalf("expected FAIL response, got %q", resp)
	}

	// Close client connection first to allow handler to exit promptly
	_ = conn.Close()

	// Initiate graceful shutdown (this also cancels server context and closes listener)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}

	// StartWithContext should return nil after shutdown
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartWithContext returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for StartWithContext to return")
	}
}

func TestShutdown_TimeoutWhenConnectionsHung(t *testing.T) {
	s := NewServer(":0")
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Simulate a stuck connection by incrementing wg without a matching Done
	s.wg.Add(1)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := s.Shutdown(shutdownCtx); err == nil {
		t.Fatalf("expected shutdown to time out, but got nil error")
	}
}

func TestNewServer(t *testing.T) {
	addr := ":8080"
	srv := NewServer(addr)

	if srv == nil {
		t.Fatal("NewServer should return a non-nil server")
	}

	if srv.addr != addr {
		t.Errorf("Expected address %s, got %s", addr, srv.addr)
	}

	if srv.indexer == nil {
		t.Error("Server should have a non-nil indexer")
	}
}

func TestServer_ProcessCommand(t *testing.T) {
	srv := NewServer(":8080")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	tests := []struct {
		name     string
		input    string
		expected wire.Response
	}{
		{
			name:     "Valid INDEX command with no dependencies",
			input:    "INDEX|test|\n",
			expected: wire.OK,
		},
		{
			name:     "Valid QUERY command for indexed package",
			input:    "QUERY|test|\n",
			expected: wire.OK,
		},
		{
			name:     "Valid QUERY command for non-existent package",
			input:    "QUERY|nonexistent|\n",
			expected: wire.FAIL,
		},
		{
			name:     "Valid INDEX command with dependencies that exist",
			input:    "INDEX|app|test\n",
			expected: wire.OK,
		},
		{
			name:     "Valid INDEX command with missing dependencies",
			input:    "INDEX|invalid|missing\n",
			expected: wire.FAIL,
		},
		{
			name:     "Valid REMOVE command for package with no dependents",
			input:    "REMOVE|app|\n",
			expected: wire.OK,
		},
		{
			name:     "Valid REMOVE command for package without dependents",
			input:    "REMOVE|test|\n",
			expected: wire.OK, // Test package has no dependents in fresh server
		},
		{
			name:     "Valid REMOVE command for non-existent package",
			input:    "REMOVE|nonexistent|\n",
			expected: wire.OK, // Should be OK (idempotent)
		},
		{
			name:     "Invalid command format - missing newline",
			input:    "INDEX|test|",
			expected: wire.ERROR,
		},
		{
			name:     "Invalid command format - too few parts",
			input:    "INDEX|test\n",
			expected: wire.ERROR,
		},
		{
			name:     "Invalid command format - too many parts",
			input:    "INDEX|test||extra\n",
			expected: wire.ERROR,
		},
		{
			name:     "Invalid command type",
			input:    "INVALID|test|\n",
			expected: wire.ERROR,
		},
		{
			name:     "Empty package name",
			input:    "INDEX||\n",
			expected: wire.ERROR,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Reset server state for clean test
			srv = NewServer(":8080")

			// For tests that depend on pre-existing state, set it up
			if strings.Contains(test.input, "QUERY|test|") && test.expected == wire.OK {
				srv.processCommand(logger, "INDEX|test|\n")
			}
			if strings.Contains(test.input, "INDEX|app|test") {
				srv.processCommand(logger, "INDEX|test|\n")
			}
			// For REMOVE tests on existing packages, set them up first
			if strings.Contains(test.input, "REMOVE|test|") && test.expected == wire.OK {
				srv.processCommand(logger, "INDEX|test|\n")
			}
			if strings.Contains(test.input, "REMOVE|app|") && test.expected == wire.OK {
				srv.processCommand(logger, "INDEX|app|\n")
			}

			result := srv.processCommand(logger, test.input)
			if result != test.expected {
				t.Errorf("processCommand(%q) = %v, expected %v", test.input, result, test.expected)
			}
		})
	}
}

func TestServer_ProcessCommand_StatefulOperations(t *testing.T) {
	srv := NewServer(":8080")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Test sequence: INDEX -> QUERY -> INDEX with deps -> REMOVE with deps -> REMOVE

	// 1. Index base package
	result := srv.processCommand(logger, "INDEX|base|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for indexing base package, got %v", result)
	}

	// 2. Query base package
	result = srv.processCommand(logger, "QUERY|base|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for querying indexed package, got %v", result)
	}

	// 3. Index app with base dependency
	result = srv.processCommand(logger, "INDEX|app|base\n")
	if result != wire.OK {
		t.Errorf("Expected OK for indexing app with valid dependency, got %v", result)
	}

	// 4. Try to remove base (should fail - app depends on it)
	result = srv.processCommand(logger, "REMOVE|base|\n")
	if result != wire.FAIL {
		t.Errorf("Expected FAIL for removing package with dependents, got %v", result)
	}

	// 5. Remove app first
	result = srv.processCommand(logger, "REMOVE|app|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for removing app, got %v", result)
	}

	// 6. Now remove base (should succeed)
	result = srv.processCommand(logger, "REMOVE|base|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for removing base after dependents removed, got %v", result)
	}

	// 7. Query removed package
	result = srv.processCommand(logger, "QUERY|base|\n")
	if result != wire.FAIL {
		t.Errorf("Expected FAIL for querying removed package, got %v", result)
	}
}

func TestServer_ProcessCommand_Reindexing(t *testing.T) {
	srv := NewServer(":8080")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Set up dependencies
	srv.processCommand(logger, "INDEX|dep1|\n")
	srv.processCommand(logger, "INDEX|dep2|\n")
	srv.processCommand(logger, "INDEX|app|dep1\n")

	// Re-index with different dependencies
	result := srv.processCommand(logger, "INDEX|app|dep2\n")
	if result != wire.OK {
		t.Errorf("Expected OK for re-indexing with different dependencies, got %v", result)
	}

	// Verify old dependency can be removed (dep1 should be removable)
	result = srv.processCommand(logger, "REMOVE|dep1|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for removing old dependency, got %v", result)
	}

	// Verify new dependency cannot be removed (dep2 should not be removable)
	result = srv.processCommand(logger, "REMOVE|dep2|\n")
	if result != wire.FAIL {
		t.Errorf("Expected FAIL for removing current dependency, got %v", result)
	}
}

func TestServer_Start_InvalidAddress(t *testing.T) {
	// Test with invalid address that should fail to bind
	srv := NewServer("invalid-address:999999")

	// Start should return an error for invalid address
	err := srv.Start()
	if err == nil {
		t.Error("Expected Start() to return error for invalid address")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected error message to contain 'failed to listen', got: %v", err)
	}
}

func TestServer_Start_PortAlreadyInUse(t *testing.T) {
	// Create a listener on a port to simulate "already in use"
	listener, err := net.Listen("tcp", ":0") // Let OS choose port
	if err != nil {
		t.Fatalf("Failed to create test listener: %v", err)
	}
	defer listener.Close()

	// Get the address that's now in use
	addr := listener.Addr().String()

	// Try to start server on the same address
	srv := NewServer(addr)

	// This should fail since the port is already in use
	err = srv.Start()
	if err == nil {
		t.Error("Expected Start() to return error for port already in use")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected error message to contain 'failed to listen', got: %v", err)
	}
}

// Note: Testing handleConnection is complex due to its blocking nature and goroutine usage.
// The existing integration tests in tests/integration/server_test.go provide comprehensive
// coverage of the full connection handling logic with real TCP connections.
// For unit testing, we focus on the processCommand logic which is the core business logic.

func TestServer_ProcessCommand_EdgeCases(t *testing.T) {
	srv := NewServer(":8080")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Test with dependencies containing empty strings (trailing commas)
	result := srv.processCommand(logger, "INDEX|test|dep1,,dep2,\n")
	if result != wire.FAIL {
		t.Errorf("Expected FAIL for missing dependencies, got %v", result)
	}

	// Index the dependencies first
	srv.processCommand(logger, "INDEX|dep1|\n")
	srv.processCommand(logger, "INDEX|dep2|\n")

	// Now it should work
	result = srv.processCommand(logger, "INDEX|test|dep1,,dep2,\n")
	if result != wire.OK {
		t.Errorf("Expected OK after dependencies are indexed, got %v", result)
	}

	// Test with complex dependency chains
	srv.processCommand(logger, "INDEX|base|\n")
	srv.processCommand(logger, "INDEX|mid|base\n")
	srv.processCommand(logger, "INDEX|top|mid\n")

	// Try to remove base (should fail - mid depends on it)
	result = srv.processCommand(logger, "REMOVE|base|\n")
	if result != wire.FAIL {
		t.Errorf("Expected FAIL for removing base of dependency chain, got %v", result)
	}

	// Remove in correct order
	srv.processCommand(logger, "REMOVE|top|\n")
	srv.processCommand(logger, "REMOVE|mid|\n")
	result = srv.processCommand(logger, "REMOVE|base|\n")
	if result != wire.OK {
		t.Errorf("Expected OK for removing base after chain is dismantled, got %v", result)
	}
}
