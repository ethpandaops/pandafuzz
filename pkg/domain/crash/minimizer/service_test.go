package minimizer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	fuzzerTypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Mock repository - minimal implementation for testing
type mockCrashRepository struct {
	mock.Mock
	crashes map[string]*types.Crash
}

func newMockCrashRepository() *mockCrashRepository {
	return &mockCrashRepository{
		crashes: make(map[string]*types.Crash),
	}
}

func (m *mockCrashRepository) Create(ctx context.Context, crash *types.Crash) error {
	args := m.Called(ctx, crash)
	if args.Error(0) == nil && m.crashes != nil {
		m.crashes[crash.ID] = crash
	}
	return args.Error(0)
}

func (m *mockCrashRepository) Update(ctx context.Context, crash *types.Crash) error {
	args := m.Called(ctx, crash)
	return args.Error(0)
}

func (m *mockCrashRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockCrashRepository) FindByID(ctx context.Context, id string) (*types.Crash, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindBySignature(ctx context.Context, signatureHash string) ([]*types.Crash, error) {
	args := m.Called(ctx, signatureHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindBySeverity(ctx context.Context, severity types.Severity) ([]*types.Crash, error) {
	args := m.Called(ctx, severity)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindByType(ctx context.Context, crashType types.CrashType) ([]*types.Crash, error) {
	args := m.Called(ctx, crashType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindByTarget(ctx context.Context, targetName string) ([]*types.Crash, error) {
	args := m.Called(ctx, targetName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*types.Crash, error) {
	args := m.Called(ctx, corpusEntryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindReproducible(ctx context.Context) ([]*types.Crash, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindUnfixed(ctx context.Context) ([]*types.Crash, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindByTag(ctx context.Context, tag string) ([]*types.Crash, error) {
	args := m.Called(ctx, tag)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindRecent(ctx context.Context, since time.Time) ([]*types.Crash, error) {
	args := m.Called(ctx, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) FindSimilar(ctx context.Context, signature *types.CrashSignature, threshold float64) ([]*types.Crash, error) {
	args := m.Called(ctx, signature, threshold)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.Crash), args.Error(1)
}

func (m *mockCrashRepository) List(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*types.Crash), args.Int(1), args.Error(2)
}

func (m *mockCrashRepository) ListBySeverity(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	args := m.Called(ctx, offset, limit)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*types.Crash), args.Int(1), args.Error(2)
}

func (m *mockCrashRepository) ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*types.Crash, int, error) {
	args := m.Called(ctx, offset, limit, ascending)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]*types.Crash), args.Int(1), args.Error(2)
}

func (m *mockCrashRepository) RecordOccurrence(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockCrashRepository) MarkAsFixed(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockCrashRepository) MarkAsNotReproducible(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockCrashRepository) Exists(ctx context.Context, id string) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockCrashRepository) ExistsBySignature(ctx context.Context, signatureHash string) (bool, error) {
	args := m.Called(ctx, signatureHash)
	return args.Bool(0), args.Error(1)
}

func (m *mockCrashRepository) Count(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *mockCrashRepository) CountBySeverity(ctx context.Context, severity types.Severity) (int, error) {
	args := m.Called(ctx, severity)
	return args.Int(0), args.Error(1)
}

func (m *mockCrashRepository) CountByType(ctx context.Context, crashType types.CrashType) (int, error) {
	args := m.Called(ctx, crashType)
	return args.Int(0), args.Error(1)
}

func (m *mockCrashRepository) CountUnfixed(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *mockCrashRepository) GetStatsByTarget(ctx context.Context) (map[string]repository.CrashStats, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]repository.CrashStats), args.Error(1)
}

// Mock fuzzer factory
type mockFuzzerFactory struct {
	mock.Mock
}

func (m *mockFuzzerFactory) CreateFuzzer(fuzzerType string, target string, args []string) (fuzzerTypes.Fuzzer, error) {
	mArgs := m.Called(fuzzerType, target, args)
	if mArgs.Get(0) == nil {
		return nil, mArgs.Error(1)
	}
	return mArgs.Get(0).(fuzzerTypes.Fuzzer), mArgs.Error(1)
}

func (m *mockFuzzerFactory) GetSupportedTypes() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *mockFuzzerFactory) IsSupported(fuzzerType string) bool {
	args := m.Called(fuzzerType)
	return args.Bool(0)
}

func TestNewService(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	t.Run("Success", func(t *testing.T) {
		service, err := NewService(mockRepo, mockFactory)
		require.NoError(t, err)
		assert.NotNil(t, service)
	})

	t.Run("NilRepository", func(t *testing.T) {
		service, err := NewService(nil, mockFactory)
		assert.Error(t, err)
		assert.Nil(t, service)
		assert.Contains(t, err.Error(), "crash repository cannot be nil")
	})

	t.Run("NilFuzzerFactory", func(t *testing.T) {
		service, err := NewService(mockRepo, nil)
		assert.Error(t, err)
		assert.Nil(t, service)
		assert.Contains(t, err.Error(), "fuzzer factory cannot be nil")
	})
}

func TestMinimizationOptions(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	service, err := NewService(mockRepo, mockFactory)
	require.NoError(t, err)

	s := service.(*Service)
	options := s.defaultOptions()

	assert.Equal(t, 1000, options.MaxIterations)
	assert.Equal(t, 30*time.Minute, options.Timeout)
	assert.Equal(t, 0.01, options.MinReduction)
	assert.Contains(t, options.Strategies, "binary_search")
	assert.Contains(t, options.Strategies, "delta_debugging")
	assert.NotNil(t, options.ResourceLimits)
	assert.Equal(t, uint64(1024*1024*1024), options.ResourceLimits.MaxMemory)
}

func TestValidateOptions(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	service, err := NewService(mockRepo, mockFactory)
	require.NoError(t, err)

	s := service.(*Service)

	t.Run("ValidOptions", func(t *testing.T) {
		options := s.defaultOptions()
		err := s.validateOptions(options)
		assert.NoError(t, err)
	})

	t.Run("InvalidMaxIterations", func(t *testing.T) {
		options := s.defaultOptions()
		options.MaxIterations = 0
		err := s.validateOptions(options)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max iterations must be positive")
	})

	t.Run("InvalidTimeout", func(t *testing.T) {
		options := s.defaultOptions()
		options.Timeout = 0
		err := s.validateOptions(options)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout must be positive")
	})

	t.Run("InvalidMinReduction", func(t *testing.T) {
		options := s.defaultOptions()
		options.MinReduction = -0.1
		err := s.validateOptions(options)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "min reduction must be between 0 and 1")
	})

	t.Run("NoStrategies", func(t *testing.T) {
		options := s.defaultOptions()
		options.Strategies = []string{}
		err := s.validateOptions(options)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one strategy must be specified")
	})
}

func TestListActiveJobs(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	service, err := NewService(mockRepo, mockFactory)
	require.NoError(t, err)

	// Initially no jobs
	jobs := service.ListActiveJobs()
	assert.Empty(t, jobs)
}

func TestGetMinimalInput(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	service, err := NewService(mockRepo, mockFactory)
	require.NoError(t, err)

	ctx := context.Background()
	crashID := "test-crash-1"
	originalInput := []byte("This is a test crash input")

	crash := &types.Crash{
		ID:           crashID,
		Input:        originalInput,
		StackTrace:   "test stack trace",
		Reproducible: true,
		TargetInfo: types.TargetInfo{
			Name: "test-target",
		},
	}

	t.Run("OriginalCrash", func(t *testing.T) {
		mockRepo.On("FindByID", ctx, crashID).Return(crash, nil).Once()
		mockRepo.On("List", ctx, 0, 1000).Return([]*types.Crash{}, 0, nil).Once()

		input, err := service.GetMinimalInput(crashID)
		require.NoError(t, err)
		assert.Equal(t, originalInput, input)

		mockRepo.AssertExpectations(t)
	})

	t.Run("CrashNotFound", func(t *testing.T) {
		mockRepo.On("FindByID", ctx, "non-existent").Return(nil, nil).Once()
		mockRepo.On("List", ctx, 0, 1000).Return([]*types.Crash{}, 0, nil).Once()

		input, err := service.GetMinimalInput("non-existent")
		assert.Error(t, err)
		assert.Nil(t, input)

		mockRepo.AssertExpectations(t)
	})
}

func TestRegisterStrategy(t *testing.T) {
	mockRepo := newMockCrashRepository()
	mockFactory := &mockFuzzerFactory{}

	service, err := NewService(mockRepo, mockFactory)
	require.NoError(t, err)

	// Create a mock strategy
	mockStrategy := &mockMinimizationStrategy{}

	// Register the strategy
	service.RegisterStrategy("custom", mockStrategy)

	// Verify it was registered by checking internal state
	s := service.(*Service)
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.strategies["custom"]
	assert.True(t, exists)
}

// Mock minimization strategy for testing
type mockMinimizationStrategy struct {
	mock.Mock
}

func (m *mockMinimizationStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	args := m.Called(ctx, input, verifier, progress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockMinimizationStrategy) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockMinimizationStrategy) Description() string {
	args := m.Called()
	return args.String(0)
}
