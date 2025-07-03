package master

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// PersistentState manages all system state with persistence and recovery
type PersistentState struct {
	db           common.Database
	mu           sync.RWMutex
	bots         map[string]*common.Bot
	jobs         map[string]*common.Job
	metadata     map[string]any
	retryManager *common.RetryManager
	logger       *logrus.Logger
	config       *common.MasterConfig
	stats        StateStats

	// Cache management
	maxCacheSize    int
	cacheAccessTime map[string]time.Time // Track last access time for cache eviction

	// Campaign management
	campaignManager *CampaignStateManager
}

// StateStats tracks statistics about the state manager
type StateStats struct {
	BotsRegistered   int64     `json:"bots_registered"`
	JobsCreated      int64     `json:"jobs_created"`
	CrashesRecorded  int64     `json:"crashes_recorded"`
	CoverageReports  int64     `json:"coverage_reports"`
	CorpusUpdates    int64     `json:"corpus_updates"`
	TransactionCount int64     `json:"transaction_count"`
	LastRecovery     time.Time `json:"last_recovery"`
	LastBackup       time.Time `json:"last_backup"`
	Uptime           time.Time `json:"uptime"`
}

// NewPersistentState creates a new persistent state manager
func NewPersistentState(db common.Database, config *common.MasterConfig, logger *logrus.Logger) *PersistentState {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	retryPolicy := config.Retry.Database
	if retryPolicy.MaxRetries == 0 {
		retryPolicy = common.DatabaseRetryPolicy
	}

	// Default cache size to 1000 items per type (bots, jobs)
	maxCacheSize := 1000
	if config.Limits.MaxCacheSize > 0 {
		maxCacheSize = config.Limits.MaxCacheSize
	}

	return &PersistentState{
		db:              db,
		bots:            make(map[string]*common.Bot),
		jobs:            make(map[string]*common.Job),
		metadata:        make(map[string]any),
		retryManager:    common.NewRetryManager(retryPolicy),
		logger:          logger,
		config:          config,
		maxCacheSize:    maxCacheSize,
		cacheAccessTime: make(map[string]time.Time),
		stats: StateStats{
			Uptime: time.Now(),
		},
	}
}

// Bot operations with retry logic
func (ps *PersistentState) SaveBotWithRetry(ctx context.Context, bot *common.Bot) error {
	return ps.retryManager.Execute(func() error {
		// Acquire lock late
		ps.mu.Lock()
		// Update in-memory state
		ps.bots[bot.ID] = bot
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Persist to database
			if err := tx.Store(ctx, "bot:"+bot.ID, bot); err != nil {
				return common.NewDatabaseError("save_bot", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"bot_id":   bot.ID,
				"hostname": bot.Hostname,
				"status":   bot.Status,
			}).Debug("Bot saved successfully")

			return nil
		})
	})
}

func (ps *PersistentState) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
	// Check in-memory cache first with RLock
	ps.mu.RLock()
	if bot, exists := ps.bots[botID]; exists {
		ps.mu.RUnlock()
		// Update access time for LRU
		ps.mu.Lock()
		ps.cacheAccessTime["bot:"+botID] = time.Now()
		ps.mu.Unlock()
		return bot, nil
	}
	ps.mu.RUnlock()

	// Load from database without holding lock
	var bot common.Bot
	err := ps.retryManager.Execute(func() error {
		return ps.db.Get(ctx, "bot:"+botID, &bot)
	})

	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, common.NewValidationError("get_bot", fmt.Errorf("bot not found: %s", botID))
		}
		return nil, common.NewDatabaseError("get_bot", err)
	}

	// Update cache with proper synchronization to avoid race condition
	ps.mu.Lock()
	// Double-check if another goroutine already cached it
	if existingBot, exists := ps.bots[botID]; exists {
		ps.mu.Unlock()
		return existingBot, nil
	}

	// Check cache size and evict if necessary
	if len(ps.bots) >= ps.maxCacheSize {
		ps.evictOldestBotFromCache()
	}

	ps.bots[botID] = &bot
	ps.cacheAccessTime["bot:"+botID] = time.Now()
	ps.mu.Unlock()

	return &bot, nil
}

func (ps *PersistentState) DeleteBot(ctx context.Context, botID string) error {
	return ps.retryManager.Execute(func() error {
		// Remove from in-memory state first
		ps.mu.Lock()
		delete(ps.bots, botID)
		delete(ps.cacheAccessTime, "bot:"+botID)
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Remove from database
			if err := tx.Delete(ctx, "bot:"+botID); err != nil {
				return common.NewDatabaseError("delete_bot", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithField("bot_id", botID).Debug("Bot deleted successfully")

			return nil
		})
	})
}

