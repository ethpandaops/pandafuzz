package service

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) CreateCampaign(ctx context.Context, campaign *common.Campaign) error {
	args := m.Called(ctx, campaign)
	return args.Error(0)
}

func (m *MockStorage) GetCampaign(ctx context.Context, id string) (*common.Campaign, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Campaign), args.Error(1)
}

func (m *MockStorage) ListCampaigns(ctx context.Context, limit, offset int, status string) ([]*common.Campaign, error) {
	args := m.Called(ctx, limit, offset, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Campaign), args.Error(1)
}

func (m *MockStorage) UpdateCampaign(ctx context.Context, id string, updates map[string]interface{}) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteCampaign(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Job), args.Error(1)
}

func (m *MockStorage) LinkJobToCampaign(ctx context.Context, campaignID, jobID string) error {
	args := m.Called(ctx, campaignID, jobID)
	return args.Error(0)
}

func (m *MockStorage) GetCampaignStatistics(ctx context.Context, campaignID string) (*common.CampaignStats, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CampaignStats), args.Error(1)
}

func (m *MockStorage) AddCorpusFile(ctx context.Context, cf *common.CorpusFile) error {
	args := m.Called(ctx, cf)
	return args.Error(0)
}

func (m *MockStorage) AddQuarantinedFile(ctx context.Context, qf *common.QuarantinedFile) error {
	args := m.Called(ctx, qf)
	return args.Error(0)
}

// Additional required methods for Storage interface
func (m *MockStorage) CreateCrashGroup(ctx context.Context, cg *common.CrashGroup) error {
	args := m.Called(ctx, cg)
	return args.Error(0)
}

func (m *MockStorage) GetCrashGroup(ctx context.Context, campaignID, stackHash string) (*common.CrashGroup, error) {
	args := m.Called(ctx, campaignID, stackHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CrashGroup), args.Error(1)
}

func (m *MockStorage) UpdateCrashGroupCount(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) ListCrashGroups(ctx context.Context, campaignID string) ([]*common.CrashGroup, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashGroup), args.Error(1)
}

func (m *MockStorage) CreateStackTrace(ctx context.Context, crashID string, st *common.StackTrace) error {
	args := m.Called(ctx, crashID, st)
	return args.Error(0)
}

func (m *MockStorage) GetStackTrace(ctx context.Context, crashID string) (*common.StackTrace, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.StackTrace), args.Error(1)
}

func (m *MockStorage) LinkCrashToGroup(ctx context.Context, crashID, groupID string) error {
	args := m.Called(ctx, crashID, groupID)
	return args.Error(0)
}

func (m *MockStorage) GetCorpusFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockStorage) GetCorpusFile(ctx context.Context, fileID string) (*common.CorpusFile, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusFile), args.Error(1)
}

func (m *MockStorage) GetCorpusFileByHash(ctx context.Context, hash string) (*common.CorpusFile, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusFile), args.Error(1)
}

func (m *MockStorage) UpdateCorpusFile(ctx context.Context, fileID string, updates map[string]interface{}) error {
	args := m.Called(ctx, fileID, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteCorpusFile(ctx context.Context, fileID string) error {
	args := m.Called(ctx, fileID)
	return args.Error(0)
}

func (m *MockStorage) UpdateCorpusCoverage(ctx context.Context, id string, coverage, newCoverage int64) error {
	args := m.Called(ctx, id, coverage, newCoverage)
	return args.Error(0)
}

func (m *MockStorage) RecordCorpusEvolution(ctx context.Context, ce *common.CorpusEvolution) error {
	args := m.Called(ctx, ce)
	return args.Error(0)
}

func (m *MockStorage) GetCorpusEvolution(ctx context.Context, campaignID string, limit int) ([]*common.CorpusEvolution, error) {
	args := m.Called(ctx, campaignID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusEvolution), args.Error(1)
}

func (m *MockStorage) GetUnsyncedCorpusFiles(ctx context.Context, campaignID, botID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, campaignID, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockStorage) MarkCorpusFilesSynced(ctx context.Context, fileIDs []string, botID string) error {
	args := m.Called(ctx, fileIDs, botID)
	return args.Error(0)
}

func (m *MockStorage) GetQuarantinedFile(ctx context.Context, fileID string) (*common.QuarantinedFile, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.QuarantinedFile), args.Error(1)
}

func (m *MockStorage) GetQuarantinedFiles(ctx context.Context, campaignID string) ([]*common.QuarantinedFile, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.QuarantinedFile), args.Error(1)
}

