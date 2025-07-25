package bot

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/sirupsen/logrus"
)

// Agent represents a fuzzing bot agent
type Agent struct {
	config          *common.BotConfig
	client          *RetryClient
	logger          *logrus.Logger
	currentJob      *common.Job
	executor        *FuzzerJobExecutor
	heartbeatTicker *time.Ticker
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex
	running         bool
	stats           AgentStats
	lastHeartbeat   time.Time

	// API server for master polling
	apiServer         *APIServer
	ID                string
	jobStartTime      time.Time
	startTime         time.Time
	version           string
	jobsCompleted     int64
	jobsFailed        int64
	totalCrashes      int64
	currentJobCrashes int

	// Resource monitoring and cleanup
	resourceMonitor *SystemResourceMonitor
	cleanupManager  *JobCleanupManager
	resultCollector *ResultCollector

	// Reproducibility executor for crash reproduction
	reproExecutor *ReproducibilityExecutor

	// Minimizer client for crash minimization
	minimizerClient *MinimizerClient

	// Worker mode fields
	workerMode bool
	worker     *Worker
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
func NewAgent(config *common.BotConfig, logger *logrus.Logger) (*Agent, error) {
	// Create retry client for master communication
	client, err := NewRetryClient(config, logger)
	if err != nil {
		return nil, common.NewSystemError("create_retry_client", err)
	}

	// Create job executor with fuzzer implementation
	executor := NewFuzzerJobExecutor(config, logger)

	// Create resource monitor
	resourceMonitor := NewResourceMonitor(config, logger)

	// Create cleanup manager
	cleanupManager := NewCleanupManager(config, logger)

	// Create result collector
	resultCollector, err := NewResultCollector(config, config.MasterURL, logger)
	if err != nil {
		return nil, common.NewSystemError("create_result_collector", err)
	}

	// Create reproducibility executor
	reproExecutor := NewReproducibilityExecutor(client, config, logger)

	// Create minimizer client
	minimizerClient, err := NewMinimizerClient(config, logger)
	if err != nil {
		return nil, common.NewSystemError("create_minimizer_client", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Agent{
		config:          config,
		client:          client,
		logger:          logger,
		executor:        executor,
		ctx:             ctx,
		cancel:          cancel,
		resourceMonitor: resourceMonitor,
		cleanupManager:  cleanupManager,
		resultCollector: resultCollector,
		reproExecutor:   reproExecutor,
		minimizerClient: minimizerClient,
		stats: AgentStats{
			StartTime:     time.Now(),
			CurrentStatus: "initialized",
		},
		ID:         config.ID,
		startTime:  time.Now(),
		version:    "1.0.0", // TODO: Get from build info
		workerMode: false,   // Default to polling mode
	}, nil
}

// Start starts the bot agent
func (a *Agent) Start() error {
	return a.StartWithMode(false) // Default to polling mode
}

// StartWithMode starts the bot agent in specified mode
func (a *Agent) StartWithMode(workerMode bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return common.NewSystemError("start_agent", fmt.Errorf("agent already running"))
	}

	a.workerMode = workerMode

	a.logger.WithFields(logrus.Fields{
		"bot_id":      a.config.ID,
		"master_url":  a.config.MasterURL,
		"worker_mode": workerMode,
	}).Info("Starting bot agent")

	// Use configured API port or default to 9049
	apiPort := a.config.APIPort
	if apiPort == 0 {
		apiPort = 9049 // Default port
	}

	a.logger.WithFields(logrus.Fields{
		"bot_id":   a.config.ID,
		"api_port": apiPort,
	}).Info("Using API port for bot")

	// Start API server before registration so it's ready for polling
	a.apiServer = NewAPIServer(a, apiPort, a.logger)
	if err := a.apiServer.Start(); err != nil {
		return common.NewSystemError("start_api_server", err)
	}
	a.logger.WithField("api_port", apiPort).Info("Started bot API server")

	// Register with master (this will use the same apiPort)
	if err := a.registerWithMaster(); err != nil {
		// Stop API server if registration fails
		// Note: We don't have a Stop method, so just log the error
		return common.NewSystemError("register_with_master", err)
	}

	// Start heartbeat
	a.startHeartbeat()

	// Start resource monitoring
	monitorInterval := 30 * time.Second // Monitor every 30 seconds
	if err := a.resourceMonitor.StartMonitoring(a.ctx, monitorInterval); err != nil {
		a.logger.WithError(err).Error("Failed to start resource monitoring")
		// Non-fatal, continue
	} else {
		// Monitor resource alerts
		a.wg.Add(1)
		go a.monitorResourceAlerts()
	}

	// Start cleanup scheduler
	if err := a.cleanupManager.ScheduleCleanup(); err != nil {
		a.logger.WithError(err).Error("Failed to schedule cleanup")
		// Non-fatal, continue
	}

	// Start result collector
	if err := a.resultCollector.Start(a.ctx); err != nil {
		return common.NewSystemError("start_result_collector", err)
	}

	// Connect result collector to executor event channel
	a.wg.Add(1)
	go a.connectResultCollectorToExecutor()

	// Start appropriate mode
	if a.workerMode {
		// Worker mode: Start asynq worker
		a.logger.Info("Starting in worker mode")

		// Create worker if not already created
		if a.worker == nil {
			worker, err := NewWorker(a.config, a.logger)
			if err != nil {
				return common.NewSystemError("create_worker", err)
			}
			a.worker = worker
		}

		// Configure worker
		workerCfg := WorkerConfig{
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			Concurrency:    1, // One job at a time per bot
			StrictPriority: true,
			RetryConfig: config.RetryConfig{
				MaxRetries:         3,
				RetryDelay:         30 * time.Second,
				ExponentialBackoff: true,
			},
			ShutdownWait: 30 * time.Second,
		}

		// Start worker
		if err := a.worker.Start(a.ctx, workerCfg); err != nil {
			return common.NewSystemError("start_worker", err)
		}
	} else {
		// Polling mode: Start traditional polling loop
		a.logger.Info("Starting in polling mode")
		a.wg.Add(1)
		go a.run()
	}

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

	// Stop any active reproductions
	if a.reproExecutor != nil {
		activeRepros := a.reproExecutor.GetActiveReproductions()
		if len(activeRepros) > 0 {
			a.logger.WithField("count", len(activeRepros)).Info("Stopping active reproductions")
			for requestID := range activeRepros {
				a.reproExecutor.StopReproduction(requestID)
			}
			// Give some time for reproductions to finish gracefully
			time.Sleep(2 * time.Second)
		}
	}

	// Stop result collector
	if a.resultCollector != nil {
		if err := a.resultCollector.Stop(); err != nil {
			a.logger.WithError(err).Warn("Failed to stop result collector")
		}
	}

	// Stop resource monitor
	if a.resourceMonitor != nil && a.resourceMonitor.IsMonitoring() {
		if err := a.resourceMonitor.Stop(); err != nil {
			a.logger.WithError(err).Warn("Failed to stop resource monitor")
		}
	}

	// Stop cleanup manager
	if a.cleanupManager != nil {
		a.cleanupManager.Stop()
	}

	// Stop worker if in worker mode
	if a.workerMode && a.worker != nil {
		if err := a.worker.Stop(); err != nil {
			a.logger.WithError(err).Warn("Failed to stop worker cleanly")
		}
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

// IsWorkerMode returns true if the agent is running in worker mode
func (a *Agent) IsWorkerMode() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workerMode
}

// SetWorkerMode sets the worker mode (must be called before Start)
func (a *Agent) SetWorkerMode(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("cannot change mode while agent is running")
	}

	a.workerMode = enabled
	return nil
}

// run is the main agent loop (polling mode only)
func (a *Agent) run() {
	defer a.wg.Done()

	// Skip if in worker mode
	if a.workerMode {
		a.logger.Debug("Skipping polling loop in worker mode")
		return
	}

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

	// First check if we have any pending acknowledgments to retry
	a.retryPendingAcknowledgments()

	if hasJob {
		// Continue working on current job
		a.continueCurrentJob()
	} else {
		// Check for reproduction requests first (higher priority)
		reproRequest, err := a.client.GetReproductionRequest(a.config.ID)
		if err != nil {
			a.logger.WithError(err).Debug("Failed to check for reproduction requests")
		} else if reproRequest != nil {
			// Handle reproduction request
			a.logger.WithFields(logrus.Fields{
				"request_id": reproRequest.ID,
				"crash_id":   reproRequest.CrashID,
				"priority":   reproRequest.Priority,
			}).Info("Processing reproduction request")

			// Execute reproduction in background
			go func() {
				if err := a.reproExecutor.HandleReproductionRequest(reproRequest); err != nil {
					a.logger.WithError(err).Error("Failed to handle reproduction request")
				}
			}()

			// Don't request a regular job if we're handling a reproduction
			return
		}

		// No reproduction requests, try to get a regular job
		a.requestNewJob()
	}
}

// registerWithMaster registers the bot with the master
func (a *Agent) registerWithMaster() error {
	a.logger.Info("Registering with master")

	// Get the API port from the running server
	apiPort := a.apiServer.port

	// In Docker, use the container's hostname which is accessible within the Docker network
	hostname, _ := os.Hostname()
	var apiEndpoint string

	// Check if we're running in Docker by looking for common Docker environment variables
	if _, inDocker := os.LookupEnv("HOSTNAME"); inDocker {
		// In Docker, the hostname is the container ID which is accessible within the network
		apiEndpoint = fmt.Sprintf("http://%s:%d", hostname, apiPort)
		a.logger.WithFields(logrus.Fields{
			"hostname": hostname,
			"port":     apiPort,
			"endpoint": apiEndpoint,
		}).Info("Bot API endpoint (Docker)")
	} else {
		// For non-Docker environments, use localhost
		apiEndpoint = fmt.Sprintf("http://localhost:%d", apiPort)
	}

	response, err := a.client.RegisterBot(a.config.ID, a.config.Capabilities, apiEndpoint)
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

	if job == nil || job.ID == "" {
		a.logger.Debug("No jobs available")
		return
	}

	a.mu.Lock()
	a.currentJob = job
	a.mu.Unlock()

	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"job_name":   job.Name,
		"fuzzer":     job.Fuzzer,
		"target":     job.Target,
		"job_status": job.Status,
		"work_dir":   job.WorkDir,
		"timeout_at": job.TimeoutAt,
	}).Info("Bot received new job from master")

	// Update API server immediately to reflect we have the job
	if a.apiServer != nil {
		status := &JobStatus{
			JobID:     job.ID,
			Status:    "preparing",
			StartTime: time.Now(),
			Message:   "Job received, preparing environment",
			UpdatedAt: time.Now(),
		}
		a.apiServer.UpdateJobStatus(job.ID, status)
	}

	// Prepare job for execution (download binary if needed)
	go a.prepareAndExecuteJob(job)
}

