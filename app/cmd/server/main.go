// Package main provides the entry point for the package indexer TCP server.
// This server manages package dependency relationships with high concurrency support,
// designed for production observability workloads requiring 100+ simultaneous connections.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"package-indexer/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
	log.Printf("Server stopped successfully")
}

// run encapsulates the server startup and graceful shutdown logic.
// Separating this from main() enables unit testing and follows Go best practices
// for production servers requiring reliable operational characteristics.
func run() error {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "Server listen address")
	quiet := flag.Bool("quiet", false, "Disable logging for performance")
	adminAddr := flag.String("admin", "", "Admin HTTP server address (disabled if empty)")
	flag.Parse()

	// Disable logging for performance in high-throughput scenarios
	if *quiet {
		log.SetOutput(io.Discard)
	}

	// Application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Create and start main TCP server
	srv := server.NewServer(*addr)
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Starting package indexer server on %s", *addr)
		serverErr <- srv.StartWithContext(ctx)
	}()

	// Start optional admin HTTP server for observability
	var adminServer *http.Server
	if *adminAddr != "" {
		adminServer = startAdminServer(ctx, *adminAddr, srv)
	}

	// Wait for stop signal or server error
	select {
	case <-stop:
		log.Println("Received shutdown signal")
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	// Initiate graceful shutdown with timeout
	log.Println("Initiating graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown main server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("main server shutdown failed: %w", err)
	}

	// Shutdown admin server if running
	if adminServer != nil {
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("admin server shutdown failed: %w", err)
		}
	}

	return nil
}

// startAdminServer creates and starts the optional admin HTTP server for observability.
// Provides health checks, metrics endpoint, and pprof debugging capabilities isolated
// from the main TCP protocol. Designed for production monitoring and debugging workflows.
func startAdminServer(ctx context.Context, addr string, srv *server.Server) *http.Server {
	mux := http.NewServeMux()

	// Health check endpoint with readiness/liveness semantics
	// Readiness: TCP listener must be operational
	// Liveness: Process is running (always true if we reach this handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// In production, readiness would check if TCP server is accepting connections
		// For this implementation, we assume readiness once the main server starts
		response := map[string]interface{}{
			"status":    "healthy",
			"readiness": true, // TCP listener operational
			"liveness":  true, // Process operational
		}

		json.NewEncoder(w).Encode(response)
	})

	// Metrics endpoint exposing operational statistics as structured JSON
	// Enables monitoring dashboards, alerting, and capacity planning integration
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		metrics := srv.GetMetrics()
		json.NewEncoder(w).Encode(metrics)
	})

	// Standard pprof debugging endpoints explicitly mounted on admin server only
	// Architecture decision: Isolates debugging capabilities from main TCP protocol for security
	// Provides CPU profiling, memory analysis, goroutine inspection, and more
	// Access via /debug/pprof/, /debug/pprof/goroutine, /debug/pprof/heap, etc.
	mux.HandleFunc("/debug/pprof/", pprof.Index)          // Profile index and navigation
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline) // Command line arguments
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile) // CPU profiling
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)   // Symbol resolution
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)     // Execution tracing

	adminServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("Starting admin HTTP server on %s", addr)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Admin server error: %v", err)
		}
	}()

	return adminServer
}
