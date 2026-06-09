package master

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorage implements common.Storage interface for testing
type MockStorage struct {
	mock.Mock
}

// Campaign operations
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

// Crash group operations
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

// Corpus operations
func (m *MockStorage) AddCorpusFile(ctx context.Context, cf *common.CorpusFile) error {
	args := m.Called(ctx, cf)
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

// Quarantine operations
func (m *MockStorage) AddQuarantinedFile(ctx context.Context, qf *common.QuarantinedFile) error {
	args := m.Called(ctx, qf)
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

// Corpus metrics operations
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

// Bot operations
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

// Job operations
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

func (m *MockStorage) StoreJobLogs(ctx context.Context, jobID string, logs []*common.JobLog) error {
	args := m.Called(ctx, jobID, logs)
	return args.Error(0)
}

func (m *MockStorage) GetJobLogs(ctx context.Context, jobID string, limit, offset int) ([]*common.JobLog, int, error) {
	args := m.Called(ctx, jobID, limit, offset)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*common.JobLog), args.Int(1), args.Error(2)
}

func (m *MockStorage) DeleteJob(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Crash operations
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

func (m *MockStorage) GetCrashCount(ctx context.Context, jobID string) (int, error) {
	args := m.Called(ctx, jobID)
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	args := m.Called(ctx, jobID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashResult), args.Error(1)
}

func (m *MockStorage) StoreCrashInput(ctx context.Context, crashID string, input []byte) error {
	args := m.Called(ctx, crashID, input)
	return args.Error(0)
}

func (m *MockStorage) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
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

// Coverage operations
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

func (m *MockStorage) Backup(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
	return args.Error(0)
}

// Health check
func (m *MockStorage) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Close(ctx context.Context) error {
	args := m.Called(ctx)
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

// Corpus collection operations
func (m *MockStorage) CreateCorpusCollection(ctx context.Context, collection *common.CorpusCollection) error {
	args := m.Called(ctx, collection)
	return args.Error(0)
}

func (m *MockStorage) GetCorpusCollection(ctx context.Context, collectionID string) (*common.CorpusCollection, error) {
	args := m.Called(ctx, collectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusCollection), args.Error(1)
}

func (m *MockStorage) GetCorpusCollections(ctx context.Context) ([]*common.CorpusCollection, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusCollection), args.Error(1)
}

func (m *MockStorage) UpdateCorpusCollection(ctx context.Context, collection *common.CorpusCollection) error {
	args := m.Called(ctx, collection)
	return args.Error(0)
}

func (m *MockStorage) DeleteCorpusCollection(ctx context.Context, collectionID string) error {
	args := m.Called(ctx, collectionID)
	return args.Error(0)
}

// Corpus collection file operations
func (m *MockStorage) AddCorpusCollectionFile(ctx context.Context, file *common.CorpusCollectionFile) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockStorage) GetCorpusCollectionFiles(ctx context.Context, collectionID string) ([]*common.CorpusCollectionFile, error) {
	args := m.Called(ctx, collectionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusCollectionFile), args.Error(1)
}

func (m *MockStorage) DeleteCorpusCollectionFile(ctx context.Context, fileID string) error {
	args := m.Called(ctx, fileID)
	return args.Error(0)
}

// MockJobService implements common.JobService interface
type MockJobService struct {
	mock.Mock
}

func (m *MockJobService) CreateJob(ctx context.Context, job *common.Job) error {
	args := m.Called(ctx, job)
	return args.Error(0)
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

func (m *MockJobService) AssignJob(ctx context.Context, jobID, botID string) error {
	args := m.Called(ctx, jobID, botID)
	return args.Error(0)
}

func (m *MockJobService) CompleteJob(ctx context.Context, jobID string) error {
	args := m.Called(ctx, jobID)
	return args.Error(0)
}

func (m *MockJobService) GetJobsByStatus(ctx context.Context, status common.JobStatus) ([]*common.Job, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Job), args.Error(1)
}

// MockCorpusService implements common.CorpusService interface
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

func (m *MockCorpusService) PromoteCrashToCorpus(ctx context.Context, crashID, campaignID string) (*common.CorpusFile, error) {
	args := m.Called(ctx, crashID, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusService) GetCorpusForJob(ctx context.Context, jobID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, jobID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusService) LinkJobCorpus(ctx context.Context, jobID, campaignID string) error {
	args := m.Called(ctx, jobID, campaignID)
	return args.Error(0)
}

// MockDeduplicationService implements common.DeduplicationService interface
type MockDeduplicationService struct {
	mock.Mock
}

func (m *MockDeduplicationService) ProcessCrash(ctx context.Context, crash *common.CrashResult) (*common.CrashGroup, bool, error) {
	args := m.Called(ctx, crash)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*common.CrashGroup), args.Bool(1), args.Error(2)
}

func (m *MockDeduplicationService) GetCrashGroups(ctx context.Context, campaignID string) ([]*common.CrashGroup, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashGroup), args.Error(1)
}

