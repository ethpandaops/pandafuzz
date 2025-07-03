package master

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing
type MockCampaignService struct {
	mock.Mock
}

func (m *MockCampaignService) Create(ctx context.Context, campaign *common.Campaign) error {
	args := m.Called(ctx, campaign)
	return args.Error(0)
}

func (m *MockCampaignService) Get(ctx context.Context, id string) (*common.Campaign, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Campaign), args.Error(1)
}

func (m *MockCampaignService) List(ctx context.Context, filters common.CampaignFilters) ([]*common.Campaign, error) {
	args := m.Called(ctx, filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Campaign), args.Error(1)
}

func (m *MockCampaignService) Update(ctx context.Context, id string, updates common.CampaignUpdates) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

func (m *MockCampaignService) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockCampaignService) GetStatistics(ctx context.Context, id string) (*common.CampaignStats, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CampaignStats), args.Error(1)
}

func (m *MockCampaignService) RestartCampaign(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type MockCorpusService struct {
	mock.Mock
}

func (m *MockCorpusService) AddFile(ctx context.Context, file *common.CorpusFile) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockCorpusService) GetEvolution(ctx context.Context, campaignID string) ([]*common.CorpusEvolution, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusEvolution), args.Error(1)
}

func (m *MockCorpusService) SyncCorpus(ctx context.Context, campaignID string, botID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, campaignID, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusService) ShareCorpus(ctx context.Context, fromCampaign, toCampaign string) error {
	args := m.Called(ctx, fromCampaign, toCampaign)
	return args.Error(0)
}

type MockWSHub struct {
	mock.Mock
}

func (m *MockWSHub) Broadcast(message WSMessage) {
	m.Called(message)
}

func (m *MockWSHub) BroadcastToTopic(topic string, message WSMessage) {
	m.Called(topic, message)
}

func (m *MockWSHub) RegisterClient(client *WSClient) {
	m.Called(client)
}

func (m *MockWSHub) UnregisterClient(client *WSClient) {
	m.Called(client)
}

func (m *MockWSHub) Run() {
	m.Called()
}

func (m *MockWSHub) Stop() {
	m.Called()
}

func TestCampaignStateManager_Start(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("start campaign state manager", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := NewCampaignStateManager(
			mockStorage,
			mockJobSvc,
			mockCampaignSvc,
			mockCorpusSvc,
			mockWSHub,
			logger,
		)

		ctx := context.Background()

		// Mock listing campaigns on startup
		mockCampaignSvc.On("List", ctx, mock.Anything).
			Return([]*common.Campaign{}, nil).Once()

		err := csm.Start(ctx)
		assert.NoError(t, err)

		// Give goroutine time to start
		time.Sleep(100 * time.Millisecond)

		// Stop the manager
		csm.Stop()

		mockCampaignSvc.AssertExpectations(t)
	})
}

func TestCampaignStateManager_monitorCampaigns(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("monitor campaigns and check completion", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:    make(map[string]*CampaignState),
			storage:      mockStorage,
			jobSvc:       mockJobSvc,
			campaignSvc:  mockCampaignSvc,
			corpusSvc:    mockCorpusSvc,
			wsHub:        mockWSHub,
			logger:       logger,
			stopCh:       make(chan struct{}),
			tickInterval: 100 * time.Millisecond, // Fast for testing
		}

		ctx := context.Background()

		// Add a running campaign to monitor
		campaign := &common.Campaign{
			ID:     "campaign1",
			Status: common.CampaignStatusRunning,
		}

		csm.campaigns["campaign1"] = &CampaignState{
			Campaign: campaign,
			ActiveJobs: map[string]*common.Job{
				"job1": {ID: "job1", Status: common.JobStatusRunning},
			},
			CompletedJobs: make(map[string]*common.Job),
			LastUpdate:    time.Now(),
		}

		// Mock campaign service list
		mockCampaignSvc.On("List", ctx, mock.Anything).
			Return([]*common.Campaign{campaign}, nil).Once()

		// Mock getting campaign statistics
		stats := &common.CampaignStats{
			TotalJobs:     1,
			ActiveJobs:    1,
			CompletedJobs: 0,
		}
		mockCampaignSvc.On("GetStatistics", ctx, "campaign1").
			Return(stats, nil).Once()

		// Mock WebSocket broadcast
		mockWSHub.On("BroadcastToTopic", "campaign:campaign1", mock.Anything).Maybe()

		// Run one iteration
		go csm.monitorCampaigns(ctx)

		// Wait for one tick
		time.Sleep(200 * time.Millisecond)

		// Stop monitoring
		close(csm.stopCh)

		mockCampaignSvc.AssertExpectations(t)
		mockWSHub.AssertExpectations(t)
	})
}