// prepareAndExecuteJob prepares and executes a fuzzing job
func (a *Agent) prepareAndExecuteJob(job *common.Job) {
	a.logger.WithField("job_id", job.ID).Info("Preparing job for execution")

	// Resolve work directory - if it's a relative path, prepend the bot's work directory
	if !filepath.IsAbs(job.WorkDir) {
		// Use bot's configured work directory as base
		baseWorkDir := a.config.Fuzzing.WorkDir
		if baseWorkDir == "" {
			baseWorkDir = "./work"
		}
		job.WorkDir = filepath.Join(baseWorkDir, "jobs", job.WorkDir)
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":        job.ID,
		"work_dir":      job.WorkDir,
		"resolved_path": job.WorkDir,
	}).Info("Resolved job work directory")

	// Create work directory first
	if err := os.MkdirAll(job.WorkDir, 0755); err != nil {
		a.logger.WithError(err).WithField("work_dir", job.WorkDir).Error("Failed to create work directory")
		a.completeCurrentJob(false, fmt.Sprintf("Failed to create work directory: %v", err))
		return
	}

	// Always download binary from master since the path refers to the master's filesystem
	localBinaryPath := filepath.Join(job.WorkDir, "target_binary")

	// Log job details for debugging
	a.logger.WithFields(logrus.Fields{
		"job_id":            job.ID,
		"job_status":        job.Status,
		"job_fuzzer":        job.Fuzzer,
		"work_dir":          job.WorkDir,
		"abs_work_dir":      func() string { p, _ := filepath.Abs(job.WorkDir); return p }(),
		"target_path":       job.Target,
		"local_binary_path": localBinaryPath,
	}).Info("Starting job preparation")

	// Remove any existing file to avoid confusion
	if _, err := os.Stat(localBinaryPath); err == nil {
		a.logger.WithField("path", localBinaryPath).Warn("Removing existing target_binary before download")
		os.Remove(localBinaryPath)
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"remote_path": job.Target,
		"local_path":  localBinaryPath,
	}).Info("Downloading binary from master")

	if err := a.client.DownloadJobBinary(job.ID, a.config.ID, localBinaryPath); err != nil {
		a.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":      job.ID,
			"bot_id":      a.config.ID,
			"target_path": localBinaryPath,
		}).Error("Failed to download binary")
		a.completeCurrentJob(false, fmt.Sprintf("Failed to download binary: %v", err))
		return
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"local_path": localBinaryPath,
	}).Info("Binary download completed, verifying file existence")

	// Verify binary was actually downloaded
	stat, err := os.Stat(localBinaryPath)
	if os.IsNotExist(err) {
		a.logger.WithFields(logrus.Fields{
			"job_id":        job.ID,
			"expected_path": localBinaryPath,
		}).Error("Binary download succeeded but file does not exist")
		a.completeCurrentJob(false, "Binary download verification failed: file not found")
		return
	} else if err != nil {
		a.logger.WithError(err).WithField("path", localBinaryPath).Error("Failed to stat binary file")
		a.completeCurrentJob(false, fmt.Sprintf("Failed to stat binary: %v", err))
		return
	}

	// Check file details
	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"path":       localBinaryPath,
		"size":       stat.Size(),
		"mode":       stat.Mode().String(),
		"is_regular": stat.Mode().IsRegular(),
	}).Info("Binary file details")

	if stat.Size() == 0 {
		a.logger.Error("Downloaded binary is empty (0 bytes)")
		a.completeCurrentJob(false, "Downloaded binary is empty")
		return
	}

	// fileInfo is already set from stat above
	fileInfo := stat

	// Check if file has execute permissions
	if fileInfo.Mode().Perm()&0111 == 0 {
		a.logger.WithFields(logrus.Fields{
			"job_id": job.ID,
			"path":   localBinaryPath,
			"mode":   fileInfo.Mode(),
		}).Warn("Downloaded binary is not executable, attempting to fix permissions")

		// Try to make it executable
		if err := os.Chmod(localBinaryPath, 0755); err != nil {
			a.logger.WithError(err).Error("Failed to make binary executable")
			a.completeCurrentJob(false, fmt.Sprintf("Failed to make binary executable: %v", err))
			return
		}
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"local_path": localBinaryPath,
		"size":       fileInfo.Size(),
		"mode":       fileInfo.Mode(),
	}).Info("Binary download verified successfully")

	// Update job target to local path
	job.Target = localBinaryPath

	// Create input directory for corpus
	inputDir := filepath.Join(job.WorkDir, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		a.logger.WithError(err).Error("Failed to create input directory")
		a.completeCurrentJob(false, fmt.Sprintf("Failed to create input directory: %v", err))
		return
	}

	// Check if job should use corpus collection
	if job.CollectionID != nil && *job.CollectionID != "" {
		a.logger.WithFields(logrus.Fields{
			"job_id":        job.ID,
			"collection_id": *job.CollectionID,
		}).Info("Job configured to use corpus collection")

		// Download corpus collection files
		if err := a.downloadCorpusCollection(*job.CollectionID, inputDir); err != nil {
			a.logger.WithError(err).Error("Failed to download corpus collection")
			// Continue anyway - corpus initialization failure is not fatal
		}
	} else if job.UseCampaignCorpus && job.CampaignID != nil && *job.CampaignID != "" {
		// Check if job should use campaign corpus
		a.logger.WithFields(logrus.Fields{
			"job_id":      job.ID,
			"campaign_id": *job.CampaignID,
		}).Info("Job configured to use campaign corpus")

		// Create a temporary corpus syncer for this job initialization
		corpusSyncer := NewCorpusSyncer(
			a.client,
			*job.CampaignID,
			a.config.ID,
			inputDir, // Use input directory as sync directory
			a.logger,
		)

		// Initialize job with campaign corpus
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if err := corpusSyncer.InitializeJobCorpus(ctx, job, inputDir); err != nil {
			a.logger.WithError(err).Error("Failed to initialize job with campaign corpus")
			// Continue anyway - corpus initialization failure is not fatal
		}
	}

	// Try to download job-specific seed corpus (if available)
	corpusPath := filepath.Join(job.WorkDir, "seed_corpus.zip")
	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"local_path": corpusPath,
	}).Info("Checking for job-specific seed corpus from master")

	if err := a.client.DownloadJobCorpus(job.ID, a.config.ID, corpusPath); err != nil {
		// Corpus download failure is not fatal
		a.logger.WithError(err).Debug("No job-specific seed corpus available or failed to download, continuing without it")
	} else {
		// Check if the corpus file exists before trying to extract
		if _, err := os.Stat(corpusPath); err == nil {
			// Extract zip file to input directory (will merge with campaign corpus if present)
			if err := a.extractZipFile(corpusPath, inputDir); err != nil {
				a.logger.WithError(err).Warn("Failed to extract seed corpus")
			} else {
				a.logger.WithField("input_dir", inputDir).Info("Job-specific seed corpus extracted successfully")
			}
		} else {
			a.logger.Debug("Seed corpus file does not exist, skipping extraction")
		}
	}

	// Execute the job
	a.executeJob(job)
}

