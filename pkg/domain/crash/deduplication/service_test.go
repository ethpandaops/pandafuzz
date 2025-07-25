package deduplication_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/deduplication"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/deduplication/algorithms"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// mockRepository implements a simple in-memory crash repository for testing
type mockRepository struct {
	crashes map[string]*types.Crash
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		crashes: make(map[string]*types.Crash),
	}
}

func (m *mockRepository) Create(ctx context.Context, crash *types.Crash) error {
	m.crashes[crash.ID] = crash
	return nil
}

func (m *mockRepository) Update(ctx context.Context, crash *types.Crash) error {
	m.crashes[crash.ID] = crash
	return nil
}

func (m *mockRepository) Delete(ctx context.Context, id string) error {
	delete(m.crashes, id)
	return nil
}

func (m *mockRepository) FindByID(ctx context.Context, id string) (*types.Crash, error) {
	crash, exists := m.crashes[id]
	if !exists {
		return nil, nil
	}
	return crash, nil
}

func (m *mockRepository) FindBySignature(ctx context.Context, signatureHash string) ([]*types.Crash, error) {
	var results []*types.Crash
	for _, crash := range m.crashes {
		if crash.Signature != nil && crash.Signature.Hash == signatureHash {
			results = append(results, crash)
		}
	}
	return results, nil
}

func (m *mockRepository) FindBySeverity(ctx context.Context, severity types.Severity) ([]*types.Crash, error) {
	var results []*types.Crash
	for _, crash := range m.crashes {
		if crash.Severity == severity {
			results = append(results, crash)
		}
	}
	return results, nil
}

func (m *mockRepository) FindByType(ctx context.Context, crashType types.CrashType) ([]*types.Crash, error) {
	var results []*types.Crash
	for _, crash := range m.crashes {
		if crash.Type == crashType {
			results = append(results, crash)
		}
	}
	return results, nil
}

func (m *mockRepository) FindSimilar(ctx context.Context, signature *types.CrashSignature, threshold float64) ([]*types.Crash, error) {
	var results []*types.Crash
	for _, crash := range m.crashes {
		if crash.Signature != nil && crash.Signature.IsSimilar(signature, threshold) {
			results = append(results, crash)
		}
	}
	return results, nil
}

func (m *mockRepository) List(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	crashes := make([]*types.Crash, 0, len(m.crashes))
	for _, crash := range m.crashes {
		crashes = append(crashes, crash)
	}

	total := len(crashes)
	if limit < 0 {
		return crashes, total, nil
	}

	if offset >= len(crashes) {
		return nil, total, nil
	}

	end := offset + limit
	if end > len(crashes) {
		end = len(crashes)
	}

	return crashes[offset:end], total, nil
}

func (m *mockRepository) RecordOccurrence(ctx context.Context, id string) error {
	if crash, exists := m.crashes[id]; exists {
		crash.RecordOccurrence()
	}
	return nil
}

// Implement remaining required methods with basic functionality
func (m *mockRepository) FindByTarget(ctx context.Context, targetName string) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) FindReproducible(ctx context.Context) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) FindUnfixed(ctx context.Context) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) FindByTag(ctx context.Context, tag string) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) FindRecent(ctx context.Context, since time.Time) ([]*types.Crash, error) {
	return nil, nil
}

func (m *mockRepository) ListBySeverity(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	return nil, 0, nil
}

func (m *mockRepository) ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*types.Crash, int, error) {
	return nil, 0, nil
}

func (m *mockRepository) MarkAsFixed(ctx context.Context, id string) error {
	return nil
}

func (m *mockRepository) MarkAsNotReproducible(ctx context.Context, id string) error {
	return nil
}

func (m *mockRepository) Exists(ctx context.Context, id string) (bool, error) {
	_, exists := m.crashes[id]
	return exists, nil
}

