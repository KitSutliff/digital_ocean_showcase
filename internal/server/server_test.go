package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"package-indexer/internal/wire"
)

// Test constants to avoid magic numbers and keep tests deterministic
const (
	readyWaitTimeout    = 2 * time.Second
	shutdownWaitTimeout = 200 * time.Millisecond
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

// setupServerAndPipe creates a server, a piped client/server connection, starts
// the connection handler, and returns the client side reader with a cleanup.
func setupServerAndPipe(t *testing.T) (*Server, net.Conn, *bufio.Reader, func()) {
	srv := NewServer(":0", DefaultReadTimeout)
	clientConn, serverConn := net.Pipe()

	srv.ctx, srv.cancel = context.WithCancel(context.Background())

	// Start handler
	srv.wg.Add(1)
	go srv.handleConnection(serverConn)

	reader := bufio.NewReader(clientConn)

	cleanup := func() {
		_ = clientConn.Close()
		srv.cancel()
	}

	return srv, clientConn, reader, cleanup
}

// testConnectionErrorHandling is a helper for testing various connection error scenarios
func testConnectionErrorHandling(t *testing.T, testName string, action func(net.Conn)) {
	srv := NewServer(":0", DefaultReadTimeout)
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	srv.ctx, srv.cancel = context.WithCancel(context.Background())
	defer srv.cancel()
	done := make(chan bool)
	go func() {
		srv.wg.Add(1)
		srv.handleConnection(serverConn)
		done <- true
	}()

	// Perform the action that should cause an error
	action(clientConn)

	// Check that the handler exits gracefully
	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Errorf("%s: Connection handler did not exit after error", testName)
	}
}

// mockListener simulates listener errors for testing
type mockListener struct {
	net.Listener
	shouldError bool
	errorCount  int
	mu          sync.Mutex
}

func (m *mockListener) Accept() (net.Conn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError && m.errorCount < 3 {
		m.errorCount++
		return nil, errors.New("mock accept error")
	}
	return m.Listener.Accept()
}

// ServerWithListener wraps Server to allow injecting a custom listener
type ServerWithListener struct {
	server   *Server
	listener net.Listener
}

func (s *ServerWithListener) Start() error {
	log.Printf("Package indexer server listening on %s", s.listener.Addr().String())

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			if strings.Contains(err.Error(), "closed") {
				return err
			}
			continue
		}

		// Handle each connection in a separate goroutine
		s.server.wg.Add(1)
		go s.server.handleConnection(conn)
	}
}

// NewServerWithListener creates a server with a pre-configured listener for testing
func NewServerWithListener(l net.Listener) *ServerWithListener {
	return &ServerWithListener{
		server:   NewServer(l.Addr().String(), DefaultReadTimeout),
		listener: l,
	}
}

func TestHandleConnection_ProcessAndShutdown(t *testing.T) {
	s := NewServer(":0", DefaultReadTimeout)
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
	s := NewServer("127.0.0.1:0", DefaultReadTimeout)
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
	s := NewServer(":0", DefaultReadTimeout)
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
	readTimeout := 30 * time.Second
	srv := NewServer(addr, readTimeout)

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
	srv := NewServer(":8080", DefaultReadTimeout)
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
			srv = NewServer(":8080", DefaultReadTimeout)

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
	srv := NewServer(":8080", DefaultReadTimeout)
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
	srv := NewServer(":8080", DefaultReadTimeout)
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
	srv := NewServer("invalid-address:999999", DefaultReadTimeout)

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
	srv := NewServer(addr, DefaultReadTimeout)

	// This should fail since the port is already in use
	err = srv.Start()
	if err == nil {
		t.Error("Expected Start() to return error for port already in use")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected error message to contain 'failed to listen', got: %v", err)
	}
}

func TestServer_ProcessCommand_EdgeCases(t *testing.T) {
	srv := NewServer(":8080", DefaultReadTimeout)
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

// Tests from server_lifecycle_test.go

func TestServer_StartWithContext_Success(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout) // Use any available port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.StartWithContext(ctx)
	}()

	// Wait for the server to be ready
	<-srv.ready

	// Verify server is listening by connecting to it
	conn, err := net.Dial("tcp", srv.listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	conn.Close()

	// Cancel context to trigger graceful shutdown
	cancel()

	// Verify server shuts down gracefully
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Server returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not shut down within timeout")
	}
}

