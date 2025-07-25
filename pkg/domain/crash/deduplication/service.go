package deduplication

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// Algorithm defines the interface for deduplication algorithms
type Algorithm interface {
	// Name returns the algorithm name
	Name() string

	// IsDuplicate checks if the new crash is a duplicate of an existing crash
	IsDuplicate(existing, new *types.Crash) bool

	// FindDuplicates finds all duplicates of the given crash in a collection
	FindDuplicates(crash *types.Crash, candidates []*types.Crash) []*types.Crash

	// CalculateSimilarity returns a similarity score between 0 and 1
	CalculateSimilarity(crash1, crash2 *types.Crash) float64

	// GroupCrashes groups crashes by similarity
	GroupCrashes(crashes []*types.Crash, threshold float64) [][]*types.Crash
}

// Service provides crash deduplication functionality
type Service struct {
	repo       repository.CrashRepository
	algorithms map[string]Algorithm
	config     Config
	stats      *Statistics
	mu         sync.RWMutex
}

// Config holds configuration for the deduplication service
type Config struct {
	// DefaultAlgorithm specifies the default algorithm to use
	DefaultAlgorithm string

	// SimilarityThreshold is the minimum similarity score to consider crashes as duplicates
	SimilarityThreshold float64

	// BatchSize for batch processing
	BatchSize int

	// EnableStatistics enables tracking of deduplication statistics
	EnableStatistics bool

	// CacheDuration for caching deduplication results
	CacheDuration time.Duration

	// MaxCandidates limits the number of candidates to check for duplicates
	MaxCandidates int
}

// Statistics tracks deduplication metrics
type Statistics struct {
	TotalProcessed   uint64
	DuplicatesFound  uint64
	UniqueGroups     uint64
	AverageGroupSize float64
	ProcessingTime   time.Duration
	LastProcessedAt  time.Time
	AlgorithmUsage   map[string]uint64
	mu               sync.RWMutex
}

// DeduplicationResult represents the result of a deduplication operation
type DeduplicationResult struct {
	IsDuplicate    bool
	OriginalCrash  *types.Crash
	Duplicates     []*types.Crash
	SimilarCrashes []*types.Crash
	GroupID        string
	Confidence     float64
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		DefaultAlgorithm:    "hash_based",
		SimilarityThreshold: 0.85,
		BatchSize:           100,
		EnableStatistics:    true,
		CacheDuration:       5 * time.Minute,
		MaxCandidates:       1000,
	}
}

// NewService creates a new deduplication service
func NewService(repo repository.CrashRepository, config Config) *Service {
	service := &Service{
		repo:       repo,
		algorithms: make(map[string]Algorithm),
		config:     config,
	}

	if config.EnableStatistics {
		service.stats = &Statistics{
			AlgorithmUsage: make(map[string]uint64),
		}
	}

	return service
}

// RegisterAlgorithm registers a deduplication algorithm
func (s *Service) RegisterAlgorithm(algorithm Algorithm) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if algorithm == nil {
		return errors.New("algorithm cannot be nil")
	}

	name := algorithm.Name()
	if name == "" {
		return errors.New("algorithm name cannot be empty")
	}

	s.algorithms[name] = algorithm
	return nil
}

