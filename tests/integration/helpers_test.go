package integration

import (
	"context"
	// "database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/master"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestEnvironment holds the test environment components
type TestEnvironment struct {
	t            testing.TB
	ctx          context.Context
	cancel       context.CancelFunc
	masterConfig *common.MasterConfig
	botConfig    *common.BotConfig
	apiKey       string
	database     common.Database
	state        *master.PersistentState
	timeoutMgr   *master.TimeoutManager
	recoveryMgr  *master.RecoveryManager
	server       *master.Server
	tempDir      string
	masterURL    string
	httpClient   *http.Client
	logger       *logrus.Logger
}

const testAPIKey = "test-api-key"

// SetupTestEnvironment creates a test environment
func SetupTestEnvironment(t testing.TB) *TestEnvironment {
	t.Helper()

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "pandafuzz-test-*")
	require.NoError(t, err)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping integration tests: cannot bind TCP listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	// Create master config
	masterConfig := &common.MasterConfig{
		Server: common.ServerConfig{
			Host: "127.0.0.1",
			Port: port,
		},
		Database: common.DatabaseConfig{
			Type: "sqlite",
			Path: filepath.Join(tempDir, "test.db"),
		},
		Storage: config.StorageConfig{
			Type:        "filesystem",
			MaxFileSize: 100 * 1024 * 1024, // 100MB
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
		Security: common.SecurityConfig{
			EnableAuth:            true,
			AllowInsecure:         false,
			APIKeys:               map[string]string{testAPIKey: "integration-tests"},
			EnableInputValidation: true,
			MaxRequestSize:        10 * 1024 * 1024, // 10MB
			AllowedFileExtensions: []string{".txt", ".bin", ".data", ".input"},
			EnableSanitization:    true,
			MaxCrashFileSize:      10 * 1024 * 1024, // 10MB
			MaxCorpusFileSize:     1024 * 1024,      // 1MB
			ProcessIsolationLevel: "sandbox",
		},
		Logging: common.LoggingConfig{},
	}

	// Create bot config
	botConfig := &common.BotConfig{
		ID:           "test-bot-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:         "test-bot",
		MasterURL:    "", // Will be set after server starts
		APIKey:       testAPIKey,
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

	err = db.CreateTables(ctx)
	require.NoError(t, err)

	// Create master components
	state := master.NewPersistentState(db, masterConfig, logger)

	timeoutMgr := master.NewTimeoutManager(state, masterConfig, logger)
	recoveryMgr := master.NewRecoveryManager(state, timeoutMgr, masterConfig, logger)
	server := master.NewServer(masterConfig, state, timeoutMgr, nil, logger)

	// Create HTTP client with shorter timeout for tests
	httpClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: newAuthRoundTripper(testAPIKey, http.DefaultTransport),
	}

	env := &TestEnvironment{
		t:            t,
		ctx:          ctx,
		cancel:       cancel,
		masterConfig: masterConfig,
		botConfig:    botConfig,
		apiKey:       testAPIKey,
		database:     db,
		state:        state,
		timeoutMgr:   timeoutMgr,
		recoveryMgr:  recoveryMgr,
		server:       server,
		tempDir:      tempDir,
		httpClient:   httpClient,
		logger:       logger,
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

	// Initialize storage which creates service manager and enables API v1
	err = env.server.InitializeStorage()
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
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
		env.database.Close(context.Background())
	}

	// Remove temp directory
	if env.tempDir != "" {
		os.RemoveAll(env.tempDir)
	}
}

// CreateTestJob creates a test job with a valid UUID
func (env *TestEnvironment) CreateTestJob(name string) (*common.Job, error) {
	// Generate a proper UUID for API compatibility
	jobID := uuid.New().String()
	job := &common.Job{
		ID:        jobID,
		Name:      name,
		Status:    common.JobStatusPending,
		Fuzzer:    "afl++",
		Target:    "/bin/test",
		CreatedAt: time.Now(),
		TimeoutAt: time.Now().Add(5 * time.Minute),
		Config: common.JobConfig{
			Duration:    5 * time.Minute,
			MemoryLimit: 1024 * 1024 * 1024, // 1GB
			Timeout:     10 * time.Minute,
		},
	}

	return job, env.state.SaveJobWithRetry(context.Background(), job)
}

// CreateTestJobWithTime creates a test job with a specific creation timestamp
// This is useful for testing FIFO ordering where jobs need deterministic ordering
// Note: TimeoutAt is set relative to now (not createdAt) to ensure jobs remain valid
func (env *TestEnvironment) CreateTestJobWithTime(name string, createdAt time.Time) (*common.Job, error) {
	jobID := uuid.New().String()
	job := &common.Job{
		ID:        jobID,
		Name:      name,
		Status:    common.JobStatusPending,
		Fuzzer:    "afl++",
		Target:    "/bin/test",
		CreatedAt: createdAt,
		TimeoutAt: time.Now().Add(5 * time.Minute), // TimeoutAt must be in the future
		Config: common.JobConfig{
			Duration:    5 * time.Minute,
			MemoryLimit: 1024 * 1024 * 1024, // 1GB
			Timeout:     10 * time.Minute,
		},
	}

	return job, env.state.SaveJobWithRetry(context.Background(), job)
}

// CreateTestBot creates a test bot with a valid UUID
func (env *TestEnvironment) CreateTestBot(id string) (*common.Bot, error) {
	// Generate a proper UUID for API compatibility, using the id as the name
	botID := uuid.New().String()
	bot := &common.Bot{
		ID:           botID,
		Name:         id, // Use the parameter as name for easy identification
		Status:       common.BotStatusIdle,
		Hostname:     "test-host",
		Capabilities: []string{"afl++", "libfuzzer"},
		LastSeen:     time.Now(),
		RegisteredAt: time.Now(),
	}

	return bot, env.state.SaveBotWithRetry(context.Background(), bot)
}

// CreateTestCrash creates a test crash result
func (env *TestEnvironment) CreateTestCrash(jobID string) *common.CrashResult {
	return &common.CrashResult{
		ID:         fmt.Sprintf("crash-%d", time.Now().UnixNano()),
		JobID:      jobID,
		BotID:      env.botConfig.ID,
		Timestamp:  time.Now(),
		Input:      []byte("AAAA"),
		Size:       4,
		Hash:       "deadbeef",
		Type:       "segmentation_fault",
		Output:     "Segmentation fault (core dumped)",
		StackTrace: "#0 0x00000000 in main()",
	}
}

// CreateTestCoverage creates a test coverage result
func (env *TestEnvironment) CreateTestCoverage(jobID string) *common.CoverageResult {
	return &common.CoverageResult{
		ID:        fmt.Sprintf("coverage-%d", time.Now().UnixNano()),
		JobID:     jobID,
		BotID:     env.botConfig.ID,
		Timestamp: time.Now(),
		Edges:     1000,
		NewEdges:  10,
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
		job, err := env.state.GetJob(context.Background(), jobID)
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
		bot, err := env.state.GetBot(context.Background(), botID)
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

type authRoundTripper struct {
	apiKey string
	base   http.RoundTripper
}

func newAuthRoundTripper(apiKey string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &authRoundTripper{
		apiKey: apiKey,
		base:   base,
	}
}

func (rt *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.apiKey != "" && req.Header.Get("X-API-Key") == "" {
		req.Header.Set("X-API-Key", rt.apiKey)
	}
	return rt.base.RoundTrip(req)
}

func waitForPortRelease(host string, port int, timeout time.Duration) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("port %s not released within %v", addr, timeout)
}
