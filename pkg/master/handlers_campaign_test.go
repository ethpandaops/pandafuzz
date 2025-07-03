package master

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestServer_handleCreateCampaign(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("create campaign successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignReq := &CreateCampaignRequest{
			Name:         "Test Campaign",
			Description:  "Test Description",
			TargetBinary: "/bin/test",
			MaxJobs:      5,
			AutoRestart:  true,
			SharedCorpus: true,
			JobTemplate: common.JobConfig{
				Duration:    3600,
				MemoryLimit: 2048 * 1024 * 1024,
				Timeout:     1000,
				FuzzerType:  "afl++",
			},
			Tags: []string{"test", "fuzzing"},
		}

		body, _ := json.Marshal(campaignReq)

		// Mock campaign service
		mockCampaignSvc.On("Create", mock.Anything, mock.MatchedBy(func(c *common.Campaign) bool {
			return c.Name == campaignReq.Name &&
				c.TargetBinary == campaignReq.TargetBinary &&
				c.MaxJobs == campaignReq.MaxJobs
		})).Return(nil).Once()

		req := httptest.NewRequest("POST", "/api/v1/campaigns", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleCreateCampaign(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response CreateCampaignResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotEmpty(t, response.Campaign.ID)
		assert.Equal(t, campaignReq.Name, response.Campaign.Name)

		mockCampaignSvc.AssertExpectations(t)
	})

	t.Run("create campaign with invalid request", func(t *testing.T) {
		server := &Server{
			logger: logger,
		}

		req := httptest.NewRequest("POST", "/api/v1/campaigns", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleCreateCampaign(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response.Error, "Invalid request body")
	})

	t.Run("create campaign with missing required fields", func(t *testing.T) {
		server := &Server{
			logger: logger,
		}

		campaignReq := &CreateCampaignRequest{
			// Missing Name and TargetBinary
			MaxJobs: 5,
		}

		body, _ := json.Marshal(campaignReq)

		req := httptest.NewRequest("POST", "/api/v1/campaigns", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleCreateCampaign(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response.Error, "Campaign name is required")
	})
}

func TestServer_handleGetCampaign(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("get campaign successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "test-campaign-id"
		expectedCampaign := &common.Campaign{
			ID:           campaignID,
			Name:         "Test Campaign",
			Status:       common.CampaignStatusRunning,
			TargetBinary: "/bin/test",
		}

		mockCampaignSvc.On("Get", mock.Anything, campaignID).
			Return(expectedCampaign, nil).Once()

		req := httptest.NewRequest("GET", "/api/v1/campaigns/"+campaignID, nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleGetCampaign(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response GetCampaignResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedCampaign.ID, response.Campaign.ID)
		assert.Equal(t, expectedCampaign.Name, response.Campaign.Name)

		mockCampaignSvc.AssertExpectations(t)
	})

	t.Run("get non-existent campaign", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "non-existent"

		mockCampaignSvc.On("Get", mock.Anything, campaignID).
			Return(nil, common.ErrCampaignNotFound).Once()

		req := httptest.NewRequest("GET", "/api/v1/campaigns/"+campaignID, nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleGetCampaign(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		mockCampaignSvc.AssertExpectations(t)
	})
}

func TestServer_handleListCampaigns(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("list campaigns with filters", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
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

		mockCampaignSvc.On("List", mock.Anything, mock.MatchedBy(func(f common.CampaignFilters) bool {
			return f.Status == "running" && f.Limit == 10 && f.Offset == 0
		})).Return(expectedCampaigns, nil).Once()

		req := httptest.NewRequest("GET", "/api/v1/campaigns?status=running&limit=10&offset=0", nil)
		w := httptest.NewRecorder()

		server.handleListCampaigns(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response ListCampaignsResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response.Campaigns, 2)
		assert.Equal(t, expectedCampaigns[0].ID, response.Campaigns[0].ID)

		mockCampaignSvc.AssertExpectations(t)
	})

	t.Run("list campaigns with invalid limit", func(t *testing.T) {
		server := &Server{
			logger: logger,
		}

		req := httptest.NewRequest("GET", "/api/v1/campaigns?limit=invalid", nil)
		w := httptest.NewRecorder()

		server.handleListCampaigns(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestServer_handleUpdateCampaign(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("update campaign successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "test-campaign-id"
		updateReq := &UpdateCampaignRequest{
			Status:      "paused",
			MaxJobs:     10,
			AutoRestart: false,
		}

		body, _ := json.Marshal(updateReq)

		mockCampaignSvc.On("Update", mock.Anything, campaignID, mock.MatchedBy(func(u common.CampaignUpdates) bool {
			statusStr, hasStatus := u["status"].(string)
			maxJobs, hasMaxJobs := u["max_jobs"].(int)
			autoRestart, hasAutoRestart := u["auto_restart"].(bool)

			return hasStatus && statusStr == "paused" &&
				hasMaxJobs && maxJobs == 10 &&
				hasAutoRestart && !autoRestart
		})).Return(nil).Once()

		req := httptest.NewRequest("PUT", "/api/v1/campaigns/"+campaignID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleUpdateCampaign(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response UpdateCampaignResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)

		mockCampaignSvc.AssertExpectations(t)
	})
}

func TestServer_handleDeleteCampaign(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("delete campaign successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "test-campaign-id"

		mockCampaignSvc.On("Delete", mock.Anything, campaignID).
			Return(nil).Once()

		req := httptest.NewRequest("DELETE", "/api/v1/campaigns/"+campaignID, nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleDeleteCampaign(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response DeleteCampaignResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)

		mockCampaignSvc.AssertExpectations(t)
	})

	t.Run("delete non-existent campaign", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "non-existent"

		mockCampaignSvc.On("Delete", mock.Anything, campaignID).
			Return(common.ErrCampaignNotFound).Once()

		req := httptest.NewRequest("DELETE", "/api/v1/campaigns/"+campaignID, nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleDeleteCampaign(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		mockCampaignSvc.AssertExpectations(t)
	})
}

func TestServer_handleRestartCampaign(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("restart campaign successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		mockCampaignStateManager := new(MockCampaignStateManager)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
			state: &PersistentState{
				campaignManager: mockCampaignStateManager,
			},
		}

		campaignID := "test-campaign-id"

		mockCampaignSvc.On("RestartCampaign", mock.Anything, campaignID).
			Return(nil).Once()

		req := httptest.NewRequest("POST", "/api/v1/campaigns/"+campaignID+"/restart", nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleRestartCampaign(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response RestartCampaignResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.True(t, response.Success)

		mockCampaignSvc.AssertExpectations(t)
	})
}

func TestServer_handleGetCampaignStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("get campaign statistics successfully", func(t *testing.T) {
		mockCampaignSvc := new(MockCampaignService)
		server := &Server{
			logger: logger,
			services: &ServiceRegistry{
				Campaign: mockCampaignSvc,
			},
		}

		campaignID := "test-campaign-id"
		expectedStats := &common.CampaignStats{
			TotalJobs:     10,
			ActiveJobs:    3,
			CompletedJobs: 7,
			TotalCoverage: 15000,
			UniqueCrashes: 5,
			CorpusSize:    1024 * 1024 * 50,
			ExecPerSecond: 10000,
		}

		mockCampaignSvc.On("GetStatistics", mock.Anything, campaignID).
			Return(expectedStats, nil).Once()

		req := httptest.NewRequest("GET", "/api/v1/campaigns/"+campaignID+"/stats", nil)
		req = mux.SetURLVars(req, map[string]string{"id": campaignID})
		w := httptest.NewRecorder()

		server.handleGetCampaignStats(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response GetCampaignStatsResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedStats.TotalJobs, response.Stats.TotalJobs)
		assert.Equal(t, expectedStats.UniqueCrashes, response.Stats.UniqueCrashes)

		mockCampaignSvc.AssertExpectations(t)
	})
}

// Mock campaign state manager for testing
type MockCampaignStateManager struct {
	mock.Mock
}

func (m *MockCampaignStateManager) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCampaignStateManager) Stop() {
	m.Called()
}

func (m *MockCampaignStateManager) GetState(campaignID string) *CampaignState {
	args := m.Called(campaignID)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*CampaignState)
}

func (m *MockCampaignStateManager) GetAllStates() map[string]*CampaignState {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(map[string]*CampaignState)
}

func (m *MockCampaignStateManager) monitorCampaigns(ctx context.Context) {
	m.Called(ctx)
}

func (m *MockCampaignStateManager) checkAutoRestart(ctx context.Context, campaignID string) {
	m.Called(ctx, campaignID)
}

func (m *MockCampaignStateManager) updateMetrics(campaignID string) {
	m.Called(campaignID)
}

func (m *MockCampaignStateManager) handleJobCompletion(jobID string) {
	m.Called(jobID)
}
