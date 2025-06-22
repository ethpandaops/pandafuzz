package service

import (
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
	
	// Job operations
	SaveJobWithRetry(job *common.Job) error
	GetJob(jobID string) (*common.Job, error)
	ListJobs() ([]*common.Job, error)
	AtomicJobAssignmentWithRetry(botID string) (*common.Job, error)
	CompleteJobWithRetry(jobID, botID string, success bool) error
	
	// Result processing
	ProcessCrashResultWithRetry(crash *common.CrashResult) error
	ProcessCoverageResultWithRetry(coverage *common.CoverageResult) error
	ProcessCorpusUpdateWithRetry(corpus *common.CorpusUpdate) error
	
	// Stats and health
	GetStats() interface{}
	GetDatabaseStats() interface{}
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
	GetStats() interface{}
	ForceTimeout(timeoutType string, entityID string) error
}

// RecoveryManager defines the interface for recovery operations
type RecoveryManager interface {
	RecoverOnStartup() error
}