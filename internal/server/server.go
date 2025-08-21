// Package server implements a high-performance TCP server with graceful shutdown capabilities.
// The architecture uses goroutine-per-connection for natural resource management and scales
// efficiently to 100+ concurrent clients. Includes operational metrics, connection timeouts,
// and comprehensive error handling for production observability workloads.
package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"package-indexer/internal/indexer"
	"package-indexer/internal/wire"
)

var nextConnID uint64

// Server manages TCP connections and coordinates with the indexer using a goroutine-per-connection model.
// Architecture decision: This approach provides natural connection lifecycle management and scales
// well to the required 100+ concurrent clients while maintaining operational simplicity.
type Server struct {
	indexer     *indexer.Indexer
	addr        string
	listener    net.Listener
	wg          sync.WaitGroup // Tracks active connections for graceful shutdown
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	metrics     *Metrics
	ready       chan bool // Signals when the listener is ready for connections
	isReady     atomic.Bool
	readTimeout time.Duration // Configurable per-read deadline to prevent slowloris attacks
}

// Default timeout configuration constants
const (
	DefaultReadTimeout = 30 * time.Second // Default per-read deadline to prevent slowloris attacks
)

// NewServer creates a new server instance
func NewServer(addr string, readTimeout time.Duration) *Server {
	return &Server{
		indexer:     indexer.NewIndexer(),
		addr:        addr,
		metrics:     NewMetrics(),
		ready:       make(chan bool),
		readTimeout: readTimeout,
	}
}

// Start begins listening for connections on the configured address
func (s *Server) Start() error {
	return s.StartWithContext(context.Background())
}

// StartWithContext begins listening for connections with context support for graceful shutdown
func (s *Server) StartWithContext(ctx context.Context) error {
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	localCtx := s.ctx
	s.mu.Unlock()

	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		close(s.ready) // Signal readiness even on failure to unblock tests
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	s.isReady.Store(true)
	close(s.ready) // Signal that the listener is ready

	// Close the listener when context is cancelled to unblock Accept
	go func() {
		<-localCtx.Done()
		s.mu.Lock()
		ln := s.listener
		s.mu.Unlock()
		if ln != nil {
			_ = ln.Close()
		}
	}()

	slog.Info("Package indexer server listening", "addr", s.addr)

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil // Graceful shutdown
			default:
				slog.Warn("Failed to accept connection", "error", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection processes all messages from a single client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("Error closing connection", "error", err)
		}
	}()

	connID := atomic.AddUint64(&nextConnID, 1)
	s.serveConn(s.ctx, conn, connID)
}

// serveConn contains the core connection processing loop.
// It enforces newline framing, resets a read deadline before each read,
// and exits gracefully on context cancellation or client disconnect.
func (s *Server) serveConn(ctx context.Context, conn net.Conn, connID uint64) {
	clientAddr := conn.RemoteAddr().String()
	logger := slog.With("connID", connID, "clientAddr", clientAddr)

	logger.Info("Client connected")

	s.metrics.IncrementConnections()

	// Initial deadline to prevent slowloris attacks
	s.setConnectionDeadline(conn, logger, "initial")

	reader := bufio.NewReader(conn)

	// Graceful shutdown coordination: Background goroutine monitors for context cancellation
	// and closes connection to unblock ReadString(), enabling clean shutdown under load
	doneCh := make(chan struct{})
	defer close(doneCh) // Ensure the goroutine exits to prevent resource leaks
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-doneCh:
		}
	}()

	for {
		// Reset deadline on each read
		s.setConnectionDeadline(conn, logger, "reset")

		// Read line from client
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				logger.Info("Client disconnected")
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logger.Warn("Client timeout")
			} else {
				logger.Warn("Error reading from client", "error", err)
			}
			return
		}

		// Process the command and get response
		s.metrics.IncrementCommands()
		response := s.processCommand(logger, line)

		// Send response back to client
		if _, err := conn.Write([]byte(response.String())); err != nil {
			logger.Warn("Error writing response to client", "error", err)
			return
		}
	}
}

// setConnectionDeadline sets the read deadline and logs any errors with context
func (s *Server) setConnectionDeadline(conn net.Conn, logger *slog.Logger, context string) {
	if err := conn.SetReadDeadline(time.Now().Add(s.readTimeout)); err != nil {
		logger.Warn("Failed to set read deadline", "error", err, "context", context)
	}
}

// processCommand parses and executes a single command
func (s *Server) processCommand(logger *slog.Logger, line string) wire.Response {
	// Parse the command
	cmd, err := wire.ParseCommand(line)
	if err != nil {
		logger.Warn("Parse error", "error", err, "line", strings.TrimSpace(line))
		s.metrics.IncrementErrors()
		return wire.ERROR
	}

	logger = logger.With("cmd", cmd.Type, "pkg", cmd.Package)

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
		logger.Warn("Unknown command type")
		s.metrics.IncrementErrors()
		return wire.ERROR
	}
}

// GetMetrics returns a snapshot of current server metrics
func (s *Server) GetMetrics() MetricsSnapshot {
	return s.metrics.GetSnapshot()
}

// GetStats returns a snapshot of current indexer statistics.
// Architecture decision: Decouples server metrics from indexer state, allowing
// each component to be monitored independently in production environments.
func (s *Server) GetStats() (stats struct{ Indexed int }) {
	indexed, _, _ := s.indexer.GetStats()
	stats.Indexed = indexed
	return
}

// IsReady checks if the server's TCP listener is active and ready to accept connections.
// Used by the /healthz readiness probe for production monitoring and service discovery.
func (s *Server) IsReady() bool {
	return s.isReady.Load()
}

// Ready returns a channel that is closed when the server is ready to accept connections.
// Used for test synchronization.
func (s *Server) Ready() <-chan bool {
	return s.ready
}

// SetListener injects a listener into the server. Used for testing purposes.
func (s *Server) SetListener(l net.Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listener = l
}

// Shutdown gracefully shuts down the server with configurable timeout
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Initiating graceful shutdown...")

	// Mark server as not ready immediately when shutdown starts
	// This ensures /healthz returns false during shutdown window
	s.isReady.Store(false)

	s.mu.Lock()
	cancel := s.cancel
	ln := s.listener
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if ln != nil {
		ln.Close()
	}

	// Wait for connections to finish or timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("All connections closed gracefully")
		return nil
	case <-ctx.Done():
		slog.Warn("Shutdown timeout exceeded")
		return ctx.Err()
	}
}
