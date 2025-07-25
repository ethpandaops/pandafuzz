package sync

import (
	"sort"
	"sync"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// CollectionDiff represents the differences between two collections
type CollectionDiff struct {
	Added    []*types.CorpusEntry
	Removed  []*types.CorpusEntry
	Modified []ModifiedEntry
}

// ModifiedEntry represents an entry that exists in both collections but differs
type ModifiedEntry struct {
	Source *types.CorpusEntry
	Target *types.CorpusEntry
}

// DiffStats provides statistics about the differences
type DiffStats struct {
	TotalSourceEntries int
	TotalTargetEntries int
	AddedCount         int
	RemovedCount       int
	ModifiedCount      int
	UnchangedCount     int
	CoverageDelta      float64
}

// Differ calculates differences between corpus collections
type Differ struct {
	mu              sync.RWMutex
	compareMetadata bool
	compareTags     bool
	compareCoverage bool
}

// NewDiffer creates a new corpus differ
func NewDiffer() *Differ {
	return &Differ{
		compareMetadata: true,
		compareTags:     true,
		compareCoverage: true,
	}
}

// SetCompareOptions configures what fields to compare
func (d *Differ) SetCompareOptions(metadata, tags, coverage bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.compareMetadata = metadata
	d.compareTags = tags
	d.compareCoverage = coverage
}

// CalculateDiff calculates the differences between source and target collections
func (d *Differ) CalculateDiff(source, target []*types.CorpusEntry) *CollectionDiff {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Create maps for efficient lookup
	sourceMap := make(map[string]*types.CorpusEntry)
	targetMap := make(map[string]*types.CorpusEntry)

	for _, entry := range source {
		sourceMap[entry.ID] = entry
	}

	for _, entry := range target {
		targetMap[entry.ID] = entry
	}

	diff := &CollectionDiff{
		Added:    make([]*types.CorpusEntry, 0),
		Removed:  make([]*types.CorpusEntry, 0),
		Modified: make([]ModifiedEntry, 0),
	}

	// Find added and modified entries
	for id, sourceEntry := range sourceMap {
		if targetEntry, exists := targetMap[id]; exists {
			// Entry exists in both - check if modified
			if d.entriesAreDifferent(sourceEntry, targetEntry) {
				diff.Modified = append(diff.Modified, ModifiedEntry{
					Source: sourceEntry,
					Target: targetEntry,
				})
			}
		} else {
			// Entry only exists in source - it's added
			diff.Added = append(diff.Added, sourceEntry)
		}
	}

	// Find removed entries
	for id, targetEntry := range targetMap {
		if _, exists := sourceMap[id]; !exists {
			// Entry only exists in target - it's removed
			diff.Removed = append(diff.Removed, targetEntry)
		}
	}

	// Sort results for consistent output
	d.sortDiff(diff)

	return diff
}

// CalculateDiffWithStats calculates differences and provides statistics
func (d *Differ) CalculateDiffWithStats(source, target []*types.CorpusEntry) (*CollectionDiff, *DiffStats) {
	diff := d.CalculateDiff(source, target)

	stats := &DiffStats{
		TotalSourceEntries: len(source),
		TotalTargetEntries: len(target),
		AddedCount:         len(diff.Added),
		RemovedCount:       len(diff.Removed),
		ModifiedCount:      len(diff.Modified),
	}

	// Calculate unchanged count
	stats.UnchangedCount = len(source) - stats.AddedCount - stats.ModifiedCount

	// Calculate coverage delta
	sourceCoverage := d.calculateAverageCoverage(source)
	targetCoverage := d.calculateAverageCoverage(target)
	stats.CoverageDelta = sourceCoverage - targetCoverage

	return diff, stats
}

// DiffByHash calculates differences using hash comparison (faster but less detailed)
func (d *Differ) DiffByHash(source, target []*types.CorpusEntry) *CollectionDiff {
	// Create hash maps
	sourceHashes := make(map[string]*types.CorpusEntry)
	targetHashes := make(map[string]*types.CorpusEntry)

	for _, entry := range source {
		sourceHashes[entry.Hash] = entry
	}

	for _, entry := range target {
		targetHashes[entry.Hash] = entry
	}

	diff := &CollectionDiff{
		Added:    make([]*types.CorpusEntry, 0),
		Removed:  make([]*types.CorpusEntry, 0),
		Modified: make([]ModifiedEntry, 0),
	}

	// Find added entries (by hash)
	for hash, sourceEntry := range sourceHashes {
		if _, exists := targetHashes[hash]; !exists {
			diff.Added = append(diff.Added, sourceEntry)
		}
	}

	// Find removed entries (by hash)
	for hash, targetEntry := range targetHashes {
		if _, exists := sourceHashes[hash]; !exists {
			diff.Removed = append(diff.Removed, targetEntry)
		}
	}

	// Sort results
	d.sortDiff(diff)

	return diff
}

// MergeDiffs combines multiple diffs into a single diff
func (d *Differ) MergeDiffs(diffs ...*CollectionDiff) *CollectionDiff {
	merged := &CollectionDiff{
		Added:    make([]*types.CorpusEntry, 0),
		Removed:  make([]*types.CorpusEntry, 0),
		Modified: make([]ModifiedEntry, 0),
	}

	// Track seen entries to avoid duplicates
	seenAdded := make(map[string]bool)
	seenRemoved := make(map[string]bool)
	seenModified := make(map[string]bool)

	for _, diff := range diffs {
		// Merge added entries
		for _, entry := range diff.Added {
			if !seenAdded[entry.ID] {
				merged.Added = append(merged.Added, entry)
				seenAdded[entry.ID] = true
			}
		}

		// Merge removed entries
		for _, entry := range diff.Removed {
			if !seenRemoved[entry.ID] {
				merged.Removed = append(merged.Removed, entry)
				seenRemoved[entry.ID] = true
			}
		}

		// Merge modified entries
		for _, mod := range diff.Modified {
			if !seenModified[mod.Source.ID] {
				merged.Modified = append(merged.Modified, mod)
				seenModified[mod.Source.ID] = true
			}
		}
	}

	// Sort results
	d.sortDiff(merged)

	return merged
}

// entriesAreDifferent checks if two entries are different based on configured options
func (d *Differ) entriesAreDifferent(e1, e2 *types.CorpusEntry) bool {
	// Always compare basic fields
	if e1.Hash != e2.Hash || e1.Size != e2.Size {
		return true
	}

	// Compare coverage if enabled
	if d.compareCoverage {
		if d.coverageIsDifferent(&e1.Coverage, &e2.Coverage) {
			return true
		}
	}

	// Compare tags if enabled
	if d.compareTags {
		if d.tagsAreDifferent(e1.Tags, e2.Tags) {
			return true
		}
	}

	// Compare metadata if enabled
	if d.compareMetadata {
		if d.metadataIsDifferent(e1.Metadata, e2.Metadata) {
			return true
		}
	}

	return false
}

// coverageIsDifferent checks if coverage information differs
func (d *Differ) coverageIsDifferent(c1, c2 *types.CoverageInfo) bool {
	return c1.TotalBlocks != c2.TotalBlocks ||
		c1.CoveredBlocks != c2.CoveredBlocks ||
		c1.TotalEdges != c2.TotalEdges ||
		c1.CoveredEdges != c2.CoveredEdges ||
		c1.CoverageScore != c2.CoverageScore ||
		c1.NewCoverage != c2.NewCoverage ||
		c1.CoverageGained != c2.CoverageGained
}

// tagsAreDifferent checks if tag lists differ
func (d *Differ) tagsAreDifferent(tags1, tags2 []string) bool {
	if len(tags1) != len(tags2) {
		return true
	}

	// Create sorted copies
	sorted1 := make([]string, len(tags1))
	sorted2 := make([]string, len(tags2))
	copy(sorted1, tags1)
	copy(sorted2, tags2)
	sort.Strings(sorted1)
	sort.Strings(sorted2)

	// Compare sorted lists
	for i := range sorted1 {
		if sorted1[i] != sorted2[i] {
			return true
		}
	}

	return false
}

// metadataIsDifferent checks if metadata maps differ
func (d *Differ) metadataIsDifferent(meta1, meta2 map[string]string) bool {
	if len(meta1) != len(meta2) {
		return true
	}

	for key, val1 := range meta1 {
		if val2, exists := meta2[key]; !exists || val1 != val2 {
			return true
		}
	}

	return false
}

// calculateAverageCoverage calculates the average coverage score for a collection
func (d *Differ) calculateAverageCoverage(entries []*types.CorpusEntry) float64 {
	if len(entries) == 0 {
		return 0.0
	}

	var total float64
	for _, entry := range entries {
		total += entry.Coverage.CoverageScore
	}

	return total / float64(len(entries))
}

// sortDiff sorts the diff results for consistent output
func (d *Differ) sortDiff(diff *CollectionDiff) {
	// Sort added entries by ID
	sort.Slice(diff.Added, func(i, j int) bool {
		return diff.Added[i].ID < diff.Added[j].ID
	})

	// Sort removed entries by ID
	sort.Slice(diff.Removed, func(i, j int) bool {
		return diff.Removed[i].ID < diff.Removed[j].ID
	})

	// Sort modified entries by source ID
	sort.Slice(diff.Modified, func(i, j int) bool {
		return diff.Modified[i].Source.ID < diff.Modified[j].Source.ID
	})
}

// FilterDiff filters a diff based on criteria
func (d *Differ) FilterDiff(diff *CollectionDiff, filter DiffFilter) *CollectionDiff {
	filtered := &CollectionDiff{
		Added:    make([]*types.CorpusEntry, 0),
		Removed:  make([]*types.CorpusEntry, 0),
		Modified: make([]ModifiedEntry, 0),
	}

	// Filter added entries
	for _, entry := range diff.Added {
		if filter.ShouldInclude(entry) {
			filtered.Added = append(filtered.Added, entry)
		}
	}

	// Filter removed entries
	for _, entry := range diff.Removed {
		if filter.ShouldInclude(entry) {
			filtered.Removed = append(filtered.Removed, entry)
		}
	}

	// Filter modified entries
	for _, mod := range diff.Modified {
		if filter.ShouldInclude(mod.Source) || filter.ShouldInclude(mod.Target) {
			filtered.Modified = append(filtered.Modified, mod)
		}
	}

	return filtered
}

// DiffFilter defines criteria for filtering diff results
type DiffFilter interface {
	ShouldInclude(entry *types.CorpusEntry) bool
}

// CoverageDiffFilter filters based on coverage score
type CoverageDiffFilter struct {
	MinCoverage float64
}

// ShouldInclude checks if entry meets coverage criteria
func (f CoverageDiffFilter) ShouldInclude(entry *types.CorpusEntry) bool {
	return entry.Coverage.CoverageScore >= f.MinCoverage
}

// TagDiffFilter filters based on tags
type TagDiffFilter struct {
	RequiredTags []string
	ExcludedTags []string
}

// ShouldInclude checks if entry has required tags and no excluded tags
func (f TagDiffFilter) ShouldInclude(entry *types.CorpusEntry) bool {
	// Check excluded tags first
	for _, excludedTag := range f.ExcludedTags {
		for _, entryTag := range entry.Tags {
			if entryTag == excludedTag {
				return false
			}
		}
	}

	// Check required tags
	if len(f.RequiredTags) > 0 {
		for _, requiredTag := range f.RequiredTags {
			found := false
			for _, entryTag := range entry.Tags {
				if entryTag == requiredTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// SizeDiffFilter filters based on entry size
type SizeDiffFilter struct {
	MinSize int
	MaxSize int
}

// ShouldInclude checks if entry size is within range
func (f SizeDiffFilter) ShouldInclude(entry *types.CorpusEntry) bool {
	if f.MinSize > 0 && entry.Size < f.MinSize {
		return false
	}
	if f.MaxSize > 0 && entry.Size > f.MaxSize {
		return false
	}
	return true
}
