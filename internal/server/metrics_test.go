package server

import (
	"sync"
	"testing"
	"time"
)

// assertMetrics compares the actual metrics snapshot against expected values.
func assertMetrics(t *testing.T, actual, expected MetricsSnapshot) {
	t.Helper()
	if actual.ConnectionsTotal != expected.ConnectionsTotal {
		t.Errorf("ConnectionsTotal: got %d, want %d", actual.ConnectionsTotal, expected.ConnectionsTotal)
	}
	if actual.CommandsProcessed != expected.CommandsProcessed {
		t.Errorf("CommandsProcessed: got %d, want %d", actual.CommandsProcessed, expected.CommandsProcessed)
	}
	if actual.ErrorCount != expected.ErrorCount {
		t.Errorf("ErrorCount: got %d, want %d", actual.ErrorCount, expected.ErrorCount)
	}
	if actual.PackagesIndexed != expected.PackagesIndexed {
		t.Errorf("PackagesIndexed: got %d, want %d", actual.PackagesIndexed, expected.PackagesIndexed)
	}
}

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	if m == nil {
		t.Fatal("NewMetrics should return a non-nil metrics instance")
	}

	// Check initial values
	snapshot := m.GetSnapshot()
	assertMetrics(t, snapshot, MetricsSnapshot{})

	// Check that start time is recent
	if time.Since(m.StartTime) > time.Second {
		t.Error("StartTime should be recent")
	}
}

func TestMetrics_IncrementOperations(t *testing.T) {
	tests := []struct {
		name           string
		incrementFunc  func(*Metrics)
		expectedMetric func(*MetricsSnapshot) int64
	}{
		{"Connections", (*Metrics).IncrementConnections, func(s *MetricsSnapshot) int64 { return s.ConnectionsTotal }},
		{"Commands", (*Metrics).IncrementCommands, func(s *MetricsSnapshot) int64 { return s.CommandsProcessed }},
		{"Errors", (*Metrics).IncrementErrors, func(s *MetricsSnapshot) int64 { return s.ErrorCount }},
		{"Packages", (*Metrics).IncrementPackages, func(s *MetricsSnapshot) int64 { return s.PackagesIndexed }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMetrics()
			tt.incrementFunc(m)
			snapshot := m.GetSnapshot()
			if val := tt.expectedMetric(&snapshot); val != 1 {
				t.Errorf("Expected 1 for %s, got %d", tt.name, val)
			}
		})
	}
}

func TestMetrics_ConcurrentIncrements(t *testing.T) {
	m := NewMetrics()
	const numGoroutines = 100
	const incrementsPerGoroutine = 10

	var wg sync.WaitGroup

	// Helper to run concurrent increments for a given metric function
	testConcurrentIncrement := func(incrementFunc func()) {
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < incrementsPerGoroutine; j++ {
					incrementFunc()
				}
			}()
		}
	}

	// Run concurrent tests for all metrics
	testConcurrentIncrement(m.IncrementConnections)
	testConcurrentIncrement(m.IncrementCommands)
	testConcurrentIncrement(m.IncrementErrors)
	testConcurrentIncrement(m.IncrementPackages)

	wg.Wait()

	expectedCount := int64(numGoroutines * incrementsPerGoroutine)
	snapshot := m.GetSnapshot()
	assertMetrics(t, snapshot, MetricsSnapshot{
		ConnectionsTotal:  expectedCount,
		CommandsProcessed: expectedCount,
		ErrorCount:        expectedCount,
		PackagesIndexed:   expectedCount,
	})
}

func TestMetrics_UptimeCalculation(t *testing.T) {
	m := NewMetrics()

	// Wait a small amount to ensure uptime is measurable
	time.Sleep(10 * time.Millisecond)

	snapshot := m.GetSnapshot()

	if snapshot.Uptime <= 0 {
		t.Error("Uptime should be greater than 0")
	}

	if snapshot.Uptime < 10*time.Millisecond {
		t.Error("Uptime should be at least 10ms")
	}

	if snapshot.Uptime > time.Second {
		t.Error("Uptime should be less than 1 second for this test")
	}
}

func TestMetrics_MultipleSnapshots(t *testing.T) {
	m := NewMetrics()

	// First snapshot
	snapshot1 := m.GetSnapshot()

	// Increment some counters
	m.IncrementConnections()
	m.IncrementCommands()

	// Second snapshot
	snapshot2 := m.GetSnapshot()

	// Verify snapshots are independent
	if snapshot1.ConnectionsTotal != 0 {
		t.Errorf("First snapshot ConnectionsTotal should be 0, got %d", snapshot1.ConnectionsTotal)
	}
	if snapshot2.ConnectionsTotal != 1 {
		t.Errorf("Second snapshot ConnectionsTotal should be 1, got %d", snapshot2.ConnectionsTotal)
	}

	// Verify uptime progresses
	if snapshot2.Uptime <= snapshot1.Uptime {
		t.Error("Second snapshot uptime should be greater than first")
	}
}

func TestServer_MetricsIntegration(t *testing.T) {
	srv := NewServer(":0", 30*time.Second)

	// Verify server has metrics
	if srv.metrics == nil {
		t.Fatal("Server should have metrics instance")
	}

	// Test GetMetrics method
	snapshot := srv.GetMetrics()

	if snapshot.ConnectionsTotal != 0 {
		t.Error("New server should have 0 connections")
	}

	// Simulate some activity
	srv.metrics.IncrementConnections()
	srv.metrics.IncrementCommands()

	snapshot = srv.GetMetrics()
	if snapshot.ConnectionsTotal != 1 {
		t.Errorf("Expected 1 connection, got %d", snapshot.ConnectionsTotal)
	}
	if snapshot.CommandsProcessed != 1 {
		t.Errorf("Expected 1 command processed, got %d", snapshot.CommandsProcessed)
	}
}

func BenchmarkMetrics_IncrementConnections(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.IncrementConnections()
		}
	})
}

func BenchmarkMetrics_GetSnapshot(b *testing.B) {
	m := NewMetrics()

	// Add some data
	for i := 0; i < 1000; i++ {
		m.IncrementConnections()
		m.IncrementCommands()
		m.IncrementErrors()
		m.IncrementPackages()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.GetSnapshot()
	}
}
