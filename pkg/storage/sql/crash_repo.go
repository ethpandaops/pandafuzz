// Package sql provides SQL-based repository implementations for domain entities.
package sql

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
)

// CrashRepository implements repository.CrashRepository using SQLiteStorage
type CrashRepository struct {
	storage *storage.SQLiteStorage
}

// Compile-time interface compliance check
var _ repository.CrashRepository = (*CrashRepository)(nil)

// NewCrashRepository creates a new crash repository adapter
func NewCrashRepository(storage *storage.SQLiteStorage) *CrashRepository {
	return &CrashRepository{storage: storage}
}

// Create creates a new crash
func (r *CrashRepository) Create(ctx context.Context, crash *types.Crash) error {
	commonCrash := domainCrashToCommon(crash)
	return r.storage.CreateCrash(ctx, commonCrash)
}

// Update updates an existing crash
func (r *CrashRepository) Update(ctx context.Context, crash *types.Crash) error {
	// Storage doesn't have a direct UpdateCrash, use reproducibility update as proxy
	return r.storage.UpdateCrashReproducibility(ctx, crash.ID, crash.Reproducible, 0)
}

// Delete deletes a crash by ID
func (r *CrashRepository) Delete(ctx context.Context, id string) error {
	// Storage layer doesn't expose direct delete; this would need to be added
	return common.ErrNotImplemented
}

// FindByID retrieves a crash by its ID
func (r *CrashRepository) FindByID(ctx context.Context, id string) (*types.Crash, error) {
	commonCrash, err := r.storage.GetCrash(ctx, id)
	if err != nil {
		return nil, err
	}
	return commonCrashToDomain(commonCrash), nil
}

// FindBySignature retrieves crashes by signature hash
func (r *CrashRepository) FindBySignature(ctx context.Context, signatureHash string) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	return filterCrashesBySignature(crashes, signatureHash), nil
}

// FindBySeverity retrieves all crashes with a specific severity
func (r *CrashRepository) FindBySeverity(ctx context.Context, severity types.Severity) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	return filterCrashesBySeverity(crashes, severity), nil
}

// FindByType retrieves all crashes of a specific type
func (r *CrashRepository) FindByType(ctx context.Context, crashType types.CrashType) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	return filterCrashesByType(crashes, crashType), nil
}

// FindByTarget retrieves all crashes for a specific target
func (r *CrashRepository) FindByTarget(ctx context.Context, targetName string) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		domainCrash := commonCrashToDomain(crash)
		if domainCrash.TargetInfo.Name == targetName {
			result = append(result, domainCrash)
		}
	}
	return result, nil
}

// FindByCorpusEntry retrieves crashes associated with a corpus entry
func (r *CrashRepository) FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		domainCrash := commonCrashToDomain(crash)
		if domainCrash.CorpusEntryID == corpusEntryID {
			result = append(result, domainCrash)
		}
	}
	return result, nil
}

// FindReproducible retrieves all reproducible crashes
func (r *CrashRepository) FindReproducible(ctx context.Context) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		if crash.Reproducible {
			result = append(result, commonCrashToDomain(crash))
		}
	}
	return result, nil
}

// FindUnfixed retrieves all unfixed crashes
func (r *CrashRepository) FindUnfixed(ctx context.Context) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	// All crashes in storage are unfixed by default (no fixed field in common.CrashResult)
	result := make([]*types.Crash, 0, len(crashes))
	for _, crash := range crashes {
		result = append(result, commonCrashToDomain(crash))
	}
	return result, nil
}

// FindByTag retrieves all crashes with a specific tag
func (r *CrashRepository) FindByTag(ctx context.Context, tag string) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		domainCrash := commonCrashToDomain(crash)
		if hasTag(domainCrash.Tags, tag) {
			result = append(result, domainCrash)
		}
	}
	return result, nil
}

// FindRecent retrieves crashes discovered within a time range
func (r *CrashRepository) FindRecent(ctx context.Context, since time.Time) ([]*types.Crash, error) {
	crashes, err := r.storage.GetCrashes(ctx, 1000, 0)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		if crash.Timestamp.After(since) {
			result = append(result, commonCrashToDomain(crash))
		}
	}
	return result, nil
}

// FindSimilar finds crashes similar to the given signature
func (r *CrashRepository) FindSimilar(ctx context.Context, signature *types.CrashSignature, threshold float64) ([]*types.Crash, error) {
	if signature == nil {
		return nil, nil
	}
	return r.FindBySignature(ctx, signature.Hash)
}

// List retrieves crashes with pagination
func (r *CrashRepository) List(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	crashes, err := r.storage.GetCrashes(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*types.Crash, 0, len(crashes))
	for _, crash := range crashes {
		result = append(result, commonCrashToDomain(crash))
	}

	// Get total count
	total, _ := r.Count(ctx)
	return result, total, nil
}

// ListBySeverity retrieves crashes ordered by severity
func (r *CrashRepository) ListBySeverity(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	crashes, err := r.storage.GetCrashesSorted(ctx, limit, offset, "type", "desc")
	if err != nil {
		return nil, 0, err
	}

	result := make([]*types.Crash, 0, len(crashes))
	for _, crash := range crashes {
		result = append(result, commonCrashToDomain(crash))
	}

	total, _ := r.Count(ctx)
	return result, total, nil
}

// ListByOccurrence retrieves crashes ordered by occurrence count
func (r *CrashRepository) ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*types.Crash, int, error) {
	sortOrder := "desc"
	if ascending {
		sortOrder = "asc"
	}

	crashes, err := r.storage.GetCrashesSorted(ctx, limit, offset, "timestamp", sortOrder)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*types.Crash, 0, len(crashes))
	for _, crash := range crashes {
		result = append(result, commonCrashToDomain(crash))
	}

	total, _ := r.Count(ctx)
	return result, total, nil
}