// ProcessCrash checks if a crash is a duplicate and processes it accordingly
func (s *Service) ProcessCrash(ctx context.Context, crash *types.Crash) (*DeduplicationResult, error) {
	if crash == nil {
		return nil, errors.New("crash cannot be nil")
	}

	if err := crash.Validate(); err != nil {
		return nil, fmt.Errorf("invalid crash: %w", err)
	}

	startTime := time.Now()
	defer func() {
		if s.stats != nil {
			s.updateStatistics(time.Since(startTime))
		}
	}()

	// Get the algorithm to use
	algorithm, err := s.getAlgorithm(s.config.DefaultAlgorithm)
	if err != nil {
		return nil, err
	}

	// Find potential duplicates
	candidates, err := s.findCandidates(ctx, crash)
	if err != nil {
		return nil, fmt.Errorf("failed to find candidates: %w", err)
	}

	result := &DeduplicationResult{
		IsDuplicate:    false,
		SimilarCrashes: make([]*types.Crash, 0),
		Duplicates:     make([]*types.Crash, 0),
	}

	// Check for exact duplicates first
	for _, candidate := range candidates {
		if algorithm.IsDuplicate(candidate, crash) {
			result.IsDuplicate = true
			result.OriginalCrash = candidate
			result.Duplicates = append(result.Duplicates, candidate)
			result.Confidence = 1.0

			// Update occurrence count for the original crash
			if err := s.repo.RecordOccurrence(ctx, candidate.ID); err != nil {
				return nil, fmt.Errorf("failed to record occurrence: %w", err)
			}

			break
		}
	}

	// If no exact duplicate found, check for similar crashes
	if !result.IsDuplicate {
		for _, candidate := range candidates {
			similarity := algorithm.CalculateSimilarity(crash, candidate)
			if similarity >= s.config.SimilarityThreshold {
				result.SimilarCrashes = append(result.SimilarCrashes, candidate)
				if similarity > result.Confidence {
					result.Confidence = similarity
				}
			}
		}
	}

	// Generate group ID if crash belongs to a group
	if result.IsDuplicate || len(result.SimilarCrashes) > 0 {
		result.GroupID = s.generateGroupID(result)
	}

	if s.stats != nil {
		s.updateDeduplicationStats(result, algorithm.Name())
	}

	return result, nil
}

// ProcessBatch processes multiple crashes in batch for efficiency
func (s *Service) ProcessBatch(ctx context.Context, crashes []*types.Crash) ([]*DeduplicationResult, error) {
	if len(crashes) == 0 {
		return nil, nil
	}

	results := make([]*DeduplicationResult, len(crashes))
	var wg sync.WaitGroup
	errChan := make(chan error, len(crashes))

	// Process in batches
	for i := 0; i < len(crashes); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(crashes) {
			end = len(crashes)
		}

		batch := crashes[i:end]

		for idx, crash := range batch {
			wg.Add(1)
			go func(index int, c *types.Crash) {
				defer wg.Done()

				result, err := s.ProcessCrash(ctx, c)
				if err != nil {
					errChan <- fmt.Errorf("failed to process crash %s: %w", c.ID, err)
					return
				}

				results[i+index] = result
			}(idx, crash)
		}
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return results, fmt.Errorf("batch processing encountered %d errors: %v", len(errs), errs[0])
	}

	return results, nil
}

// GroupSimilarCrashes groups crashes by similarity
func (s *Service) GroupSimilarCrashes(ctx context.Context, algorithmName string) ([][]*types.Crash, error) {
	algorithm, err := s.getAlgorithm(algorithmName)
	if err != nil {
		return nil, err
	}

	// Fetch all crashes
	crashes, _, err := s.repo.List(ctx, 0, -1)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch crashes: %w", err)
	}

	if len(crashes) == 0 {
		return nil, nil
	}

	// Group crashes using the algorithm
	groups := algorithm.GroupCrashes(crashes, s.config.SimilarityThreshold)

	if s.stats != nil {
		s.updateGroupingStats(groups)
	}

	return groups, nil
}

// FindDuplicatesOf finds all duplicates of a specific crash
func (s *Service) FindDuplicatesOf(ctx context.Context, crashID string, algorithmName string) ([]*types.Crash, error) {
	crash, err := s.repo.FindByID(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to find crash: %w", err)
	}

	algorithm, err := s.getAlgorithm(algorithmName)
	if err != nil {
		return nil, err
	}

	candidates, err := s.findCandidates(ctx, crash)
	if err != nil {
		return nil, fmt.Errorf("failed to find candidates: %w", err)
	}

	return algorithm.FindDuplicates(crash, candidates), nil
}