func (ps *PersistentState) ListBots(ctx context.Context) ([]*common.Bot, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	bots := make([]*common.Bot, 0, len(ps.bots))
	for _, bot := range ps.bots {
		bots = append(bots, bot)
	}

	return bots, nil
}

// Job operations with retry logic
func (ps *PersistentState) SaveJobWithRetry(ctx context.Context, job *common.Job) error {
	return ps.retryManager.Execute(func() error {
		// Acquire lock late
		ps.mu.Lock()
		// Update in-memory state
		ps.jobs[job.ID] = job
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Persist to database
			if err := tx.Store(ctx, "job:"+job.ID, job); err != nil {
				return common.NewDatabaseError("save_job", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"job_id":       job.ID,
				"job_name":     job.Name,
				"fuzzer":       job.Fuzzer,
				"status":       job.Status,
				"assigned_bot": job.AssignedBot,
			}).Debug("Job saved successfully")

			return nil
		})
	})
}

func (ps *PersistentState) GetJob(ctx context.Context, jobID string) (*common.Job, error) {
	// Check in-memory cache first with RLock
	ps.mu.RLock()
	if job, exists := ps.jobs[jobID]; exists {
		ps.mu.RUnlock()
		// Update access time for LRU
		ps.mu.Lock()
		ps.cacheAccessTime["job:"+jobID] = time.Now()
		ps.mu.Unlock()
		return job, nil
	}
	ps.mu.RUnlock()

	// Load from database without holding lock
	var job common.Job
	err := ps.retryManager.Execute(func() error {
		return ps.db.Get(ctx, "job:"+jobID, &job)
	})

	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, common.NewValidationError("get_job", fmt.Errorf("job not found: %s", jobID))
		}
		return nil, common.NewDatabaseError("get_job", err)
	}

	// Update cache with proper synchronization to avoid race condition
	ps.mu.Lock()
	// Double-check if another goroutine already cached it
	if existingJob, exists := ps.jobs[jobID]; exists {
		ps.mu.Unlock()
		return existingJob, nil
	}

	// Check cache size and evict if necessary
	if len(ps.jobs) >= ps.maxCacheSize {
		ps.evictOldestJobFromCache()
	}

	ps.jobs[jobID] = &job
	ps.cacheAccessTime["job:"+jobID] = time.Now()
	ps.mu.Unlock()

	return &job, nil
}

func (ps *PersistentState) DeleteJob(ctx context.Context, jobID string) error {
	return ps.retryManager.Execute(func() error {
		// Remove from in-memory state first
		ps.mu.Lock()
		delete(ps.jobs, jobID)
		delete(ps.cacheAccessTime, "job:"+jobID)
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Remove from database
			if err := tx.Delete(ctx, "job:"+jobID); err != nil {
				return common.NewDatabaseError("delete_job", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithField("job_id", jobID).Debug("Job deleted successfully")

			return nil
		})
	})
}