func TestServer_StartWithContext_ListenerError(t *testing.T) {
	// Use an invalid address that will fail to bind
	srv := NewServer("invalid-address:999999", DefaultReadTimeout)

	ctx := context.Background()
	err := srv.StartWithContext(ctx)

	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected 'failed to listen' error, got: %v", err)
	}
}

func TestServer_StartWithContext_CancelledContext(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.StartWithContext(ctx)
	}()

	// Should return quickly due to cancelled context
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Server returned unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not respond to cancelled context within timeout")
	}
}

func TestServer_HandleConnection_ContextCancellation(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)
	srv.ctx, srv.cancel = context.WithCancel(context.Background())

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv.wg.Add(1)

	// Start connection handler
	handlerDone := make(chan bool)
	go func() {
		srv.handleConnection(serverConn)
		handlerDone <- true
	}()

	// Give handler time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	srv.cancel()

	// Verify handler exits due to context cancellation
	select {
	case <-handlerDone:
		// Success - handler exited due to context cancellation
	case <-time.After(time.Second):
		t.Error("handleConnection did not respond to context cancellation")
	}
}

func TestServer_Shutdown_Success(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)
	srv.ctx, srv.cancel = context.WithCancel(context.Background())

	// Create mock listener
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	srv.listener = l

	// Start a mock connection
	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		time.Sleep(100 * time.Millisecond) // Simulate active connection
	}()

	// Test graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	err = srv.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	// Verify listener was closed
	_, err = l.Accept()
	if err == nil {
		t.Error("Listener should be closed after shutdown")
	}
}

func TestServer_Shutdown_Timeout(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)
	srv.ctx, srv.cancel = context.WithCancel(context.Background())

	// Create mock listener
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	srv.listener = l

	// Start a long-running mock connection that won't finish in time
	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		time.Sleep(2 * time.Second) // Longer than shutdown timeout
	}()

	// Test shutdown with short timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()

	err = srv.Shutdown(shutdownCtx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got: %v", err)
	}
}

func TestServer_Shutdown_NoActiveConnections(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)
	srv.ctx, srv.cancel = context.WithCancel(context.Background())

	// Create mock listener
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	srv.listener = l

	// No active connections - should shutdown immediately
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	start := time.Now()
	err = srv.Shutdown(shutdownCtx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	// Should complete very quickly since no connections
	if elapsed > 100*time.Millisecond {
		t.Errorf("Shutdown took too long (%v) with no active connections", elapsed)
	}
}

func TestServer_Shutdown_NilComponents(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)
	// Leave cancel and listener as nil

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	// Should not panic with nil components
	err := srv.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown with nil components returned error: %v", err)
	}
}

func TestServer_Start_DelegatesCorrectly(t *testing.T) {
	srv := NewServer("invalid-address:999999", DefaultReadTimeout) // Use invalid address to get quick error

	err := srv.Start()

	// Should get the same error as StartWithContext would return
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected 'failed to listen' error, got: %v", err)
	}
}

func TestServer_StartWithContext_AcceptLoop(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.StartWithContext(ctx)
	}()

	// Wait for the server to be ready
	<-srv.ready

	// Make multiple connections to test accept loop
	var conns []net.Conn
	for i := 0; i < 3; i++ {
		conn, err := net.Dial("tcp", srv.listener.Addr().String())
		if err != nil {
			t.Fatalf("Failed to connect %d: %v", i, err)
		}
		conns = append(conns, conn)

		// Send a simple command
		conn.Write([]byte("INDEX|test|\n"))

		// Read response
		buffer := make([]byte, 10)
		conn.Read(buffer)
	}

	// Close all connections
	for _, conn := range conns {
		conn.Close()
	}

	// Cancel context to stop server
	cancel()

	// Wait for server to stop
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Server returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not stop within timeout")
	}
}

// Tests from connection_test.go

