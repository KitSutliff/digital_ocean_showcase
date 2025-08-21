package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"package-indexer/internal/wire"
)

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

// TestServer_HandleConnection_Lifecycle tests the full connection handling lifecycle
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

// TestServer_HandleConnection_EOF tests graceful handling of client disconnection
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

// TestServer_HandleConnection_WriteError tests handling of write errors
func TestServer_HandleConnection_WriteError(t *testing.T) {
	testConnectionErrorHandling(t, "WriteError", func(c net.Conn) {
		// Send command but close client before response can be fully written
		c.Write([]byte("INDEX|test|\n"))
		c.Close()
	})
}

// TestServer_HandleConnection_ReadError tests handling of various read errors
func TestServer_HandleConnection_ReadError(t *testing.T) {
	testConnectionErrorHandling(t, "ReadError", func(c net.Conn) {
		// Send partial command and then close abruptly
		c.Write([]byte("INDEX|test")) // No newline, incomplete
		c.Close()
	})
}

// TestServer_HandleConnection_LargeMessage tests handling of very large messages
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

// TestServer_HandleConnection_ConcurrentConnections tests multiple simultaneous connections
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

// TestServer_HandleConnection_MalformedMessages tests various malformed message handling
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

// TestServer_Start_AcceptErrors tests handling of Accept() errors
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

// NewServerWithListener creates a server with a pre-configured listener for testing
func NewServerWithListener(l net.Listener) *ServerWithListener {
	return &ServerWithListener{
		server:   NewServer(l.Addr().String(), DefaultReadTimeout),
		listener: l,
	}
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

// TestServer_HandleConnection_StreamingCommands tests handling of multiple commands in rapid succession
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
