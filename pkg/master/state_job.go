package master

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/sirupsen/logrus"
)

// Job operations with retry logic

// SaveJobWithRetry persists a job to database with retry logic
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

// GetJob retrieves a job by ID
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

	// If we have SQLiteStorage, use its GetJob method to get data from the jobs table
	// This ensures we get all fields including enable_coverage and coverage_format
	ps.logger.WithFields(logrus.Fields{
		"job_id":  jobID,
		"db_type": fmt.Sprintf("%T", ps.db),
	}).Info("Checking database type for GetJob")

	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		ps.logger.WithField("job_id", jobID).Info("Using SQLiteStorage.GetJob for job retrieval")
		job, err := sqliteDB.GetJob(ctx, jobID)
		if err != nil {
			if common.IsNotFoundError(err) {
				return nil, common.NewValidationError("get_job", fmt.Errorf("job not found: %s", jobID))
			}
			return nil, common.NewDatabaseError("get_job", err)
		}

		ps.logger.WithFields(logrus.Fields{
			"job_id":          jobID,
			"enable_coverage": job.EnableCoverage,
			"coverage_format": job.CoverageFormat,
		}).Debug("Retrieved job from SQLiteStorage")

		// Update cache with proper synchronization
		ps.mu.Lock()
		ps.jobs[jobID] = job
		ps.cacheAccessTime["job:"+jobID] = time.Now()
		ps.mu.Unlock()

		return job, nil
	}

	// Fallback: Load from database metadata table without holding lock
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

// DeleteJob removes a job
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

// ListJobs returns all jobs
func (ps *PersistentState) ListJobs(ctx context.Context) ([]*common.Job, error) {
	// Always fetch fresh data from database to ensure we have all jobs
	// This fixes the phantom job issue where jobs exist in DB but not in cache
	if sqliteDB, ok := ps.db.(*storage.SQLiteStorage); ok {
		// Fetch all jobs from database
		jobs, err := sqliteDB.ListJobs(ctx, 0, 0, "") // No limit, no offset, no status filter
		if err != nil {
			ps.logger.WithError(err).Warn("Failed to fetch jobs from database, falling back to cache")
			// Fallback to cache if database query fails
			ps.mu.RLock()
			defer ps.mu.RUnlock()
			cachedJobs := make([]*common.Job, 0, len(ps.jobs))
			for _, job := range ps.jobs {
				cachedJobs = append(cachedJobs, job)
			}
			return cachedJobs, nil
		}

		// Update cache with fresh data from database
		ps.mu.Lock()
		for _, job := range jobs {
			ps.jobs[job.ID] = job
		}
		ps.mu.Unlock()

		ps.logger.WithField("job_count", len(jobs)).Debug("Synchronized jobs from database")
		return jobs, nil
	}

	// Fallback to cache for non-advanced databases
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	jobs := make([]*common.Job, 0, len(ps.jobs))
	for _, job := range ps.jobs {
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// ListJobsSorted retrieves jobs with sorting from the database
func (ps *PersistentState) ListJobsSorted(ctx context.Context, sortBy string, sortOrder string) ([]*common.Job, error) {
	// Check if database supports advanced operations
	advDB, isAdvanced := ps.db.(common.AdvancedDatabase)
	if !isAdvanced {
		// Fallback to unsorted list
		return ps.ListJobs(ctx)
	}

	// Build the ORDER BY clause
	orderClause := ""
	switch sortBy {
	case "created_at":
		orderClause = "created_at"
	case "started_at":
		orderClause = "started_at"
	case "completed_at":
		orderClause = "completed_at"
	case "name":
		orderClause = "name"
	case "status":
		orderClause = "status"
	case "fuzzer":
		orderClause = "fuzzer"
	default:
		orderClause = "created_at" // Default sort
	}

	if sortOrder == "asc" {
		orderClause += " ASC"
	} else {
		orderClause += " DESC"
	}

	query := fmt.Sprintf(`
		SELECT id, name, target, fuzzer, status, created_at, started_at, completed_at,
		       timeout_at, assigned_bot, work_dir, config, progress
		FROM jobs
		ORDER BY %s
	`, orderClause)

	rows, err := advDB.Select(ctx, query)
	if err != nil {
		return nil, err
	}

	jobs := make([]*common.Job, 0, len(rows))
	for _, row := range rows {
		job := &common.Job{}

		// Parse basic fields
		if id, ok := row["id"].(string); ok {
			job.ID = id
		}
		if name, ok := row["name"].(string); ok {
			job.Name = name
		}
		if target, ok := row["target"].(string); ok {
			job.Target = target
		}
		if fuzzer, ok := row["fuzzer"].(string); ok {
			job.Fuzzer = fuzzer
		}
		if status, ok := row["status"].(string); ok {
			job.Status = common.JobStatus(status)
		}
		if workDir, ok := row["work_dir"].(string); ok {
			job.WorkDir = workDir
		}
		if progress, ok := row["progress"].(int64); ok {
			job.Progress = int(progress)
		}

		// Parse time fields
		if createdAt, ok := row["created_at"].(time.Time); ok {
			job.CreatedAt = createdAt
		} else if createdAt, ok := row["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				job.CreatedAt = t
			}
		}

		if startedAt, ok := row["started_at"].(time.Time); ok {
			job.StartedAt = &startedAt
		} else if startedAt, ok := row["started_at"].(string); ok && startedAt != "" {
			if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
				job.StartedAt = &t
			}
		}

		if completedAt, ok := row["completed_at"].(time.Time); ok {
			job.CompletedAt = &completedAt
		} else if completedAt, ok := row["completed_at"].(string); ok && completedAt != "" {
			if t, err := time.Parse(time.RFC3339, completedAt); err == nil {
				job.CompletedAt = &t
			}
		}

		if timeoutAt, ok := row["timeout_at"].(time.Time); ok {
			job.TimeoutAt = timeoutAt
		} else if timeoutAt, ok := row["timeout_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, timeoutAt); err == nil {
				job.TimeoutAt = t
			}
		}

		// Parse assigned bot
		if assignedBot, ok := row["assigned_bot"].(string); ok && assignedBot != "" {
			job.AssignedBot = &assignedBot
		}

		// Parse config field
		if configStr, ok := row["config"].(string); ok && configStr != "" {
			if err := json.Unmarshal([]byte(configStr), &job.Config); err != nil {
				ps.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to parse job config")
			}
		}

		jobs = append(jobs, job)
	}

	return jobs, nil
}

