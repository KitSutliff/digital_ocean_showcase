package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"package-indexer/internal/indexer"
	"package-indexer/internal/wire"
)

// Server manages TCP connections and coordinates with the indexer
type Server struct {
	indexer  *indexer.Indexer
	addr     string
	listener net.Listener
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	metrics  *Metrics
	ready    chan bool // Channel to signal when the listener is ready
}

const (
	// readTimeout defines the per-read deadline to mitigate slowloris-style clients
	readTimeout = 30 * time.Second
)

// NewServer creates a new server instance
func NewServer(addr string) *Server {
	return &Server{
		indexer: indexer.NewIndexer(),
		addr:    addr,
		metrics: NewMetrics(),
		ready:   make(chan bool),
	}
}

// Start begins listening for connections on the configured address
func (s *Server) Start() error {
	return s.StartWithContext(context.Background())
}

// StartWithContext begins listening for connections with context support for graceful shutdown
func (s *Server) StartWithContext(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		close(s.ready) // Signal readiness even on failure to unblock tests
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.listener = l
	close(s.ready) // Signal that the listener is ready

	// Close the listener when context is cancelled to unblock Accept
	go func() {
		<-s.ctx.Done()
		if s.listener != nil {
			_ = s.listener.Close()
		}
	}()

	log.Printf("Package indexer server listening on %s", s.addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil // Graceful shutdown
			default:
				log.Printf("Failed to accept connection: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection processes all messages from a single client connection.
// It sets up the context for graceful shutdown and manages the connection lifecycle.
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}()
	s.serveConn(s.ctx, conn)
}

// serveConn contains the shared connection handling loop.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	clientAddr := conn.RemoteAddr().String()
	log.Printf("Client connected: %s", clientAddr)

	s.metrics.IncrementConnections()

	// Initial deadline to prevent slowloris attacks
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

	reader := bufio.NewReader(conn)

	// Close the connection if context is cancelled to unblock reads
	doneCh := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-doneCh:
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Graceful shutdown: closing connection to %s", clientAddr)
			close(doneCh)
			return
		default:
			// Reset deadline on each read
			_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

			// Read line from client
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					log.Printf("Client disconnected: %s", clientAddr)
				} else {
					log.Printf("Error reading from client %s: %v", clientAddr, err)
				}
				close(doneCh)
				return
			}

			// Process the command and get response
			s.metrics.IncrementCommands()
			response := s.processCommand(line)

			// Send response back to client
			if _, err := conn.Write([]byte(response.String())); err != nil {
				log.Printf("Error writing response to client %s: %v", clientAddr, err)
				close(doneCh)
				return
			}
		}
	}
}

// processCommand parses and executes a single command, returning the appropriate response
func (s *Server) processCommand(line string) wire.Response {
	// Parse the command
	cmd, err := wire.ParseCommand(line)
	if err != nil {
		log.Printf("Parse error: %v (line: %q)", err, strings.TrimSpace(line))
		s.metrics.IncrementErrors()
		return wire.ERROR
	}

	// Execute the command
	switch cmd.Type {
	case wire.IndexCommand:
		if s.indexer.IndexPackage(cmd.Package, cmd.Dependencies) {
			s.metrics.IncrementPackages()
			return wire.OK
		}
		return wire.FAIL

	case wire.RemoveCommand:
		switch s.indexer.RemovePackage(cmd.Package) {
		case indexer.RemoveResultOK, indexer.RemoveResultNotIndexed:
			return wire.OK
		case indexer.RemoveResultBlocked:
			return wire.FAIL
		}
		return wire.ERROR // Should be unreachable

	case wire.QueryCommand:
		if s.indexer.QueryPackage(cmd.Package) {
			return wire.OK
		}
		return wire.FAIL

	default:
		log.Printf("Unknown command type: %v", cmd.Type)
		s.metrics.IncrementErrors()
		return wire.ERROR
	}
}

// GetMetrics returns a snapshot of current server metrics
func (s *Server) GetMetrics() MetricsSnapshot {
	return s.metrics.GetSnapshot()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Printf("Initiating graceful shutdown...")

	if s.cancel != nil {
		s.cancel()
	}

	if s.listener != nil {
		s.listener.Close()
	}

	// Wait for connections to finish or timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("All connections closed gracefully")
		return nil
	case <-ctx.Done():
		log.Printf("Shutdown timeout exceeded")
		return ctx.Err()
	}
}
