package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/adapters"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	pandafuzzClient "github.com/ethpandaops/pandafuzz/pkg/clients/go"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// APIIntegrationTestSuite provides a test suite for API v1 integration tests
type APIIntegrationTestSuite struct {
	suite.Suite
	ctx          context.Context
	cancel       context.CancelFunc
	tempDir      string
	server       *v1.Server
	client       *pandafuzzClient.SimpleClient
	database     common.Database
	masterConfig *common.MasterConfig
	logger       *logrus.Logger
	baseURL      string
}

// SetupSuite sets up the test environment before all tests
func (s *APIIntegrationTestSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "pandafuzz-api-test-*")
	s.Require().NoError(err)
	s.tempDir = tempDir

	// Setup logger
	s.logger = logrus.New()
	s.logger.SetLevel(logrus.InfoLevel)
	s.logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05.000",
		DisableColors:   true,
	})

	// Create master config
	s.masterConfig = &common.MasterConfig{
		Server: common.ServerConfig{
			Host: "127.0.0.1",
			Port: 8766, // Different port for API tests
		},
		Database: common.DatabaseConfig{
			Type: "sqlite",
			Path: filepath.Join(tempDir, "api-test.db"),
		},
		Storage: config.StorageConfig{
			Type: "filesystem",
			Filesystem: config.FilesystemConfig{
				BasePath: tempDir,
			},
		},
		Timeouts: common.TimeoutConfig{
			BotHeartbeat:   10 * time.Second,
			JobExecution:   5 * time.Minute,
			MasterRecovery: 30 * time.Second,
			HTTPRequest:    10 * time.Second,
		},
		Retry: common.RetryConfigs{
			Network:  common.NetworkRetryPolicy,
			Database: common.DatabaseRetryPolicy,
		},
		Limits: common.ResourceLimits{
			MaxCrashSize:      1024 * 1024,      // 1MB
			MaxCorpusSize:     10 * 1024 * 1024, // 10MB
			MaxJobDuration:    5 * time.Hour,
			MaxConcurrentJobs: 100,
			MaxCrashCount:     1000,
		},
		Circuit: common.CircuitConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
			Enabled:      true,
		},
		Monitoring: common.MonitoringConfig{},
		Security:   common.SecurityConfig{},
		Logging:    common.LoggingConfig{},
	}

	// Setup database
	db, err := storage.NewSQLiteStorage(s.masterConfig.Database, s.logger)
	s.Require().NoError(err)
	s.database = db

	err = db.CreateTables(s.ctx)
	s.Require().NoError(err)

	// Create adapters
	state := master.NewPersistentState(db, s.masterConfig, s.logger)

	adapters := &adapters.CompositeAdapter{
		BotAdapter:       adapters.NewBotAdapter(state, s.logger),
		JobAdapter:       adapters.NewJobAdapter(state, s.logger),
		CampaignAdapter:  adapters.NewCampaignAdapter(state, s.logger),
		CorpusAdapter:    adapters.NewCorpusAdapter(state, s.logger),
		CrashAdapter:     adapters.NewCrashAdapter(state, s.logger),
		AnalyticsAdapter: adapters.NewAnalyticsAdapter(state, s.logger),
	}

	// Create and start API server
	s.server = v1.NewServer(s.masterConfig, adapters, s.logger)

	go func() {
		if err := s.server.Start(s.ctx); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("Server failed to start")
		}
	}()

	// Wait for server to be ready
	s.baseURL = fmt.Sprintf("http://%s:%d", s.masterConfig.Server.Host, s.masterConfig.Server.Port)
	s.waitForServer()

	// Create client
	client, err := pandafuzzClient.NewSimpleClient(
		s.baseURL,
		pandafuzzClient.WithSimpleLogger(s.logger),
		pandafuzzClient.WithSimpleHTTPClient(&http.Client{Timeout: 10 * time.Second}),
	)
	s.Require().NoError(err)
	s.client = client
}

// TearDownSuite cleans up after all tests
func (s *APIIntegrationTestSuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}

	if s.client != nil {
		s.client.Close()
	}

	if s.database != nil {
		s.database.Close(context.Background())
	}

	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
}

// SetupTest sets up each individual test
func (s *APIIntegrationTestSuite) SetupTest() {
	// Clean up any existing data between tests
	s.cleanupTestData()
}

