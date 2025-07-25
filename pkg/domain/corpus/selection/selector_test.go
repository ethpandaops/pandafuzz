package selection

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/selection/strategies"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// MockCorpusEntryRepository is a mock implementation for testing
type MockCorpusEntryRepository struct {
	entries []*types.CorpusEntry
}

func (m *MockCorpusEntryRepository) Create(ctx context.Context, entry *types.CorpusEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

func (m *MockCorpusEntryRepository) Update(ctx context.Context, entry *types.CorpusEntry) error {
	return nil
}

func (m *MockCorpusEntryRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *MockCorpusEntryRepository) FindByID(ctx context.Context, id string) (*types.CorpusEntry, error) {
	for _, e := range m.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, nil
}

func (m *MockCorpusEntryRepository) FindByHash(ctx context.Context, hash string) (*types.CorpusEntry, error) {
	return nil, nil
}

func (m *MockCorpusEntryRepository) FindByTag(ctx context.Context, tag string) ([]*types.CorpusEntry, error) {
	return nil, nil
}

func (m *MockCorpusEntryRepository) FindInteresting(ctx context.Context) ([]*types.CorpusEntry, error) {
	interesting := make([]*types.CorpusEntry, 0)
	for _, e := range m.entries {
		if e.IsInteresting() {
			interesting = append(interesting, e)
		}
	}
	return interesting, nil
}

func (m *MockCorpusEntryRepository) FindByParent(ctx context.Context, parentID string) ([]*types.CorpusEntry, error) {
	return nil, nil
}

func (m *MockCorpusEntryRepository) FindByCoverage(ctx context.Context, minCoverage float64) ([]*types.CorpusEntry, error) {
	matching := make([]*types.CorpusEntry, 0)
	for _, e := range m.entries {
		if e.Coverage.CoverageScore >= minCoverage {
			matching = append(matching, e)
		}
	}
	return matching, nil
}

func (m *MockCorpusEntryRepository) List(ctx context.Context, offset, limit int) ([]*types.CorpusEntry, int, error) {
	end := offset + limit
	if end > len(m.entries) {
		end = len(m.entries)
	}
	return m.entries[offset:end], len(m.entries), nil
}

func (m *MockCorpusEntryRepository) ListByExecutionCount(ctx context.Context, offset, limit int, ascending bool) ([]*types.CorpusEntry, int, error) {
	return nil, 0, nil
}

func (m *MockCorpusEntryRepository) UpdateExecutionStats(ctx context.Context, id string) error {
	return nil
}

func (m *MockCorpusEntryRepository) Exists(ctx context.Context, id string) (bool, error) {
	return false, nil
}

func (m *MockCorpusEntryRepository) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	return false, nil
}

func (m *MockCorpusEntryRepository) Count(ctx context.Context) (int, error) {
	return len(m.entries), nil
}

func (m *MockCorpusEntryRepository) CountInteresting(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *MockCorpusEntryRepository) GetStats(ctx context.Context) (*types.CollectionStats, error) {
	return &types.CollectionStats{
		TotalEntries:    len(m.entries),
		AverageCoverage: 0.5,
	}, nil
}

// MockCorpusCollectionRepository is a mock implementation for testing
type MockCorpusCollectionRepository struct {
	collections map[string]*types.CorpusCollection
}

func (m *MockCorpusCollectionRepository) CreateCollection(ctx context.Context, collection *types.CorpusCollection) error {
	return nil
}

func (m *MockCorpusCollectionRepository) UpdateCollection(ctx context.Context, collection *types.CorpusCollection) error {
	return nil
}

func (m *MockCorpusCollectionRepository) DeleteCollection(ctx context.Context, name string) error {
	return nil
}

func (m *MockCorpusCollectionRepository) FindCollectionByName(ctx context.Context, name string) (*types.CorpusCollection, error) {
	return nil, nil
}

func (m *MockCorpusCollectionRepository) ListCollections(ctx context.Context) ([]*types.CorpusCollection, error) {
	return nil, nil
}

func (m *MockCorpusCollectionRepository) AddEntryToCollection(ctx context.Context, collectionName string, entryID string) error {
	return nil
}

func (m *MockCorpusCollectionRepository) RemoveEntryFromCollection(ctx context.Context, collectionName string, entryID string) error {
	return nil
}

func (m *MockCorpusCollectionRepository) GetCollectionEntries(ctx context.Context, collectionName string) ([]*types.CorpusEntry, error) {
	return nil, nil
}

func (m *MockCorpusCollectionRepository) CollectionExists(ctx context.Context, name string) (bool, error) {
	return false, nil
}

// Helper function to create test corpus entries
func createTestEntries(count int) []*types.CorpusEntry {
	entries := make([]*types.CorpusEntry, count)
	for i := 0; i < count; i++ {
		input := []byte(string(rune('a' + i)))
		entry, _ := types.NewCorpusEntry(input)
		entry.Coverage = types.CoverageInfo{
			TotalBlocks:   100,
			CoveredBlocks: uint32(50 + i*5),
			TotalEdges:    200,
			CoveredEdges:  uint32(100 + i*10),
			CoverageScore: float64(50+i*5) / 100.0,
			NewCoverage:   i%3 == 0,
		}
		entry.ExecutionCount = uint64(i * 10)
		entries[i] = entry
	}
	return entries
}

func TestSelector_Creation(t *testing.T) {
	logger := logrus.New()
	entryRepo := &MockCorpusEntryRepository{}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)
	require.NotNil(t, selector)

	// Check default strategies are registered
	strategies := selector.GetStrategies()
	assert.Contains(t, strategies, "uniform-random")
	assert.Contains(t, strategies, "weighted-random")
	assert.Contains(t, strategies, "coverage-based")
	assert.Contains(t, strategies, "weighted")
}