// GetStatistics returns deduplication statistics
func (s *Service) GetStatistics() *Statistics {
	if s.stats == nil {
		return nil
	}

	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	// Create a copy to avoid race conditions
	statsCopy := &Statistics{
		TotalProcessed:   s.stats.TotalProcessed,
		DuplicatesFound:  s.stats.DuplicatesFound,
		UniqueGroups:     s.stats.UniqueGroups,
		AverageGroupSize: s.stats.AverageGroupSize,
		ProcessingTime:   s.stats.ProcessingTime,
		LastProcessedAt:  s.stats.LastProcessedAt,
		AlgorithmUsage:   make(map[string]uint64),
	}

	for k, v := range s.stats.AlgorithmUsage {
		statsCopy.AlgorithmUsage[k] = v
	}

	return statsCopy
}

// ResetStatistics resets deduplication statistics
func (s *Service) ResetStatistics() {
	if s.stats == nil {
		return
	}

	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	s.stats.TotalProcessed = 0
	s.stats.DuplicatesFound = 0
	s.stats.UniqueGroups = 0
	s.stats.AverageGroupSize = 0
	s.stats.ProcessingTime = 0
	s.stats.AlgorithmUsage = make(map[string]uint64)
}

// Private helper methods

func (s *Service) getAlgorithm(name string) (Algorithm, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	algorithm, exists := s.algorithms[name]
	if !exists {
		return nil, fmt.Errorf("algorithm '%s' not found", name)
	}

	return algorithm, nil
}

func (s *Service) findCandidates(ctx context.Context, crash *types.Crash) ([]*types.Crash, error) {
	// Start with signature-based search
	var candidates []*types.Crash

	if crash.Signature != nil {
		// Find crashes with similar signatures
		similar, err := s.repo.FindSimilar(ctx, crash.Signature, 0.5)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, similar...)
	}

	// Add crashes with the same type
	sameType, err := s.repo.FindByType(ctx, crash.Type)
	if err != nil {
		return nil, err
	}

	// Merge and deduplicate candidates
	candidateMap := make(map[string]*types.Crash)
	for _, c := range candidates {
		candidateMap[c.ID] = c
	}
	for _, c := range sameType {
		candidateMap[c.ID] = c
	}

	// Convert back to slice
	uniqueCandidates := make([]*types.Crash, 0, len(candidateMap))
	for _, c := range candidateMap {
		uniqueCandidates = append(uniqueCandidates, c)
	}

	// Limit candidates if necessary
	if len(uniqueCandidates) > s.config.MaxCandidates {
		uniqueCandidates = uniqueCandidates[:s.config.MaxCandidates]
	}

	return uniqueCandidates, nil
}

func (s *Service) generateGroupID(result *DeduplicationResult) string {
	if result.OriginalCrash != nil {
		return fmt.Sprintf("group_%s", result.OriginalCrash.ID)
	}

	if len(result.SimilarCrashes) > 0 {
		// Use the ID of the oldest crash as the group ID
		oldest := result.SimilarCrashes[0]
		for _, crash := range result.SimilarCrashes[1:] {
			if crash.DiscoveredAt.Before(oldest.DiscoveredAt) {
				oldest = crash
			}
		}
		return fmt.Sprintf("group_%s", oldest.ID)
	}

	return ""
}

func (s *Service) updateStatistics(processingTime time.Duration) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	s.stats.TotalProcessed++
	s.stats.ProcessingTime += processingTime
	s.stats.LastProcessedAt = time.Now()
}

func (s *Service) updateDeduplicationStats(result *DeduplicationResult, algorithmName string) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	if result.IsDuplicate {
		s.stats.DuplicatesFound++
	}

	s.stats.AlgorithmUsage[algorithmName]++
}

func (s *Service) updateGroupingStats(groups [][]*types.Crash) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	s.stats.UniqueGroups = uint64(len(groups))

	if len(groups) > 0 {
		totalSize := 0
		for _, group := range groups {
			totalSize += len(group)
		}
		s.stats.AverageGroupSize = float64(totalSize) / float64(len(groups))
	}
}
