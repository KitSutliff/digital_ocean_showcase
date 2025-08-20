package server

import (
	"sync"
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()
	
	if m == nil {
		t.Fatal("NewMetrics should return a non-nil metrics instance")
	}
	
	// Check initial values
	snapshot := m.GetSnapshot()
	if snapshot.ConnectionsTotal != 0 {
		t.Errorf("Expected ConnectionsTotal to be 0, got %d", snapshot.ConnectionsTotal)
	}
	if snapshot.CommandsProcessed != 0 {
		t.Errorf("Expected CommandsProcessed to be 0, got %d", snapshot.CommandsProcessed)
	}
	if snapshot.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount to be 0, got %d", snapshot.ErrorCount)
	}
	if snapshot.PackagesIndexed != 0 {
		t.Errorf("Expected PackagesIndexed to be 0, got %d", snapshot.PackagesIndexed)
	}
	
	// Check that start time is recent
	if time.Since(m.StartTime) > time.Second {
		t.Error("StartTime should be recent")
	}
}

func TestMetrics_IncrementOperations(t *testing.T) {
	m := NewMetrics()
	
	// Test each increment operation
	m.IncrementConnections()
	m.IncrementCommands()
	m.IncrementErrors()
	m.IncrementPackages()
	
	snapshot := m.GetSnapshot()
	
	if snapshot.ConnectionsTotal != 1 {
		t.Errorf("Expected ConnectionsTotal to be 1, got %d", snapshot.ConnectionsTotal)
	}
	if snapshot.CommandsProcessed != 1 {
		t.Errorf("Expected CommandsProcessed to be 1, got %d", snapshot.CommandsProcessed)
	}
	if snapshot.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount to be 1, got %d", snapshot.ErrorCount)
	}
	if snapshot.PackagesIndexed != 1 {
		t.Errorf("Expected PackagesIndexed to be 1, got %d", snapshot.PackagesIndexed)
	}
}

func TestMetrics_ConcurrentIncrements(t *testing.T) {
	m := NewMetrics()
	const numGoroutines = 100
	const incrementsPerGoroutine = 10
	
	var wg sync.WaitGroup
	
	// Test concurrent connections increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				m.IncrementConnections()
			}
		}()
	}
	
	// Test concurrent commands increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				m.IncrementCommands()
			}
		}()
	}
	
	// Test concurrent errors increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				m.IncrementErrors()
			}
		}()
	}
	
	// Test concurrent packages increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				m.IncrementPackages()
			}
		}()
	}
	
	wg.Wait()
	
	expectedCount := int64(numGoroutines * incrementsPerGoroutine)
	snapshot := m.GetSnapshot()
	
	if snapshot.ConnectionsTotal != expectedCount {
		t.Errorf("Expected ConnectionsTotal to be %d, got %d", expectedCount, snapshot.ConnectionsTotal)
	}
	if snapshot.CommandsProcessed != expectedCount {
		t.Errorf("Expected CommandsProcessed to be %d, got %d", expectedCount, snapshot.CommandsProcessed)
	}
	if snapshot.ErrorCount != expectedCount {
		t.Errorf("Expected ErrorCount to be %d, got %d", expectedCount, snapshot.ErrorCount)
	}
	if snapshot.PackagesIndexed != expectedCount {
		t.Errorf("Expected PackagesIndexed to be %d, got %d", expectedCount, snapshot.PackagesIndexed)
	}
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
	srv := NewServer(":0")
	
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