// executeJob executes a fuzzing job
func (a *Agent) executeJob(job *common.Job) {
	startTime := time.Now()
	a.jobStartTime = startTime
	a.currentJobCrashes = 0
	a.stats.CurrentStatus = "executing_job"

	a.logger.WithField("job_id", job.ID).Info("Starting job execution")

	var success bool
	var message string
	var err error

	// Handle different job types
	switch job.Type {
	case common.JobTypeMinimization:
		// Handle minimization job
		success, message, err = a.executeMinimizationJob(job)
	case common.JobTypeReproduction:
		// Handle reproduction job
		success, message, err = a.executeReproductionJob(job)
	default:
		// Default to fuzzing job
		success, message, err = a.executor.ExecuteJob(job)
	}

	duration := time.Since(startTime)
	a.stats.LastJobDuration = duration

	if err != nil {
		a.logger.WithError(err).WithField("job_id", job.ID).Error("Job execution failed")
		a.stats.JobsFailed++

		// Check for crashes even when job fails - crashes might still have been found
		crashesFound := a.checkAndReportCrashes(job)
		if crashesFound > 0 {
			a.logger.WithFields(logrus.Fields{
				"job_id":  job.ID,
				"crashes": crashesFound,
			}).Info("Crashes found and reported despite job failure")
			// Update the error message to include crash count
			err = fmt.Errorf("%v (but found %d crashes)", err, crashesFound)
		}

		// Clean up the fuzzer instance even on error
		if cleanupErr := a.executor.CleanupJob(job.ID); cleanupErr != nil {
			a.logger.WithError(cleanupErr).WithField("job_id", job.ID).Warn("Failed to cleanup job after error")
		}

		a.completeCurrentJob(false, fmt.Sprintf("Execution failed: %v", err))
	} else if success {
		a.logger.WithFields(logrus.Fields{
			"job_id":   job.ID,
			"duration": duration,
			"message":  message,
		}).Info("Job completed successfully")

		// Check for crashes BEFORE completing the job
		// This ensures crashes are reported while job is still active
		crashesFound := a.checkAndReportCrashes(job)
		if crashesFound > 0 {
			a.logger.WithFields(logrus.Fields{
				"job_id":  job.ID,
				"crashes": crashesFound,
			}).Info("Crashes found and reported")
			message = fmt.Sprintf("%s (found %d crashes)", message, crashesFound)

			// Give some time for crash reports to be processed
			time.Sleep(1 * time.Second)
		}

		// Clean up the fuzzer instance after crash detection
		if err := a.executor.CleanupJob(job.ID); err != nil {
			a.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to cleanup job")
		}

		a.stats.JobsCompleted++
		a.completeCurrentJob(true, message)
	} else {
		a.logger.WithFields(logrus.Fields{
			"job_id":   job.ID,
			"duration": duration,
			"message":  message,
		}).Warn("Job completed with issues")
		a.stats.JobsFailed++

		// Clean up the fuzzer instance even on failure
		if err := a.executor.CleanupJob(job.ID); err != nil {
			a.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to cleanup job")
		}

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

		// Check for crashes even when job times out
		crashesFound := a.checkAndReportCrashes(job)
		if crashesFound > 0 {
			a.logger.WithFields(logrus.Fields{
				"job_id":  job.ID,
				"crashes": crashesFound,
			}).Info("Crashes found and reported despite job timeout")
			a.completeCurrentJob(false, fmt.Sprintf("Job timeout (but found %d crashes)", crashesFound))
		} else {
			a.completeCurrentJob(false, "Job timeout")
		}
		return
	}

	// Check if job has been cancelled by master
	// Query master for job status
	masterJob, err := a.client.GetJob(a.config.ID)
	if err != nil {
		a.logger.WithError(err).Warn("Failed to check job status with master")
		// Continue with job execution on error
	} else if masterJob == nil || masterJob.ID != job.ID {
		// Master has no job for us or a different job
		a.logger.WithField("job_id", job.ID).Warn("Job no longer assigned by master")
		a.completeCurrentJob(false, "Job cancelled or reassigned")
		return
	} else if masterJob.Status == common.JobStatusCancelled {
		// Job has been explicitly cancelled
		a.logger.WithField("job_id", job.ID).Info("Job has been cancelled by master")
		a.completeCurrentJob(false, "Job cancelled by master")
		return
	}

	// Continue monitoring the job
	a.logger.WithField("job_id", job.ID).Debug("Continuing job execution")
}