// AtomicJobAssignmentWithRetry performs atomic job assignment with retry logic
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
			job, err := ps.findAvailableJobTx(botID)
			ps.mu.RUnlock()

			if err != nil {
				return err
			}
			if job == nil {
				return common.NewValidationError("job_assignment", fmt.Errorf("no jobs available for bot capabilities"))
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

			// Keep the relative work directory - bot will resolve it based on its config
			// job.WorkDir is already set to a relative path during job creation

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
// Now accepts botID to check capabilities
func (ps *PersistentState) findAvailableJobTx(botID string) (*common.Job, error) {
	// Get bot capabilities (must be called with lock held)
	bot, exists := ps.bots[botID]
	if !exists {
		return nil, fmt.Errorf("bot not found: %s", botID)
	}

	// Convert bot capabilities to a map for fast lookup
	botCapabilities := make(map[string]bool)
	for _, cap := range bot.Capabilities {
		// Normalize capability names (e.g., "aflplusplus" -> "afl++")
		normalized := normalizeCapability(cap)
		botCapabilities[normalized] = true
	}

	// Find a job that matches bot capabilities
	now := time.Now()
	for _, job := range ps.jobs {
		if job.Status == common.JobStatusPending {
			// Check if job has not timed out
			if now.Before(job.TimeoutAt) {
				// Check if bot has capability for this fuzzer type
				normalizedFuzzer := normalizeFuzzer(job.Fuzzer)
				if botCapabilities[normalizedFuzzer] {
					return job, nil
				}
			}
		}
	}
	return nil, nil
}

// normalizeCapability converts capability names to a standard format
func normalizeCapability(capability string) string {
	capability = strings.ToLower(strings.TrimSpace(capability))
	// Handle common variations
	switch capability {
	case "aflplusplus", "afl++":
		return "afl++"
	case "libfuzzer":
		return "libfuzzer"
	case "honggfuzz":
		return "honggfuzz"
	default:
		return capability
	}
}

// normalizeFuzzer converts fuzzer names to match capability format
func normalizeFuzzer(fuzzer string) string {
	fuzzer = strings.ToLower(strings.TrimSpace(fuzzer))
	// Handle common variations
	switch fuzzer {
	case "aflplusplus", "afl++":
		return "afl++"
	case "libfuzzer":
		return "libfuzzer"
	case "honggfuzz":
		return "honggfuzz"
	default:
		return fuzzer
	}
}

// UpdateJobStatusToTimedOut updates a job status to timed out (for unassigned jobs)
func (ps *PersistentState) UpdateJobStatusToTimedOut(ctx context.Context, jobID string) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()

			// Get job
			job, exists := ps.jobs[jobID]
			if !exists {
				return common.NewValidationError("update_job_timeout", fmt.Errorf("job not found: %s", jobID))
			}

			// Only update if job is still pending
			if job.Status != common.JobStatusPending {
				ps.logger.WithFields(logrus.Fields{
					"job_id": jobID,
					"status": job.Status,
				}).Debug("Job is not pending, skipping timeout status update")
				return nil
			}

			// Update job status
			now := time.Now()
			job.Status = common.JobStatusFailed // Mark as failed due to timeout
			job.CompletedAt = &now

			// Add timeout metadata
			if job.Metadata == nil {
				job.Metadata = make(map[string]interface{})
			}
			job.Metadata["failure_reason"] = "timeout"
			job.Metadata["timed_out_at"] = now.Format(time.RFC3339)

			// Persist changes
			if err := tx.Store(ctx, "job:"+jobID, job); err != nil {
				return common.NewDatabaseError("save_job_timeout", err)
			}

			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"job_id": jobID,
				"status": job.Status,
			}).Info("Job marked as timed out")

			return nil
		})
	})
}

// CompleteJobWithRetry completes a job with retry logic
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

// UpdateJobInCache updates job information in the in-memory cache
func (ps *PersistentState) UpdateJobInCache(job *common.Job) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.jobs[job.ID] = job
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

// GetCampaignJobs retrieves all jobs for a campaign
func (ps *PersistentState) GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error) {
	jobs, err := ps.ListJobs(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by campaign ID
	var filtered []*common.Job
	for _, job := range jobs {
		if job.CampaignID != nil && *job.CampaignID == campaignID {
			filtered = append(filtered, job)
		}
	}

	return filtered, nil
}

// GetJobsInTimeRange retrieves all jobs created within a time range
func (ps *PersistentState) GetJobsInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*common.Job, error) {
	jobs, err := ps.ListJobs(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by time range
	var filtered []*common.Job
	for _, job := range jobs {
		if job.CreatedAt.After(startTime) && job.CreatedAt.Before(endTime) {
			filtered = append(filtered, job)
		}
	}

	return filtered, nil
}
