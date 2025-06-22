package bot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// Agent represents a fuzzing bot agent
type Agent struct {
	config          *common.BotConfig
	client          *RetryClient
	logger          *logrus.Logger
	currentJob      *common.Job
	executor        *RealJobExecutor
	heartbeatTicker *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex
	running         bool
	stats           AgentStats
	lastHeartbeat   time.Time
}

// AgentStats tracks bot agent statistics
type AgentStats struct {
	StartTime        time.Time     `json:"start_time"`
	JobsCompleted    int64         `json:"jobs_completed"`
	JobsFailed       int64         `json:"jobs_failed"`
	CrashesReported  int64         `json:"crashes_reported"`
	CoverageReports  int64         `json:"coverage_reports"`
	CorpusUpdates    int64         `json:"corpus_updates"`
	HeartbeatsSent   int64         `json:"heartbeats_sent"`
	ConnectionErrors int64         `json:"connection_errors"`
	LastJobDuration  time.Duration `json:"last_job_duration"`
	TotalUptime      time.Duration `json:"total_uptime"`
	CurrentStatus    string        `json:"current_status"`
}

// NewAgent creates a new bot agent
func NewAgent(config *common.BotConfig) (*Agent, error) {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	
	// Create retry client for master communication
	client, err := NewRetryClient(config)
	if err != nil {
		return nil, common.NewSystemError("create_retry_client", err)
	}
	
	// Create job executor with real implementation
	executor := NewRealJobExecutor(config, logger)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Agent{
		config:   config,
		client:   client,
		logger:   logger,
		executor: executor,
		ctx:      ctx,
		cancel:   cancel,
		stats: AgentStats{
			StartTime:     time.Now(),
			CurrentStatus: "initialized",
		},
	}, nil
}

// Start starts the bot agent
func (a *Agent) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if a.running {
		return common.NewSystemError("start_agent", fmt.Errorf("agent already running"))
	}
	
	a.logger.WithFields(logrus.Fields{
		"bot_id":     a.config.ID,
		"master_url": a.config.MasterURL,
	}).Info("Starting bot agent")
	
	// Register with master
	if err := a.registerWithMaster(); err != nil {
		return common.NewSystemError("register_with_master", err)
	}
	
	// Start heartbeat
	a.startHeartbeat()
	
	// Start main loop
	a.wg.Add(1)
	go a.run()
	
	// Setup signal handling
	a.setupSignalHandling()
	
	a.running = true
	a.stats.CurrentStatus = "running"
	
	a.logger.Info("Bot agent started successfully")
	return nil
}

// Stop gracefully stops the bot agent
func (a *Agent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	if !a.running {
		return nil
	}
	
	a.logger.Info("Stopping bot agent")
	
	// Cancel context to stop all goroutines
	a.cancel()
	
	// Stop heartbeat
	if a.heartbeatTicker != nil {
		a.heartbeatTicker.Stop()
	}
	
	// Complete current job if any
	if a.currentJob != nil {
		a.logger.WithField("job_id", a.currentJob.ID).Info("Completing current job before shutdown")
		a.completeCurrentJob(false, "Agent shutdown")
	}
	
	// Deregister from master
	if err := a.deregisterFromMaster(); err != nil {
		a.logger.WithError(err).Warn("Failed to deregister from master")
	}
	
	// Wait for goroutines to finish
	a.wg.Wait()
	
	a.running = false
	a.stats.CurrentStatus = "stopped"
	
	a.logger.Info("Bot agent stopped")
	return nil
}

// run is the main agent loop
func (a *Agent) run() {
	defer a.wg.Done()
	
	ticker := time.NewTicker(10 * time.Second) // Check for jobs every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.processWorkCycle()
		}
	}
}

// processWorkCycle handles one work cycle
func (a *Agent) processWorkCycle() {
	a.mu.RLock()
	hasJob := a.currentJob != nil
	a.mu.RUnlock()
	
	if hasJob {
		// Continue working on current job
		a.continueCurrentJob()
	} else {
		// Try to get a new job
		a.requestNewJob()
	}
}

// registerWithMaster registers the bot with the master
func (a *Agent) registerWithMaster() error {
	a.logger.Info("Registering with master")
	
	response, err := a.client.RegisterBot(a.config.ID, a.config.Capabilities)
	if err != nil {
		a.stats.ConnectionErrors++
		return err
	}
	
	a.logger.WithFields(logrus.Fields{
		"bot_id":    response.BotID,
		"status":    response.Status,
		"timestamp": response.Timestamp,
	}).Info("Successfully registered with master")
	
	// Update the bot's ID to use the master-assigned ID
	a.config.ID = response.BotID
	
	return nil
}

// deregisterFromMaster deregisters the bot from the master
func (a *Agent) deregisterFromMaster() error {
	a.logger.Info("Deregistering from master")
	
	return a.client.DeregisterBot(a.config.ID)
}

