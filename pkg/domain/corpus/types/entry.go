package types

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// CorpusEntry represents a test case in the fuzzing corpus
type CorpusEntry struct {
	ID             string            `json:"id"`
	Input          []byte            `json:"input"`
	Hash           string            `json:"hash"`
	Size           int               `json:"size"`
	CreatedAt      time.Time         `json:"created_at"`
	LastExecutedAt *time.Time        `json:"last_executed_at,omitempty"`
	ExecutionCount uint64            `json:"execution_count"`
	Coverage       CoverageInfo      `json:"coverage"`
	MutationInfo   MutationInfo      `json:"mutation_info"`
	Tags           []string          `json:"tags,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// CoverageInfo contains coverage information for a corpus entry
type CoverageInfo struct {
	TotalBlocks    uint32  `json:"total_blocks"`
	CoveredBlocks  uint32  `json:"covered_blocks"`
	TotalEdges     uint32  `json:"total_edges"`
	CoveredEdges   uint32  `json:"covered_edges"`
	CoverageScore  float64 `json:"coverage_score"`
	NewCoverage    bool    `json:"new_coverage"`
	CoverageGained uint32  `json:"coverage_gained"`
}

// MutationInfo tracks how this entry was created
type MutationInfo struct {
	ParentID       string    `json:"parent_id,omitempty"`
	MutationMethod string    `json:"mutation_method,omitempty"`
	Generation     uint32    `json:"generation"`
	CreatedAt      time.Time `json:"created_at"`
}

// NewCorpusEntry creates a new corpus entry with the given input
func NewCorpusEntry(input []byte) (*CorpusEntry, error) {
	if len(input) == 0 {
		return nil, errors.New("input cannot be empty")
	}

	hash := sha256.Sum256(input)
	hashStr := hex.EncodeToString(hash[:])

	return &CorpusEntry{
		ID:        hashStr[:16], // Use first 16 chars of hash as ID
		Input:     input,
		Hash:      hashStr,
		Size:      len(input),
		CreatedAt: time.Now().UTC(),
		Coverage: CoverageInfo{
			CoverageScore: 0.0,
		},
		MutationInfo: MutationInfo{
			Generation: 0,
			CreatedAt:  time.Now().UTC(),
		},
		Tags:     make([]string, 0),
		Metadata: make(map[string]string),
	}, nil
}

// NewCorpusEntryWithParent creates a new corpus entry derived from a parent
func NewCorpusEntryWithParent(input []byte, parentID string, mutationMethod string) (*CorpusEntry, error) {
	entry, err := NewCorpusEntry(input)
	if err != nil {
		return nil, err
	}

	entry.MutationInfo.ParentID = parentID
	entry.MutationInfo.MutationMethod = mutationMethod
	entry.MutationInfo.Generation = 1 // Will be updated based on parent

	return entry, nil
}

// UpdateCoverage updates the coverage information for this entry
func (c *CorpusEntry) UpdateCoverage(coverage CoverageInfo) {
	c.Coverage = coverage
	c.Coverage.CoverageScore = c.calculateCoverageScore()
}

// calculateCoverageScore computes a normalized coverage score
func (c *CorpusEntry) calculateCoverageScore() float64 {
	if c.Coverage.TotalBlocks == 0 && c.Coverage.TotalEdges == 0 {
		return 0.0
	}

	blockScore := float64(c.Coverage.CoveredBlocks) / float64(c.Coverage.TotalBlocks)
	edgeScore := float64(c.Coverage.CoveredEdges) / float64(c.Coverage.TotalEdges)

	// Weight edges more heavily than blocks
	return (blockScore * 0.3) + (edgeScore * 0.7)
}

// Execute marks this entry as executed
func (c *CorpusEntry) Execute() {
	now := time.Now().UTC()
	c.LastExecutedAt = &now
	c.ExecutionCount++
}

// AddTag adds a tag to the corpus entry
func (c *CorpusEntry) AddTag(tag string) {
	for _, t := range c.Tags {
		if t == tag {
			return // Tag already exists
		}
	}
	c.Tags = append(c.Tags, tag)
}

// RemoveTag removes a tag from the corpus entry
func (c *CorpusEntry) RemoveTag(tag string) {
	for i, t := range c.Tags {
		if t == tag {
			c.Tags = append(c.Tags[:i], c.Tags[i+1:]...)
			return
		}
	}
}

// SetMetadata sets a metadata key-value pair
func (c *CorpusEntry) SetMetadata(key, value string) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
}

// IsInteresting determines if this entry provides interesting coverage
func (c *CorpusEntry) IsInteresting() bool {
	return c.Coverage.NewCoverage || c.Coverage.CoverageGained > 0
}

// Age returns the duration since this entry was created
func (c *CorpusEntry) Age() time.Duration {
	return time.Since(c.CreatedAt)
}

// Validate ensures the corpus entry is in a valid state
func (c *CorpusEntry) Validate() error {
	if c.ID == "" {
		return errors.New("corpus entry ID cannot be empty")
	}
	if len(c.Input) == 0 {
		return errors.New("corpus entry input cannot be empty")
	}
	if c.Hash == "" {
		return errors.New("corpus entry hash cannot be empty")
	}
	if c.Size != len(c.Input) {
		return errors.New("corpus entry size does not match input length")
	}
	return nil
}