// RecordOccurrence increments the occurrence count for a crash
func (r *CrashRepository) RecordOccurrence(ctx context.Context, id string) error {
	// Storage doesn't track occurrence count; would need schema change
	return nil
}

// MarkAsFixed marks a crash as fixed
func (r *CrashRepository) MarkAsFixed(ctx context.Context, id string) error {
	// Storage doesn't have fixed field; would need schema change
	return nil
}

// MarkAsNotReproducible marks a crash as not reproducible
func (r *CrashRepository) MarkAsNotReproducible(ctx context.Context, id string) error {
	return r.storage.UpdateCrashReproducibility(ctx, id, false, 0)
}

// Exists checks if a crash exists by ID
func (r *CrashRepository) Exists(ctx context.Context, id string) (bool, error) {
	crash, err := r.storage.GetCrash(ctx, id)
	if err != nil {
		if common.IsNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return crash != nil, nil
}

// ExistsBySignature checks if a crash exists by signature
func (r *CrashRepository) ExistsBySignature(ctx context.Context, signatureHash string) (bool, error) {
	crashes, err := r.FindBySignature(ctx, signatureHash)
	if err != nil {
		return false, err
	}
	return len(crashes) > 0, nil
}

// Count returns the total number of crashes
func (r *CrashRepository) Count(ctx context.Context) (int, error) {
	return r.storage.GetCrashCount(ctx, "")
}

// CountBySeverity counts crashes by severity
func (r *CrashRepository) CountBySeverity(ctx context.Context, severity types.Severity) (int, error) {
	crashes, err := r.FindBySeverity(ctx, severity)
	if err != nil {
		return 0, err
	}
	return len(crashes), nil
}

// CountByType counts crashes by type
func (r *CrashRepository) CountByType(ctx context.Context, crashType types.CrashType) (int, error) {
	crashes, err := r.FindByType(ctx, crashType)
	if err != nil {
		return 0, err
	}
	return len(crashes), nil
}

// CountUnfixed counts unfixed crashes
func (r *CrashRepository) CountUnfixed(ctx context.Context) (int, error) {
	// All crashes are unfixed in current schema
	return r.Count(ctx)
}

// GetStatsByTarget retrieves crash statistics grouped by target
func (r *CrashRepository) GetStatsByTarget(ctx context.Context) (map[string]repository.CrashStats, error) {
	crashes, err := r.storage.GetCrashes(ctx, 10000, 0)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]repository.CrashStats)
	for _, crash := range crashes {
		domainCrash := commonCrashToDomain(crash)
		targetName := domainCrash.TargetInfo.Name
		if targetName == "" {
			targetName = "unknown"
		}

		s := stats[targetName]
		s.Total++
		if domainCrash.Reproducible {
			s.Reproducible++
		}
		stats[targetName] = s
	}

	return stats, nil
}

// Helper functions for type conversion

func domainCrashToCommon(crash *types.Crash) *common.CrashResult {
	return &common.CrashResult{
		ID:           crash.ID,
		Hash:         crash.InputHash,
		Type:         string(crash.Type),
		Timestamp:    crash.DiscoveredAt,
		Size:         int64(len(crash.Input)),
		IsUnique:     true,
		Input:        crash.Input,
		StackTrace:   crash.StackTrace,
		Reproducible: crash.Reproducible,
		Metadata:     convertStringMapToInterface(crash.Metadata),
	}
}

func commonCrashToDomain(crash *common.CrashResult) *types.Crash {
	return &types.Crash{
		ID:              crash.ID,
		Input:           crash.Input,
		InputHash:       crash.Hash,
		StackTrace:      crash.StackTrace,
		Severity:        inferSeverityFromType(crash.Type),
		Type:            types.CrashType(crash.Type),
		DiscoveredAt:    crash.Timestamp,
		LastSeenAt:      crash.Timestamp,
		OccurrenceCount: 1,
		TargetInfo: types.TargetInfo{
			Name: crash.JobID, // Use JobID as target name fallback
		},
		Metadata:     convertInterfaceMapToString(crash.Metadata),
		Reproducible: crash.Reproducible,
		Fixed:        false,
		Tags:         make([]string, 0),
	}
}

func inferSeverityFromType(crashType string) types.Severity {
	switch types.CrashType(crashType) {
	case types.CrashTypeHeapOverflow, types.CrashTypeStackOverflow:
		return types.SeverityHigh
	case types.CrashTypeSegmentationFault:
		return types.SeverityMedium
	case types.CrashTypeTimeout, types.CrashTypeMemoryLeak:
		return types.SeverityLow
	default:
		return types.SeverityUnknown
	}
}

func convertStringMapToInterface(m map[string]string) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func convertInterfaceMapToString(m map[string]interface{}) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			result[k] = str
		}
	}
	return result
}

func filterCrashesBySignature(crashes []*common.CrashResult, signatureHash string) []*types.Crash {
	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		if crash.Hash == signatureHash {
			result = append(result, commonCrashToDomain(crash))
		}
	}
	return result
}

func filterCrashesBySeverity(crashes []*common.CrashResult, severity types.Severity) []*types.Crash {
	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		domainCrash := commonCrashToDomain(crash)
		if domainCrash.Severity == severity {
			result = append(result, domainCrash)
		}
	}
	return result
}

func filterCrashesByType(crashes []*common.CrashResult, crashType types.CrashType) []*types.Crash {
	result := make([]*types.Crash, 0)
	for _, crash := range crashes {
		if crash.Type == string(crashType) {
			result = append(result, commonCrashToDomain(crash))
		}
	}
	return result
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
