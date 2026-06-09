package scheduler_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockJobRepository is a mock implementation of repository.JobRepository
type MockJobRepository struct {
	mock.Mock
	mu   sync.RWMutex
	jobs map[string]*types.Job
}

func NewMockJobRepository() *MockJobRepository {
	return &MockJobRepository{
		jobs: make(map[string]*types.Job),
	}
}

func (m *MockJobRepository) Create(ctx context.Context, job *types.Job) error {
	args := m.Called(ctx, job)
	if args.Error(0) == nil {
		m.mu.Lock()
		m.jobs[job.ID] = job
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *MockJobRepository) Get(ctx context.Context, id string) (*types.Job, error) {
	args := m.Called(ctx, id)
	if args.Error(1) == nil {
		m.mu.RLock()
		job := m.jobs[id]
		m.mu.RUnlock()
		return job, nil
	}
	return args.Get(0).(*types.Job), args.Error(1)
}

func (m *MockJobRepository) Update(ctx context.Context, job *types.Job) error {
	args := m.Called(ctx, job)
	if args.Error(0) == nil {
		m.mu.Lock()
		m.jobs[job.ID] = job
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *MockJobRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	if args.Error(0) == nil {
		m.mu.Lock()
		delete(m.jobs, id)
		m.mu.Unlock()
	}
	return args.Error(0)
}

func (m *MockJobRepository) List(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) ListByStatus(ctx context.Context, status types.JobStatus) ([]*types.Job, error) {
	args := m.Called(ctx, status)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) ListPending(ctx context.Context, limit int) ([]*types.Job, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) ListScheduled(ctx context.Context, before time.Time) ([]*types.Job, error) {
	args := m.Called(ctx, before)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) CountByStatus(ctx context.Context) (map[types.JobStatus]int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[types.JobStatus]int64), args.Error(1)
}

func (m *MockJobRepository) UpdateStatus(ctx context.Context, id string, from, to types.JobStatus) error {
	args := m.Called(ctx, id, from, to)
	return args.Error(0)
}

func (m *MockJobRepository) IncrementRetries(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockJobRepository) GetDependencies(ctx context.Context, jobID string) ([]*types.Job, error) {
	args := m.Called(ctx, jobID)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) GetDependents(ctx context.Context, jobID string) ([]*types.Job, error) {
	args := m.Called(ctx, jobID)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) AddDependency(ctx context.Context, jobID, dependsOnID string) error {
	args := m.Called(ctx, jobID, dependsOnID)
	return args.Error(0)
}

func (m *MockJobRepository) RemoveDependency(ctx context.Context, jobID, dependsOnID string) error {
	args := m.Called(ctx, jobID, dependsOnID)
	return args.Error(0)
}

func (m *MockJobRepository) LockForProcessing(ctx context.Context, jobID string, workerID string, lockDuration time.Duration) (*types.Job, error) {
	args := m.Called(ctx, jobID, workerID, lockDuration)
	if args.Error(1) == nil && args.Get(0) != nil {
		return args.Get(0).(*types.Job), nil
	}
	return nil, args.Error(1)
}

func (m *MockJobRepository) UnlockJob(ctx context.Context, jobID string, workerID string) error {
	args := m.Called(ctx, jobID, workerID)
	return args.Error(0)
}

func (m *MockJobRepository) GetStaleJobs(ctx context.Context, staleDuration time.Duration) ([]*types.Job, error) {
	args := m.Called(ctx, staleDuration)
	return args.Get(0).([]*types.Job), args.Error(1)
}

func (m *MockJobRepository) GetMetrics(ctx context.Context) (*repository.JobRepositoryMetrics, error) {
	args := m.Called(ctx)
	return args.Get(0).(*repository.JobRepositoryMetrics), args.Error(1)
}

// MockJobProcessor is a mock implementation of scheduler.JobProcessor
type MockJobProcessor struct {
	mock.Mock
}

func (m *MockJobProcessor) Process(ctx context.Context, job *types.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func TestQueue_BasicOperations(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	config.Workers = 2
	config.PollInterval = 10 * time.Millisecond

	queue := scheduler.NewQueue(config, repo, processor, log)

	// Setup mock expectations
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)
	repo.On("Get", mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("ListPending", mock.Anything, 10).Return([]*types.Job{}, nil)
	repo.On("ListScheduled", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("GetStaleJobs", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("CountByStatus", mock.Anything).Return(map[types.JobStatus]int64{
		types.StatusQueued: 0,
	}, nil)

	// Start the queue
	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Stop()

	// Create a test job
	job, err := types.NewJob("test-job", "libfuzzer", "/bin/target", "/corpus", "/output")
	require.NoError(t, err)

	// Enqueue the job
	err = queue.Enqueue(ctx, job)
	assert.NoError(t, err)

	// Verify stats
	stats := queue.GetStats()
	assert.Equal(t, int64(1), stats.EnqueuedCount)
	assert.Equal(t, 2, stats.WorkersTotal)
}

func TestQueue_PriorityScheduling(t *testing.T) {
	log := logrus.New()

	// Create jobs with different priorities
	jobs := []*types.Job{}
	priorities := []types.JobPriority{
		types.PriorityLow,
		types.PriorityCritical,
		types.PriorityNormal,
		types.PriorityHigh,
	}

	for i, priority := range priorities {
		job, err := types.NewJob(fmt.Sprintf("job-%d", i), "libfuzzer", "/bin/target", "/corpus", "/output")
		require.NoError(t, err)
		job.Priority = priority
		jobs = append(jobs, job)
	}

	// Test priority queue
	pq := scheduler.NewPriorityQueue(log)

	// Add jobs to priority queue
	for _, job := range jobs {
		err := pq.Push(job)
		assert.NoError(t, err)
	}

	// Verify size
	assert.Equal(t, 4, pq.Size())

	// Pop jobs and verify priority order
	expectedOrder := []types.JobPriority{
		types.PriorityCritical,
		types.PriorityHigh,
		types.PriorityNormal,
		types.PriorityLow,
	}

	for _, expectedPriority := range expectedOrder {
		job, err := pq.Pop()
		assert.NoError(t, err)
		assert.Equal(t, expectedPriority, job.Priority)
	}

	// Queue should be empty
	assert.Equal(t, 0, pq.Size())
}

func TestQueue_JobDependencies(t *testing.T) {
	// Create jobs with dependencies
	job1, _ := types.NewJob("job-1", "libfuzzer", "/bin/target", "/corpus", "/output")
	job2, _ := types.NewJob("job-2", "libfuzzer", "/bin/target", "/corpus", "/output")
	job3, _ := types.NewJob("job-3", "libfuzzer", "/bin/target", "/corpus", "/output")

	// Set up dependencies: job3 depends on job2, job2 depends on job1
	job2.AddDependency(job1.ID)
	job3.AddDependency(job2.ID)

	// Test dependency checking
	assert.True(t, job2.HasDependencies())
	assert.True(t, job3.HasDependencies())
	assert.False(t, job1.HasDependencies())
}

func TestQueue_RetryLogic(t *testing.T) {
	job, _ := types.NewJob("retry-job", "libfuzzer", "/bin/target", "/corpus", "/output")
	job.MaxRetries = 3
	job.RetryDelay = 1 * time.Second

	// Test retry logic
	assert.False(t, job.CanRetry())
	job.Status = types.StatusFailed
	assert.True(t, job.CanRetry())

	// Simulate failures and retries
	for i := 0; i < 3; i++ {
		job.Status = types.StatusFailed
		assert.True(t, job.CanRetry())
		job.IncrementRetries()
	}

	// After max retries, should not be able to retry
	job.Status = types.StatusFailed
	assert.False(t, job.CanRetry())
	assert.Equal(t, 3, job.RetryCount)
}

func TestQueue_ScheduledJobs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	queue := scheduler.NewQueue(config, repo, processor, log)

	// Create a scheduled job
	job, _ := types.NewJob("scheduled-job", "libfuzzer", "/bin/target", "/corpus", "/output")
	futureTime := time.Now().Add(1 * time.Hour)
	job.ScheduledAt = &futureTime

	// Test scheduled job detection
	assert.True(t, job.IsScheduled())
	assert.False(t, job.IsReadyToRun())

	// Setup mock
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("ListPending", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("ListScheduled", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("GetStaleJobs", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("CountByStatus", mock.Anything).Return(map[types.JobStatus]int64{}, nil)

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Stop()

	// Enqueue with delay
	err = queue.EnqueueWithDelay(ctx, job, 2*time.Hour)
	assert.NoError(t, err)
}

func TestQueue_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	config.Workers = 4

	queue := scheduler.NewQueue(config, repo, processor, log)

	// Setup mocks for concurrent operations
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.Anything).Return(nil)
	repo.On("Get", mock.Anything, mock.Anything).Return(nil, errors.New("not found"))
	repo.On("ListPending", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("ListScheduled", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("GetStaleJobs", mock.Anything, mock.Anything).Return([]*types.Job{}, nil)
	repo.On("CountByStatus", mock.Anything).Return(map[types.JobStatus]int64{}, nil)

	err := queue.Start(ctx)
	require.NoError(t, err)
	defer queue.Stop()

	// Concurrent enqueue operations
	var wg sync.WaitGroup
	numJobs := 100

	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			job, _ := types.NewJob(fmt.Sprintf("concurrent-job-%d", idx), "libfuzzer", "/bin/target", "/corpus", "/output")
			queue.Enqueue(ctx, job)
		}(i)
	}

	wg.Wait()

	// Verify stats
	stats := queue.GetStats()
	assert.Equal(t, int64(numJobs), stats.EnqueuedCount)
}