func (m *mockRepository) ExistsBySignature(ctx context.Context, signatureHash string) (bool, error) {
	for _, crash := range m.crashes {
		if crash.Signature != nil && crash.Signature.Hash == signatureHash {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepository) Count(ctx context.Context) (int, error) {
	return len(m.crashes), nil
}

func (m *mockRepository) CountBySeverity(ctx context.Context, severity types.Severity) (int, error) {
	count := 0
	for _, crash := range m.crashes {
		if crash.Severity == severity {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) CountByType(ctx context.Context, crashType types.CrashType) (int, error) {
	count := 0
	for _, crash := range m.crashes {
		if crash.Type == crashType {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) CountUnfixed(ctx context.Context) (int, error) {
	count := 0
	for _, crash := range m.crashes {
		if !crash.Fixed {
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) GetStatsByTarget(ctx context.Context) (map[string]repository.CrashStats, error) {
	return nil, nil
}

// Test helper functions

func createTestCrash(id string, stackTrace string, crashType types.CrashType) *types.Crash {
	targetInfo := types.TargetInfo{
		Name:    "test_target",
		Version: "1.0.0",
		Command: "test_command",
	}

	crash, _ := types.NewCrash([]byte("test_input_"+id), stackTrace, targetInfo)
	crash.ID = id
	crash.Type = crashType

	return crash
}

// Tests

func TestDeduplicationService_ProcessCrash(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	config := deduplication.DefaultConfig()
	service := deduplication.NewService(repo, config)

	// Register algorithms
	hashAlgo := algorithms.NewHashBased(algorithms.DefaultHashBasedConfig())
	fuzzyAlgo := algorithms.NewFuzzyMatching(algorithms.DefaultFuzzyMatchingConfig())

	if err := service.RegisterAlgorithm(hashAlgo); err != nil {
		t.Fatalf("Failed to register hash algorithm: %v", err)
	}
	if err := service.RegisterAlgorithm(fuzzyAlgo); err != nil {
		t.Fatalf("Failed to register fuzzy algorithm: %v", err)
	}

	// Create test crashes
	crash1 := createTestCrash("crash1",
		"#0 0x12345 in malloc\n#1 0x23456 in processData\n#2 0x34567 in main",
		types.CrashTypeSegmentationFault)

	crash2 := createTestCrash("crash2",
		"#0 0x12345 in malloc\n#1 0x23456 in processData\n#2 0x34567 in main",
		types.CrashTypeSegmentationFault)

	crash3 := createTestCrash("crash3",
		"#0 0x99999 in free\n#1 0x88888 in cleanup\n#2 0x77777 in main",
		types.CrashTypeHeapOverflow)

	// Add first crash to repository
	if err := repo.Create(ctx, crash1); err != nil {
		t.Fatalf("Failed to create crash1: %v", err)
	}

	// Process duplicate crash
	result, err := service.ProcessCrash(ctx, crash2)
	if err != nil {
		t.Fatalf("Failed to process crash2: %v", err)
	}

	if !result.IsDuplicate {
		t.Error("Expected crash2 to be identified as duplicate of crash1")
	}

	if result.OriginalCrash == nil || result.OriginalCrash.ID != crash1.ID {
		t.Error("Expected original crash to be crash1")
	}

	// Process non-duplicate crash
	result, err = service.ProcessCrash(ctx, crash3)
	if err != nil {
		t.Fatalf("Failed to process crash3: %v", err)
	}

	if result.IsDuplicate {
		t.Error("Expected crash3 to not be a duplicate")
	}
}

func TestDeduplicationService_ProcessBatch(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	config := deduplication.DefaultConfig()
	config.BatchSize = 2
	service := deduplication.NewService(repo, config)

	// Register algorithm
	hashAlgo := algorithms.NewHashBased(algorithms.DefaultHashBasedConfig())
	if err := service.RegisterAlgorithm(hashAlgo); err != nil {
		t.Fatalf("Failed to register algorithm: %v", err)
	}

	// Create test crashes
	crashes := make([]*types.Crash, 5)
	for i := 0; i < 5; i++ {
		stackTrace := "#0 0x12345 in malloc\n#1 0x23456 in processData\n"
		if i%2 == 0 {
			stackTrace += "#2 0x34567 in main"
		} else {
			stackTrace += "#2 0x99999 in different"
		}

		crashes[i] = createTestCrash(
			string(rune('a'+i)),
			stackTrace,
			types.CrashTypeSegmentationFault,
		)
	}

	// Process batch
	results, err := service.ProcessBatch(ctx, crashes)
	if err != nil {
		t.Fatalf("Failed to process batch: %v", err)
	}

	if len(results) != len(crashes) {
		t.Errorf("Expected %d results, got %d", len(crashes), len(results))
	}
}

func TestDeduplicationService_GroupSimilarCrashes(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	config := deduplication.DefaultConfig()
	service := deduplication.NewService(repo, config)

	// Register fuzzy matching algorithm
	fuzzyAlgo := algorithms.NewFuzzyMatching(algorithms.DefaultFuzzyMatchingConfig())
	if err := service.RegisterAlgorithm(fuzzyAlgo); err != nil {
		t.Fatalf("Failed to register algorithm: %v", err)
	}

	// Create test crashes with similar patterns
	group1Crashes := []*types.Crash{
		createTestCrash("g1_1", "#0 malloc\n#1 processData\n#2 main", types.CrashTypeSegmentationFault),
		createTestCrash("g1_2", "#0 malloc\n#1 processData\n#2 main", types.CrashTypeSegmentationFault),
		createTestCrash("g1_3", "#0 malloc\n#1 processData\n#2 main", types.CrashTypeSegmentationFault),
	}

	group2Crashes := []*types.Crash{
		createTestCrash("g2_1", "#0 free\n#1 cleanup\n#2 exit", types.CrashTypeHeapOverflow),
		createTestCrash("g2_2", "#0 free\n#1 cleanup\n#2 exit", types.CrashTypeHeapOverflow),
	}

	// Add all crashes to repository
	for _, crash := range append(group1Crashes, group2Crashes...) {
		if err := repo.Create(ctx, crash); err != nil {
			t.Fatalf("Failed to create crash: %v", err)
		}
	}

	// Group similar crashes
	groups, err := service.GroupSimilarCrashes(ctx, "fuzzy_matching")
	if err != nil {
		t.Fatalf("Failed to group crashes: %v", err)
	}

	if len(groups) < 2 {
		t.Errorf("Expected at least 2 groups, got %d", len(groups))
	}

	// Verify grouping
	foundGroup1 := false
	foundGroup2 := false

	for _, group := range groups {
		if len(group) == 3 {
			foundGroup1 = true
		} else if len(group) == 2 {
			foundGroup2 = true
		}
	}

	if !foundGroup1 || !foundGroup2 {
		t.Error("Expected to find both crash groups")
	}
}

func TestDeduplicationService_Statistics(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	config := deduplication.DefaultConfig()
	config.EnableStatistics = true
	service := deduplication.NewService(repo, config)

	// Register algorithm
	hashAlgo := algorithms.NewHashBased(algorithms.DefaultHashBasedConfig())
	if err := service.RegisterAlgorithm(hashAlgo); err != nil {
		t.Fatalf("Failed to register algorithm: %v", err)
	}

	// Process some crashes
	crash1 := createTestCrash("stat1", "#0 malloc", types.CrashTypeSegmentationFault)
	crash2 := createTestCrash("stat2", "#0 malloc", types.CrashTypeSegmentationFault)

	repo.Create(ctx, crash1)
	service.ProcessCrash(ctx, crash2)

	// Get statistics
	stats := service.GetStatistics()
	if stats == nil {
		t.Fatal("Expected statistics to be available")
	}

	if stats.TotalProcessed != 1 {
		t.Errorf("Expected 1 processed crash, got %d", stats.TotalProcessed)
	}

	if stats.DuplicatesFound != 1 {
		t.Errorf("Expected 1 duplicate found, got %d", stats.DuplicatesFound)
	}

	if stats.AlgorithmUsage["hash_based"] != 1 {
		t.Errorf("Expected hash_based algorithm usage to be 1, got %d", stats.AlgorithmUsage["hash_based"])
	}
}
