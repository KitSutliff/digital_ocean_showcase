package integration

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"

	"package-indexer/internal/server"
)

// testClient represents a test client connection
type testClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

// newTestClient creates a new test client connected to the server
func newTestClient(addr string) (*testClient, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &testClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// sendCommand sends a command to the server and returns the response
func (c *testClient) sendCommand(cmd string) (string, error) {
	// Send command
	if _, err := fmt.Fprintf(c.conn, "%s\n", cmd); err != nil {
		return "", err
	}

	// Read response
	response, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return response, nil
}

// close closes the client connection
func (c *testClient) close() error {
	return c.conn.Close()
}

// startTestServer starts a server in a goroutine for testing
func startTestServer(addr string) {
	srv := server.NewServer(addr)
	go func() {
		if err := srv.Start(); err != nil {
			panic(fmt.Sprintf("Test server failed: %v", err))
		}
	}()
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
}

func TestServer_BasicOperations(t *testing.T) {
	// Start test server on different port to avoid conflicts
	testAddr := ":9080"
	startTestServer(testAddr)

	// Connect test client
	client, err := newTestClient(testAddr)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer client.close()

	// Test basic INDEX command
	resp, err := client.sendCommand("INDEX|base|")
	if err != nil {
		t.Fatalf("Failed to send INDEX command: %v", err)
	}
	if resp != "OK\n" {
		t.Errorf("Expected OK response, got: %q", resp)
	}

	// Test QUERY command
	resp, err = client.sendCommand("QUERY|base|")
	if err != nil {
		t.Fatalf("Failed to send QUERY command: %v", err)
	}
	if resp != "OK\n" {
		t.Errorf("Expected OK response for indexed package, got: %q", resp)
	}

	// Test INDEX with dependencies
	resp, err = client.sendCommand("INDEX|app|base")
	if err != nil {
		t.Fatalf("Failed to send INDEX command: %v", err)
	}
	if resp != "OK\n" {
		t.Errorf("Expected OK response for valid dependencies, got: %q", resp)
	}

	// Test INDEX with missing dependencies
	resp, err = client.sendCommand("INDEX|invalid|missing")
	if err != nil {
		t.Fatalf("Failed to send INDEX command: %v", err)
	}
	if resp != "FAIL\n" {
		t.Errorf("Expected FAIL response for missing dependencies, got: %q", resp)
	}

	// Test REMOVE blocked by dependents
	resp, err = client.sendCommand("REMOVE|base|")
	if err != nil {
		t.Fatalf("Failed to send REMOVE command: %v", err)
	}
	if resp != "FAIL\n" {
		t.Errorf("Expected FAIL response for package with dependents, got: %q", resp)
	}

	// Test REMOVE successful
	resp, err = client.sendCommand("REMOVE|app|")
	if err != nil {
		t.Fatalf("Failed to send REMOVE command: %v", err)
	}
	if resp != "OK\n" {
		t.Errorf("Expected OK response for valid removal, got: %q", resp)
	}
}

func TestServer_ProtocolErrors(t *testing.T) {
	// Start test server
	testAddr := ":9081"
	startTestServer(testAddr)

	// Connect test client
	client, err := newTestClient(testAddr)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer client.close()

	// Test malformed commands
	malformedCmds := []string{
		"INVALID|package|",       // Unknown command
		"INDEX||",                // Empty package name  
		"INDEX",                  // Missing parts
		"INDEX|package",          // Missing third part
		"INDEX|package|deps|extra", // Too many parts
	}

	for _, cmd := range malformedCmds {
		resp, err := client.sendCommand(cmd)
		if err != nil {
			t.Fatalf("Failed to send command %q: %v", cmd, err)
		}
		if resp != "ERROR\n" {
			t.Errorf("Expected ERROR response for malformed command %q, got: %q", cmd, resp)
		}
	}
}

func TestServer_ConcurrentClients(t *testing.T) {
	// Start test server
	testAddr := ":9082"
	startTestServer(testAddr)

	numClients := 10
	commandsPerClient := 20

	// Channel to collect results
	results := make(chan error, numClients)

	// Worker function for each client
	worker := func(clientID int) {
		client, err := newTestClient(testAddr)
		if err != nil {
			results <- fmt.Errorf("client %d: failed to connect: %v", clientID, err)
			return
		}
		defer client.close()

		// Each client performs a series of operations
		for i := 0; i < commandsPerClient; i++ {
			pkgName := fmt.Sprintf("pkg-%d-%d", clientID, i)

			// INDEX package
			resp, err := client.sendCommand(fmt.Sprintf("INDEX|%s|", pkgName))
			if err != nil {
				results <- fmt.Errorf("client %d: INDEX failed: %v", clientID, err)
				return
			}
			if resp != "OK\n" {
				results <- fmt.Errorf("client %d: expected OK for INDEX, got: %q", clientID, resp)
				return
			}

			// QUERY package
			resp, err = client.sendCommand(fmt.Sprintf("QUERY|%s|", pkgName))
			if err != nil {
				results <- fmt.Errorf("client %d: QUERY failed: %v", clientID, err)
				return
			}
			if resp != "OK\n" {
				results <- fmt.Errorf("client %d: expected OK for QUERY, got: %q", clientID, resp)
				return
			}

			// REMOVE package
			resp, err = client.sendCommand(fmt.Sprintf("REMOVE|%s|", pkgName))
			if err != nil {
				results <- fmt.Errorf("client %d: REMOVE failed: %v", clientID, err)
				return
			}
			if resp != "OK\n" {
				results <- fmt.Errorf("client %d: expected OK for REMOVE, got: %q", clientID, resp)
				return
			}
		}

		results <- nil // Success
	}

	// Start all clients
	for i := 0; i < numClients; i++ {
		go worker(i)
	}

	// Collect results
	for i := 0; i < numClients; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent client test failed: %v", err)
		}
	}
}