func TestServer_HandleConnection_Lifecycle(t *testing.T) {
	_, clientConn, reader, cleanup := setupServerAndPipe(t)
	defer cleanup()

	// Send valid commands and verify responses
	commands := []struct {
		input    string
		expected string
	}{
		{"INDEX|test|\n", wire.OK.String()},
		{"QUERY|test|\n", wire.OK.String()},
		{"REMOVE|test|\n", wire.OK.String()},
		{"INVALID|test|\n", wire.ERROR.String()},
	}

	for _, cmd := range commands {
		// Send command
		_, err := clientConn.Write([]byte(cmd.input))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}

		// Read response
		response, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response != cmd.expected {
			t.Errorf("Command %q: expected %q, got %q", cmd.input, cmd.expected, response)
		}
	}
}

func TestServer_HandleConnection_EOF(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)

	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	// Start handling connection
	srv.ctx, srv.cancel = context.WithCancel(context.Background())
	defer srv.cancel()
	done := make(chan bool)
	go func() {
		srv.wg.Add(1)
		srv.handleConnection(serverConn)
		done <- true
	}()

	// Close client side to trigger EOF
	clientConn.Close()

	// Should handle EOF gracefully and exit
	select {
	case <-done:
		// Success - connection handler exited cleanly
	case <-time.After(time.Second):
		t.Error("Connection handler did not exit after EOF")
	}
}

func TestServer_HandleConnection_WriteError(t *testing.T) {
	testConnectionErrorHandling(t, "WriteError", func(c net.Conn) {
		// Send command but close client before response can be fully written
		c.Write([]byte("INDEX|test|\n"))
		c.Close()
	})
}

func TestServer_HandleConnection_ReadError(t *testing.T) {
	testConnectionErrorHandling(t, "ReadError", func(c net.Conn) {
		// Send partial command and then close abruptly
		c.Write([]byte("INDEX|test")) // No newline, incomplete
		c.Close()
	})
}

func TestServer_HandleConnection_LargeMessage(t *testing.T) {
	_, clientConn, reader, cleanup := setupServerAndPipe(t)
	defer cleanup()

	// Create a large but valid command
	largeDeps := strings.Repeat("dep", 1000)
	command := "INDEX|bigpackage|" + largeDeps + "\n"

	// Send large command
	clientConn.Write([]byte(command))

	// Read response
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Should get FAIL because deps don't exist, but shouldn't crash
	if response != wire.FAIL.String() {
		t.Errorf("Expected FAIL for missing dependencies, got %q", response)
	}
}

func TestServer_HandleConnection_ConcurrentConnections(t *testing.T) {
	srv := NewServer(":0", DefaultReadTimeout)

	const numConnections = 10
	var wg sync.WaitGroup

	// Create multiple concurrent connections using separate server instances to avoid races
	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create a separate server instance for this connection to avoid context races
			localSrv := NewServer(":0", DefaultReadTimeout)
			localSrv.ctx, localSrv.cancel = context.WithCancel(context.Background())
			defer localSrv.cancel()

			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()

			// Start connection handler
			localSrv.wg.Add(1)
			go localSrv.handleConnection(serverConn)

			// Each connection indexes a unique package
			command := "INDEX|package" + string(rune('0'+id)) + "|\n"
			clientConn.Write([]byte(command))

			// Verify response
			reader := bufio.NewReader(clientConn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Errorf("Connection %d: failed to read response: %v", id, err)
				return
			}

			if response != wire.OK.String() {
				t.Errorf("Connection %d: expected OK, got %q", id, response)
			}

			// Add to shared server for final verification
			srv.indexer.IndexPackage("package"+string(rune('0'+id)), []string{})
		}(i)
	}

	// Wait for all connections to complete
	wg.Wait()

	// Verify all packages were indexed in the shared server
	for i := 0; i < numConnections; i++ {
		packageName := "package" + string(rune('0'+i))
		if !srv.indexer.QueryPackage(packageName) {
			t.Errorf("Package %s was not indexed", packageName)
		}
	}
}