func (ps *PersistentState) ListJobs(ctx context.Context) ([]*common.Job, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	jobs := make([]*common.Job, 0, len(ps.jobs))
	for _, job := range ps.jobs {
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Atomic job assignment with retry logic
func (ps *PersistentState) AtomicJobAssignmentWithRetry(ctx context.Context, botID string) (*common.Job, error) {
	var assignedJob *common.Job

	err := ps.retryManager.Execute(func() error {
		// Get bot info before transaction
		ps.mu.RLock()
		bot, exists := ps.bots[botID]
		if exists {
			// Make a copy to avoid race conditions
			botCopy := *bot
			bot = &botCopy
		}
		ps.mu.RUnlock()

		if !exists {
			return common.NewValidationError("job_assignment", fmt.Errorf("bot not found: %s", botID))
		}

		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Find available job with minimal locking
			ps.mu.RLock()
			job, err := ps.findAvailableJobTx()
			ps.mu.RUnlock()

			if err != nil {
				return err
			}
			if job == nil {
				return common.NewValidationError("job_assignment", fmt.Errorf("no jobs available"))
			}

			// Check bot availability
			if bot.Status != common.BotStatusIdle {
				return common.NewValidationError("job_assignment", fmt.Errorf("bot not available: %s", bot.Status))
			}

			// Update job status
			now := time.Now()
			job.Status = common.JobStatusAssigned
			job.AssignedBot = &botID
			job.StartedAt = &now

			// Set the work directory for the bot
			job.WorkDir = fmt.Sprintf("/tmp/pandafuzz/job_%s", job.ID)

			// Update bot status
			bot.Status = common.BotStatusBusy
			bot.CurrentJob = &job.ID
			bot.LastSeen = now

			// Create assignment record
			assignment := &common.JobAssignment{
				JobID:     job.ID,
				BotID:     botID,
				Timestamp: now,
				Status:    "assigned",
			}

			// Persist all changes atomically
			if err := tx.Store(ctx, "job:"+job.ID, job); err != nil {
				return common.NewDatabaseError("save_job_assignment", err)
			}
			if err := tx.Store(ctx, "bot:"+botID, bot); err != nil {
				return common.NewDatabaseError("save_bot_assignment", err)
			}
			if err := tx.Store(ctx, "assignment:"+job.ID, assignment); err != nil {
				return common.NewDatabaseError("save_assignment", err)
			}

			assignedJob = job
			return nil
		})
	})

	if err == nil && assignedJob != nil {
		// Update in-memory state after successful commit
		ps.mu.Lock()
		ps.jobs[assignedJob.ID] = assignedJob
		if bot, exists := ps.bots[botID]; exists {
			bot.Status = common.BotStatusBusy
			bot.CurrentJob = &assignedJob.ID
			bot.LastSeen = time.Now()
		}
		ps.stats.TransactionCount++
		ps.mu.Unlock()

		ps.logger.WithFields(logrus.Fields{
			"job_id":   assignedJob.ID,
			"bot_id":   botID,
			"job_name": assignedJob.Name,
			"fuzzer":   assignedJob.Fuzzer,
		}).Info("Job assigned successfully")
	}

	return assignedJob, err
}

// findAvailableJobTx finds an available job for assignment (transaction context)
func (ps *PersistentState) findAvailableJobTx() (*common.Job, error) {
	for _, job := range ps.jobs {
		if job.Status == common.JobStatusPending {
			// Check if job has not timed out
			if time.Now().Before(job.TimeoutAt) {
				return job, nil
			}
		}
	}
	return nil, nil
}

// Job completion with retry logic
func (ps *PersistentState) CompleteJobWithRetry(ctx context.Context, jobID, botID string, success bool) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()

			// Get job
			job, exists := ps.jobs[jobID]
			if !exists {
				return common.NewValidationError("complete_job", fmt.Errorf("job not found: %s", jobID))
			}

			// Get bot
			bot, exists := ps.bots[botID]
			if !exists {
				return common.NewValidationError("complete_job", fmt.Errorf("bot not found: %s", botID))
			}

			// Validate assignment
			if job.AssignedBot == nil || *job.AssignedBot != botID {
				return common.NewValidationError("complete_job", fmt.Errorf("job not assigned to bot"))
			}

			// Update job status
			now := time.Now()
			if success {
				job.Status = common.JobStatusCompleted
			} else {
				job.Status = common.JobStatusFailed
			}
			job.CompletedAt = &now
			job.AssignedBot = nil

			// Update bot status
			bot.Status = common.BotStatusIdle
			bot.CurrentJob = nil
			bot.LastSeen = now

			// Update assignment record
			assignment := &common.JobAssignment{
				JobID:     jobID,
				BotID:     botID,
				Timestamp: now,
				Status:    "completed",
			}

			// Persist changes
			if err := tx.Store(ctx, "job:"+jobID, job); err != nil {
				return common.NewDatabaseError("save_job_completion", err)
			}
			if err := tx.Store(ctx, "bot:"+botID, bot); err != nil {
				return common.NewDatabaseError("save_bot_completion", err)
			}
			if err := tx.Store(ctx, "assignment:"+jobID, assignment); err != nil {
				return common.NewDatabaseError("save_assignment_completion", err)
			}

			// Update in-memory state
			ps.jobs[jobID] = job
			ps.bots[botID] = bot

			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"job_id":  jobID,
				"bot_id":  botID,
				"success": success,
				"status":  job.Status,
			}).Info("Job completed successfully")

			return nil
		})
	})
}

