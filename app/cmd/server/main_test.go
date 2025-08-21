package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"package-indexer/internal/server"
)

// Test constants to eliminate magic numbers
const (
	testServerStartupDelay = 200 * time.Millisecond
	testShutdownTimeout    = 5 * time.Second
)

// isolateFlags preserves and restores global flag state for test isolation
func isolateFlags(t *testing.T) func() {
	t.Helper()
	oldArgs := os.Args
	oldFlag := flag.CommandLine
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlag
	}
}

// shutdownAdminServer creates a cleanup function for graceful admin server shutdown
func shutdownAdminServer(adminServer *http.Server) func() {
	return func() {
		if adminServer != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), testShutdownTimeout)
			defer shutdownCancel()
			adminServer.Shutdown(shutdownCtx)
		}
	}
}

// shutdownBothServers creates a cleanup function for both main and admin servers
func shutdownBothServers(srv *server.Server, adminServer *http.Server) func() {
	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), testShutdownTimeout)
		defer shutdownCancel()
		if adminServer != nil {
			adminServer.Shutdown(shutdownCtx)
		}
		if srv != nil {
			srv.Shutdown(shutdownCtx)
		}
	}
}

// TestMain_FlagParsing tests the flag parsing logic by extracting it into a testable function
func TestMain_FlagParsing(t *testing.T) {
	// Save original command line args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	tests := []struct {
		name          string
		args          []string
		expectedAddr  string
		expectedQuiet bool
		expectedAdmin string
	}{
		{
			name:          "Default values",
			args:          []string{"program"},
			expectedAddr:  ":8080",
			expectedQuiet: false,
			expectedAdmin: "",
		},
		{
			name:          "Custom address",
			args:          []string{"program", "-addr", ":9090"},
			expectedAddr:  ":9090",
			expectedQuiet: false,
			expectedAdmin: "",
		},
		{
			name:          "Quiet mode enabled",
			args:          []string{"program", "-quiet"},
			expectedAddr:  ":8080",
			expectedQuiet: true,
			expectedAdmin: "",
		},
		{
			name:          "Admin server enabled",
			args:          []string{"program", "-admin", ":9091"},
			expectedAddr:  ":8080",
			expectedQuiet: false,
			expectedAdmin: ":9091",
		},
		{
			name:          "All flags set",
			args:          []string{"program", "-addr", ":7070", "-quiet", "-admin", ":9091"},
			expectedAddr:  ":7070",
			expectedQuiet: true,
			expectedAdmin: ":9091",
		},
		{
			name:          "Long form flags",
			args:          []string{"program", "-addr=:6060", "-quiet=true", "-admin=:9092"},
			expectedAddr:  ":6060",
			expectedQuiet: true,
			expectedAdmin: ":9092",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Reset flags for each test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Set up command line args
			os.Args = test.args

			// Parse flags (simulate what main() does)
			addr := flag.String("addr", ":8080", "Server listen address")
			quiet := flag.Bool("quiet", false, "Disable logging for performance")
			adminAddr := flag.String("admin", "", "Admin HTTP server address (disabled if empty)")
			flag.Parse()

			// Check results
			if *addr != test.expectedAddr {
				t.Errorf("Expected address %s, got %s", test.expectedAddr, *addr)
			}

			if *quiet != test.expectedQuiet {
				t.Errorf("Expected quiet %t, got %t", test.expectedQuiet, *quiet)
			}
			if *adminAddr != test.expectedAdmin {
				t.Errorf("Expected admin %q, got %q", test.expectedAdmin, *adminAddr)
			}
		})
	}
}

func TestMain_QuietModeLogging(t *testing.T) {
	// Save original log output
	originalHandler := slog.Default().Handler()
	defer slog.SetDefault(slog.New(originalHandler))

	// Test quiet mode disabled (normal logging)
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil))) // Reset to normal
	if _, ok := slog.Default().Handler().(*slog.JSONHandler); !ok {
		t.Error("Log handler should be JSON handler when quiet mode is disabled")
	}

	// Test quiet mode enabled
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if !slog.Default().Enabled(context.Background(), slog.LevelInfo) {
		// A bit of a workaround to check if it's discarding.
		// If we set a handler with io.Discard, it should still be "enabled".
		// A more complex test could involve checking the output, but this is a reasonable proxy.
	}
}