// completeCurrentJob completes the current job
func (a *Agent) completeCurrentJob(success bool, message string) {
	a.mu.Lock()
	job := a.currentJob
	// Don't clear currentJob yet - we need it for the API server
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

	// Update API server cache with job status
	if a.apiServer != nil {
		output := fmt.Sprintf("Job completed: %s", message)
		logPath := filepath.Join(job.WorkDir, "job.log")
		if _, err := os.Stat(logPath); err == nil {
			// Try to read last few lines of log
			// TODO: Implement tail functionality
			output = fmt.Sprintf("%s\nLog: %s", output, logPath)
		}
		a.apiServer.MarkJobCompleted(job.ID, success, message, output)
	}

	// Try to notify master of job completion with acknowledgment
	err := a.client.CompleteJob(a.config.ID, success, message)
	if err != nil {
		a.logger.WithError(err).Error("Failed to complete job - master did not acknowledge")
		a.stats.ConnectionErrors++

		// Keep the job status as "pending completion" in our cache
		// The master's poller will eventually pick this up
		if a.apiServer != nil {
			a.apiServer.MarkJobPendingCompletion(job.ID, success, message, "")
		}

		// Don't clear the current job yet - wait for master acknowledgment via polling
		return
	}

	// Master acknowledged - now we can safely log success
	a.logger.WithField("job_id", job.ID).Info("Job completion acknowledged by master")

	// Update stats
	if success {
		a.jobsCompleted++
	} else {
		a.jobsFailed++
	}

	// Stop job execution
	a.executor.StopJob(job.ID)

	// Now clear the current job
	a.mu.Lock()
	a.currentJob = nil
	a.mu.Unlock()

	a.stats.CurrentStatus = "idle"
}

