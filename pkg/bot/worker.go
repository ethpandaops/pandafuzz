package bot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/config"
	queueasynq "github.com/ethpandaops/pandafuzz/pkg/infrastructure/queue/asynq"
	"github.com/hibiken/asynq"
	"github.com/sirupsen/logrus"
)

// Worker represents an asynq-based bot worker
type Worker struct {
	server         *asynq.Server
	executor       *FuzzerJobExecutor
	client         *RetryClient
	config         *common.BotConfig
	logger         logrus.FieldLogger
	handlers       map[string]asynq.HandlerFunc
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	running        bool
	resultReporter ResultReporter

	// Stats tracking
	jobsHandled int64
	lastJobTime time.Time
}

// ResultReporter handles reporting results back to master
type ResultReporter interface {
	ReportJobCompletion(ctx context.Context, jobID, botID string, success bool, message string) error
	ReportCrash(ctx context.Context, crash *common.CrashResult) error
	ReportCoverage(ctx context.Context, coverage *common.CoverageResult) error
	ReportCorpusUpdate(ctx context.Context, corpus *common.CorpusUpdate) error
}

// WorkerConfig holds worker-specific configuration
type WorkerConfig struct {
	Queues         map[string]int // Queue priorities
	Concurrency    int
	StrictPriority bool
	RetryConfig    config.RetryConfig
	ShutdownWait   time.Duration
}

// NewWorker creates a new asynq worker for bot operations
func NewWorker(botConfig *common.BotConfig, logger logrus.FieldLogger) (*Worker, error) {
	// Create retry client for master communication
	// Type assert logger to *logrus.Logger
	logrusLogger, ok := logger.(*logrus.Logger)
	if !ok {
		// If not a *logrus.Logger, create a new one
		logrusLogger = logrus.New()
		logrusLogger.SetLevel(logrus.GetLevel())
	}

	client, err := NewRetryClient(botConfig, logrusLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create retry client: %w", err)
	}

	// Create fuzzer executor
	executor := NewFuzzerJobExecutor(botConfig, logrusLogger)

	// Create worker
	w := &Worker{
		executor: executor,
		client:   client,
		config:   botConfig,
		logger:   logger.WithField("component", "worker"),
		handlers: make(map[string]asynq.HandlerFunc),
	}

	// Set up result reporter (using the retry client)
	w.resultReporter = &MasterResultReporter{
		client: client,
		botID:  botConfig.ID,
		logger: logger,
	}

	return w, nil
}

// Start starts the worker
func (w *Worker) Start(ctx context.Context, workerCfg WorkerConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("worker already running")
	}

	w.ctx, w.cancel = context.WithCancel(ctx)

	// Create Redis config from bot config or use defaults
	var redisCfg *config.RedisConfig
	if w.config.Redis != nil {
		redisCfg = w.config.Redis
	} else {
		// Default Redis config if not specified in bot config
		redisCfg = &config.RedisConfig{
			Host:         "localhost",
			Port:         6379,
			Password:     "",
			DB:           0,
			PoolSize:     10,
			MinIdleConns: 5,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			IdleTimeout:  5 * time.Minute,
			MaxConnAge:   30 * time.Minute,
		}
		w.logger.Warn("No Redis config provided, using default localhost:6379")
	}

	// Create server config
	serverCfg := asynq.Config{
		Concurrency: workerCfg.Concurrency,
		Queues:      workerCfg.Queues,
		// Enable strict priority if configured
		StrictPriority: workerCfg.StrictPriority,
		// Error handler
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			w.logger.WithError(err).WithFields(logrus.Fields{
				"task_type":    task.Type(),
				"task_payload": string(task.Payload()),
			}).Error("Task processing failed")
		}),
		// Retry configuration
		RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
			return workerCfg.RetryConfig.RetryDelay * time.Duration(n)
		},
		// Logger
		Logger: &asynqLoggerAdapter{logger: w.logger},
	}

	// Create asynq server
	w.server = asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port),
			Password: redisCfg.Password,
			DB:       redisCfg.DB,
		},
		serverCfg,
	)

	// Create mux and register handlers
	mux := asynq.NewServeMux()

	// Register task handlers directly
	mux.HandleFunc(queueasynq.TypeFuzzingJob, w.handleFuzzingJob)

	// Start server in background
	go func() {
		w.logger.Info("Starting asynq worker")
		if err := w.server.Run(mux); err != nil {
			w.logger.WithError(err).Error("Worker server error")
		}
	}()

	w.running = true
	w.logger.WithFields(logrus.Fields{
		"concurrency": workerCfg.Concurrency,
		"queues":      workerCfg.Queues,
		"bot_id":      w.config.ID,
	}).Info("Worker started successfully")

	return nil
}

// Stop stops the worker gracefully
func (w *Worker) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	w.logger.Info("Stopping worker...")

	if w.cancel != nil {
		w.cancel()
	}

	// Shutdown server
	if w.server != nil {
		w.server.Shutdown()
	}

	// Stop executor
	// FuzzerJobExecutor doesn't have Shutdown method, it manages its own resources

	w.running = false
	w.logger.Info("Worker stopped")
	return nil
}

// IsRunning returns whether the worker is running
func (w *Worker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// GetStats returns worker statistics
func (w *Worker) GetStats() WorkerStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return WorkerStats{
		BotID:       w.config.ID,
		Running:     w.running,
		JobsHandled: int(w.jobsHandled),
		LastJobTime: w.lastJobTime,
	}
}