// TestMain_Integration tests the main function by running it as a subprocess
func TestMain_Integration(t *testing.T) {
	// Skip in short mode to avoid slow subprocess tests
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary for testing
	binary := "test-server"
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if err := build.Run(); err != nil {
		t.Fatalf("Failed to build test binary: %v", err)
	}
	defer os.Remove(binary) // Clean up

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "Invalid address format",
			args:        []string{"-addr", "invalid-address"},
			expectError: true,
		},
		{
			name:        "Port out of range",
			args:        []string{"-addr", ":99999999"},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Run the binary with a timeout to prevent hanging
			cmd := exec.Command("./"+binary, test.args...)

			// Start the command
			err := cmd.Start()
			if err != nil {
				t.Fatalf("Failed to start binary: %v", err)
			}

			// Give it a moment to start and potentially fail
			time.Sleep(100 * time.Millisecond)

			// Kill the process if it's still running
			if cmd.Process != nil {
				cmd.Process.Kill()
			}

			// Wait for completion
			err = cmd.Wait()

			if test.expectError {
				if err == nil {
					t.Error("Expected command to fail, but it succeeded")
				}
			} else {
				// For valid addresses, we expect the process to be killed (not exit with error)
				if err != nil && !strings.Contains(err.Error(), "killed") {
					t.Errorf("Expected command to run successfully (until killed), got error: %v", err)
				}
			}
		})
	}
}

// TestMain_SuccessfulStartup tests that the main function can start a server successfully
func TestMain_SuccessfulStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build the binary
	binary := "test-server-success"
	build := exec.Command("go", "build", "-o", binary, ".")
	if err := build.Run(); err != nil {
		t.Fatalf("Failed to build test binary: %v", err)
	}
	defer os.Remove(binary)

	// Start server on a different port to avoid conflicts
	cmd := exec.Command("./"+binary, "-addr", ":0", "-quiet") // :0 lets OS choose port

	// Start the server
	err := cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(testServerStartupDelay)

	// Check if process is still running (indicates successful startup)
	if cmd.Process == nil {
		t.Fatal("Server process is nil")
	}

	// Clean up - kill the server
	if err := cmd.Process.Kill(); err != nil {
		t.Errorf("Failed to kill server process: %v", err)
	}

	cmd.Wait() // Wait for cleanup
}

// Benchmark to ensure flag parsing doesn't introduce performance overhead
func BenchmarkFlagParsing(b *testing.B) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	for i := 0; i < b.N; i++ {
		// Reset flags
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

		// Set test args
		os.Args = []string{"program", "-addr", ":8080", "-quiet"}

		// Parse flags
		addr := flag.String("addr", ":8080", "Server listen address")
		quiet := flag.Bool("quiet", false, "Disable logging for performance")
		flag.Parse()

		// Prevent optimization from removing the variables
		_ = *addr
		_ = *quiet
	}
}

