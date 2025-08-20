// Package indexer implements a thread-safe in-memory dependency graph for package management.
// This is the core business logic component, optimized for O(1) query operations and O(D) 
// modification operations where D is the dependency count. The dual-map architecture enables
// efficient validation of dependency constraints in both directions.
package indexer

import (
	"sync"
)

// StringSet represents a set of strings using Go's map implementation for O(1) operations.
// This provides memory-efficient storage with constant-time lookups, essential for dependency
// graph performance under high concurrent load.
type StringSet map[string]struct{}

// NewStringSet creates a new empty string set
func NewStringSet() StringSet {
	return make(StringSet)
}

// Add adds an item to the set
func (s StringSet) Add(item string) {
	s[item] = struct{}{}
}

// Remove removes an item from the set
func (s StringSet) Remove(item string) {
	delete(s, item)
}

// Contains checks if an item exists in the set
func (s StringSet) Contains(item string) bool {
	_, exists := s[item]
	return exists
}

// Len returns the number of items in the set
func (s StringSet) Len() int {
	return len(s)
}

// Copy creates a copy of the set
func (s StringSet) Copy() StringSet {
	result := NewStringSet()
	for item := range s {
		result.Add(item)
	}
	return result
}

// Indexer manages the package dependency graph with thread-safe operations.
// Architecture decision: Single RWMutex provides simple correctness guarantees while allowing
// concurrent reads (QUERY operations). The dual-map design enables O(1) dependency validation
// in both directions, critical for production performance under 100+ concurrent clients.
type Indexer struct {
	mu sync.RWMutex // RWMutex enables concurrent reads while ensuring exclusive writes for scalability

	indexed      StringSet                // Tracks indexed packages for O(1) existence checks
	dependencies map[string]StringSet     // Maps package to its dependencies (forward edges)
	dependents   map[string]StringSet     // Maps package to its dependents (reverse edges)
}

// RemoveResult represents the outcome of a remove operation using type-safe enums.
// This replaces the previous error-prone boolean tuple approach, improving code clarity
// and reducing operational mistakes in production environments.
type RemoveResult int

// RemoveResult enumeration for type-safe remove operation outcomes
const (
	RemoveResultOK         RemoveResult = iota // Package successfully removed
	RemoveResultNotIndexed                     // Package was not indexed (idempotent success)
	RemoveResultBlocked                        // Package has dependents (cannot remove)
)

// removeDependentReference removes a reverse dependency reference with cleanup
func (idx *Indexer) removeDependentReference(dependency string, pkg string) {
	if idx.dependents[dependency] != nil {
		idx.dependents[dependency].Remove(pkg)
		if idx.dependents[dependency].Len() == 0 {
			delete(idx.dependents, dependency)
		}
	}
}

// NewIndexer creates a new empty package indexer
func NewIndexer() *Indexer {
	return &Indexer{
		indexed:      NewStringSet(),
		dependencies: make(map[string]StringSet),
		dependents:   make(map[string]StringSet),
	}
}

// IndexPackage attempts to add/update a package with given dependencies.
// Returns true if successful (OK), false if dependencies missing (FAIL).
func (idx *Indexer) IndexPackage(pkg string, deps []string) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Check if all dependencies are already indexed
	for _, dep := range deps {
		if !idx.indexed.Contains(dep) {
			return false // FAIL - dependency not indexed
		}
	}

	// Get old dependencies for cleanup
	oldDeps := idx.dependencies[pkg]
	if oldDeps == nil {
		oldDeps = NewStringSet()
	}

	// Create new dependency set
	newDeps := NewStringSet()
	for _, dep := range deps {
		newDeps.Add(dep)
	}

	// Remove old reverse dependencies that are no longer needed
	for oldDep := range oldDeps {
		if !newDeps.Contains(oldDep) { // Only remove if not in new deps
			idx.removeDependentReference(oldDep, pkg)
		}
	}

	// Add new reverse dependencies
	for _, newDep := range deps {
		if idx.dependents[newDep] == nil {
			idx.dependents[newDep] = NewStringSet()
		}
		idx.dependents[newDep].Add(pkg)
	}

	// Update package state
	idx.indexed.Add(pkg)
	idx.dependencies[pkg] = newDeps

	return true // OK
}

// RemovePackage attempts to remove a package from the index.
// Cannot remove packages with active dependents. Operation is idempotent.
func (idx *Indexer) RemovePackage(pkg string) RemoveResult {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// If not indexed, removal is OK (idempotent)
	if !idx.indexed.Contains(pkg) {
		return RemoveResultNotIndexed
	}

	// Check if any packages depend on this one
	if dependents := idx.dependents[pkg]; dependents != nil && dependents.Len() > 0 {
		return RemoveResultBlocked // FAIL - has dependents
	}

	// Remove from index
	idx.indexed.Remove(pkg)

	// Clean up forward dependencies and their reverse links
	if deps := idx.dependencies[pkg]; deps != nil {
		for dep := range deps {
			idx.removeDependentReference(dep, pkg)
		}
		delete(idx.dependencies, pkg)
	}

	// Clean up reverse dependencies (should be empty but defensive)
	delete(idx.dependents, pkg)

	return RemoveResultOK // OK
}

// QueryPackage checks if a package is indexed (read-only operation)
func (idx *Indexer) QueryPackage(pkg string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.indexed.Contains(pkg)
}

// GetStats returns current index statistics for monitoring
func (idx *Indexer) GetStats() (indexed int, totalDeps int, totalReverseDeps int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	indexed = idx.indexed.Len()
	totalDeps = len(idx.dependencies)
	totalReverseDeps = len(idx.dependents)
	return
}