// checkAndReportCrashes checks for crash files and reports them to the master
func (a *Agent) checkAndReportCrashes(job *common.Job) int {
	crashCount := 0

	a.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"work_dir": job.WorkDir,
	}).Info("Scanning for crash files in job directory")

	// First, try to get crashes from the fuzzer instance if it's still active
	if fuzz, exists := a.executor.GetFuzzer(job.ID); exists {
		a.logger.WithField("job_id", job.ID).Info("Using fuzzer instance to get crashes")
		crashes, err := fuzz.GetCrashes()
		if err != nil {
			a.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to get crashes from fuzzer")
		} else {
			a.logger.WithFields(logrus.Fields{
				"job_id":                  job.ID,
				"crashes_found_by_fuzzer": len(crashes),
			}).Info("Fuzzer reported crashes")
			for _, crash := range crashes {
				// Update crash with job and bot information
				crash.JobID = job.ID
				crash.BotID = a.config.ID

				a.logger.WithFields(logrus.Fields{
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
					"bot_id":     crash.BotID,
					"hash":       crash.Hash,
					"size":       crash.Size,
					"file_path":  crash.FilePath,
					"crash_type": crash.Type,
					"timestamp":  crash.Timestamp,
				}).Info("Detected crash from fuzzer, attempting to report to master")

				if err := a.ReportCrash(crash); err != nil {
					a.logger.WithError(err).WithFields(logrus.Fields{
						"crash_id": crash.ID,
					}).Error("Failed to report crash to master")
				} else {
					a.logger.WithFields(logrus.Fields{
						"crash_id": crash.ID,
					}).Info("Successfully reported crash to master")
					crashCount++
				}
			}

			// If we got crashes from the fuzzer, return early
			if len(crashes) > 0 {
				return crashCount
			}
		}
	}

	// Fall back to manual directory scanning
	a.logger.WithField("job_id", job.ID).Info("Falling back to manual directory scanning for crashes")

	// Look for crash files in the working directory
	entries, err := os.ReadDir(job.WorkDir)
	if err != nil {
		a.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to read work directory for crashes")
		return 0
	}

	a.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"file_count": len(entries),
	}).Debug("Found files in work directory, checking for crashes")

	// Also check subdirectories for crash files
	dirsToCheck := []string{job.WorkDir}

	// Check corpus directory as well
	corpusDir := filepath.Join(job.WorkDir, "corpus")
	if stat, err := os.Stat(corpusDir); err == nil && stat.IsDir() {
		dirsToCheck = append(dirsToCheck, corpusDir)
	}

	// Check AFL++ output directories
	aflOutput := filepath.Join(job.WorkDir, "output")
	if stat, err := os.Stat(aflOutput); err == nil && stat.IsDir() {
		// AFL++ stores crashes in output/afl_output/crashes/
		aflCrashesDir := filepath.Join(aflOutput, "afl_output", "crashes")
		if stat, err := os.Stat(aflCrashesDir); err == nil && stat.IsDir() {
			dirsToCheck = append(dirsToCheck, aflCrashesDir)
			a.logger.WithFields(logrus.Fields{
				"job_id":          job.ID,
				"afl_crashes_dir": aflCrashesDir,
			}).Debug("Found AFL++ crashes directory")
		}

		// Also check the old location for backwards compatibility
		oldAflCrashesDir := filepath.Join(aflOutput, "crashes")
		if stat, err := os.Stat(oldAflCrashesDir); err == nil && stat.IsDir() {
			dirsToCheck = append(dirsToCheck, oldAflCrashesDir)
			a.logger.WithFields(logrus.Fields{
				"job_id":          job.ID,
				"afl_crashes_dir": oldAflCrashesDir,
			}).Debug("Found AFL++ crashes directory (old location)")
		}
	}

	// Check LibFuzzer output directories
	libfuzzerOutput := filepath.Join(job.WorkDir, "output", "libfuzzer_output")
	if stat, err := os.Stat(libfuzzerOutput); err == nil && stat.IsDir() {
		// Check artifacts directory where LibFuzzer writes crashes
		artifactsDir := filepath.Join(libfuzzerOutput, "artifacts")
		if stat, err := os.Stat(artifactsDir); err == nil && stat.IsDir() {
			dirsToCheck = append(dirsToCheck, artifactsDir)
		}

		// Also check crashes directory
		crashesDir := filepath.Join(libfuzzerOutput, "crashes")
		if stat, err := os.Stat(crashesDir); err == nil && stat.IsDir() {
			dirsToCheck = append(dirsToCheck, crashesDir)
		}
	}

	// Check HongFuzz output directories
	// HongFuzz can save crashes in both corpus and crashes directories
	honggfuzzCorpus := filepath.Join(job.WorkDir, "output", "honggfuzz_output", "corpus")
	if stat, err := os.Stat(honggfuzzCorpus); err == nil && stat.IsDir() {
		dirsToCheck = append(dirsToCheck, honggfuzzCorpus)
		a.logger.WithFields(logrus.Fields{
			"job_id":               job.ID,
			"honggfuzz_corpus_dir": honggfuzzCorpus,
		}).Debug("Found HongFuzz corpus directory")
	}

	// Also check HongFuzz crashes directory (--output flag)
	honggfuzzCrashes := filepath.Join(job.WorkDir, "output", "honggfuzz_output", "crashes")
	if stat, err := os.Stat(honggfuzzCrashes); err == nil && stat.IsDir() {
		dirsToCheck = append(dirsToCheck, honggfuzzCrashes)
		a.logger.WithFields(logrus.Fields{
			"job_id":                job.ID,
			"honggfuzz_crashes_dir": honggfuzzCrashes,
		}).Debug("Found HongFuzz crashes directory")
	}

	a.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"dirs":   dirsToCheck,
	}).Debug("Checking directories for crash files")

	for _, dir := range dirsToCheck {
		entries, err := os.ReadDir(dir)
		if err != nil {
			a.logger.WithError(err).WithField("dir", dir).Debug("Failed to read directory")
			continue
		}

		a.logger.WithFields(logrus.Fields{
			"dir":        dir,
			"file_count": len(entries),
		}).Debug("Checking directory for crashes")

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Check if this is a crash file
			isCrashFile := false
			crashType := ""

			// LibFuzzer crash files start with "crash-"
			if strings.HasPrefix(entry.Name(), "crash-") {
				isCrashFile = true
				crashType = "libfuzzer"
			} else if strings.Contains(dir, filepath.Join("output", "crashes")) && !strings.HasPrefix(entry.Name(), "README") {
				// AFL++ crash files are in output/crashes/ directory
				// Skip README files that AFL++ creates
				isCrashFile = true
				crashType = "afl++"
			} else if (strings.Contains(dir, filepath.Join("honggfuzz_output", "corpus")) ||
				strings.Contains(dir, filepath.Join("honggfuzz_output", "crashes"))) &&
				(strings.Contains(entry.Name(), "SIG") || strings.Contains(entry.Name(), "crash") ||
					!strings.HasPrefix(entry.Name(), ".")) {
				// HongFuzz crash files can be in either corpus or crashes directory
				// They often contain "SIG" or "crash" in the filename
				// In the crashes directory, any non-hidden file is likely a crash
				isCrashFile = true
				crashType = "honggfuzz"
			}

			if isCrashFile {
				crashPath := filepath.Join(dir, entry.Name())

				a.logger.WithFields(logrus.Fields{
					"job_id":     job.ID,
					"crash_file": entry.Name(),
					"crash_path": crashPath,
					"crash_type": crashType,
					"directory":  dir,
				}).Info("Found crash file")

				// Read crash file
				crashData, err := os.ReadFile(crashPath)
				if err != nil {
					a.logger.WithError(err).WithField("crash_file", entry.Name()).Error("Failed to read crash file")
					continue
				}

				// Get file info
				info, err := entry.Info()
				if err != nil {
					a.logger.WithError(err).WithField("crash_file", entry.Name()).Error("Failed to get crash file info")
					continue
				}

				// Create crash result
				crash := &common.CrashResult{
					ID:          fmt.Sprintf("%s_%s", job.ID, entry.Name()),
					JobID:       job.ID,
					BotID:       a.config.ID,
					Timestamp:   info.ModTime(),
					FilePath:    crashPath,
					Size:        info.Size(),
					Hash:        a.hashCrashInput(crashData),
					Type:        crashType,
					Input:       crashData,
					InputBase64: base64.StdEncoding.EncodeToString(crashData),
				}

				// Report crash to master
				a.logger.WithFields(logrus.Fields{
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
					"bot_id":     crash.BotID,
					"hash":       crash.Hash,
					"size":       crash.Size,
					"file_path":  crash.FilePath,
					"crash_type": crash.Type,
					"timestamp":  crash.Timestamp,
				}).Info("Detected crash, attempting to report to master")

				if err := a.ReportCrash(crash); err != nil {
					a.logger.WithError(err).WithFields(logrus.Fields{
						"crash_file": entry.Name(),
						"crash_id":   crash.ID,
					}).Error("Failed to report crash to master")
				} else {
					a.logger.WithFields(logrus.Fields{
						"crash_file": entry.Name(),
						"crash_id":   crash.ID,
					}).Info("Successfully reported crash to master")
					crashCount++
				}
			}
		}
	}

	return crashCount
}