// Result processing with retry logic
func (ps *PersistentState) ProcessCrashResultWithRetry(ctx context.Context, crash *common.CrashResult) error {
	// Log incoming crash for processing
	ps.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"size":      crash.Size,
		"timestamp": crash.Timestamp,
	}).Info("Processing crash result from bot")

	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate crash ID if not provided
			if crash.ID == "" {
				crash.ID = uuid.New().String()
				ps.logger.WithFields(logrus.Fields{
					"crash_id": crash.ID,
					"job_id":   crash.JobID,
				}).Debug("Generated new crash ID")
			}

			// Check for duplicates based on hash
			duplicate, err := ps.checkCrashDuplicateTx(ctx, tx, crash.Hash)
			if err != nil {
				return err
			}

			crash.IsUnique = !duplicate

			if duplicate {
				ps.logger.WithFields(logrus.Fields{
					"crash_hash": crash.Hash,
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
				}).Info("Crash is a duplicate of existing crash")
			} else {
				ps.logger.WithFields(logrus.Fields{
					"crash_hash": crash.Hash,
					"crash_id":   crash.ID,
					"job_id":     crash.JobID,
				}).Info("Crash is unique")
			}

			// Store crash input separately if provided
			hasInput := len(crash.Input) > 0
			if hasInput {
				// Check if we're using SQLiteStorage
				if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
					// Store outside transaction for SQLite (it has its own locking)
					if err := sqliteDB.StoreCrashInput(ctx, crash.ID, crash.Input); err != nil {
						return common.NewDatabaseError("save_crash_input", err)
					}
				} else {
					// Fallback for other databases
					if err := tx.Store(ctx, "crash_input:"+crash.ID, crash.Input); err != nil {
						return common.NewDatabaseError("save_crash_input", err)
					}
				}
				// Clear the input from the crash object to avoid storing it twice
				crash.Input = nil
			}

			// Store crash result (without input data)
			if err := tx.Store(ctx, "crash:"+crash.ID, crash); err != nil {
				return common.NewDatabaseError("save_crash", err)
			}

			ps.stats.CrashesRecorded++
			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"crash_id":  crash.ID,
				"job_id":    crash.JobID,
				"bot_id":    crash.BotID,
				"hash":      crash.Hash,
				"is_unique": crash.IsUnique,
				"type":      crash.Type,
				"signal":    crash.Signal,
				"exit_code": crash.ExitCode,
				"size":      crash.Size,
				"file_path": crash.FilePath,
				"has_input": hasInput,
				"timestamp": crash.Timestamp,
			}).Info("Crash result successfully processed and stored")

			return nil
		})
	})
}

func (ps *PersistentState) ProcessCoverageResultWithRetry(ctx context.Context, coverage *common.CoverageResult) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate coverage ID if not provided
			if coverage.ID == "" {
				coverage.ID = uuid.New().String()
			}

			// Store coverage result
			if err := tx.Store(ctx, "coverage:"+coverage.ID, coverage); err != nil {
				return common.NewDatabaseError("save_coverage", err)
			}

			ps.stats.CoverageReports++
			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"coverage_id": coverage.ID,
				"job_id":      coverage.JobID,
				"bot_id":      coverage.BotID,
				"edges":       coverage.Edges,
				"new_edges":   coverage.NewEdges,
				"exec_count":  coverage.ExecCount,
			}).Debug("Coverage result processed")

			return nil
		})
	})
}

func (ps *PersistentState) ProcessCorpusUpdateWithRetry(ctx context.Context, corpus *common.CorpusUpdate) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate corpus ID if not provided
			if corpus.ID == "" {
				corpus.ID = uuid.New().String()
			}

			// Store corpus update
			if err := tx.Store(ctx, "corpus:"+corpus.ID, corpus); err != nil {
				return common.NewDatabaseError("save_corpus", err)
			}

			ps.stats.CorpusUpdates++
			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"corpus_id":  corpus.ID,
				"job_id":     corpus.JobID,
				"bot_id":     corpus.BotID,
				"file_count": len(corpus.Files),
				"total_size": corpus.TotalSize,
			}).Debug("Corpus update processed")

			return nil
		})
	})
}

// checkCrashDuplicateTx checks if a crash with the given hash already exists
func (ps *PersistentState) checkCrashDuplicateTx(ctx context.Context, tx common.Transaction, hash string) (bool, error) {
	// This is a simplified implementation
	// In a real implementation, you would query existing crashes by hash
	// For now, we'll assume it's unique
	return false, nil
}

