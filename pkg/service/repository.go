// Package service provides business logic services for PandaFuzz.
// This file defines service-level repository interfaces using common types.
// These interfaces are narrower than the domain repository interfaces and
// are designed for gradual migration away from the StateStore god interface.
package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// BotRepository defines the interface for bot persistence operations.
// Services depend on this interface rather than the full StateStore.
//
// Note: Create and Update have upsert semantics in the current adapter
// implementation - both will create if not exists or update if exists.
type BotRepository interface {
	// Create persists a new bot (upsert: creates or updates if exists)
	Create(ctx context.Context, bot *common.Bot) error

	// Get retrieves a bot by ID
	Get(ctx context.Context, botID string) (*common.Bot, error)

	// Update updates an existing bot (upsert: creates or updates if exists)
	Update(ctx context.Context, bot *common.Bot) error

	// Delete removes a bot by ID
	Delete(ctx context.Context, botID string) error

	// List returns all bots
	List(ctx context.Context) ([]*common.Bot, error)

	// ListByStatus returns bots filtered by status
	ListByStatus(ctx context.Context, status common.BotStatus) ([]*common.Bot, error)

	// UpdateHeartbeat updates the heartbeat timestamp and status for a bot
	UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error

	// FindAvailableWithCapabilities finds an available bot with the required capabilities
	FindAvailableWithCapabilities(ctx context.Context, capabilities []string) (*common.Bot, error)

	// BatchUpdateStatus updates status for multiple bots
	BatchUpdateStatus(ctx context.Context, botIDs []string, status common.BotStatus) error
}

// JobRepository defines the interface for job persistence operations.
// Services depend on this interface rather than the full StateStore.
type JobRepository interface {
	// Create persists a new job
	Create(ctx context.Context, job *common.Job) error

	// Get retrieves a job by ID
	Get(ctx context.Context, jobID string) (*common.Job, error)

	// Update updates an existing job
	Update(ctx context.Context, job *common.Job) error

	// List returns jobs with optional status filter
	List(ctx context.Context, status *common.JobStatus, fuzzer *string, limit, offset int) ([]*common.Job, error)

	// ListAll returns all jobs
	ListAll(ctx context.Context) ([]*common.Job, error)

	// AtomicAssign atomically assigns a pending job to a bot
	AtomicAssign(ctx context.Context, botID string) (*common.Job, error)

	// Complete marks a job as completed
	Complete(ctx context.Context, jobID, botID string, success bool) error

	// GetLogs retrieves logs for a job
	GetLogs(ctx context.Context, jobID string, limit, offset int) ([]string, int, error)

	// StoreLogs stores logs for a job
	StoreLogs(ctx context.Context, jobID string, logs []string) error

	// GetCoverageData retrieves coverage data for a job
	GetCoverageData(ctx context.Context, jobID string) (*common.CoverageData, error)

	// GetCoverageHistory retrieves coverage history for a job
	GetCoverageHistory(ctx context.Context, jobID string, start, end time.Time) ([]*common.CoverageResult, error)
}

// CrashRepository defines the interface for crash persistence operations.
// Services depend on this interface rather than the full StateStore.
type CrashRepository interface {
	// Create persists a new crash
	Create(ctx context.Context, crash *common.CrashResult) error

	// List returns crashes with pagination
	List(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error)

	// ListByJob returns all crashes for a job
	ListByJob(ctx context.Context, jobID string) ([]*common.CrashResult, error)

	// ListInTimeRange returns crashes in a time range
	ListInTimeRange(ctx context.Context, start, end time.Time) ([]*common.CrashResult, error)
}

// ResultRepository defines the interface for result processing operations.
// This consolidates crash, coverage, and corpus result storage.
type ResultRepository interface {
	// ProcessCrash stores and processes a crash result
	ProcessCrash(ctx context.Context, crash *common.CrashResult) error

	// ProcessCoverage stores and processes coverage data
	ProcessCoverage(ctx context.Context, coverage *common.CoverageResult) error

	// ProcessCorpusUpdate stores and processes a corpus update
	ProcessCorpusUpdate(ctx context.Context, update *common.CorpusUpdate) error
}

// AnalyticsRepository defines the interface for analytics operations.
type AnalyticsRepository interface {
	// GetCampaignJobs returns all jobs for a campaign
	GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error)

	// GetJobCrashes returns crashes for a job
	GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error)

	// GetCrashesInTimeRange returns crashes in a time range
	GetCrashesInTimeRange(ctx context.Context, start, end time.Time) ([]*common.CrashResult, error)

	// GetCoverageHistory returns coverage history for a job
	GetCoverageHistory(ctx context.Context, jobID string, start, end time.Time) ([]*common.CoverageResult, error)
}

// SystemRepository defines the interface for system-level operations.
type SystemRepository interface {
	// GetStats returns general statistics
	GetStats() any

	// GetDatabaseStats returns database statistics
	GetDatabaseStats() any

	// HealthCheck performs a health check
	HealthCheck() error
}

// SystemRepositoryWithContext extends SystemRepository with context support.
// Use this interface when context propagation is needed for cancellation and timeouts.
type SystemRepositoryWithContext interface {
	SystemRepository

	// GetStatsCtx returns general statistics with context support
	GetStatsCtx(ctx context.Context) (any, error)

	// GetDatabaseStatsCtx returns database statistics with context support
	GetDatabaseStatsCtx(ctx context.Context) (any, error)

	// HealthCheckCtx performs a health check with context support
	HealthCheckCtx(ctx context.Context) error
}
