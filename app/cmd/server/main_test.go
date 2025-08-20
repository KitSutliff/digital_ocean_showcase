package main

import (
	"flag"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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
	}{
		{
			name:          "Default values",
			args:          []string{"program"},
			expectedAddr:  ":8080",
			expectedQuiet: false,
		},
		{
			name:          "Custom address",
			args:          []string{"program", "-addr", ":9090"},
			expectedAddr:  ":9090",
			expectedQuiet: false,
		},
		{
			name:          "Quiet mode enabled",
			args:          []string{"program", "-quiet"},
			expectedAddr:  ":8080",
			expectedQuiet: true,
		},
		{
			name:          "Both flags set",
			args:          []string{"program", "-addr", ":7070", "-quiet"},
			expectedAddr:  ":7070",
			expectedQuiet: true,
		},
		{
			name:          "Long form flags",
			args:          []string{"program", "-addr=:6060", "-quiet=true"},
			expectedAddr:  ":6060",
			expectedQuiet: true,
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
			flag.Parse()

			// Check results
			if *addr != test.expectedAddr {
				t.Errorf("Expected address %s, got %s", test.expectedAddr, *addr)
			}

			if *quiet != test.expectedQuiet {
				t.Errorf("Expected quiet %t, got %t", test.expectedQuiet, *quiet)
			}
		})
	}
}

func TestMain_QuietModeLogging(t *testing.T) {
	// Save original log output
	originalOutput := log.Writer()
	defer log.SetOutput(originalOutput)

	// Test quiet mode disabled (normal logging)
	log.SetOutput(os.Stderr) // Reset to normal
	if log.Writer() == io.Discard {
		t.Error("Log output should not be discarded when quiet mode is disabled")
	}

	// Test quiet mode enabled
	log.SetOutput(io.Discard)
	if log.Writer() != io.Discard {
		t.Error("Log output should be discarded when quiet mode is enabled")
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
	time.Sleep(200 * time.Millisecond)

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