// Recovery operations
func (ps *PersistentState) LoadPersistedState(ctx context.Context) error {
	ps.logger.Info("Loading persisted state from database")

	return ps.retryManager.Execute(func() error {
		// Check if database supports advanced operations
		advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
		if isAdvanced {
			// Use Iterate method if available
			ps.mu.Lock()
			defer ps.mu.Unlock()

			// Load all jobs with "job:" prefix
			jobsLoaded := 0
			if err := advDB.Iterate(ctx, "job:", func(key string, value []byte) error {
				var job common.Job
				if err := json.Unmarshal(value, &job); err != nil {
					ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal job")
					return nil // Continue with other jobs
				}
				ps.jobs[job.ID] = &job
				jobsLoaded++
				return nil
			}); err != nil {
				ps.logger.WithError(err).Warn("Failed to iterate jobs, continuing without loaded state")
			}

			// Load all bots with "bot:" prefix
			botsLoaded := 0
			if err := advDB.Iterate(ctx, "bot:", func(key string, value []byte) error {
				var bot common.Bot
				if err := json.Unmarshal(value, &bot); err != nil {
					ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal bot")
					return nil // Continue with other bots
				}
				ps.bots[bot.ID] = &bot
				botsLoaded++
				return nil
			}); err != nil {
				ps.logger.WithError(err).Warn("Failed to iterate bots, continuing without loaded state")
			}

			ps.stats.LastRecovery = time.Now()
			ps.logger.WithFields(logrus.Fields{
				"bots_loaded": botsLoaded,
				"jobs_loaded": jobsLoaded,
			}).Info("Persisted state loaded from advanced database")
		} else {
			// For basic database, we can't iterate
			// This is a limitation - jobs/bots will need to be loaded as they're accessed
			ps.stats.LastRecovery = time.Now()
			ps.logger.Warn("Database doesn't support iteration, state will be loaded on-demand")
		}

		return nil
	})
}

func (ps *PersistentState) FindOrphanedJobs(ctx context.Context) ([]*common.Job, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var orphaned []*common.Job
	now := time.Now()

	for _, job := range ps.jobs {
		// Job is orphaned if it's assigned but the bot is not available
		if job.Status == common.JobStatusAssigned || job.Status == common.JobStatusRunning {
			if job.AssignedBot != nil {
				bot, exists := ps.bots[*job.AssignedBot]
				if !exists || bot.Status == common.BotStatusFailed || bot.Status == common.BotStatusTimedOut {
					orphaned = append(orphaned, job)
				}
			}

			// Also check for timed out jobs
			if now.After(job.TimeoutAt) {
				orphaned = append(orphaned, job)
			}
		}
	}

	return orphaned, nil
}

func (ps *PersistentState) FindTimedOutBots(ctx context.Context) ([]string, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var timedOut []string
	now := time.Now()

	for _, bot := range ps.bots {
		if now.After(bot.TimeoutAt) && bot.Status != common.BotStatusTimedOut {
			timedOut = append(timedOut, bot.ID)
		}
	}

	return timedOut, nil
}

func (ps *PersistentState) ResetBot(ctx context.Context, botID string) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()

			bot, exists := ps.bots[botID]
			if !exists {
				return common.NewValidationError("reset_bot", fmt.Errorf("bot not found: %s", botID))
			}

			// Reset bot state
			bot.Status = common.BotStatusTimedOut
			bot.CurrentJob = nil
			bot.FailureCount++

			// Persist changes
			if err := tx.Store(ctx, "bot:"+botID, bot); err != nil {
				return common.NewDatabaseError("reset_bot", err)
			}

			// Update in-memory state
			ps.bots[botID] = bot

			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"bot_id":        botID,
				"failure_count": bot.FailureCount,
			}).Warn("Bot reset due to timeout")

			return nil
		})
	})
}

// Metadata operations
func (ps *PersistentState) SetMetadata(ctx context.Context, key string, value any) error {
	return ps.retryManager.Execute(func() error {
		// Update in-memory state first
		ps.mu.Lock()
		ps.metadata[key] = value
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Persist to database
			if err := tx.Store(ctx, "metadata:"+key, value); err != nil {
				return common.NewDatabaseError("set_metadata", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			return nil
		})
	})
}

