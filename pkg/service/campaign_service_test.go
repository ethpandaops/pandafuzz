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

// MockJobService is a mock implementation of the JobService interface
type MockJobService struct {
	mock.Mock
}

func (m *MockJobService) Create(ctx context.Context, job *common.Job) error {
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

func (m *MockJobService) List(ctx context.Context, filters common.JobFilters) ([]*common.Job, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Job), args.Error(1)
}

func (m *MockJobService) Cancel(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockJobService) GetLogs(ctx context.Context, jobID string, offset int64) ([]byte, error) {
	args := m.Called(ctx, jobID, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
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
				Duration:    3600,
				MemoryLimit: 2048 * 1024 * 1024,
				Timeout:     1000,
				FuzzerType:  "afl++",
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
				Duration:    3600,
				MemoryLimit: 2048 * 1024 * 1024,
				Timeout:     1000,
				FuzzerType:  "afl++",
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
				return job.CampaignID == campaignID && job.Status == common.JobStatusPending
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
			ActiveJobs:    3,
			CompletedJobs: 7,
			TotalCoverage: 15000,
			UniqueCrashes: 5,
			TotalCrashes:  25,
			CorpusSize:    1024 * 1024 * 50, // 50MB
			ExecPerSecond: 10000,
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