// hashCrashInput computes a simple hash for crash deduplication
func (a *Agent) hashCrashInput(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// findCrashFiles finds all crash files in a directory
func (a *Agent) findCrashFiles(workDir string) ([]string, error) {
	var crashFiles []string

	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// LibFuzzer crash files start with "crash-"
		if strings.HasPrefix(entry.Name(), "crash-") {
			crashPath := filepath.Join(workDir, entry.Name())
			crashFiles = append(crashFiles, crashPath)
		}
	}

	return crashFiles, nil
}

// hash computes a simple numeric hash for a string
func hash(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
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
	a.currentJobCrashes++
	a.totalCrashes++
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

// retryPendingAcknowledgments checks for jobs pending acknowledgment and retries them
func (a *Agent) retryPendingAcknowledgments() {
	if a.apiServer == nil {
		return
	}

	// Check job cache for pending acknowledgments
	a.apiServer.mu.RLock()
	pendingJobs := make([]*JobStatus, 0)
	for _, status := range a.apiServer.jobCache {
		if status.Status == "pending_ack" {
			// Only retry if it's been pending for more than 30 seconds
			if time.Since(status.UpdatedAt) > 30*time.Second {
				pendingJobs = append(pendingJobs, status)
			}
		}
	}
	a.apiServer.mu.RUnlock()

	for _, jobStatus := range pendingJobs {
		a.logger.WithFields(logrus.Fields{
			"job_id": jobStatus.JobID,
			"age":    time.Since(jobStatus.UpdatedAt),
		}).Info("Retrying job completion for pending acknowledgment")

		// Try to complete the job again
		err := a.client.CompleteJob(a.config.ID, jobStatus.Success, jobStatus.Message)
		if err != nil {
			a.logger.WithError(err).WithField("job_id", jobStatus.JobID).Warn("Retry failed for pending job completion")
			// Keep it as pending - the poller will eventually handle it
		} else {
			// Success! Update the cache
			a.logger.WithField("job_id", jobStatus.JobID).Info("Job completion acknowledged on retry")
			a.apiServer.MarkJobCompleted(jobStatus.JobID, jobStatus.Success, jobStatus.Message, jobStatus.Output)

			// Clear current job if this was it
			a.mu.Lock()
			if a.currentJob != nil && a.currentJob.ID == jobStatus.JobID {
				a.currentJob = nil
			}
			a.mu.Unlock()
		}
	}
}

// extractZipFile extracts a zip file to the specified directory
func (a *Agent) extractZipFile(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		// Construct the file path
		path := filepath.Join(destDir, file.Name)

		// Check for directory traversal
		if !strings.HasPrefix(path, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in zip: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, file.Mode()); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Extract file in a separate function to ensure proper cleanup
		if err := a.extractSingleFile(file, path); err != nil {
			return err
		}
	}

	return nil
}