// startHeartbeat starts the heartbeat routine
func (a *Agent) startHeartbeat() {
	interval := a.config.Timeouts.HeartbeatInterval
	if interval == 0 {
		interval = 30 * time.Second
	}
	
	a.heartbeatTicker = time.NewTicker(interval)
	
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.heartbeatTicker.C:
				a.sendHeartbeat()
			}
		}
	}()
}

// sendHeartbeat sends a heartbeat to the master
func (a *Agent) sendHeartbeat() {
	a.mu.RLock()
	var currentJobID *string
	var status common.BotStatus = common.BotStatusIdle
	
	if a.currentJob != nil {
		currentJobID = &a.currentJob.ID
		status = common.BotStatusBusy
	}
	a.mu.RUnlock()
	
	err := a.client.SendHeartbeat(a.config.ID, status, currentJobID)
	if err != nil {
		a.logger.WithError(err).Error("Failed to send heartbeat")
		a.stats.ConnectionErrors++
	} else {
		a.stats.HeartbeatsSent++
		a.lastHeartbeat = time.Now()
		a.logger.Debug("Heartbeat sent successfully")
	}
}

// requestNewJob requests a new job from the master
func (a *Agent) requestNewJob() {
	a.logger.Debug("Requesting new job from master")
	
	job, err := a.client.GetJob(a.config.ID)
	if err != nil {
		a.logger.WithError(err).Error("Failed to get job from master")
		a.stats.ConnectionErrors++
		return
	}
	
	if job == nil {
		a.logger.Debug("No jobs available")
		return
	}
	
	a.mu.Lock()
	a.currentJob = job
	a.mu.Unlock()
	
	a.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
		"target":   job.Target,
	}).Info("Received new job")
	
	// Prepare job for execution (download binary if needed)
	go a.prepareAndExecuteJob(job)
}

// prepareAndExecuteJob prepares and executes a fuzzing job
func (a *Agent) prepareAndExecuteJob(job *common.Job) {
	a.logger.WithField("job_id", job.ID).Info("Preparing job for execution")
	
	// Always download binary from master since the path refers to the master's filesystem
	localBinaryPath := filepath.Join(job.WorkDir, "target_binary")
	a.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"remote_path": job.Target,
		"local_path": localBinaryPath,
	}).Info("Downloading binary from master")
	
	if err := a.client.DownloadJobBinary(job.ID, a.config.ID, localBinaryPath); err != nil {
		a.logger.WithError(err).Error("Failed to download binary")
		a.completeCurrentJob(false, fmt.Sprintf("Failed to download binary: %v", err))
		return
	}
	
	// Update job target to local path
	job.Target = localBinaryPath
	
	// Try to download seed corpus (if available)
	corpusPath := filepath.Join(job.WorkDir, "seed_corpus.zip")
	a.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"local_path": corpusPath,
	}).Info("Checking for seed corpus from master")
	
	if err := a.client.DownloadJobCorpus(job.ID, a.config.ID, corpusPath); err != nil {
		// Corpus download failure is not fatal
		a.logger.WithError(err).Debug("No seed corpus available or failed to download, continuing without it")
	} else {
		// Extract corpus
		inputDir := filepath.Join(job.WorkDir, "input")
		if err := os.MkdirAll(inputDir, 0755); err != nil {
			a.logger.WithError(err).Warn("Failed to create input directory")
		}
		// TODO: Extract zip file to input directory
		a.logger.Info("Seed corpus downloaded successfully")
	}
	
	// Execute the job
	a.executeJob(job)
}

// executeJob executes a fuzzing job
func (a *Agent) executeJob(job *common.Job) {
	startTime := time.Now()
	a.stats.CurrentStatus = "executing_job"
	
	a.logger.WithField("job_id", job.ID).Info("Starting job execution")
	
	// Execute the job
	success, message, err := a.executor.ExecuteJob(job)
	
	duration := time.Since(startTime)
	a.stats.LastJobDuration = duration
	
	if err != nil {
		a.logger.WithError(err).WithField("job_id", job.ID).Error("Job execution failed")
		a.stats.JobsFailed++
		a.completeCurrentJob(false, fmt.Sprintf("Execution failed: %v", err))
	} else if success {
		a.logger.WithFields(logrus.Fields{
			"job_id":   job.ID,
			"duration": duration,
			"message":  message,
		}).Info("Job completed successfully")
		a.stats.JobsCompleted++
		a.completeCurrentJob(true, message)
	} else {
		a.logger.WithFields(logrus.Fields{
			"job_id":   job.ID,
			"duration": duration,
			"message":  message,
		}).Warn("Job completed with issues")
		a.stats.JobsFailed++
		a.completeCurrentJob(false, message)
	}
}