func (m *MockDeduplicationService) GetStackTrace(ctx context.Context, crashID string) (*common.StackTrace, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.StackTrace), args.Error(1)
}

// Test helper to create a test CampaignStateManager with a mock WSHub
func newTestCampaignStateManager(t *testing.T) (*CampaignStateManager, *MockStorage, *MockJobService, *MockCorpusService, *MockDeduplicationService, *WSHub) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockStorage)
	mockJobSvc := new(MockJobService)
	mockCorpusSvc := new(MockCorpusService)
	mockDedupSvc := new(MockDeduplicationService)

	// Create a real WSHub for tests that need it
	wsHub := NewWSHub(logger)
	go wsHub.Run()

	csm := NewCampaignStateManager(
		mockStorage,
		mockJobSvc,
		mockCorpusSvc,
		mockDedupSvc,
		wsHub,
		logger,
	)

	return csm, mockStorage, mockJobSvc, mockCorpusSvc, mockDedupSvc, wsHub
}

func TestCampaignStateManager_GetCampaignState(t *testing.T) {
	t.Run("get existing campaign state", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		campaignID := "campaign1"
		expectedCampaign := &common.Campaign{
			ID:     campaignID,
			Status: common.CampaignStatusRunning,
		}
		expectedState := &CampaignState{
			Campaign:      expectedCampaign,
			ActiveJobs:    make(map[string]*common.Job),
			CompletedJobs: make(map[string]*common.Job),
			LastUpdate:    time.Now(),
			Metrics:       &CampaignMetrics{LastUpdateTime: time.Now()},
		}

		csm.campaigns[campaignID] = expectedState

		state, err := csm.GetCampaignState(campaignID)
		assert.NoError(t, err)
		assert.Equal(t, expectedState, state)
	})

	t.Run("get non-existent campaign state returns error", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		state, err := csm.GetCampaignState("non-existent")
		assert.Error(t, err)
		assert.Nil(t, state)
		assert.Contains(t, err.Error(), "campaign state not found")
	})
}

func TestCampaignStateManager_GetActiveCampaigns(t *testing.T) {
	t.Run("get all active campaign states", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		// Add multiple campaign states with different statuses
		csm.campaigns["campaign1"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign1",
				Status: common.CampaignStatusRunning,
			},
			Metrics: &CampaignMetrics{},
		}
		csm.campaigns["campaign2"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign2",
				Status: common.CampaignStatusCompleted,
			},
			Metrics: &CampaignMetrics{},
		}
		csm.campaigns["campaign3"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign3",
				Status: common.CampaignStatusRunning,
			},
			Metrics: &CampaignMetrics{},
		}

		states := csm.GetActiveCampaigns()
		assert.Len(t, states, 2) // Only running campaigns

		// Verify all returned campaigns are running
		for _, state := range states {
			assert.Equal(t, common.CampaignStatusRunning, state.Campaign.Status)
		}
	})

	t.Run("returns empty when no active campaigns", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		csm.campaigns["campaign1"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign1",
				Status: common.CampaignStatusCompleted,
			},
			Metrics: &CampaignMetrics{},
		}

		states := csm.GetActiveCampaigns()
		assert.Len(t, states, 0)
	})
}

