package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"package-indexer/internal/server"
)

func main() {
	// Parse command line flags
	addr := flag.String("addr", ":8080", "Server listen address")
	quiet := flag.Bool("quiet", false, "Disable logging for performance")
	flag.Parse()

	// Disable logging if quiet mode is enabled
	if *quiet {
		log.SetOutput(io.Discard)
	}

	// Create server
	srv := server.NewServer(*addr)
	
	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Starting package indexer server on %s", *addr)
		if err := srv.StartWithContext(ctx); err != nil {
			serverErr <- err
		}
	}()
	
	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
		// Wait for connections to finish (with timeout)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown completed with error: %v", err)
		}
		
	case err := <-serverErr:
		log.Fatalf("Server failed: %v", err)
	}
	
	log.Printf("Server stopped")
}