func TestCampaignStateManager_checkAutoRestart(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("auto restart completed campaign", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:   make(map[string]*CampaignState),
			storage:     mockStorage,
			jobSvc:      mockJobSvc,
			campaignSvc: mockCampaignSvc,
			corpusSvc:   mockCorpusSvc,
			wsHub:       mockWSHub,
			logger:      logger,
		}

		ctx := context.Background()
		campaignID := "campaign1"

		// Create a completed campaign with auto-restart enabled
		campaign := &common.Campaign{
			ID:          campaignID,
			Status:      common.CampaignStatusCompleted,
			AutoRestart: true,
		}

		// Mock getting campaign
		mockCampaignSvc.On("Get", ctx, campaignID).
			Return(campaign, nil).Once()

		// Mock restart campaign
		mockCampaignSvc.On("RestartCampaign", ctx, campaignID).
			Return(nil).Once()

		// Mock WebSocket broadcast
		mockWSHub.On("Broadcast", mock.MatchedBy(func(msg WSMessage) bool {
			return msg.Type == "campaign_restarted"
		})).Once()

		csm.checkAutoRestart(ctx, campaignID)

		mockCampaignSvc.AssertExpectations(t)
		mockWSHub.AssertExpectations(t)
	})

	t.Run("skip restart for running campaign", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:   make(map[string]*CampaignState),
			storage:     mockStorage,
			jobSvc:      mockJobSvc,
			campaignSvc: mockCampaignSvc,
			corpusSvc:   mockCorpusSvc,
			wsHub:       mockWSHub,
			logger:      logger,
		}

		ctx := context.Background()
		campaignID := "campaign1"

		// Create a running campaign
		campaign := &common.Campaign{
			ID:          campaignID,
			Status:      common.CampaignStatusRunning,
			AutoRestart: true,
		}

		// Mock getting campaign
		mockCampaignSvc.On("Get", ctx, campaignID).
			Return(campaign, nil).Once()

		// Should not attempt restart
		csm.checkAutoRestart(ctx, campaignID)

		mockCampaignSvc.AssertExpectations(t)
		// Ensure RestartCampaign was NOT called
		mockCampaignSvc.AssertNotCalled(t, "RestartCampaign", ctx, campaignID)
	})
}

func TestCampaignStateManager_handleJobCompletion(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("handle job completion for campaign", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:   make(map[string]*CampaignState),
			storage:     mockStorage,
			jobSvc:      mockJobSvc,
			campaignSvc: mockCampaignSvc,
			corpusSvc:   mockCorpusSvc,
			wsHub:       mockWSHub,
			logger:      logger,
		}

		jobID := "job1"
		campaignID := "campaign1"

		// Create campaign state with active job
		csm.campaigns[campaignID] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     campaignID,
				Status: common.CampaignStatusRunning,
			},
			ActiveJobs: map[string]*common.Job{
				jobID: {
					ID:         jobID,
					CampaignID: campaignID,
					Status:     common.JobStatusRunning,
				},
			},
			CompletedJobs: make(map[string]*common.Job),
			Metrics: &CampaignMetrics{
				TotalJobs:     1,
				ActiveJobs:    1,
				CompletedJobs: 0,
			},
		}

		csm.handleJobCompletion(jobID)

		// Verify job moved from active to completed
		state := csm.campaigns[campaignID]
		assert.Empty(t, state.ActiveJobs)
		assert.Contains(t, state.CompletedJobs, jobID)
		assert.Equal(t, 0, state.Metrics.ActiveJobs)
		assert.Equal(t, 1, state.Metrics.CompletedJobs)
	})

	t.Run("handle job completion for non-campaign job", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:   make(map[string]*CampaignState),
			storage:     mockStorage,
			jobSvc:      mockJobSvc,
			campaignSvc: mockCampaignSvc,
			corpusSvc:   mockCorpusSvc,
			wsHub:       mockWSHub,
			logger:      logger,
		}

		// Job without campaign - should not panic
		csm.handleJobCompletion("non-campaign-job")

		// No assertions needed, just ensure it doesn't panic
	})
}

