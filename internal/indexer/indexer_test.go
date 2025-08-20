package indexer

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// assertQuery checks if a package exists and fails the test if the expectation is not met.
func assertQuery(t *testing.T, idx *Indexer, pkg string, shouldExist bool) {
	t.Helper()
	if idx.QueryPackage(pkg) != shouldExist {
		t.Errorf("QueryPackage(%q) = %v, want %v", pkg, !shouldExist, shouldExist)
	}
}

// assertIndex checks the result of an index operation.
func assertIndex(t *testing.T, idx *Indexer, pkg string, deps []string, shouldSucceed bool) {
	t.Helper()
	if idx.IndexPackage(pkg, deps) != shouldSucceed {
		t.Errorf("IndexPackage(%q, %v) = %v, want %v", pkg, deps, !shouldSucceed, shouldSucceed)
	}
}

// assertRemove checks the result of a remove operation.
func assertRemove(t *testing.T, idx *Indexer, pkg string, expectedResult RemoveResult) {
	t.Helper()
	result := idx.RemovePackage(pkg)
	if result != expectedResult {
		t.Errorf("RemovePackage(%q) = %v, want %v", pkg, result, expectedResult)
	}
}

func TestIndexer_BasicOperations(t *testing.T) {
	idx := NewIndexer()

	// Test query on empty index
	assertQuery(t, idx, "nonexistent", false)

	// Test indexing package with no dependencies
	assertIndex(t, idx, "base", []string{}, true)

	// Test query after indexing
	assertQuery(t, idx, "base", true)

	// Test indexing package with satisfied dependencies
	assertIndex(t, idx, "app", []string{"base"}, true)

	// Test indexing package with missing dependencies
	assertIndex(t, idx, "invalid", []string{"missing"}, false)

	// Test removing package that has dependents
	assertRemove(t, idx, "base", RemoveResultBlocked)

	// Test removing leaf package
	assertRemove(t, idx, "app", RemoveResultOK)

	// Test removing non-existent package (idempotent)
	assertRemove(t, idx, "nonexistent", RemoveResultNotIndexed)

	// Now base should be removable
	assertRemove(t, idx, "base", RemoveResultOK)
}

func TestIndexer_RemoveOperations(t *testing.T) {
	idx := NewIndexer()

	// Set up test data
	assertIndex(t, idx, "base", []string{}, true)
	assertIndex(t, idx, "app", []string{"base"}, true)

	// Test removing package that has dependents
	assertRemove(t, idx, "base", RemoveResultBlocked)

	// Test removing leaf package
	assertRemove(t, idx, "app", RemoveResultOK)

	// Test removing non-existent package (idempotent)
	assertRemove(t, idx, "nonexistent", RemoveResultNotIndexed)

	// Now base should be removable
	assertRemove(t, idx, "base", RemoveResultOK)
}

func TestIndexer_ReindexOperations(t *testing.T) {
	idx := NewIndexer()

	// Set up initial state
	assertIndex(t, idx, "base1", []string{}, true)
	assertIndex(t, idx, "base2", []string{}, true)
	assertIndex(t, idx, "app", []string{"base1"}, true)

	// Test re-indexing with different dependencies
	assertIndex(t, idx, "app", []string{"base2"}, true)

	// Verify old dependency relationship is removed
	assertRemove(t, idx, "base1", RemoveResultOK)

	// Verify new dependency relationship exists
	assertRemove(t, idx, "base2", RemoveResultBlocked)
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
			if result := idx.RemovePackage(pkgName); result != RemoveResultOK {
				t.Errorf("Should be able to remove package %s, got result %v", pkgName, result)
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