func (m *MockStorage) UpdateQuarantinedFile(ctx context.Context, id string, updates map[string]interface{}) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorage) GetCorpusFileMetrics(ctx context.Context, fileID string) (*common.CorpusFileMetrics, error) {
	args := m.Called(ctx, fileID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusFileMetrics), args.Error(1)
}

func (m *MockStorage) UpdateCorpusFileMetrics(ctx context.Context, fileID string, metrics *common.CorpusFileMetrics) error {
	args := m.Called(ctx, fileID, metrics)
	return args.Error(0)
}

func (m *MockStorage) CreateBot(ctx context.Context, bot *common.Bot) error {
	args := m.Called(ctx, bot)
	return args.Error(0)
}

func (m *MockStorage) GetBot(ctx context.Context, id string) (*common.Bot, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Bot), args.Error(1)
}

func (m *MockStorage) UpdateBot(ctx context.Context, id string, updates map[string]interface{}) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorage) ListBots(ctx context.Context) ([]*common.Bot, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Bot), args.Error(1)
}

func (m *MockStorage) DeleteBot(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) CreateJob(ctx context.Context, job *common.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockStorage) GetJob(ctx context.Context, id string) (*common.Job, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Job), args.Error(1)
}

func (m *MockStorage) UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockStorage) ListJobs(ctx context.Context, limit, offset int, status string) ([]*common.Job, error) {
	args := m.Called(ctx, limit, offset, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Job), args.Error(1)
}

func (m *MockStorage) DeleteJob(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) CreateCrash(ctx context.Context, crash *common.CrashResult) error {
	args := m.Called(ctx, crash)
	return args.Error(0)
}

func (m *MockStorage) GetCrash(ctx context.Context, id string) (*common.CrashResult, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CrashResult), args.Error(1)
}

func (m *MockStorage) ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	args := m.Called(ctx, jobID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashResult), args.Error(1)
}

func (m *MockStorage) GetCrashesByCampaign(ctx context.Context, campaignID string) ([]*common.CrashResult, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashResult), args.Error(1)
}

func (m *MockStorage) UpdateCrashWithCampaign(ctx context.Context, crashID, campaignID string) error {
	args := m.Called(ctx, crashID, campaignID)
	return args.Error(0)
}

func (m *MockStorage) CreateCoverage(ctx context.Context, coverage *common.CoverageResult) error {
	args := m.Called(ctx, coverage)
	return args.Error(0)
}

func (m *MockStorage) GetLatestCoverage(ctx context.Context, jobID string) (*common.CoverageResult, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CoverageResult), args.Error(1)
}

func (m *MockStorage) RecordCorpusUpdate(ctx context.Context, update *common.CorpusUpdate) error {
	args := m.Called(ctx, update)
	return args.Error(0)
}

func (m *MockStorage) GetSystemStats(ctx context.Context) (map[string]interface{}, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// Transaction support
func (m *MockStorage) BeginTx(ctx context.Context) (common.Transaction, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(common.Transaction), args.Error(1)
}

// Maintenance operations
func (m *MockStorage) Cleanup(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Backup(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Health check
func (m *MockStorage) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Reproduction operations
func (m *MockStorage) CreateReproductionResult(ctx context.Context, result *common.ReproductionResult) error {
	args := m.Called(ctx, result)
	return args.Error(0)
}

func (m *MockStorage) GetReproductionResults(ctx context.Context, crashID string) ([]*common.ReproductionResult, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.ReproductionResult), args.Error(1)
}

// Minimization operations
func (m *MockStorage) CreateMinimizationResult(ctx context.Context, result *common.MinimizationResult) error {
	args := m.Called(ctx, result)
	return args.Error(0)
}

func (m *MockStorage) GetMinimizationResult(ctx context.Context, resultID string) (*common.MinimizationResult, error) {
	args := m.Called(ctx, resultID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.MinimizationResult), args.Error(1)
}

func (m *MockStorage) ListMinimizationResults(ctx context.Context, crashID string) ([]*common.MinimizationResult, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.MinimizationResult), args.Error(1)
}

func (m *MockStorage) GetMinimizationStats(ctx context.Context, campaignID string) (map[string]interface{}, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// MockJobService is a mock implementation of the JobService interface
type MockJobService struct {
	mock.Mock
}

func (m *MockJobService) Create(ctx context.Context, job *common.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockJobService) CreateJob(ctx context.Context, job *common.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
}

func (m *MockJobService) Get(ctx context.Context, id string) (*common.Job, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Job), args.Error(1)
}

func (m *MockJobService) GetJobsByStatus(ctx context.Context, status common.JobStatus) ([]*common.Job, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Job), args.Error(1)
}

