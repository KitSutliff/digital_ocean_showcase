// Package server metrics provide real-time operational visibility for production monitoring.
// Thread-safe atomic operations ensure accurate counters under high concurrency without
// performance impact. Metrics enable capacity planning, alerting, and operational insights
// essential for observability platform reliability.
package server

import (
	"sync/atomic"
	"time"
)

// Metrics contains runtime statistics for the server using atomic operations for thread safety.
// Lock-free design ensures minimal performance impact while providing accurate operational
// data essential for production monitoring and capacity planning in high-throughput environments.
type Metrics struct {
	ConnectionsTotal    int64
	CommandsProcessed   int64
	ErrorCount         int64
	PackagesIndexed    int64
	StartTime          time.Time
}

// MetricsSnapshot represents a point-in-time view of server metrics for consistent reporting.
// Atomic snapshot prevents torn reads during concurrent updates, ensuring reliable metrics
// data for monitoring dashboards, alerting systems, and operational decision-making.
type MetricsSnapshot struct {
	ConnectionsTotal  int64
	CommandsProcessed int64
	ErrorCount       int64
	PackagesIndexed  int64
	Uptime           time.Duration
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime: time.Now(),
	}
}

// IncrementConnections atomically increments the connection counter for thread safety.
// Operational metric: Tracks total connections handled for capacity planning and monitoring.
func (m *Metrics) IncrementConnections() {
	atomic.AddInt64(&m.ConnectionsTotal, 1)
}

// IncrementCommands atomically increments the command counter for throughput monitoring.
// Performance metric: Measures protocol commands processed for load analysis and optimization.
func (m *Metrics) IncrementCommands() {
	atomic.AddInt64(&m.CommandsProcessed, 1)
}

// IncrementErrors atomically increments the error counter for reliability monitoring.
// Operational metric: Tracks protocol and processing errors for alerting and diagnostics.
func (m *Metrics) IncrementErrors() {
	atomic.AddInt64(&m.ErrorCount, 1)
}

// IncrementPackages atomically increments the package counter for business logic monitoring.
// Business metric: Tracks successful package indexing operations for usage analysis.
func (m *Metrics) IncrementPackages() {
	atomic.AddInt64(&m.PackagesIndexed, 1)
}

// GetSnapshot returns a consistent snapshot of current metrics for external monitoring systems.
// Thread-safe atomic reads provide point-in-time consistency essential for accurate
// reporting to observability platforms and operational dashboards.
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ConnectionsTotal:  atomic.LoadInt64(&m.ConnectionsTotal),
		CommandsProcessed: atomic.LoadInt64(&m.CommandsProcessed),
		ErrorCount:       atomic.LoadInt64(&m.ErrorCount),
		PackagesIndexed:  atomic.LoadInt64(&m.PackagesIndexed),
		Uptime:           time.Since(m.StartTime),
	}
}
