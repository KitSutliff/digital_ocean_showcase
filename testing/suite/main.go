// Package main provides the test harness entry point for package indexer integration testing.
// This comprehensive test suite validates server behavior under concurrent load using real
// package dependency data, including chaos testing and failure injection scenarios.
package main

import (
	"flag"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof" // Enable pprof HTTP endpoints for debugging
	"os"
)

// main initializes the test harness with command-line configuration and executes
// the comprehensive test suite against the package indexer server.
func main() {
	// Configure logging with microsecond precision for performance analysis
	log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	
	// Parse command-line flags for test configuration
	host := flag.String("host", "127.0.0.1", "The host of your server")
	port := flag.Int("port", 8080, "The port your server exposes to clients")
	concurrencyLevel := flag.Int("concurrency", 10, "A positive value indicating how many concurrent clients to use")
	randomSeed := flag.Int64("seed", 42, "A positive value used to seed the random number generator")
	debugMode := flag.Bool("debug", false, "Prints some extra information and opens a HTTP server on port 8081")
	unluckiness := flag.Int("unluckiness", 5, "A % showing the probability of something bad happenning, like broken messages being sent or random disconnects")
	flag.Parse()
	
	// Initialize random seed for deterministic chaos testing
	rand.Seed(*randomSeed)

	// Create test run instance with configured parameters
	test := MakeTestRun(*host, *port, *concurrencyLevel, *unluckiness)

	// Enable debug HTTP server with pprof endpoints if requested
	if *debugMode {
		log.Println("Running in DEBUG mode")
		go func() {
			log.Println(http.ListenAndServe(":8081", nil))
		}()
	}

	// Execute test phases: initialization, comprehensive testing, cleanup
	test.Start()
	test.Run()
	test.Finish()
}