// extractSingleFile extracts a single file from a zip, ensuring proper resource cleanup
func (a *Agent) extractSingleFile(file *zip.File, destPath string) error {
	// Create the directories if necessary
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open the file in the zip
	fileReader, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in zip: %w", err)
	}
	defer fileReader.Close()

	// Create the destination file
	targetFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer targetFile.Close()

	// Copy the file contents
	_, err = io.Copy(targetFile, fileReader)
	if err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Ensure the file is written to disk
	if err := targetFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// executeMinimizationJob executes a crash minimization job
func (a *Agent) executeMinimizationJob(job *common.Job) (bool, string, error) {
	a.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"job_type": job.Type,
		"metadata": job.Metadata,
	}).Info("Executing minimization job")

	// Extract crash ID from job metadata
	crashID, ok := job.Metadata["crash_id"].(string)
	if !ok || crashID == "" {
		return false, "crash_id not found in job metadata", fmt.Errorf("invalid minimization job: missing crash_id")
	}

	// Get crash details
	crash, err := a.minimizerClient.GetCrashDetails(a.ctx, crashID)
	if err != nil {
		return false, fmt.Sprintf("Failed to get crash details: %v", err), err
	}

	// Extract strategy from metadata
	strategy := MinimizationStrategyDeltaDebug
	if s, ok := job.Metadata["strategy"].(string); ok {
		strategy = MinimizationStrategy(s)
	}

	// Prepare minimization config
	config := MinimizationConfig{
		Strategy:      strategy,
		MaxIterations: 100,
		Timeout:       job.Config.Timeout,
	}

	// Perform minimization
	result, err := a.minimizerClient.MinimizeCrash(a.ctx, crash, config)
	if err != nil {
		return false, fmt.Sprintf("Minimization failed: %v", err), err
	}

	// Update result with job ID
	result.JobID = job.ID

	if result.Success {
		return true, fmt.Sprintf("Minimization completed: %.2f%% reduction", result.ReductionPercent), nil
	} else {
		return false, fmt.Sprintf("Minimization failed: %s", result.Error), nil
	}
}

