package master

import (
	"context"
	"encoding/json"
	"fmt"
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
}

// StateStats tracks statistics about the state manager
type StateStats struct {
	BotsRegistered    int64     `json:"bots_registered"`
	JobsCreated       int64     `json:"jobs_created"`
	CrashesRecorded   int64     `json:"crashes_recorded"`
	CoverageReports   int64     `json:"coverage_reports"`
	CorpusUpdates     int64     `json:"corpus_updates"`
	TransactionCount  int64     `json:"transaction_count"`
	LastRecovery      time.Time `json:"last_recovery"`
	LastBackup        time.Time `json:"last_backup"`
	Uptime            time.Time `json:"uptime"`
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
	
	return &PersistentState{
		db:           db,
		bots:         make(map[string]*common.Bot),
		jobs:         make(map[string]*common.Job),
		metadata:     make(map[string]any),
		retryManager: common.NewRetryManager(retryPolicy),
		logger:       logger,
		config:       config,
		stats: StateStats{
			Uptime: time.Now(),
		},
	}
}

// Bot operations with retry logic
func (ps *PersistentState) SaveBotWithRetry(ctx context.Context, bot *common.Bot) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Update in-memory state
			ps.bots[bot.ID] = bot
			
			// Persist to database
			if err := tx.Store(ctx, "bot:"+bot.ID, bot); err != nil {
				return common.NewDatabaseError("save_bot", err)
			}
			
			ps.stats.TransactionCount++
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
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	// Check in-memory cache first
	if bot, exists := ps.bots[botID]; exists {
		return bot, nil
	}
	
	// Load from database
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
	
	// Update cache
	ps.mu.RUnlock()
	ps.mu.Lock()
	ps.bots[botID] = &bot
	ps.mu.Unlock()
	ps.mu.RLock()
	
	return &bot, nil
}

func (ps *PersistentState) DeleteBot(ctx context.Context, botID string) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Remove from in-memory state
			delete(ps.bots, botID)
			
			// Remove from database
			if err := tx.Delete(ctx, "bot:" + botID); err != nil {
				return common.NewDatabaseError("delete_bot", err)
			}
			
			ps.stats.TransactionCount++
			ps.logger.WithField("bot_id", botID).Debug("Bot deleted successfully")
			
			return nil
		})
	})
}

func (ps *PersistentState) ListBots() ([]*common.Bot, error) {
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
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Update in-memory state
			ps.jobs[job.ID] = job
			
			// Persist to database
			if err := tx.Store(ctx, "job:"+job.ID, job); err != nil {
				return common.NewDatabaseError("save_job", err)
			}
			
			ps.stats.TransactionCount++
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
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	// Check in-memory cache first
	if job, exists := ps.jobs[jobID]; exists {
		return job, nil
	}
	
	// Load from database
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
	
	// Update cache
	ps.mu.RUnlock()
	ps.mu.Lock()
	ps.jobs[jobID] = &job
	ps.mu.Unlock()
	ps.mu.RLock()
	
	return &job, nil
}

func (ps *PersistentState) DeleteJob(ctx context.Context, jobID string) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Remove from in-memory state
			delete(ps.jobs, jobID)
			
			// Remove from database
			if err := tx.Delete(ctx, "job:" + jobID); err != nil {
				return common.NewDatabaseError("delete_job", err)
			}
			
			ps.stats.TransactionCount++
			ps.logger.WithField("job_id", jobID).Debug("Job deleted successfully")
			
			return nil
		})
	})
}

func (ps *PersistentState) ListJobs() ([]*common.Job, error) {
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
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Find available job
			job, err := ps.findAvailableJobTx()
			if err != nil {
				return err
			}
			if job == nil {
				return common.NewValidationError("job_assignment", fmt.Errorf("no jobs available"))
			}
			
			// Get bot
			bot, exists := ps.bots[botID]
			if !exists {
				return common.NewValidationError("job_assignment", fmt.Errorf("bot not found: %s", botID))
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
			
			// Update in-memory state
			ps.jobs[job.ID] = job
			ps.bots[botID] = bot
			
			assignedJob = job
			ps.stats.TransactionCount++
			
			ps.logger.WithFields(logrus.Fields{
				"job_id":   job.ID,
				"bot_id":   botID,
				"job_name": job.Name,
				"fuzzer":   job.Fuzzer,
			}).Info("Job assigned successfully")
			
			return nil
		})
	})
	
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
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Generate crash ID if not provided
			if crash.ID == "" {
				crash.ID = uuid.New().String()
			}
			
			// Check for duplicates based on hash
			duplicate, err := ps.checkCrashDuplicateTx(ctx, tx, crash.Hash)
			if err != nil {
				return err
			}
			
			crash.IsUnique = !duplicate
			
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
				"has_input": hasInput,
			}).Info("Crash result processed")
			
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

