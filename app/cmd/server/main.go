package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
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
func run() error {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "Server listen address")
	quiet := flag.Bool("quiet", false, "Disable logging for performance")
	flag.Parse()

	// Disable logging if quiet mode is enabled
	if *quiet {
		log.SetOutput(io.Discard)
	}

	// Application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Create and start server
	srv := server.NewServer(*addr)
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Starting package indexer server on %s", *addr)
		serverErr <- srv.StartWithContext(ctx)
	}()

	// Wait for stop signal or server error
	select {
	case <-stop:
		log.Println("Received shutdown signal")
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	}

	// Initiate graceful shutdown
	log.Println("Initiating graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}

	return nil
}