// executeReproductionJob executes a crash reproduction job
func (a *Agent) executeReproductionJob(job *common.Job) (bool, string, error) {
	a.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"job_type": job.Type,
		"metadata": job.Metadata,
	}).Info("Executing reproduction job")

	// For now, use the regular fuzzer executor for reproduction jobs
	// In the future, this could be handled by the reproExecutor
	return a.executor.ExecuteJob(job)
}

// connectResultCollectorToExecutor connects the result collector to executor events
func (a *Agent) connectResultCollectorToExecutor() {
	defer a.wg.Done()

	// Get the event channel from the executor
	eventChan := a.executor.GetEventChannel()

	for {
		select {
		case <-a.ctx.Done():
			return
		case event := <-eventChan:
			// Forward event to result collector
			if err := a.resultCollector.HandleEvent(event); err != nil {
				a.logger.WithError(err).WithField("event_type", event.Type).Warn("Failed to handle fuzzer event")
			}
		}
	}
}

// monitorResourceAlerts monitors for resource threshold alerts
func (a *Agent) monitorResourceAlerts() {
	defer a.wg.Done()

	alertChan := a.resourceMonitor.GetAlertChannel()

	for {
		select {
		case <-a.ctx.Done():
			return
		case alert := <-alertChan:
			a.logger.WithFields(logrus.Fields{
				"cpu_percent":   fmt.Sprintf("%.2f%%", alert.CPU),
				"memory_mb":     alert.Memory / (1024 * 1024),
				"disk_mb":       alert.Disk / (1024 * 1024),
				"process_count": alert.ProcessCount,
			}).Warn("Resource threshold alert received")

			// If resource usage is critical, consider pausing or stopping current job
			if alert.CPU > 95.0 {
				a.logger.Error("Critical CPU usage detected, may impact fuzzing performance")
			}

			// Could also report to master for cluster-wide resource management
			statusMap := map[string]any{
				"type":          "resource_alert",
				"cpu_percent":   alert.CPU,
				"memory_bytes":  alert.Memory,
				"disk_bytes":    alert.Disk,
				"process_count": alert.ProcessCount,
				"timestamp":     alert.Timestamp,
			}

			if err := a.client.ReportStatus(statusMap); err != nil {
				a.logger.WithError(err).Warn("Failed to report resource alert to master")
			}
		}
	}
}

// downloadCorpusCollection downloads all files from a corpus collection to the specified directory
func (a *Agent) downloadCorpusCollection(collectionID, targetDir string) error {
	a.logger.WithFields(logrus.Fields{
		"collection_id": collectionID,
		"target_dir":    targetDir,
	}).Info("Downloading corpus collection files")

	// Get collection files from master
	files, err := a.client.GetCorpusCollectionFiles(collectionID)
	if err != nil {
		return fmt.Errorf("failed to get collection files: %w", err)
	}

	if len(files) == 0 {
		a.logger.WithField("collection_id", collectionID).Warn("No files in corpus collection")
		return nil
	}

	a.logger.WithFields(logrus.Fields{
		"collection_id": collectionID,
		"file_count":    len(files),
	}).Info("Retrieved corpus collection file list")

	// Download each file
	downloadedCount := 0
	for _, file := range files {
		// Create file path
		filePath := filepath.Join(targetDir, file.Filename)

		// Download file
		if err := a.client.DownloadCorpusCollectionFile(collectionID, file.ID, filePath); err != nil {
			a.logger.WithError(err).WithFields(logrus.Fields{
				"file_id":   file.ID,
				"filename":  file.Filename,
				"file_hash": file.Hash,
			}).Warn("Failed to download corpus file, continuing with others")
			continue
		}

		// Verify downloaded file
		if stat, err := os.Stat(filePath); err != nil {
			a.logger.WithError(err).WithField("file_path", filePath).Warn("Failed to stat downloaded file")
			continue
		} else if stat.Size() != file.Size {
			a.logger.WithFields(logrus.Fields{
				"file_path":     filePath,
				"expected_size": file.Size,
				"actual_size":   stat.Size(),
			}).Warn("Downloaded file size mismatch")
			// Continue anyway, file might still be useful
		}

		downloadedCount++
	}

	a.logger.WithFields(logrus.Fields{
		"collection_id":    collectionID,
		"total_files":      len(files),
		"downloaded_files": downloadedCount,
	}).Info("Corpus collection download completed")

	if downloadedCount == 0 {
		return fmt.Errorf("failed to download any files from collection")
	}

	return nil
}
