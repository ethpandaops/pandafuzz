package integration

import (
	"context"
	// "database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/stretchr/testify/require"
	"github.com/sirupsen/logrus"
)

// TestEnvironment holds the test environment components
type TestEnvironment struct {
	t            *testing.T
	ctx          context.Context
	cancel       context.CancelFunc
	masterConfig *common.MasterConfig
	botConfig    *common.BotConfig
	database     common.Database
	state        *master.PersistentState
	timeoutMgr   *master.TimeoutManager
	recoveryMgr  *master.RecoveryManager
	server       *master.Server
	tempDir      string
	masterURL    string
	httpClient   *http.Client
}

// SetupTestEnvironment creates a test environment
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "pandafuzz-test-*")
	require.NoError(t, err)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	// Create master config
	masterConfig := &common.MasterConfig{
		Server: common.ServerConfig{
			Host:          "127.0.0.1",
			Port:          8765, // Use fixed port for testing
		},
		Database: common.DatabaseConfig{
			Type: "sqlite",
			Path: filepath.Join(tempDir, "test.db"),
		},
		Storage: common.StorageConfig{
			BasePath: tempDir,
		},
		Timeouts: common.TimeoutConfig{
			BotHeartbeat:   10 * time.Second,
			JobExecution:   5 * time.Minute,
			MasterRecovery: 30 * time.Second,
			HTTPRequest:    10 * time.Second,
		},
		Retry: common.RetryConfigs{
			Network: common.NetworkRetryPolicy,
			Database: common.DatabaseRetryPolicy,
		},
		Limits: common.ResourceLimits{
			MaxCrashSize:      1024 * 1024,     // 1MB
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

	// Create bot config
	botConfig := &common.BotConfig{
		ID:           "test-bot-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:         "test-bot",
		MasterURL:    "", // Will be set after server starts
		Capabilities: []string{"afl++", "libfuzzer"},
		Fuzzing: common.FuzzingConfig{
			WorkDir:           filepath.Join(tempDir, "work"),
			MaxJobs:           2,
			CorpusSync:        true,
			CrashReporting:    true,
			CoverageReporting: true,
		},
		Timeouts: common.BotTimeoutConfig{
			HeartbeatInterval:   5 * time.Second,
			JobExecution:        5 * time.Minute,
			MasterCommunication: 5 * time.Second,
		},
		Retry: common.BotRetryConfig{},
		Resources: common.BotResourceConfig{
			MaxCPUPercent:  80,
			MaxMemoryMB:    1024,
			MaxDiskSpaceMB: 10240,
		},
		Logging: common.LoggingConfig{},
	}

	// Create logger for test environment
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Create database
	db, err := storage.NewSQLiteStorage(masterConfig.Database, logger)
	require.NoError(t, err)

	err = db.CreateTables()
	require.NoError(t, err)

	// Create master components
	state := master.NewPersistentState(db, masterConfig, logger)

	timeoutMgr := master.NewTimeoutManager(state, masterConfig, logger)
	recoveryMgr := master.NewRecoveryManager(state, timeoutMgr, masterConfig, logger)
	server := master.NewServer(masterConfig, state, timeoutMgr, nil, logger)

	// Create HTTP client with shorter timeout for tests
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	env := &TestEnvironment{
		t:            t,
		ctx:          ctx,
		cancel:       cancel,
		masterConfig: masterConfig,
		botConfig:    botConfig,
		database:     db,
		state:        state,
		timeoutMgr:   timeoutMgr,
		recoveryMgr:  recoveryMgr,
		server:       server,
		tempDir:      tempDir,
		httpClient:   httpClient,
	}

	// Cleanup on test completion
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// StartMaster starts the master server and returns the URL
func (env *TestEnvironment) StartMaster() error {
	// Start timeout monitoring
	err := env.timeoutMgr.Start()
	if err != nil {
		return fmt.Errorf("failed to start timeout manager: %w", err)
	}

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- env.server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("server failed to start: %w", err)
		}
	default:
		// Server is running
	}

	// Build server URL
	env.masterURL = fmt.Sprintf("http://%s:%d", env.masterConfig.Server.Host, env.masterConfig.Server.Port)
	env.botConfig.MasterURL = env.masterURL

	// Wait for server to be ready
	return env.WaitForMaster()
}