// continueCurrentJob continues working on the current job
func (a *Agent) continueCurrentJob() {
	a.mu.RLock()
	job := a.currentJob
	a.mu.RUnlock()
	
	if job == nil {
		return
	}
	
	// Check if job has timed out
	if time.Now().After(job.TimeoutAt) {
		a.logger.WithField("job_id", job.ID).Warn("Job has timed out")
		a.completeCurrentJob(false, "Job timeout")
		return
	}
	
	// Continue monitoring the job
	a.logger.WithField("job_id", job.ID).Debug("Continuing job execution")
}

// completeCurrentJob completes the current job
func (a *Agent) completeCurrentJob(success bool, message string) {
	a.mu.Lock()
	job := a.currentJob
	a.currentJob = nil
	a.mu.Unlock()
	
	if job == nil {
		return
	}
	
	a.logger.WithFields(logrus.Fields{
		"job_id":  job.ID,
		"success": success,
		"message": message,
	}).Info("Completing job")
	
	// Push logs to master before completing the job
	logPath := filepath.Join(job.WorkDir, "job.log")
	if _, err := os.Stat(logPath); err == nil {
		// Log file exists, push it to master
		a.logger.WithField("job_id", job.ID).Info("Pushing job logs to master")
		if err := a.client.PushJobLogs(job.ID, a.config.ID, logPath); err != nil {
			a.logger.WithError(err).Error("Failed to push job logs to master")
			// Don't fail job completion if log push fails
		}
	} else {
		a.logger.WithField("job_id", job.ID).Warn("No log file found to push")
	}
	
	// Notify master of job completion
	err := a.client.CompleteJob(a.config.ID, success, message)
	if err != nil {
		a.logger.WithError(err).Error("Failed to notify master of job completion")
		a.stats.ConnectionErrors++
	}
	
	// Stop job execution
	a.executor.StopJob(job.ID)
	
	a.stats.CurrentStatus = "idle"
}

// ReportCrash reports a crash to the master
func (a *Agent) ReportCrash(crash *common.CrashResult) error {
	a.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"job_id":   crash.JobID,
		"hash":     crash.Hash,
		"type":     crash.Type,
	}).Info("Reporting crash")
	
	err := a.client.ReportCrash(crash)
	if err != nil {
		a.stats.ConnectionErrors++
		return err
	}
	
	a.stats.CrashesReported++
	return nil
}

// ReportCoverage reports coverage to the master
func (a *Agent) ReportCoverage(coverage *common.CoverageResult) error {
	a.logger.WithFields(logrus.Fields{
		"coverage_id": coverage.ID,
		"job_id":      coverage.JobID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
	}).Debug("Reporting coverage")
	
	err := a.client.ReportCoverage(coverage)
	if err != nil {
		a.stats.ConnectionErrors++
		return err
	}
	
	a.stats.CoverageReports++
	return nil
}

// ReportCorpusUpdate reports corpus update to the master
func (a *Agent) ReportCorpusUpdate(corpus *common.CorpusUpdate) error {
	a.logger.WithFields(logrus.Fields{
		"corpus_id":  corpus.ID,
		"job_id":     corpus.JobID,
		"file_count": len(corpus.Files),
		"total_size": corpus.TotalSize,
	}).Debug("Reporting corpus update")
	
	err := a.client.ReportCorpusUpdate(corpus)
	if err != nil {
		a.stats.ConnectionErrors++
		return err
	}
	
	a.stats.CorpusUpdates++
	return nil
}

// setupSignalHandling sets up graceful shutdown on signals
func (a *Agent) setupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		
		select {
		case <-c:
			a.logger.Info("Received shutdown signal")
			a.Stop()
		case <-a.ctx.Done():
			return
		}
	}()
}

// GetStats returns agent statistics
func (a *Agent) GetStats() AgentStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	stats := a.stats
	stats.TotalUptime = time.Since(a.stats.StartTime)
	
	return stats
}

// GetCurrentJob returns the current job
func (a *Agent) GetCurrentJob() *common.Job {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	return a.currentJob
}

// IsRunning returns whether the agent is running
func (a *Agent) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	return a.running
}

// GetLastHeartbeat returns the last heartbeat time
func (a *Agent) GetLastHeartbeat() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	return a.lastHeartbeat
}

// HealthCheck performs a health check
func (a *Agent) HealthCheck() error {
	// Check if agent is running
	if !a.IsRunning() {
		return common.NewSystemError("health_check", fmt.Errorf("agent not running"))
	}
	
	// Check last heartbeat
	if time.Since(a.lastHeartbeat) > 2*a.config.Timeouts.HeartbeatInterval {
		return common.NewSystemError("health_check", fmt.Errorf("heartbeat timeout"))
	}
	
	// Check connection to master
	if err := a.client.Ping(); err != nil {
		return common.NewSystemError("health_check", fmt.Errorf("master connection failed: %v", err))
	}
	
	return nil
}

// SetLogLevel sets the logging level
func (a *Agent) SetLogLevel(level logrus.Level) {
	a.logger.SetLevel(level)
}

// GetConfig returns the agent configuration
func (a *Agent) GetConfig() *common.BotConfig {
	return a.config
}