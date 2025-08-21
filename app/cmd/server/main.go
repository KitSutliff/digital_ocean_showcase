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
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"package-indexer/internal/server"
)

// Server configuration constants
const (
	defaultShutdownTimeout        = 30 * time.Second
	defaultAdminReadHeaderTimeout = 5 * time.Second
	defaultAdminReadTimeout       = 10 * time.Second
	defaultAdminWriteTimeout      = 10 * time.Second
	defaultAdminIdleTimeout       = 60 * time.Second
)

// Prometheus metric definitions
type prometheusMetric struct {
	name       string
	help       string
	metricType string
	value      interface{}
}

// writePrometheusMetric writes a single Prometheus metric in standard format
func writePrometheusMetric(w io.Writer, metric prometheusMetric) {
	fmt.Fprintf(w, "# HELP %s %s\n", metric.name, metric.help)
	fmt.Fprintf(w, "# TYPE %s %s\n", metric.name, metric.metricType)
	fmt.Fprintf(w, "%s %v\n\n", metric.name, metric.value)
}

func main() {
	if err := run(); err != nil {
		// Use slog for structured error logging at exit
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Server stopped successfully")
}

// run encapsulates the server startup and graceful shutdown logic.
// Separating this from main() enables unit testing and follows Go best practices
// for production servers requiring reliable operational characteristics.
func run() error {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "Server listen address")
	quiet := flag.Bool("quiet", false, "Disable logging for performance")
	adminAddr := flag.String("admin", "", "Admin HTTP server address (disabled if empty)")
	shutdownTimeoutFlag := flag.Duration("shutdown-timeout", defaultShutdownTimeout, "Graceful shutdown timeout")
	readTimeoutFlag := flag.Duration("read-timeout", server.DefaultReadTimeout, "Connection read timeout")
	flag.Parse()

	// Setup structured logging
	var handler slog.Handler
	if *quiet {
		handler = slog.NewJSONHandler(io.Discard, nil)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))

	// Application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Create and start main TCP server
	srv := server.NewServer(*addr, *readTimeoutFlag)
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Starting package indexer server", "addr", *addr)
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
		slog.Info("Received shutdown signal")
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	// Initiate graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), *shutdownTimeoutFlag)
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

		ready := srv.IsReady()
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)

		// In production, readiness would check if TCP server is accepting connections
		// For this implementation, we assume readiness once the main server starts
		response := map[string]interface{}{
			"status":    "healthy",
			"readiness": ready, // TCP listener operational
			"liveness":  true,  // Process operational
		}

		json.NewEncoder(w).Encode(response)
	})

	// Metrics endpoint exposing operational statistics in Prometheus format
	// Enables integration with industry-standard monitoring tools like Prometheus and Grafana
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metrics := srv.GetMetrics()
		stats := srv.GetStats()

		// Define all metrics in a structured way to eliminate duplication
		prometheusMetrics := []prometheusMetric{
			{
				name:       "package_indexer_connections_total",
				help:       "Total number of connections handled.",
				metricType: "counter",
				value:      metrics.ConnectionsTotal,
			},
			{
				name:       "package_indexer_commands_processed_total",
				help:       "Total number of commands processed.",
				metricType: "counter",
				value:      metrics.CommandsProcessed,
			},
			{
				name:       "package_indexer_errors_total",
				help:       "Total number of processing errors.",
				metricType: "counter",
				value:      metrics.ErrorCount,
			},
			{
				name:       "package_indexer_packages_indexed_current",
				help:       "Current number of indexed packages.",
				metricType: "gauge",
				value:      stats.Indexed,
			},
			{
				name:       "package_indexer_uptime_seconds",
				help:       "Server uptime in seconds.",
				metricType: "gauge",
				value:      metrics.Uptime.Seconds(),
			},
		}

		// Write all metrics using the helper function
		for _, metric := range prometheusMetrics {
			writePrometheusMetric(w, metric)
		}
	})

	// Build info endpoint provides versioning details for release diagnostics
	mux.HandleFunc("/buildinfo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if info, ok := debug.ReadBuildInfo(); ok && info != nil {
			resp := map[string]interface{}{
				"main": map[string]string{
					"path":    info.Path,
					"version": info.Main.Version,
					"sum":     info.Main.Sum,
				},
				"go_version": info.GoVersion,
				"settings":   info.Settings,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "unknown"})
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
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultAdminReadHeaderTimeout,
		ReadTimeout:       defaultAdminReadTimeout,
		WriteTimeout:      defaultAdminWriteTimeout,
		IdleTimeout:       defaultAdminIdleTimeout,
	}

	go func() {
		slog.Info("Starting admin HTTP server", "addr", addr)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Admin server error", "error", err)
		}
	}()

	return adminServer
}