func TestSelector_StartStop(t *testing.T) {
	logger := logrus.New()
	entryRepo := &MockCorpusEntryRepository{}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)

	ctx := context.Background()
	err = selector.Start(ctx)
	require.NoError(t, err)

	// Starting again should fail
	err = selector.Start(ctx)
	assert.Error(t, err)

	err = selector.Stop()
	require.NoError(t, err)
}

func TestSelector_Selection(t *testing.T) {
	logger := logrus.New()
	entries := createTestEntries(10)
	entryRepo := &MockCorpusEntryRepository{entries: entries}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)

	ctx := context.Background()
	err = selector.Start(ctx)
	require.NoError(t, err)
	defer selector.Stop()

	// Test basic selection
	selected, err := selector.Select(ctx, 5)
	require.NoError(t, err)
	assert.Len(t, selected, 5)

	// Test selection with specific strategy
	selected, err = selector.SelectWithStrategy(ctx, "coverage-based", 3)
	require.NoError(t, err)
	assert.Len(t, selected, 3)
}

func TestSelector_RegisterStrategy(t *testing.T) {
	logger := logrus.New()
	entryRepo := &MockCorpusEntryRepository{}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)

	// Register a custom strategy with a unique name
	customStrategy := &mockStrategy{name: "test-strategy"}
	err = selector.RegisterStrategy(customStrategy)
	assert.NoError(t, err)

	// Registering same strategy again should fail
	err = selector.RegisterStrategy(customStrategy)
	assert.Error(t, err)

	// Nil strategy should fail
	err = selector.RegisterStrategy(nil)
	assert.Error(t, err)
}

// mockStrategy is a simple mock implementation for testing
type mockStrategy struct {
	name string
}

func (m *mockStrategy) Name() string {
	return m.name
}

func (m *mockStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options strategies.SelectionOptions) ([]*types.CorpusEntry, error) {
	if count > len(collection) {
		count = len(collection)
	}
	return collection[:count], nil
}

func (m *mockStrategy) Priority(entry *types.CorpusEntry) float64 {
	return 1.0
}

func (m *mockStrategy) SupportsCriteria() bool {
	return true
}

func (m *mockStrategy) Reset() {
	// No-op
}

func TestSelector_SetDefaultStrategy(t *testing.T) {
	logger := logrus.New()
	entryRepo := &MockCorpusEntryRepository{}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)

	// Set valid strategy
	err = selector.SetDefaultStrategy("coverage-based")
	assert.NoError(t, err)

	// Set invalid strategy
	err = selector.SetDefaultStrategy("non-existent")
	assert.Error(t, err)
}

func TestSelector_Metrics(t *testing.T) {
	logger := logrus.New()
	entries := createTestEntries(10)
	entryRepo := &MockCorpusEntryRepository{entries: entries}
	collectionRepo := &MockCorpusCollectionRepository{}

	cfg := DefaultConfig()
	selector, err := NewSelector(cfg, logger, entryRepo, collectionRepo)
	require.NoError(t, err)

	ctx := context.Background()
	err = selector.Start(ctx)
	require.NoError(t, err)
	defer selector.Stop()

	// Perform some selections
	_, err = selector.Select(ctx, 5)
	require.NoError(t, err)

	// Get metrics
	metrics := selector.GetMetrics()
	assert.Equal(t, uint64(1), metrics.TotalSelections)
	assert.NotNil(t, metrics.StrategyPerformance)
}

func TestEntryCache(t *testing.T) {
	cache := newEntryCache(5, time.Minute)

	// Add entries
	entries := createTestEntries(3)
	for _, entry := range entries {
		cache.add(entry)
	}

	// Get entry
	entry, exists := cache.get(entries[0].ID)
	assert.True(t, exists)
	assert.Equal(t, entries[0].ID, entry.ID)

	// Get all entries
	all := cache.getAll()
	assert.Len(t, all, 3)

	// Test eviction
	moreEntries := createTestEntries(5)
	for _, entry := range moreEntries {
		cache.add(entry)
	}

	// Should have evicted oldest entries
	all = cache.getAll()
	assert.Len(t, all, 5)
}

func TestStrategies_Random(t *testing.T) {
	entries := createTestEntries(20)
	ctx := context.Background()

	// Test uniform random
	strategy := strategies.NewUniformRandomStrategy()
	selected, err := strategy.Select(ctx, entries, 5, strategies.SelectionOptions{})
	require.NoError(t, err)
	assert.Len(t, selected, 5)

	// Test weighted random
	weights := strategies.DefaultWeightFactors()
	weightedStrategy := strategies.NewWeightedRandomStrategy(0, weights)
	selected, err = weightedStrategy.Select(ctx, entries, 5, strategies.SelectionOptions{})
	require.NoError(t, err)
	assert.Len(t, selected, 5)
}

func TestStrategies_Coverage(t *testing.T) {
	entries := createTestEntries(20)
	ctx := context.Background()

	// Test coverage-based
	strategy := strategies.NewCoverageBasedStrategy(0.5)
	selected, err := strategy.Select(ctx, entries, 5, strategies.SelectionOptions{})
	require.NoError(t, err)
	assert.Len(t, selected, 5)

	// Verify high coverage entries are selected
	for _, entry := range selected {
		assert.GreaterOrEqual(t, entry.Coverage.CoverageScore, 0.5)
	}
}