func (m *MockJobService) Cancel(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockJobService) AssignJob(ctx context.Context, jobID, botID string) error {
	args := m.Called(ctx, jobID, botID)
	return args.Error(0)
}

func (m *MockJobService) CompleteJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

func (m *MockJobService) GetLogs(ctx context.Context, jobID string, offset int64) ([]byte, error) {
	args := m.Called(ctx, jobID, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockJobService) GetJob(ctx context.Context, id string) (*common.Job, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Job), args.Error(1)
}

func (m *MockJobService) UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockJobService) DeleteJob(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestCampaignService_Create(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobService := new(MockJobService)

	cs := NewCampaignService(mockStorage, mockJobService, logger)

	t.Run("successful campaign creation", func(t *testing.T) {
		campaign := &common.Campaign{
			Name:         "Test Campaign",
			TargetBinary: "/bin/test",
			Status:       common.CampaignStatusPending,
			MaxJobs:      5,
			AutoRestart:  true,
			SharedCorpus: true,
			JobTemplate: common.JobConfig{
				Duration:    3600 * time.Second,
				MemoryLimit: 2048 * 1024 * 1024,
				Timeout:     1000 * time.Millisecond,
			},
		}

		mockStorage.On("CreateCampaign", ctx, mock.MatchedBy(func(c *common.Campaign) bool {
			return c.Name == campaign.Name && c.ID != ""
		})).Return(nil).Once()

		err := cs.Create(ctx, campaign)
		assert.NoError(t, err)
		assert.NotEmpty(t, campaign.ID)
		assert.NotZero(t, campaign.CreatedAt)
		mockStorage.AssertExpectations(t)
	})

	t.Run("invalid campaign - missing name", func(t *testing.T) {
		campaign := &common.Campaign{
			TargetBinary: "/bin/test",
		}

		err := cs.Create(ctx, campaign)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "campaign name is required")
	})

	t.Run("invalid campaign - missing target binary", func(t *testing.T) {
		campaign := &common.Campaign{
			Name: "Test Campaign",
		}

		err := cs.Create(ctx, campaign)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "target binary is required")
	})
}

func TestCampaignService_RestartCampaign(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobService := new(MockJobService)

	cs := NewCampaignService(mockStorage, mockJobService, logger)

	t.Run("successful campaign restart", func(t *testing.T) {
		campaignID := "test-campaign-id"
		completedAt := time.Now().Add(-1 * time.Hour)
		campaign := &common.Campaign{
			ID:           campaignID,
			Name:         "Test Campaign",
			Status:       common.CampaignStatusCompleted,
			CompletedAt:  &completedAt,
			TargetBinary: "/bin/test",
			MaxJobs:      3,
			JobTemplate: common.JobConfig{
				Duration:    3600 * time.Second,
				MemoryLimit: 2048 * 1024 * 1024,
				Timeout:     1000 * time.Millisecond,
			},
		}

		// Mock getting the campaign
		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()

		// Mock updating campaign status
		mockStorage.On("UpdateCampaign", ctx, campaignID, mock.MatchedBy(func(updates map[string]interface{}) bool {
			status, ok := updates["status"].(string)
			return ok && status == string(common.CampaignStatusRunning)
		})).Return(nil).Once()

		// Mock creating jobs
		for i := 0; i < campaign.MaxJobs; i++ {
			mockJobService.On("Create", ctx, mock.MatchedBy(func(job *common.Job) bool {
				return job.CampaignID != nil && *job.CampaignID == campaignID && job.Status == common.JobStatusPending
			})).Return(nil).Once()

			mockStorage.On("LinkJobToCampaign", ctx, campaignID, mock.Anything).Return(nil).Once()
		}

		err := cs.RestartCampaign(ctx, campaignID)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
		mockJobService.AssertExpectations(t)
	})

	t.Run("campaign not found", func(t *testing.T) {
		campaignID := "non-existent"

		mockStorage.On("GetCampaign", ctx, campaignID).Return(nil, common.ErrCampaignNotFound).Once()

		err := cs.RestartCampaign(ctx, campaignID)
		assert.Error(t, err)
		assert.Equal(t, common.ErrCampaignNotFound, err)
		mockStorage.AssertExpectations(t)
	})

	t.Run("campaign already running", func(t *testing.T) {
		campaignID := "running-campaign"
		campaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusRunning,
		}

		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()

		err := cs.RestartCampaign(ctx, campaignID)
		assert.Error(t, err)
		assert.Equal(t, common.ErrCampaignRunning, err)
		mockStorage.AssertExpectations(t)
	})
}