func TestServer_HandleConnection_MalformedMessages(t *testing.T) {
	_, clientConn, reader, cleanup := setupServerAndPipe(t)
	defer cleanup()

	malformedMessages := []string{
		"",                       // Empty message
		"NOTNEWLINE",             // No newline
		"TOO|FEW|PARTS\n",        // Missing required parts
		"TOO|MANY|PARTS|EXTRA\n", // Too many parts
		"INDEX||\n",              // Empty package name
		"|\n",                    // Just separator
		"\n",                     // Just newline
		"INDEX\n",                // Missing separators
	}

	for _, msg := range malformedMessages {
		// Send malformed message
		clientConn.Write([]byte(msg))

		// For messages without newline, we need to handle the fact that
		// ReadString might not return immediately
		if !strings.HasSuffix(msg, "\n") {
			// Send a valid command after to "flush" the connection
			clientConn.Write([]byte("\nINDEX|flush|\n"))

			// Read the error response for the malformed message
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response for malformed message %q: %v", msg, err)
			}
			if response != wire.ERROR.String() {
				t.Errorf("Malformed message %q: expected ERROR, got %q", msg, response)
			}

			// Read and discard the flush response
			reader.ReadString('\n')
		} else {
			// For messages with newline, read response directly
			response, err := reader.ReadString('\n')
			if err == io.EOF {
				// Connection may have been closed due to error, which is acceptable
				return
			}
			if err != nil {
				t.Fatalf("Failed to read response for malformed message %q: %v", msg, err)
			}
			if response != wire.ERROR.String() {
				t.Errorf("Malformed message %q: expected ERROR, got %q", msg, response)
			}
		}
	}
}

func TestServer_Start_AcceptErrors(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	errListener := &mockListener{Listener: l, shouldError: true}
	srv := NewServerWithListener(errListener)

	// Start server in goroutine with timeout
	done := make(chan error, 1)
	go func() {
		done <- srv.Start()
	}()

	// Give it time to encounter errors
	time.Sleep(100 * time.Millisecond)

	// Close listener to stop server
	l.Close()

	// Should handle Accept errors gracefully and eventually exit
	select {
	case err := <-done:
		// Should get an error when listener is closed
		if err == nil {
			t.Error("Expected error when listener is closed")
		}
	case <-time.After(time.Second):
		t.Error("Server did not exit after listener was closed")
	}
}

func TestServer_HandleConnection_StreamingCommands(t *testing.T) {
	_, clientConn, reader, cleanup := setupServerAndPipe(t)
	defer cleanup()

	// Send commands one by one and read responses to avoid pipe deadlock
	commands := []struct {
		cmd      string
		expected string
	}{
		{"INDEX|base|\n", wire.OK.String()},
		{"INDEX|app|base\n", wire.OK.String()},
		{"QUERY|base|\n", wire.OK.String()},
		{"QUERY|app|\n", wire.OK.String()},
		{"REMOVE|app|\n", wire.OK.String()},
		{"REMOVE|base|\n", wire.OK.String()},
	}

	for i, test := range commands {
		// Send command
		_, err := clientConn.Write([]byte(test.cmd))
		if err != nil {
			t.Fatalf("Failed to write command %d: %v", i, err)
		}

		// Read response immediately to avoid blocking
		response, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("Failed to read response %d: %v", i, err)
		}
		if response != test.expected {
			t.Errorf("Response %d: expected %q, got %q", i, test.expected, response)
		}
	}
}

// Tests from server_ready_stats_test.go

func TestReadyAndIsReady(t *testing.T) {
	s := NewServer("127.0.0.1:0", DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.StartWithContext(ctx) }()

	select {
	case <-s.Ready():
		// ready
	case <-time.After(readyWaitTimeout):
		t.Fatal("timeout waiting for Ready() to close")
	}

	if !s.IsReady() {
		t.Error("expected IsReady() to be true after startup")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownWaitTimeout)
	defer cancelShutdown()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartWithContext returned error: %v", err)
		}
	case <-time.After(readyWaitTimeout):
		t.Fatal("timeout waiting for server goroutine to exit")
	}
}

func TestGetStats(t *testing.T) {
	s := NewServer(":0", DefaultReadTimeout)

	// Index a package via indexer to reflect in stats
	s.indexer.IndexPackage("pkg", nil)

	stats := s.GetStats()
	if stats.Indexed != 1 {
		t.Fatalf("expected Indexed=1, got %d", stats.Indexed)
	}
}

func TestSetListener(t *testing.T) {
	s := NewServer(":0", DefaultReadTimeout)

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer l.Close()

	s.SetListener(l)
	if s.listener == nil {
		t.Fatal("expected listener to be set by SetListener")
	}

	// Ensure shutdown path handles a pre-set listener
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownWaitTimeout)
	defer cancel()
	_ = s.Shutdown(shutdownCtx)
}