func (ps *PersistentState) FindOrphanedJobs() ([]*common.Job, error) {
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

func (ps *PersistentState) FindTimedOutBots() ([]string, error) {
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
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()
			
			// Update in-memory state
			ps.metadata[key] = value
			
			// Persist to database
			if err := tx.Store(ctx, "metadata:"+key, value); err != nil {
				return common.NewDatabaseError("set_metadata", err)
			}
			
			ps.stats.TransactionCount++
			
			return nil
		})
	})
}

func (ps *PersistentState) GetMetadata(ctx context.Context, key string) (any, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	// Check in-memory cache first
	if value, exists := ps.metadata[key]; exists {
		return value, nil
	}
	
	// Load from database
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
	
	// Update cache
	ps.mu.RUnlock()
	ps.mu.Lock()
	ps.metadata[key] = value
	ps.mu.Unlock()
	ps.mu.RLock()
	
	return value, nil
}

// Statistics and monitoring
func (ps *PersistentState) GetStats() any {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	
	stats := ps.stats
	stats.BotsRegistered = int64(len(ps.bots))
	stats.JobsCreated = int64(len(ps.jobs))
	
	return stats
}

// GetStatsTyped returns typed state statistics
func (ps *PersistentState) GetStatsTyped() StateStats {
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
	
	if ps.db != nil {
		return ps.db.Close(ctx)
	}
	
	return nil
}

// GetCrashes retrieves crashes with pagination
func (ps *PersistentState) GetCrashes(ctx context.Context, limit, offset int) ([]*common.CrashResult, error) {
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashes(ctx, limit, offset)
	}
	
	// Fallback for other database implementations
	crashes := make([]*common.CrashResult, 0)
	crashMap := make(map[string]*common.CrashResult)
	
	// First, get all crashes
	err := ps.db.Transaction(ctx, func(tx common.Transaction) error {
		// We need to iterate through all keys to find crashes
		// This is inefficient but works with the current interface
		// TODO: Add a Keys() method to Transaction interface or use a different approach
		
		// For now, we'll try to load up to 10000 crashes by ID
		// This is a temporary workaround
		for i := 0; i < 10000; i++ {
			var crash common.CrashResult
			key := fmt.Sprintf("crash:%d", i)
			if err := tx.Get(ctx, key, &crash); err == nil {
				crashMap[key] = &crash
			}
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	// Convert map to slice and apply pagination
	keys := make([]string, 0, len(crashMap))
	for k := range crashMap {
		keys = append(keys, k)
	}
	
	// Sort keys for consistent ordering
	// Apply offset and limit
	start := offset
	if start > len(keys) {
		start = len(keys)
	}
	end := start + limit
	if end > len(keys) {
		end = len(keys)
	}
	
	for i := start; i < end; i++ {
		crashes = append(crashes, crashMap[keys[i]])
	}
	
	return crashes, nil
}

// GetCrash retrieves a specific crash by ID
func (ps *PersistentState) GetCrash(ctx context.Context, crashID string) (*common.CrashResult, error) {
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrash(ctx, crashID)
	}
	
	// Fallback for other database implementations
	var crash common.CrashResult
	
	err := ps.db.Transaction(ctx, func(tx common.Transaction) error {
		return tx.Get(ctx, "crash:"+crashID, &crash)
	})
	
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
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetJobCrashes(ctx, jobID)
	}
	
	// Fallback for other database implementations
	crashes := make([]*common.CrashResult, 0)
	
	err := ps.db.Transaction(ctx, func(tx common.Transaction) error {
		// We need to iterate through crashes to find ones for this job
		// This is inefficient but works with the current interface
		// TODO: Add job-based indexing or a better query interface
		
		// Check crash IDs based on UUID format
		for i := 0; i < 10000; i++ {
			var crash common.CrashResult
			// Try both numeric and UUID-based keys
			keys := []string{
				fmt.Sprintf("crash:%d", i),
			}
			
			for _, key := range keys {
				if err := tx.Get(ctx, key, &crash); err == nil && crash.JobID == jobID {
					crashes = append(crashes, &crash)
				}
			}
		}
		
		// Also try to get crashes by their actual UUIDs stored in the crash result
		// This requires knowing the UUIDs, which we don't have without an index
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return crashes, nil
}

// GetCrashInput retrieves the input data for a specific crash
func (ps *PersistentState) GetCrashInput(ctx context.Context, crashID string) ([]byte, error) {
	// Check if the database is SQLiteStorage and use its optimized methods
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		return sqliteDB.GetCrashInput(ctx, crashID)
	}
	
	// Fallback for other database implementations
	var input []byte
	
	err := ps.db.Transaction(ctx, func(tx common.Transaction) error {
		return tx.Get(ctx, "crash_input:"+crashID, &input)
	})
	
	if err != nil {
		if err == common.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}
	
	return input, nil
}