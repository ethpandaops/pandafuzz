package types

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// CorpusCollection represents a collection of corpus entries
type CorpusCollection struct {
	mu          sync.RWMutex
	entries     map[string]*CorpusEntry
	name        string
	description string
	createdAt   time.Time
	maxSize     int
}

// CollectionStats provides statistics about the corpus collection
type CollectionStats struct {
	TotalEntries      int       `json:"total_entries"`
	TotalSize         int64     `json:"total_size"`
	TotalExecutions   uint64    `json:"total_executions"`
	AverageCoverage   float64   `json:"average_coverage"`
	UniqueEdges       uint32    `json:"unique_edges"`
	InterestingInputs int       `json:"interesting_inputs"`
	LastUpdated       time.Time `json:"last_updated"`
}

// NewCorpusCollection creates a new corpus collection
func NewCorpusCollection(name string, maxSize int) (*CorpusCollection, error) {
	if name == "" {
		return nil, errors.New("collection name cannot be empty")
	}
	if maxSize < 0 {
		return nil, errors.New("max size cannot be negative")
	}

	return &CorpusCollection{
		entries:   make(map[string]*CorpusEntry),
		name:      name,
		createdAt: time.Now().UTC(),
		maxSize:   maxSize,
	}, nil
}

// Add adds an entry to the collection
func (c *CorpusCollection) Add(entry *CorpusEntry) error {
	if entry == nil {
		return errors.New("cannot add nil entry to collection")
	}

	if err := entry.Validate(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to enforce size limits
	if c.maxSize > 0 && len(c.entries) >= c.maxSize {
		// Remove least interesting entry
		if !c.shouldReplaceEntry(entry) {
			return errors.New("corpus collection is full and new entry is not interesting enough")
		}
		c.removeLeastInteresting()
	}

	c.entries[entry.ID] = entry
	return nil
}

// Get retrieves an entry by ID
func (c *CorpusCollection) Get(id string) (*CorpusEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[id]
	return entry, exists
}

// GetByHash retrieves an entry by its hash
func (c *CorpusCollection) GetByHash(hash string) (*CorpusEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, entry := range c.entries {
		if entry.Hash == hash {
			return entry, true
		}
	}
	return nil, false
}

// Remove removes an entry from the collection
func (c *CorpusCollection) Remove(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[id]; exists {
		delete(c.entries, id)
		return true
	}
	return false
}

// Size returns the number of entries in the collection
func (c *CorpusCollection) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// IsEmpty checks if the collection is empty
func (c *CorpusCollection) IsEmpty() bool {
	return c.Size() == 0
}

// GetAll returns all entries in the collection
func (c *CorpusCollection) GetAll() []*CorpusEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*CorpusEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}
	return entries
}

// GetInteresting returns only interesting entries (those with new coverage)
func (c *CorpusCollection) GetInteresting() []*CorpusEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	interesting := make([]*CorpusEntry, 0)
	for _, entry := range c.entries {
		if entry.IsInteresting() {
			interesting = append(interesting, entry)
		}
	}
	return interesting
}

// GetByTag returns entries with a specific tag
func (c *CorpusCollection) GetByTag(tag string) []*CorpusEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tagged := make([]*CorpusEntry, 0)
	for _, entry := range c.entries {
		for _, t := range entry.Tags {
			if t == tag {
				tagged = append(tagged, entry)
				break
			}
		}
	}
	return tagged
}

// GetTopCoverage returns the top N entries by coverage score
func (c *CorpusCollection) GetTopCoverage(n int) []*CorpusEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*CorpusEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}

	// Sort by coverage score descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Coverage.CoverageScore > entries[j].Coverage.CoverageScore
	})

	if n > len(entries) {
		n = len(entries)
	}
	return entries[:n]
}

// GetStats returns statistics about the collection
func (c *CorpusCollection) GetStats() CollectionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CollectionStats{
		TotalEntries: len(c.entries),
		LastUpdated:  time.Now().UTC(),
	}

	var totalCoverage float64
	edgeSet := make(map[uint32]bool)

	for _, entry := range c.entries {
		stats.TotalSize += int64(entry.Size)
		stats.TotalExecutions += entry.ExecutionCount
		totalCoverage += entry.Coverage.CoverageScore

		if entry.IsInteresting() {
			stats.InterestingInputs++
		}

		// Count unique edges (simplified - in reality would need actual edge IDs)
		for i := uint32(0); i < entry.Coverage.CoveredEdges; i++ {
			edgeSet[i] = true
		}
	}

	if stats.TotalEntries > 0 {
		stats.AverageCoverage = totalCoverage / float64(stats.TotalEntries)
	}
	stats.UniqueEdges = uint32(len(edgeSet))

	return stats
}

// Merge merges another collection into this one
func (c *CorpusCollection) Merge(other *CorpusCollection) error {
	if other == nil {
		return errors.New("cannot merge nil collection")
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	for _, entry := range other.entries {
		if err := c.Add(entry); err != nil {
			// Continue on error, don't fail the entire merge
			continue
		}
	}

	return nil
}

// Clear removes all entries from the collection
func (c *CorpusCollection) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CorpusEntry)
}

// shouldReplaceEntry determines if a new entry should replace an existing one
func (c *CorpusCollection) shouldReplaceEntry(entry *CorpusEntry) bool {
	// New coverage is always interesting
	if entry.Coverage.NewCoverage {
		return true
	}

	// Check if it has better coverage than the worst entry
	leastInteresting := c.findLeastInteresting()
	if leastInteresting != nil {
		return entry.Coverage.CoverageScore > leastInteresting.Coverage.CoverageScore
	}

	return false
}

// removeLeastInteresting removes the least interesting entry
func (c *CorpusCollection) removeLeastInteresting() {
	entry := c.findLeastInteresting()
	if entry != nil {
		delete(c.entries, entry.ID)
	}
}

// findLeastInteresting finds the least interesting entry (lowest coverage, oldest)
func (c *CorpusCollection) findLeastInteresting() *CorpusEntry {
	var leastInteresting *CorpusEntry
	var lowestScore float64 = 1.0

	for _, entry := range c.entries {
		if !entry.IsInteresting() && entry.Coverage.CoverageScore <= lowestScore {
			if leastInteresting == nil || entry.Age() > leastInteresting.Age() {
				leastInteresting = entry
				lowestScore = entry.Coverage.CoverageScore
			}
		}
	}

	// If all entries are interesting, find the one with lowest coverage
	if leastInteresting == nil {
		for _, entry := range c.entries {
			if entry.Coverage.CoverageScore < lowestScore {
				leastInteresting = entry
				lowestScore = entry.Coverage.CoverageScore
			}
		}
	}

	return leastInteresting
}

// Name returns the collection name
func (c *CorpusCollection) Name() string {
	return c.name
}

// Description returns the collection description
func (c *CorpusCollection) Description() string {
	return c.description
}

// SetDescription sets the collection description
func (c *CorpusCollection) SetDescription(desc string) {
	c.description = desc
}

// CreatedAt returns when the collection was created
func (c *CorpusCollection) CreatedAt() time.Time {
	return c.createdAt
}
