package server

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// TestServer_StartWithContext_Success tests successful server startup
func TestServer_StartWithContext_Success(t *testing.T) {
	srv := NewServer(":0") // Use any available port

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

// TestServer_StartWithContext_ListenerError tests handling of listener creation errors
func TestServer_StartWithContext_ListenerError(t *testing.T) {
	// Use an invalid address that will fail to bind
	srv := NewServer("invalid-address:999999")

	ctx := context.Background()
	err := srv.StartWithContext(ctx)

	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected 'failed to listen' error, got: %v", err)
	}
}

// TestServer_StartWithContext_CancelledContext tests behavior with pre-cancelled context
func TestServer_StartWithContext_CancelledContext(t *testing.T) {
	srv := NewServer(":0")

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

// TestServer_HandleConnection_ContextCancellation tests graceful shutdown via context
func TestServer_HandleConnection_ContextCancellation(t *testing.T) {
	srv := NewServer(":0")
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

// TestServer_Shutdown_Success tests successful graceful shutdown
func TestServer_Shutdown_Success(t *testing.T) {
	srv := NewServer(":0")
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

// TestServer_Shutdown_Timeout tests shutdown timeout behavior
func TestServer_Shutdown_Timeout(t *testing.T) {
	srv := NewServer(":0")
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

// TestServer_Shutdown_NoActiveConnections tests shutdown with no active connections
func TestServer_Shutdown_NoActiveConnections(t *testing.T) {
	srv := NewServer(":0")
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

// TestServer_Shutdown_NilComponents tests shutdown with nil components
func TestServer_Shutdown_NilComponents(t *testing.T) {
	srv := NewServer(":0")
	// Leave cancel and listener as nil

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	// Should not panic with nil components
	err := srv.Shutdown(shutdownCtx)
	if err != nil {
		t.Errorf("Shutdown with nil components returned error: %v", err)
	}
}

// TestServer_Start_DelegatesCorrectly tests that Start properly delegates to StartWithContext
func TestServer_Start_DelegatesCorrectly(t *testing.T) {
	srv := NewServer("invalid-address:999999") // Use invalid address to get quick error

	err := srv.Start()

	// Should get the same error as StartWithContext would return
	if err == nil {
		t.Error("Expected error for invalid address, got nil")
	}

	if !strings.Contains(err.Error(), "failed to listen") {
		t.Errorf("Expected 'failed to listen' error, got: %v", err)
	}
}

// TestServer_StartWithContext_AcceptLoop tests the accept loop behavior
func TestServer_StartWithContext_AcceptLoop(t *testing.T) {
	srv := NewServer(":0")

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
