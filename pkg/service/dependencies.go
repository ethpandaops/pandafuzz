package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// StateStore defines the interface for persistent state operations
type StateStore interface {
	// Bot operations
	SaveBotWithRetry(bot *common.Bot) error
	GetBot(botID string) (*common.Bot, error)
	DeleteBot(botID string) error
	ListBots() ([]*common.Bot, error)

	// Optimized bot operations (optional - implementations should handle gracefully if not available)
	UpdateBotHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error
	GetAvailableBotWithCapabilities(ctx context.Context, requiredCapabilities []string) (*common.Bot, error)
	BatchUpdateBotStatus(ctx context.Context, botIDs []string, status common.BotStatus) error

	// Job operations
	SaveJobWithRetry(job *common.Job) error
	GetJob(jobID string) (*common.Job, error)
	ListJobs() ([]*common.Job, error)
	AtomicJobAssignmentWithRetry(botID string) (*common.Job, error)
	CompleteJobWithRetry(jobID, botID string, success bool) error

	// Optimized job operations (optional - implementations should handle gracefully if not available)
	ListJobsFiltered(ctx context.Context, status *common.JobStatus, fuzzer *string, limit, page int) ([]*common.Job, error)
	AtomicJobAssignmentOptimized(ctx context.Context, botID string) (*common.Job, error)
	CompleteJobOptimized(ctx context.Context, jobID, botID string, success bool) error

	// Result processing
	ProcessCrashResultWithRetry(crash *common.CrashResult) error
	ProcessCoverageResultWithRetry(coverage *common.CoverageResult) error
	ProcessCorpusUpdateWithRetry(corpus *common.CorpusUpdate) error

	// Stats and health
	GetStats() any
	GetDatabaseStats() any
	HealthCheck() error
}

// TimeoutManager defines the interface for timeout management
type TimeoutManager interface {
	// Bot timeouts
	SetBotTimeout(botID string, duration time.Duration)
	UpdateBotHeartbeat(botID string)
	RemoveBotTimeout(botID string)
	ListBotTimeouts() map[string]time.Time

	// Job timeouts
	SetJobTimeout(jobID string, duration time.Duration)
	UpdateJobTimeout(jobID string)
	RemoveJobTimeout(jobID string)
	ListJobTimeouts() map[string]time.Time

	// General operations
	GetStats() any
	ForceTimeout(timeoutType string, entityID string) error
}

// RecoveryManager defines the interface for recovery operations
type RecoveryManager interface {
	RecoverOnStartup(ctx context.Context) error
}