// WorkerStats represents worker statistics
type WorkerStats struct {
	BotID       string    `json:"bot_id"`
	Running     bool      `json:"running"`
	JobsHandled int       `json:"jobs_handled"`
	LastJobTime time.Time `json:"last_job_time"`
}

// MasterResultReporter reports results back to master
type MasterResultReporter struct {
	client *RetryClient
	botID  string
	logger logrus.FieldLogger
}

func (r *MasterResultReporter) ReportJobCompletion(ctx context.Context, jobID, botID string, success bool, message string) error {
	return r.client.CompleteJob(botID, jobID, success, message)
}

func (r *MasterResultReporter) ReportCrash(ctx context.Context, crash *common.CrashResult) error {
	return r.client.ReportCrash(crash)
}

func (r *MasterResultReporter) ReportCoverage(ctx context.Context, coverage *common.CoverageResult) error {
	return r.client.ReportCoverage(coverage)
}

func (r *MasterResultReporter) ReportCoverageData(data map[string]interface{}) error {
	return r.client.ReportCoverageData(data)
}

func (r *MasterResultReporter) ReportCorpusUpdate(ctx context.Context, corpus *common.CorpusUpdate) error {
	return r.client.ReportCorpusUpdate(corpus)
}

// workerResultHandler implements the ResultHandler interface for asynq handlers
type workerResultHandler struct {
	reporter ResultReporter
	logger   logrus.FieldLogger
}

func (h *workerResultHandler) ReportCrash(crash *common.CrashResult) error {
	return h.reporter.ReportCrash(context.Background(), crash)
}

func (h *workerResultHandler) ReportCoverage(coverage *common.CoverageResult) error {
	return h.reporter.ReportCoverage(context.Background(), coverage)
}

func (h *workerResultHandler) ReportCoverageData(data map[string]interface{}) error {
	// Use MasterResultReporter's method directly since it doesn't need context
	if r, ok := h.reporter.(*MasterResultReporter); ok {
		return r.ReportCoverageData(data)
	}
	return nil
}

func (h *workerResultHandler) ReportCorpusUpdate(corpus *common.CorpusUpdate) error {
	return h.reporter.ReportCorpusUpdate(context.Background(), corpus)
}

func (h *workerResultHandler) CompleteJob(botID string, success bool, message string) error {
	// Job ID would need to be tracked separately or passed through context
	// For now, this is handled by the handler itself
	h.logger.WithFields(logrus.Fields{
		"bot_id":  botID,
		"success": success,
		"message": message,
	}).Info("Job completion reported")
	return nil
}

// Handler methods for different task types
func (w *Worker) handleFuzzingJob(ctx context.Context, t *asynq.Task) error {
	// Parse task
	task, err := queueasynq.ParseFuzzingTask(t)
	if err != nil {
		return fmt.Errorf("failed to parse fuzzing task: %w", err)
	}

	w.logger.WithFields(logrus.Fields{
		"job_id": task.JobID,
		"bot_id": task.BotID,
		"fuzzer": task.Fuzzer.Type,
		"target": task.Target.BinaryPath,
	}).Info("Processing fuzzing job")

	// Convert task to common.Job
	job := &common.Job{
		ID:      task.JobID,
		Name:    fmt.Sprintf("Fuzzing job %s", task.JobID),
		Target:  task.Target.BinaryPath,
		Fuzzer:  task.Fuzzer.Type,
		Status:  common.JobStatusRunning,
		WorkDir: fmt.Sprintf("job_%s", task.JobID),
		Config: common.JobConfig{
			Duration:    task.Fuzzer.MaxDuration,
			MemoryLimit: int64(task.Fuzzer.MemoryLimit) * 1024 * 1024,
			Timeout:     time.Duration(task.Fuzzer.Timeout) * time.Second,
		},
		AssignedBot: &task.BotID,
		StartedAt:   &[]time.Time{time.Now()}[0],
	}

	// Execute the fuzzing job using existing executor
	success, message, err := w.executor.ExecuteJob(job)
	if err != nil {
		// Report failure
		if w.resultReporter != nil {
			reportErr := w.resultReporter.ReportJobCompletion(ctx, task.JobID, task.BotID, false, err.Error())
			if reportErr != nil {
				w.logger.WithError(reportErr).Error("Failed to report job failure")
			}
		}
		return fmt.Errorf("fuzzing job failed: %w", err)
	}

	// Report success
	if w.resultReporter != nil {
		reportErr := w.resultReporter.ReportJobCompletion(ctx, task.JobID, task.BotID, success, message)
		if reportErr != nil {
			w.logger.WithError(reportErr).Error("Failed to report job completion")
		}
	}

	// Update stats
	w.mu.Lock()
	w.jobsHandled++
	w.lastJobTime = time.Now()
	w.mu.Unlock()

	return nil
}

// asynqLoggerAdapter adapts logrus logger to asynq logger interface
type asynqLoggerAdapter struct {
	logger logrus.FieldLogger
}

func (l *asynqLoggerAdapter) Debug(args ...interface{}) {
	l.logger.Debug(args...)
}

func (l *asynqLoggerAdapter) Info(args ...interface{}) {
	l.logger.Info(args...)
}

func (l *asynqLoggerAdapter) Warn(args ...interface{}) {
	l.logger.Warn(args...)
}

func (l *asynqLoggerAdapter) Error(args ...interface{}) {
	l.logger.Error(args...)
}

func (l *asynqLoggerAdapter) Fatal(args ...interface{}) {
	l.logger.Fatal(args...)
}