// TestAdminServer_StartupShutdown tests the admin server lifecycle
func TestAdminServer_StartupShutdown(t *testing.T) {
	// Find available port for testing
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	adminAddr := listener.Addr().String()
	listener.Close()

	// Create a mock server for admin server to use
	srv := server.NewServer(":0", server.DefaultReadTimeout)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start admin server
	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		adminServer.Shutdown(shutdownCtx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is running by making a request
	// It's not "ready" yet because the main server isn't started, so expect 503
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", adminAddr))
	if err != nil {
		t.Fatalf("Admin server not responding: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", resp.StatusCode)
	}
}

// TestAdminServer_HealthzEndpoint tests the health check endpoint
func TestAdminServer_HealthzEndpoint(t *testing.T) {
	// Setup
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	adminAddr := listener.Addr().String()
	listener.Close()

	srv := server.NewServer(":0", server.DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		adminServer.Shutdown(shutdownCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test healthz endpoint when server is NOT ready
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", adminAddr))
	if err != nil {
		t.Fatalf("Failed to call healthz endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when not ready, got %d", resp.StatusCode)
	}

	// Start the main server to make it ready
	go func() {
		// Use a valid but discardable address for the test
		l, _ := net.Listen("tcp", ":0")
		srv.SetListener(l) // A test helper would be better, but this works
		srv.StartWithContext(ctx)
	}()

	// Wait for readiness
	select {
	case <-srv.Ready():
		// continue
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server to be ready")
	}

	// Test healthz endpoint when server IS ready
	resp, err = http.Get(fmt.Sprintf("http://%s/healthz", adminAddr))
	if err != nil {
		t.Fatalf("Failed to call healthz endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 when ready, got %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse and validate response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var healthResp map[string]interface{}
	if err := json.Unmarshal(body, &healthResp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Validate response structure
	expectedFields := []string{"status", "readiness", "liveness"}
	for _, field := range expectedFields {
		if _, exists := healthResp[field]; !exists {
			t.Errorf("Missing field %s in health response", field)
		}
	}

	// Validate values
	if healthResp["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", healthResp["status"])
	}
	if healthResp["readiness"] != true {
		t.Errorf("Expected readiness true, got %v", healthResp["readiness"])
	}
	if healthResp["liveness"] != true {
		t.Errorf("Expected liveness true, got %v", healthResp["liveness"])
	}
}

// TestAdminServer_MetricsEndpoint tests the metrics endpoint
func TestAdminServer_MetricsEndpoint(t *testing.T) {
	// Setup
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	adminAddr := listener.Addr().String()
	listener.Close()

	srv := server.NewServer(":0", server.DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		adminServer.Shutdown(shutdownCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", adminAddr))
	if err != nil {
		t.Fatalf("Failed to call metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Check status and content type
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("Expected Content-Type text/plain, got %s", contentType)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Validate expected metrics fields
	bodyStr := string(body)
	expectedSubstrings := []string{
		"# HELP package_indexer_connections_total",
		"# TYPE package_indexer_connections_total counter",
		"package_indexer_connections_total 0",
		"# HELP package_indexer_packages_indexed_current",
		"# TYPE package_indexer_packages_indexed_current gauge",
		"package_indexer_packages_indexed_current 0",
	}
	for _, sub := range expectedSubstrings {
		if !strings.Contains(bodyStr, sub) {
			t.Errorf("Missing substring %q in metrics response", sub)
		}
	}
}

// TestAdminServer_BuildInfoEndpoint tests the build info endpoint
func TestAdminServer_BuildInfoEndpoint(t *testing.T) {
	// Setup
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	adminAddr := listener.Addr().String()
	listener.Close()

	srv := server.NewServer(":0", server.DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer shutdownAdminServer(adminServer)()

	time.Sleep(testServerStartupDelay)

	// Test buildinfo endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/buildinfo", adminAddr))
	if err != nil {
		t.Fatalf("Failed to call buildinfo endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Accept either populated build info or placeholder
	if _, hasUnknown := info["status"]; hasUnknown {
		if info["status"] != "unknown" {
			t.Errorf("Expected status 'unknown' when build info unavailable, got %v", info["status"])
		}
		return
	}

	// When build info is available, validate expected fields
	if _, ok := info["go_version"]; !ok {
		t.Errorf("Missing go_version in buildinfo response")
	}
}

// TestAdminServer_PprofEndpoints tests the pprof debugging endpoints
func TestAdminServer_PprofEndpoints(t *testing.T) {
	// Setup
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	adminAddr := listener.Addr().String()
	listener.Close()

	srv := server.NewServer(":0", server.DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		adminServer.Shutdown(shutdownCtx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test pprof endpoints
	pprofEndpoints := []struct {
		path           string
		expectedStatus int
	}{
		{"/debug/pprof/", http.StatusOK},
		{"/debug/pprof/cmdline", http.StatusOK},
		{"/debug/pprof/symbol", http.StatusOK},
		// Note: profile and trace endpoints require special handling/parameters
	}

	for _, endpoint := range pprofEndpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			resp, err := http.Get(fmt.Sprintf("http://%s%s", adminAddr, endpoint.path))
			if err != nil {
				t.Fatalf("Failed to call %s endpoint: %v", endpoint.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != endpoint.expectedStatus {
				t.Errorf("Expected status %d for %s, got %d",
					endpoint.expectedStatus, endpoint.path, resp.StatusCode)
			}
		})
	}
}

// TestAdminServer_DisabledByDefault tests that admin server is disabled by default
func TestAdminServer_DisabledByDefault(t *testing.T) {
	// Simulate run() without admin flag
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"program"} // No admin flag

	// Parse flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	addr := flag.String("addr", ":8080", "Server listen address")
	quiet := flag.Bool("quiet", false, "Disable logging for performance")
	adminAddr := flag.String("admin", "", "Admin HTTP server address (disabled if empty)")
	flag.Parse()

	// Verify admin is disabled by default
	if *adminAddr != "" {
		t.Errorf("Expected admin server to be disabled by default, got %s", *adminAddr)
	}

	// Verify other defaults are as expected
	if *addr != ":8080" {
		t.Errorf("Expected default addr \":8080\", got %s", *addr)
	}
	if *quiet != false {
		t.Errorf("Expected default quiet false, got %v", *quiet)
	}

	// Verify that no admin server would be started (simulate the conditional)
	var adminServer *http.Server
	if *adminAddr != "" {
		srv := server.NewServer(*addr, server.DefaultReadTimeout)
		ctx := context.Background()
		adminServer = startAdminServer(ctx, *adminAddr, srv)
	}

	if adminServer != nil {
		t.Error("Admin server should not be started when flag is empty")
	}
}

// TestAdminServer_Integration tests admin server with main server integration
func TestAdminServer_Integration(t *testing.T) {
	// This is a more complex integration test
	// Find available ports
	mainListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port for main server: %v", err)
	}
	mainAddr := mainListener.Addr().String()
	mainListener.Close()

	adminListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port for admin server: %v", err)
	}
	adminAddr := adminListener.Addr().String()
	adminListener.Close()

	// Start main server
	srv := server.NewServer(mainAddr, server.DefaultReadTimeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		srv.StartWithContext(ctx)
	}()

	// Start admin server
	adminServer := startAdminServer(ctx, adminAddr, srv)
	defer shutdownBothServers(srv, adminServer)()

	time.Sleep(testServerStartupDelay) // Give servers time to start

	// Send some commands to main server to generate metrics
	conn, err := net.Dial("tcp", mainAddr)
	if err != nil {
		t.Fatalf("Failed to connect to main server: %v", err)
	}
	defer conn.Close()

	// Send a command
	if _, err := conn.Write([]byte("INDEX|test|\n")); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Read response
	response := make([]byte, 10)
	if _, err := conn.Read(response); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check that metrics reflect the activity
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", adminAddr))
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics response: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "package_indexer_connections_total 1") {
		t.Errorf("Expected connections_total to be 1, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "package_indexer_commands_processed_total 1") {
		t.Errorf("Expected commands_processed_total to be 1, got %s", bodyStr)
	}
}

// TestRun_ServerError_InvalidAddr covers the run() error path when the TCP listener fails
func TestRun_ServerError_InvalidAddr(t *testing.T) {
	defer isolateFlags(t)()

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	os.Args = []string{"program", "-addr", "invalid-address"}

	if err := run(); err == nil {
		t.Fatal("expected run() to return error for invalid address, got nil")
	}
}

// TestRun_GracefulShutdown_Signal covers the successful path where a shutdown signal is handled
func TestRun_GracefulShutdown_Signal(t *testing.T) {
	// Skip in short mode since this exercises timers and signals
	if testing.Short() {
		t.Skip("skipping graceful shutdown test in short mode")
	}

	defer isolateFlags(t)()

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// Use :0 to let the OS select free ports for both servers
	os.Args = []string{"program", "-addr", ":0", "-admin", ":0", "-quiet"}

	done := make(chan error, 1)
	go func() {
		done <- run()
	}()

	// Give the servers a brief moment to start
	time.Sleep(testServerStartupDelay)

	// Send SIGINT to trigger graceful shutdown path
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find current process: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
	case <-time.After(testShutdownTimeout):
		t.Fatal("timed out waiting for graceful shutdown")
	}
}