func (ps *PersistentState) GetMetadata(ctx context.Context, key string) (any, error) {
	// Check in-memory cache first with RLock
	ps.mu.RLock()
	if value, exists := ps.metadata[key]; exists {
		ps.mu.RUnlock()
		return value, nil
	}
	ps.mu.RUnlock()

	// Load from database without holding lock
	var value any
	err := ps.retryManager.Execute(func() error {
		return ps.db.Get(ctx, "metadata:"+key, &value)
	})

	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, common.NewValidationError("get_metadata", fmt.Errorf("metadata not found: %s", key))
		}
		return nil, common.NewDatabaseError("get_metadata", err)
	}

	// Update cache with proper synchronization to avoid race condition
	ps.mu.Lock()
	// Double-check if another goroutine already cached it
	if existingValue, exists := ps.metadata[key]; exists {
		ps.mu.Unlock()
		return existingValue, nil
	}
	ps.metadata[key] = value
	ps.mu.Unlock()

	return value, nil
}

// Statistics and monitoring
func (ps *PersistentState) GetStats(ctx context.Context) any {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats := ps.stats
	stats.BotsRegistered = int64(len(ps.bots))
	stats.JobsCreated = int64(len(ps.jobs))

	return stats
}

// GetStatsTyped returns typed state statistics
func (ps *PersistentState) GetStatsTyped(ctx context.Context) StateStats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats := ps.stats
	stats.BotsRegistered = int64(len(ps.bots))
	stats.JobsCreated = int64(len(ps.jobs))

	return stats
}

func (ps *PersistentState) GetDatabaseStats(ctx context.Context) any {
	return ps.db.Stats(ctx)
}

func (ps *PersistentState) GetDatabaseStatsTyped(ctx context.Context) common.DatabaseStats {
	return ps.db.Stats(ctx)
}

// Health check
func (ps *PersistentState) HealthCheck(ctx context.Context) error {
	return ps.db.Ping(ctx)
}

// Close gracefully shuts down the persistent state
func (ps *PersistentState) Close(ctx context.Context) error {
	ps.logger.Info("Shutting down persistent state manager")

	// Stop campaign manager if running
	if ps.campaignManager != nil {
		ps.campaignManager.Stop()
	}

	if ps.db != nil {
		return ps.db.Close(ctx)
	}

	return nil
}

// SetCampaignManager sets the campaign state manager
func (ps *PersistentState) SetCampaignManager(manager *CampaignStateManager) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.campaignManager = manager
}

// GetCampaignManager returns the campaign state manager
func (ps *PersistentState) GetCampaignManager() *CampaignStateManager {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.campaignManager
}

// GetCrashes retrieves crashes with pagination
func (ps *PersistentState) GetCrashes(ctx context.Context, limit, offset int) ([]*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashes(ctx, limit, offset)
	}

	// Fallback for other database implementations that support AdvancedDatabase
	if advDB, ok := ps.db.(common.AdvancedDatabase); ok {
		crashes := make([]*common.CrashResult, 0, limit)
		count := 0
		skipped := 0

		err := advDB.Iterate(ctx, "crash:", func(key string, value []byte) error {
			// Skip until we reach the offset
			if skipped < offset {
				skipped++
				return nil
			}

			// Stop when we've collected enough
			if count >= limit {
				return fmt.Errorf("limit reached") // Stop iteration
			}

			var crash common.CrashResult
			if err := json.Unmarshal(value, &crash); err != nil {
				ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal crash")
				return nil // Continue with next crash
			}

			crashes = append(crashes, &crash)
			count++
			return nil
		})

		if err != nil && err.Error() != "limit reached" {
			return nil, err
		}

		return crashes, nil
	}

	// Basic database fallback - not efficient but functional
	return nil, fmt.Errorf("database does not support efficient crash listing")
}

// GetCrash retrieves a specific crash by ID
func (ps *PersistentState) GetCrash(ctx context.Context, crashID string) (*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrash(ctx, crashID)
	}

	// Fallback for other database implementations
	var crash common.CrashResult
	err := ps.db.Get(ctx, "crash:"+crashID, &crash)

	if err != nil {
		if err == common.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &crash, nil
}