func TestCampaignStateManager_AddRemoveCampaign(t *testing.T) {
	t.Run("add and remove campaign", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		campaign := &common.Campaign{
			ID:     "test-campaign",
			Name:   "Test Campaign",
			Status: common.CampaignStatusRunning,
		}

		// Add campaign
		csm.AddCampaign(campaign)

		// Verify it was added
		state, err := csm.GetCampaignState("test-campaign")
		assert.NoError(t, err)
		assert.NotNil(t, state)
		assert.Equal(t, campaign.ID, state.Campaign.ID)

		// Remove campaign
		csm.RemoveCampaign("test-campaign")

		// Verify it was removed
		state, err = csm.GetCampaignState("test-campaign")
		assert.Error(t, err)
		assert.Nil(t, state)
	})
}

func TestCampaignStateManager_handleJobCompletion(t *testing.T) {
	t.Run("handle job completion triggers campaign state update", func(t *testing.T) {
		csm, mockStorage, _, mockCorpusSvc, mockDedupSvc, _ := newTestCampaignStateManager(t)

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
					CampaignID: &campaignID,
					Status:     common.JobStatusRunning,
				},
			},
			CompletedJobs: make(map[string]*common.Job),
			Metrics:       &CampaignMetrics{TotalJobs: 1, ActiveJobs: 1},
		}

		// Mock the storage calls that updateCampaignState will make
		mockStorage.On("GetCampaign", mock.Anything, campaignID).
			Return(&common.Campaign{ID: campaignID, Status: common.CampaignStatusRunning}, nil).Maybe()
		mockStorage.On("GetCampaignJobs", mock.Anything, campaignID).
			Return([]*common.Job{{ID: jobID, Status: common.JobStatusCompleted}}, nil).Maybe()
		mockStorage.On("UpdateCampaign", mock.Anything, campaignID, mock.Anything).
			Return(nil).Maybe()

		// Mock deduplication service calls
		mockDedupSvc.On("GetCrashGroups", mock.Anything, campaignID).
			Return([]*common.CrashGroup{}, nil).Maybe()

		// Mock corpus service calls
		mockCorpusSvc.On("GetEvolution", mock.Anything, campaignID).
			Return([]*common.CorpusEvolution{}, nil).Maybe()

		// handleJobCompletion should not panic for existing job
		csm.handleJobCompletion(jobID)
	})

	t.Run("handle job completion for non-existent job does not panic", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		// Job without campaign - should not panic
		csm.handleJobCompletion("non-campaign-job")
	})
}

func TestCampaignStateManager_GetMetrics(t *testing.T) {
	t.Run("get aggregated metrics", func(t *testing.T) {
		csm, _, _, _, _, _ := newTestCampaignStateManager(t)

		// Add campaign states with metrics
		csm.campaigns["campaign1"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign1",
				Status: common.CampaignStatusRunning,
			},
			Metrics: &CampaignMetrics{
				TotalJobs:     10,
				TotalCrashes:  5,
				TotalCoverage: 1000,
			},
		}
		csm.campaigns["campaign2"] = &CampaignState{
			Campaign: &common.Campaign{
				ID:     "campaign2",
				Status: common.CampaignStatusRunning,
			},
			Metrics: &CampaignMetrics{
				TotalJobs:     20,
				TotalCrashes:  3,
				TotalCoverage: 2000,
			},
		}

		metrics := csm.GetMetrics()

		assert.Equal(t, 2, metrics["total_campaigns"])
		assert.Equal(t, 2, metrics["active_campaigns"])
		assert.Equal(t, 30, metrics["total_jobs"])
		assert.Equal(t, 8, metrics["total_crashes"])
		assert.Equal(t, int64(3000), metrics["total_coverage"])
	})
}

func TestCampaignStateManager_StartStop(t *testing.T) {
	t.Run("start and stop campaign state manager", func(t *testing.T) {
		csm, mockStorage, _, _, _, _ := newTestCampaignStateManager(t)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Mock listing campaigns on startup
		mockStorage.On("ListCampaigns", mock.Anything, 1000, 0, "").
			Return([]*common.Campaign{}, nil).Maybe()

		err := csm.Start(ctx)
		assert.NoError(t, err)

		// Give goroutine time to start
		time.Sleep(50 * time.Millisecond)

		// Stop the manager
		err = csm.Stop()
		assert.NoError(t, err)
	})
}
