package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// BotService handles bot-related business logic
type BotService interface {
	// RegisterBot registers a new bot
	RegisterBot(ctx context.Context, hostname string, name string, capabilities []string, apiEndpoint string) (*common.Bot, error)
	
	// GetBot retrieves a bot by ID
	GetBot(ctx context.Context, botID string) (*common.Bot, error)
	
	// DeleteBot removes a bot from the system
	DeleteBot(ctx context.Context, botID string) error
	
	// UpdateHeartbeat updates bot heartbeat
	UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error
	
	// ListBots returns all bots, optionally filtered by status
	ListBots(ctx context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error)
	
	// GetAvailableBot finds an available bot for job assignment
	GetAvailableBot(ctx context.Context, requiredCapabilities []string) (*common.Bot, error)
}

// JobService handles job-related business logic
type JobService interface {
	// CreateJob creates a new job
	CreateJob(ctx context.Context, req CreateJobRequest) (*common.Job, error)
	
	// GetJob retrieves a job by ID
	GetJob(ctx context.Context, jobID string) (*common.Job, error)
	
	// ListJobs returns jobs with optional filters
	ListJobs(ctx context.Context, filter JobFilter) ([]*common.Job, error)
	
	// AssignJob assigns a job to a bot
	AssignJob(ctx context.Context, botID string) (*common.Job, error)
	
	// CompleteJob marks a job as completed
	CompleteJob(ctx context.Context, jobID, botID string, success bool) error
	
	// CancelJob cancels a job
	CancelJob(ctx context.Context, jobID string) error
	
	// GetJobLogs retrieves logs for a job
	GetJobLogs(ctx context.Context, jobID string) ([]string, error)
}

// ResultService handles result processing
type ResultService interface {
	// ProcessCrashResult processes a crash result
	ProcessCrashResult(ctx context.Context, crash *common.CrashResult) error
	
	// ProcessCoverageResult processes coverage data
	ProcessCoverageResult(ctx context.Context, coverage *common.CoverageResult) error
	
	// ProcessCorpusUpdate processes corpus updates
	ProcessCorpusUpdate(ctx context.Context, corpus *common.CorpusUpdate) error
	
	// GetCrashResults retrieves crash results for a job
	GetCrashResults(ctx context.Context, jobID string) ([]*common.CrashResult, error)
	
	// GetCoverageHistory retrieves coverage history
	GetCoverageHistory(ctx context.Context, jobID string) ([]*common.CoverageResult, error)
}

// SystemService handles system-level operations
type SystemService interface {
	// GetSystemStats returns system statistics
	GetSystemStats(ctx context.Context) (SystemStats, error)
	
	// TriggerRecovery triggers system recovery
	TriggerRecovery(ctx context.Context) error
	
	// GetActiveTimeouts returns active timeouts
	GetActiveTimeouts(ctx context.Context) (TimeoutInfo, error)
	
	// ForceTimeout forces a timeout for an entity
	ForceTimeout(ctx context.Context, entityType string, entityID string) error
}

// Request and response types

// CreateJobRequest represents a job creation request
type CreateJobRequest struct {
	Name     string           `json:"name" validate:"required"`
	Target   string           `json:"target" validate:"required"`
	Fuzzer   string           `json:"fuzzer" validate:"required"`
	Duration time.Duration    `json:"duration"`
	Config   common.JobConfig `json:"config"`
}

// JobFilter represents job list filters
type JobFilter struct {
	Status *common.JobStatus `json:"status,omitempty"`
	Fuzzer *string          `json:"fuzzer,omitempty"`
	Page   int              `json:"page"`
	Limit  int              `json:"limit"`
}

// SystemStats represents system statistics
type SystemStats struct {
	ServerStats    interface{} `json:"server"`
	StateStats     interface{} `json:"state"`
	TimeoutStats   interface{} `json:"timeouts"`
	DatabaseStats  interface{} `json:"database"`
	Timestamp      time.Time   `json:"timestamp"`
}

// TimeoutInfo represents timeout information
type TimeoutInfo struct {
	BotTimeouts []TimeoutEntry `json:"bot_timeouts"`
	JobTimeouts []TimeoutEntry `json:"job_timeouts"`
	Timestamp   time.Time      `json:"timestamp"`
}

// TimeoutEntry represents a single timeout entry
type TimeoutEntry struct {
	EntityID  string    `json:"entity_id"`
	Timeout   time.Time `json:"timeout"`
	Remaining string    `json:"remaining"`
}