func TestCampaignStateManager_updateMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("update campaign metrics", func(t *testing.T) {
		mockStorage := new(MockStorage)
		mockJobSvc := new(service.MockJobService)
		mockCampaignSvc := new(MockCampaignService)
		mockCorpusSvc := new(MockCorpusService)
		mockWSHub := new(MockWSHub)

		csm := &CampaignStateManager{
			campaigns:   make(map[string]*CampaignState),
			storage:     mockStorage,
			jobSvc:      mockJobSvc,
			campaignSvc: mockCampaignSvc,
			corpusSvc:   mockCorpusSvc,
			wsHub:       mockWSHub,
			logger:      logger,
		}

		campaignID := "campaign1"

		// Create campaign state
		csm.campaigns[campaignID] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     campaignID,
				Status: common.CampaignStatusRunning,
			},
			Metrics: &CampaignMetrics{},
		}

		// Mock getting campaign statistics
		stats := &common.CampaignStats{
			TotalJobs:     10,
			ActiveJobs:    3,
			CompletedJobs: 7,
			TotalCoverage: 15000,
			UniqueCrashes: 5,
			CorpusSize:    1024 * 1024 * 50,
			ExecPerSecond: 10000,
		}

		mockCampaignSvc.On("GetStatistics", mock.Anything, campaignID).
			Return(stats, nil).Once()

		// Mock WebSocket broadcast
		mockWSHub.On("BroadcastToTopic", "campaign:campaign1", mock.Anything).Once()

		csm.updateMetrics(campaignID)

		// Verify metrics updated
		state := csm.campaigns[campaignID]
		assert.Equal(t, stats.TotalJobs, state.Metrics.TotalJobs)
		assert.Equal(t, stats.ActiveJobs, state.Metrics.ActiveJobs)
		assert.Equal(t, stats.CompletedJobs, state.Metrics.CompletedJobs)
		assert.Equal(t, stats.TotalCoverage, state.Metrics.TotalCoverage)
		assert.Equal(t, stats.UniqueCrashes, state.Metrics.UniqueCrashes)

		mockCampaignSvc.AssertExpectations(t)
		mockWSHub.AssertExpectations(t)
	})
}

func TestCampaignStateManager_GetState(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("get existing campaign state", func(t *testing.T) {
		csm := &CampaignStateManager{
			campaigns: make(map[string]*CampaignState),
			logger:    logger,
		}

		campaignID := "campaign1"
		expectedState := &CampaignState{
			Campaign: &common.Campaign{
				ID:     campaignID,
				Status: common.CampaignStatusRunning,
			},
		}

		csm.campaigns[campaignID] = expectedState

		state := csm.GetState(campaignID)
		assert.Equal(t, expectedState, state)
	})

	t.Run("get non-existent campaign state", func(t *testing.T) {
		csm := &CampaignStateManager{
			campaigns: make(map[string]*CampaignState),
			logger:    logger,
		}

		state := csm.GetState("non-existent")
		assert.Nil(t, state)
	})
}

func TestCampaignStateManager_GetAllStates(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("get all campaign states", func(t *testing.T) {
		csm := &CampaignStateManager{
			campaigns: make(map[string]*CampaignState),
			logger:    logger,
		}

		// Add multiple campaign states
		csm.campaigns["campaign1"] = &CampaignState{
			Campaign: &common.Campaign{ID: "campaign1"},
		}
		csm.campaigns["campaign2"] = &CampaignState{
			Campaign: &common.Campaign{ID: "campaign2"},
		}

		states := csm.GetAllStates()
		assert.Len(t, states, 2)
		assert.Contains(t, states, "campaign1")
		assert.Contains(t, states, "campaign2")
	})
}