func TestCampaignService_GetStatistics(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobService := new(MockJobService)

	cs := NewCampaignService(mockStorage, mockJobService, logger)

	t.Run("successful statistics retrieval", func(t *testing.T) {
		campaignID := "test-campaign-id"
		expectedStats := &common.CampaignStats{
			TotalJobs:     10,
			CompletedJobs: 7,
			TotalCoverage: 15000,
			UniqueCrashes: 5,
			TotalCrashes:  25,
			CorpusSize:    1024 * 1024 * 50, // 50MB
			LastUpdated:   time.Now(),
		}

		mockStorage.On("GetCampaignStatistics", ctx, campaignID).Return(expectedStats, nil).Once()

		stats, err := cs.GetStatistics(ctx, campaignID)
		assert.NoError(t, err)
		assert.Equal(t, expectedStats, stats)
		mockStorage.AssertExpectations(t)
	})
}

func TestCampaignService_checkCampaignCompletion(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("all jobs completed - campaign should complete", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobService := new(MockJobService)
		cs := &campaignService{
			storage:    mockStorage,
			jobService: mockJobService,
			logger:     logger,
		}

		campaignID := "test-campaign-id"
		campaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusRunning,
		}

		completedJob1 := &common.Job{
			ID:     "job1",
			Status: common.JobStatusCompleted,
		}
		completedJob2 := &common.Job{
			ID:     "job2",
			Status: common.JobStatusCompleted,
		}

		jobs := []*common.Job{completedJob1, completedJob2}

		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()
		mockStorage.On("GetCampaignJobs", ctx, campaignID).Return(jobs, nil).Once()
		mockStorage.On("UpdateCampaign", ctx, campaignID, mock.MatchedBy(func(updates map[string]interface{}) bool {
			status, ok := updates["status"].(string)
			return ok && status == string(common.CampaignStatusCompleted)
		})).Return(nil).Once()

		err := cs.checkCampaignCompletion(ctx, campaignID)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})

	t.Run("some jobs still running - campaign continues", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobService := new(MockJobService)
		cs := &campaignService{
			storage:    mockStorage,
			jobService: mockJobService,
			logger:     logger,
		}

		campaignID := "test-campaign-id"
		campaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusRunning,
		}

		runningJob := &common.Job{
			ID:     "job1",
			Status: common.JobStatusRunning,
		}
		completedJob := &common.Job{
			ID:     "job2",
			Status: common.JobStatusCompleted,
		}

		jobs := []*common.Job{runningJob, completedJob}

		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()
		mockStorage.On("GetCampaignJobs", ctx, campaignID).Return(jobs, nil).Once()

		err := cs.checkCampaignCompletion(ctx, campaignID)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})
}

// Additional tests for error cases and edge conditions
func TestCampaignService_Delete(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobService := new(MockJobService)

	cs := NewCampaignService(mockStorage, mockJobService, logger)

	t.Run("successful deletion", func(t *testing.T) {
		campaignID := "test-campaign-id"
		campaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusCompleted,
		}

		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()
		mockStorage.On("DeleteCampaign", ctx, campaignID).Return(nil).Once()

		err := cs.Delete(ctx, campaignID)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})

	t.Run("cannot delete running campaign", func(t *testing.T) {
		campaignID := "running-campaign"
		campaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusRunning,
		}

		mockStorage.On("GetCampaign", ctx, campaignID).Return(campaign, nil).Once()

		err := cs.Delete(ctx, campaignID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete running campaign")
		mockStorage.AssertExpectations(t)
	})
}

func TestCampaignService_List(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobService := new(MockJobService)

	cs := NewCampaignService(mockStorage, mockJobService, logger)

	t.Run("list campaigns with filter", func(t *testing.T) {
		filters := common.CampaignFilters{
			Status: "running",
			Limit:  10,
			Offset: 0,
		}

		expectedCampaigns := []*common.Campaign{
			{
				ID:     "campaign1",
				Name:   "Campaign 1",
				Status: common.CampaignStatusRunning,
			},
			{
				ID:     "campaign2",
				Name:   "Campaign 2",
				Status: common.CampaignStatusRunning,
			},
		}

		mockStorage.On("ListCampaigns", ctx, filters.Limit, filters.Offset, filters.Status).
			Return(expectedCampaigns, nil).Once()

		campaigns, err := cs.List(ctx, filters)
		assert.NoError(t, err)
		assert.Equal(t, expectedCampaigns, campaigns)
		mockStorage.AssertExpectations(t)
	})
}