// GetJobCrashes retrieves all crashes for a specific job
func (ps *PersistentState) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetJobCrashes(ctx, jobID)
	}

	// Fallback for other database implementations that support AdvancedDatabase
	if advDB, ok := ps.db.(common.AdvancedDatabase); ok {
		crashes := make([]*common.CrashResult, 0)

		err := advDB.Iterate(ctx, "crash:", func(key string, value []byte) error {
			var crash common.CrashResult
			if err := json.Unmarshal(value, &crash); err != nil {
				ps.logger.WithError(err).WithField("key", key).Warn("Failed to unmarshal crash")
				return nil // Continue with next crash
			}

			if crash.JobID == jobID {
				crashes = append(crashes, &crash)
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		return crashes, nil
	}

	// Basic database fallback - not efficient but functional
	return nil, fmt.Errorf("database does not support efficient job crash listing")
}

// GetCrashInput retrieves the input data for a specific crash
func (ps *PersistentState) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	// Don't hold any locks - database has its own concurrency control
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashInput(ctx, crashID)
	}

	// Fallback for other database implementations
	var input []byte
	err := ps.db.Get(ctx, "crash_input:"+crashID, &input)

	if err != nil {
		if err == common.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	return input, nil
}

// Cache update methods for optimized operations

// UpdateBotInCache updates bot information in the in-memory cache
func (ps *PersistentState) UpdateBotInCache(botID string, status common.BotStatus, currentJob *string, lastSeen, timeoutAt time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if bot, exists := ps.bots[botID]; exists {
		bot.Status = status
		bot.CurrentJob = currentJob
		bot.LastSeen = lastSeen
		bot.TimeoutAt = timeoutAt
		bot.IsOnline = true
	}
}

// UpdateJobInCache updates job information in the in-memory cache
func (ps *PersistentState) UpdateJobInCache(job *common.Job) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.jobs[job.ID] = job
}

// UpdateBotInCacheForJob updates bot status related to job assignment
func (ps *PersistentState) UpdateBotInCacheForJob(botID string, jobID *string, status common.BotStatus) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if bot, exists := ps.bots[botID]; exists {
		bot.Status = status
		bot.CurrentJob = jobID
		bot.LastSeen = time.Now()
	}
}

// UpdateJobStatusInCache updates job status in the in-memory cache
func (ps *PersistentState) UpdateJobStatusInCache(jobID string, status common.JobStatus, completedAt *time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if job, exists := ps.jobs[jobID]; exists {
		job.Status = status
		job.CompletedAt = completedAt
		if completedAt != nil {
			job.AssignedBot = nil
		}
	}
}

// Cache eviction methods

// evictOldestBotFromCache removes the least recently accessed bot from cache
func (ps *PersistentState) evictOldestBotFromCache() {
	// Must be called with lock held
	var oldestKey string
	oldestTime := time.Now()

	for botID := range ps.bots {
		key := "bot:" + botID
		if accessTime, exists := ps.cacheAccessTime[key]; exists {
			if accessTime.Before(oldestTime) {
				oldestTime = accessTime
				oldestKey = botID
			}
		} else {
			// If no access time, it's the oldest
			oldestKey = botID
			break
		}
	}

	if oldestKey != "" {
		delete(ps.bots, oldestKey)
		delete(ps.cacheAccessTime, "bot:"+oldestKey)
		ps.logger.WithField("bot_id", oldestKey).Debug("Evicted bot from cache")
	}
}

// evictOldestJobFromCache removes the least recently accessed job from cache
func (ps *PersistentState) evictOldestJobFromCache() {
	// Must be called with lock held
	var oldestKey string
	oldestTime := time.Now()

	for jobID := range ps.jobs {
		key := "job:" + jobID
		if accessTime, exists := ps.cacheAccessTime[key]; exists {
			if accessTime.Before(oldestTime) {
				oldestTime = accessTime
				oldestKey = jobID
			}
		} else {
			// If no access time, it's the oldest
			oldestKey = jobID
			break
		}
	}

	if oldestKey != "" {
		delete(ps.jobs, oldestKey)
		delete(ps.cacheAccessTime, "job:"+oldestKey)
		ps.logger.WithField("job_id", oldestKey).Debug("Evicted job from cache")
	}
}

// cleanupCacheAccessTimes periodically removes stale access time entries
func (ps *PersistentState) cleanupCacheAccessTimes() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Remove access times for items no longer in cache
	for key := range ps.cacheAccessTime {
		if strings.HasPrefix(key, "bot:") {
			botID := strings.TrimPrefix(key, "bot:")
			if _, exists := ps.bots[botID]; !exists {
				delete(ps.cacheAccessTime, key)
			}
		} else if strings.HasPrefix(key, "job:") {
			jobID := strings.TrimPrefix(key, "job:")
			if _, exists := ps.jobs[jobID]; !exists {
				delete(ps.cacheAccessTime, key)
			}
		}
	}
}

// Database maintenance operations