// WaitForMaster waits for the master server to be ready
func (env *TestEnvironment) WaitForMaster() error {
	maxRetries := 20
	for i := 0; i < maxRetries; i++ {
		resp, err := env.httpClient.Get(env.masterURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("master server not ready after %d retries", maxRetries)
}

// Cleanup cleans up the test environment
func (env *TestEnvironment) Cleanup() {
	// Cancel context
	if env.cancel != nil {
		env.cancel()
	}

	// Stop server
	if env.server != nil {
		env.server.Stop()
	}

	// Stop timeout manager
	if env.timeoutMgr != nil {
		env.timeoutMgr.Stop()
	}

	// Close database
	if env.database != nil {
		env.database.Close()
	}

	// Remove temp directory
	if env.tempDir != "" {
		os.RemoveAll(env.tempDir)
	}
}

// CreateTestJob creates a test job
func (env *TestEnvironment) CreateTestJob(name string) (*common.Job, error) {
	job := &common.Job{
		ID:          fmt.Sprintf("job-%s-%d", name, time.Now().UnixNano()),
		Name:        name,
		Status:      common.JobStatusPending,
		// Priority:    common.JobPriorityNormal, // TODO: Find correct priority constant
		Fuzzer:      "afl++",
		Target:      "/bin/test",
		// TargetArgs:  []string{"@@"}, // TODO: Check if this field exists
		// Corpus:      []string{"/corpus"}, // TODO: Check if this field exists
		// Dictionary:  "/dict.txt", // TODO: Check if this field exists
		// TimeoutSec:  300, // TODO: Check if this field exists
		// MemoryLimit: 1024, // TODO: Check if this field exists
		CreatedAt:   time.Now(),
		TimeoutAt:   time.Now().Add(5 * time.Minute),
		Config: common.JobConfig{
			Duration:    5 * time.Minute,
			MemoryLimit: 1024 * 1024 * 1024, // 1GB
			Timeout:     10 * time.Minute,
		},
	}

	return job, env.state.SaveJobWithRetry(job)
}

// CreateTestBot creates a test bot
func (env *TestEnvironment) CreateTestBot(id string) (*common.Bot, error) {
	bot := &common.Bot{
		ID:           id,
		Status:       common.BotStatusIdle,
		Hostname:     "test-host",
		Capabilities: []string{"afl++", "libfuzzer"},
		LastSeen:     time.Now(),
		RegisteredAt: time.Now(),
		// IP:           "127.0.0.1", // TODO: Check if this field exists
	}

	return bot, env.state.SaveBotWithRetry(bot)
}

// CreateTestCrash creates a test crash result
func (env *TestEnvironment) CreateTestCrash(jobID string) *common.CrashResult {
	return &common.CrashResult{
		ID:        fmt.Sprintf("crash-%d", time.Now().UnixNano()),
		JobID:     jobID,
		BotID:     env.botConfig.ID,
		Timestamp: time.Now(),
		Input:     []byte("AAAA"),
		Size:      4,
		Hash:      "deadbeef",
		Type:      "segmentation_fault",
		Output:    "Segmentation fault (core dumped)",
		StackTrace: "#0 0x00000000 in main()",
	}
}

// CreateTestCoverage creates a test coverage result
func (env *TestEnvironment) CreateTestCoverage(jobID string) *common.CoverageResult {
	return &common.CoverageResult{
		ID:              fmt.Sprintf("coverage-%d", time.Now().UnixNano()),
		JobID:           jobID,
		BotID:           env.botConfig.ID,
		Timestamp:       time.Now(),
		Edges:           1000,
		// CoveredEdges:    500, // TODO: Check if this field exists
		NewEdges:        10,
		// CoveragePercent: 50.0, // TODO: Check if this field exists
	}
}

// AssertEventually asserts that a condition is met within a timeout
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Condition not met within %v: %s", timeout, message)
}

// WaitForJobStatus waits for a job to reach a specific status
func (env *TestEnvironment) WaitForJobStatus(jobID string, expectedStatus common.JobStatus, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := env.state.GetJob(jobID)
		if err != nil {
			return err
		}
		if job.Status == expectedStatus {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("job %s did not reach status %s within %v", jobID, expectedStatus, timeout)
}

// WaitForBotStatus waits for a bot to reach a specific status
func (env *TestEnvironment) WaitForBotStatus(botID string, expectedStatus common.BotStatus, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		bot, err := env.state.GetBot(botID)
		if err != nil {
			return err
		}
		if bot.Status == expectedStatus {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("bot %s did not reach status %s within %v", botID, expectedStatus, timeout)
}

// EnableDebugLogging enables debug logging for troubleshooting
func EnableDebugLogging() {
	logger := logrus.StandardLogger()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05.000",
	})
}

// CreateMockFuzzer creates a mock fuzzer for testing
type MockFuzzer struct {
	running bool
	crashes []*common.CrashResult
}

func (m *MockFuzzer) Start() error {
	m.running = true
	return nil
}

func (m *MockFuzzer) Stop() error {
	m.running = false
	return nil
}

func (m *MockFuzzer) IsRunning() bool {
	return m.running
}

func (m *MockFuzzer) AddCrash(crash *common.CrashResult) {
	m.crashes = append(m.crashes, crash)
}

func (m *MockFuzzer) GetCrashes() []*common.CrashResult {
	return m.crashes
}