// TearDownTest cleans up after each test
func (s *APIIntegrationTestSuite) TearDownTest() {
	s.cleanupTestData()
}

// waitForServer waits for the API server to be ready
func (s *APIIntegrationTestSuite) waitForServer() {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(s.baseURL + "/api/v1/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.Require().Fail("API server not ready after retries")
}

// cleanupTestData removes test data between tests
func (s *APIIntegrationTestSuite) cleanupTestData() {
	ctx := context.Background()

	// Clean bots
	if _, err := s.database.Exec(ctx, "DELETE FROM bots", []interface{}{}); err != nil {
		s.logger.WithError(err).Warn("Failed to clean bots table")
	}

	// Clean jobs
	if _, err := s.database.Exec(ctx, "DELETE FROM jobs", []interface{}{}); err != nil {
		s.logger.WithError(err).Warn("Failed to clean jobs table")
	}

	// Clean campaigns
	if _, err := s.database.Exec(ctx, "DELETE FROM campaigns", []interface{}{}); err != nil {
		s.logger.WithError(err).Warn("Failed to clean campaigns table")
	}

	// Clean crashes
	if _, err := s.database.Exec(ctx, "DELETE FROM crashes", []interface{}{}); err != nil {
		s.logger.WithError(err).Warn("Failed to clean crashes table")
	}

	// Clean corpus
	if _, err := s.database.Exec(ctx, "DELETE FROM corpus", []interface{}{}); err != nil {
		s.logger.WithError(err).Warn("Failed to clean corpus table")
	}
}

// Helper methods for creating test data

// createTestBot creates a test bot and returns its ID
func (s *APIIntegrationTestSuite) createTestBot(name string) uuid.UUID {
	createReq := generated.BotCreateRequest{
		Name:         name,
		Hostname:     "test-host",
		Capabilities: []generated.BotCreateRequestCapabilities{generated.BotCreateRequestCapabilitiesFuzzing},
		ResourceUsage: &generated.BotResourceUsage{
			CpuPercent:           50.0,
			MemoryMb:             512,
			DiskSpaceMb:          1024,
			ActiveJobs:           0,
			QueueLength:          0,
			NetworkBytesSent:     0,
			NetworkBytesReceived: 0,
		},
	}

	resp, err := s.client.CreateBot(s.ctx, createReq)
	s.Require().NoError(err)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var bot generated.Bot
	err = parseJSONResponse(resp, &bot)
	s.Require().NoError(err)

	return bot.Id
}

// createTestCampaign creates a test campaign and returns its ID
func (s *APIIntegrationTestSuite) createTestCampaign(name string) uuid.UUID {
	createReq := generated.CampaignCreateRequest{
		Name:        name,
		Description: "Test campaign",
		JobTemplate: generated.CampaignCreateRequestJobTemplate{
			Fuzzer:       generated.FuzzerTypeAflplusplus,
			TargetBinary: "/bin/test",
			TargetArgs:   []string{"@@"},
			Timeout:      300,
			MemoryLimit:  1024,
		},
	}

	resp, err := s.client.CreateCampaign(s.ctx, createReq)
	s.Require().NoError(err)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	return campaign.Id
}

// createTestJob creates a test job and returns its ID
func (s *APIIntegrationTestSuite) createTestJob(name string, campaignID *uuid.UUID) uuid.UUID {
	createReq := generated.JobCreateRequest{
		Name:         name,
		CampaignId:   campaignID,
		Fuzzer:       generated.FuzzerTypeAflplusplus,
		TargetBinary: "/bin/test",
		TargetArgs:   []string{"@@"},
		Timeout:      300,
		MemoryLimit:  1024,
	}

	client := s.client.GetClient()
	resp, err := client.CreateJob(s.ctx, createReq)
	s.Require().NoError(err)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	return job.Id
}

// assertEventually checks if a condition becomes true within a timeout
func (s *APIIntegrationTestSuite) assertEventually(condition func() bool, timeout time.Duration, msgAndArgs ...interface{}) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.Fail("Condition not met within timeout", msgAndArgs...)
}

// TestSuite entry point
func TestAPIIntegrationSuite(t *testing.T) {
	suite.Run(t, new(APIIntegrationTestSuite))
}