// GetJobCoverageStats retrieves coverage statistics for a job
func (ps *PersistentState) GetJobCoverageStats(ctx context.Context, jobID string) (*CoverageStats, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return nil, fmt.Errorf("database doesn't support coverage stats query")
	}

	// Query for latest coverage data
	query := `
		SELECT 
			MAX(edges) as total_edges,
			MAX(blocks) as total_blocks,
			MAX(features) as total_features,
			COUNT(*) as report_count
		FROM coverage 
		WHERE job_id = ?
	`

	result, err := advDB.SelectOne(ctx, query, jobID)
	if err != nil {
		return nil, err
	}

	stats := &CoverageStats{
		JobID: jobID,
	}

	if edges, ok := result["total_edges"].(int64); ok {
		stats.TotalEdges = int(edges)
	}
	if blocks, ok := result["total_blocks"].(int64); ok {
		stats.TotalBlocks = int(blocks)
	}
	if features, ok := result["total_features"].(int64); ok {
		stats.TotalFeatures = int(features)
	}
	if count, ok := result["report_count"].(int64); ok {
		stats.ReportCount = int(count)
	}

	return stats, nil
}

// GetBotCompletedJobs returns the number of jobs completed by a bot
func (ps *PersistentState) GetBotCompletedJobs(ctx context.Context, botID string) (int, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		// Fallback to counting in memory
		ps.mu.RLock()
		defer ps.mu.RUnlock()

		count := 0
		for _, job := range ps.jobs {
			if job.AssignedBot != nil && *job.AssignedBot == botID &&
				(job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed) {
				count++
			}
		}
		return count, nil
	}

	query := `
		SELECT COUNT(*) as count
		FROM jobs 
		WHERE assigned_bot = ? 
		AND status IN ('completed', 'failed')
	`

	result, err := advDB.SelectOne(ctx, query, botID)
	if err != nil {
		return 0, err
	}

	if count, ok := result["count"].(int64); ok {
		return int(count), nil
	}

	return 0, nil
}

// OptimizeDatabase optimizes the database for better performance
func (ps *PersistentState) OptimizeDatabase(ctx context.Context) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support optimization")
	}

	// Run ANALYZE to update statistics
	if _, err := advDB.Execute(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("failed to analyze database: %v", err)
	}

	ps.logger.Info("Database optimization completed")
	return nil
}

// CleanupOldRecords removes old records from the database
func (ps *PersistentState) CleanupOldRecords(ctx context.Context, maxAge time.Duration) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support cleanup operations")
	}

	cutoffTime := time.Now().Add(-maxAge)
	totalDeleted := int64(0)

	// Clean up old completed jobs
	query := `
		DELETE FROM jobs 
		WHERE status IN ('completed', 'failed', 'cancelled', 'timed_out') 
		AND completed_at < ?
	`
	deleted, err := advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup jobs: %v", err)
	}
	totalDeleted += deleted

	// Clean up old crashes
	query = `DELETE FROM crashes WHERE timestamp < ?`
	deleted, err = advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup crashes: %v", err)
	}
	totalDeleted += deleted

	// Clean up old coverage data
	query = `DELETE FROM coverage WHERE timestamp < ?`
	deleted, err = advDB.Execute(ctx, query, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup coverage: %v", err)
	}
	totalDeleted += deleted

	ps.logger.WithFields(logrus.Fields{
		"max_age": maxAge,
		"deleted": totalDeleted,
	}).Info("Cleaned up old records")

	return nil
}

// VacuumDatabase performs database vacuum operation
func (ps *PersistentState) VacuumDatabase(ctx context.Context) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support vacuum operation")
	}

	// For SQLite, VACUUM reclaims unused space
	if err := advDB.Vacuum(ctx); err != nil {
		return fmt.Errorf("failed to vacuum database: %v", err)
	}

	ps.logger.Info("Database vacuum completed")
	return nil
}

// BackupDatabase creates a backup of the database
func (ps *PersistentState) BackupDatabase(ctx context.Context, backupPath string) error {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		return fmt.Errorf("database doesn't support backup operation")
	}

	// Perform backup
	if err := advDB.Backup(ctx, backupPath); err != nil {
		return fmt.Errorf("failed to backup database: %v", err)
	}

	ps.mu.Lock()
	ps.stats.LastBackup = time.Now()
	ps.mu.Unlock()

	ps.logger.WithField("path", backupPath).Info("Database backup completed")
	return nil
}

// CoverageStats represents coverage statistics for a job
type CoverageStats struct {
	JobID         string `json:"job_id"`
	TotalEdges    int    `json:"total_edges"`
	TotalBlocks   int    `json:"total_blocks"`
	TotalFeatures int    `json:"total_features"`
	ReportCount   int    `json:"report_count"`
}
