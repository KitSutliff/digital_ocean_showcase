package indexer

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestIndexer_BasicOperations(t *testing.T) {
	idx := NewIndexer()
	
	// Test query on empty index
	if idx.QueryPackage("nonexistent") {
		t.Error("Query should return false for non-existent package")
	}
	
	// Test indexing package with no dependencies
	if !idx.IndexPackage("base", []string{}) {
		t.Error("Should be able to index package with no dependencies")
	}
	
	// Test query after indexing
	if !idx.QueryPackage("base") {
		t.Error("Query should return true for indexed package")
	}
	
	// Test indexing package with satisfied dependencies
	if !idx.IndexPackage("app", []string{"base"}) {
		t.Error("Should be able to index package with satisfied dependencies")
	}
	
	// Test indexing package with missing dependencies
	if idx.IndexPackage("invalid", []string{"missing"}) {
		t.Error("Should not be able to index package with missing dependencies")
	}
}

func TestIndexer_RemoveOperations(t *testing.T) {
	idx := NewIndexer()
	
	// Set up test data
	idx.IndexPackage("base", []string{})
	idx.IndexPackage("app", []string{"base"})
	
	// Test removing package that has dependents
	ok, blocked := idx.RemovePackage("base")
	if ok || !blocked {
		t.Error("Should not be able to remove package with dependents")
	}
	
	// Test removing leaf package
	ok, blocked = idx.RemovePackage("app")
	if !ok || blocked {
		t.Error("Should be able to remove package without dependents")
	}
	
	// Test removing non-existent package (idempotent)
	ok, blocked = idx.RemovePackage("nonexistent")
	if !ok || blocked {
		t.Error("Removing non-existent package should be OK")
	}
	
	// Now base should be removable
	ok, blocked = idx.RemovePackage("base")
	if !ok || blocked {
		t.Error("Should be able to remove base after removing its dependents")
	}
}

func TestIndexer_ReindexOperations(t *testing.T) {
	idx := NewIndexer()
	
	// Set up initial state
	idx.IndexPackage("base1", []string{})
	idx.IndexPackage("base2", []string{})
	idx.IndexPackage("app", []string{"base1"})
	
	// Test re-indexing with different dependencies
	if !idx.IndexPackage("app", []string{"base2"}) {
		t.Error("Should be able to re-index package with different dependencies")
	}
	
	// Verify old dependency relationship is removed
	ok, blocked := idx.RemovePackage("base1")
	if !ok || blocked {
		t.Error("base1 should be removable after app no longer depends on it")
	}
	
	// Verify new dependency relationship exists
	ok, blocked = idx.RemovePackage("base2")
	if ok || !blocked {
		t.Error("base2 should not be removable while app depends on it")
	}
}

func TestIndexer_ConcurrentOperations(t *testing.T) {
	idx := NewIndexer()
	
	// Number of workers and operations per worker
	numWorkers := 20
	opsPerWorker := 50
	
	var wg sync.WaitGroup
	
	// Worker that performs mixed operations
	worker := func(workerID int) {
		defer wg.Done()
		
		for i := 0; i < opsPerWorker; i++ {
			pkgName := fmt.Sprintf("pkg-%d-%d", workerID, i)
			
			// Index package
			idx.IndexPackage(pkgName, []string{})
			
			// Query package multiple times (read operations should be concurrent)
			for j := 0; j < 5; j++ {
				if !idx.QueryPackage(pkgName) {
					t.Errorf("Package %s should be indexed", pkgName)
				}
			}
			
			// Small delay to increase chance of contention
			time.Sleep(time.Microsecond)
			
			// Remove package
			ok, blocked := idx.RemovePackage(pkgName)
			if !ok || blocked {
				t.Errorf("Should be able to remove package %s", pkgName)
			}
		}
	}
	
	// Start workers
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(i)
	}
	
	// Wait for completion
	wg.Wait()
	
	// Verify final state is clean
	indexed, deps, dependents := idx.GetStats()
	if indexed != 0 || deps != 0 || dependents != 0 {
		t.Errorf("Expected clean final state, got: indexed=%d, deps=%d, dependents=%d", 
			indexed, deps, dependents)
	}
}

func TestStringSet_Operations(t *testing.T) {
	s := NewStringSet()
	
	// Test empty set
	if s.Len() != 0 {
		t.Error("New set should be empty")
	}
	if s.Contains("item") {
		t.Error("Empty set should not contain any items")
	}
	
	// Test add operation
	s.Add("item1")
	s.Add("item2")
	if s.Len() != 2 {
		t.Error("Set should contain 2 items")
	}
	if !s.Contains("item1") || !s.Contains("item2") {
		t.Error("Set should contain added items")
	}
	
	// Test duplicate add (should be idempotent)
	s.Add("item1")
	if s.Len() != 2 {
		t.Error("Adding duplicate should not change set size")
	}
	
	// Test remove operation
	s.Remove("item1")
	if s.Len() != 1 {
		t.Error("Set should contain 1 item after removal")
	}
	if s.Contains("item1") {
		t.Error("Set should not contain removed item")
	}
	
	// Test copy operation
	s.Add("item3")
	copy := s.Copy()
	if copy.Len() != s.Len() {
		t.Error("Copy should have same size as original")
	}
	copy.Add("item4")
	if s.Contains("item4") {
		t.Error("Modifying copy should not affect original")
	}
}
