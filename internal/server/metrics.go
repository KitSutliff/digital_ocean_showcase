// Package server metrics provide real-time operational visibility for production monitoring.
// Thread-safe atomic operations ensure accurate counters under high concurrency
// for capacity planning, alerting, and operational insights.
package server

import (
	"sync/atomic"
	"time"
)

// Metrics contains runtime statistics using atomic operations for thread safety.
// Lock-free design ensures minimal performance impact for production monitoring.
type Metrics struct {
	ConnectionsTotal  int64
	CommandsProcessed int64
	ErrorCount        int64
	PackagesIndexed   int64
	StartTime         time.Time
}

// MetricsSnapshot represents a point-in-time view of server metrics for consistent reporting.
// Atomic snapshot prevents torn reads during concurrent updates, ensuring reliable metrics
// data for monitoring dashboards, alerting systems, and operational decision-making.
type MetricsSnapshot struct {
	ConnectionsTotal  int64
	CommandsProcessed int64
	ErrorCount        int64
	PackagesIndexed   int64
	Uptime            time.Duration
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime: time.Now(),
	}
}

// IncrementConnections atomically increments the connection counter
func (m *Metrics) IncrementConnections() {
	atomic.AddInt64(&m.ConnectionsTotal, 1)
}

// IncrementCommands atomically increments the command counter
func (m *Metrics) IncrementCommands() {
	atomic.AddInt64(&m.CommandsProcessed, 1)
}

// IncrementErrors atomically increments the error counter
func (m *Metrics) IncrementErrors() {
	atomic.AddInt64(&m.ErrorCount, 1)
}

// IncrementPackages atomically increments the package counter
func (m *Metrics) IncrementPackages() {
	atomic.AddInt64(&m.PackagesIndexed, 1)
}

// GetSnapshot returns a consistent point-in-time view of current metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ConnectionsTotal:  atomic.LoadInt64(&m.ConnectionsTotal),
		CommandsProcessed: atomic.LoadInt64(&m.CommandsProcessed),
		ErrorCount:        atomic.LoadInt64(&m.ErrorCount),
		PackagesIndexed:   atomic.LoadInt64(&m.PackagesIndexed),
		Uptime:            time.Since(m.StartTime),
	}
}
