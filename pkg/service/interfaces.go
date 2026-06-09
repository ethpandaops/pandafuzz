package service

import (
	"context"
	"errors"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
)

// Common errors
var (
	ErrNoJobsAvailable = errors.New("no jobs available")
)

// BotService handles bot-related business logic
type BotService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

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

	// DeregisterBot deregisters a bot
	DeregisterBot(ctx context.Context, botID string) error

	// Heartbeat updates bot heartbeat (simple version)
	Heartbeat(ctx context.Context, botID string) error

	// GetCurrentJob retrieves the current job for a bot
	GetCurrentJob(ctx context.Context, botID string) (*common.Job, error)

	// GetMetrics retrieves metrics for a bot
	GetMetrics(ctx context.Context, botID string) (*BotMetrics, error)
}

// JobService handles job-related business logic
type JobService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

	// CreateJob creates a new job
	CreateJob(ctx context.Context, req CreateJobRequest) (*common.Job, error)

	// GetJob retrieves a job by ID
	GetJob(ctx context.Context, jobID string) (*common.Job, error)

	// UpdateJob updates an existing job
	UpdateJob(ctx context.Context, job *common.Job) error

	// ListJobs returns jobs with optional filters
	ListJobs(ctx context.Context, filter JobFilter) ([]*common.Job, error)

	// AssignJob assigns a job to a bot
	AssignJob(ctx context.Context, botID string) (*common.Job, error)

	// AssignNextJob assigns the next available job to a bot
	AssignNextJob(ctx context.Context, botID string) (*common.Job, error)

	// CompleteJob marks a job as completed
	CompleteJob(ctx context.Context, jobID, botID string, success bool) error

	// CancelJob cancels a job (updates status to cancelled)
	CancelJob(ctx context.Context, jobID string) error

	// CancelJobExecution cancels a running job and signals the executor
	CancelJobExecution(ctx context.Context, jobID string) error

	// GetJobLogs retrieves logs for a job
	GetJobLogs(ctx context.Context, jobID string) ([]string, error)

	// GetJobLogsPaginated retrieves paginated logs for a job
	GetJobLogsPaginated(ctx context.Context, jobID string, limit, offset int) ([]string, int, error)

	// StoreJobLogs stores logs for a job
	StoreJobLogs(ctx context.Context, jobID string, logs []string) error

	// GetJobCorpus retrieves corpus files for a job
	GetJobCorpus(ctx context.Context, jobID string) ([]*common.CorpusFile, error)

	// StreamLogs streams job logs
	StreamLogs(ctx context.Context, jobID string) (<-chan string, error)

	// GetLogs retrieves job logs
	GetLogs(ctx context.Context, jobID string) ([]string, error)

	// GetJobStats retrieves statistics for a job
	GetJobStats(ctx context.Context, jobID string) (*JobStats, error)

	// GetJobCrashes retrieves crashes for a job
	GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error)

	// GetJobCrashesPaginated retrieves paginated crashes for a job
	GetJobCrashesPaginated(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, int, error)

	// GetQueueStats retrieves queue statistics (asynq mode only)
	GetQueueStats(ctx context.Context) (*QueueStats, error)

	// SetQueue sets the queue instance (for asynq mode)
	SetQueue(queue scheduler.Queue)

	// Coverage-related methods

	// GetCoverageReport retrieves the coverage report for a job
	GetCoverageReport(ctx context.Context, jobID string) (*CoverageReport, error)

	// GetCoverageFile retrieves a specific coverage file
	GetCoverageFile(ctx context.Context, jobID string, filePath string) ([]byte, error)

	// StoreCoverageFile stores a coverage file for a job
	StoreCoverageFile(ctx context.Context, jobID string, filePath string, data []byte) error

	// ListCoverageFiles lists all coverage files for a job
	ListCoverageFiles(ctx context.Context, jobID string) ([]string, error)
}

// ResultService handles result processing
type ResultService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

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
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

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
	Name              string           `json:"name" validate:"required"`
	Target            string           `json:"target" validate:"required"`
	Fuzzer            string           `json:"fuzzer" validate:"required"`
	Duration          time.Duration    `json:"duration"`
	Config            common.JobConfig `json:"config"`
	CampaignID        string           `json:"campaign_id,omitempty"`
	CorpusID          string           `json:"corpus_id,omitempty"`           // Use standalone corpus
	CollectionID      string           `json:"collection_id,omitempty"`       // Use corpus collection
	UseCampaignCorpus bool             `json:"use_campaign_corpus,omitempty"` // Whether to inherit corpus from campaign
	Priority          int              `json:"priority,omitempty"`            // Job priority (0-100, higher is more important)
	EnableCoverage    bool             `json:"enable_coverage,omitempty"`     // Whether to enable coverage collection
	CoverageFormat    string           `json:"coverage_format,omitempty"`     // Coverage format (lcov, html, json)
}

// JobFilter represents job list filters
type JobFilter struct {
	Status *common.JobStatus `json:"status,omitempty"`
	Fuzzer *string           `json:"fuzzer,omitempty"`
	Page   int               `json:"page"`
	Limit  int               `json:"limit"`
}

// SystemStats represents system statistics
type SystemStats struct {
	ServerStats   any       `json:"server"`
	StateStats    any       `json:"state"`
	TimeoutStats  any       `json:"timeouts"`
	DatabaseStats any       `json:"database"`
	Timestamp     time.Time `json:"timestamp"`
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

// CrashFilter represents crash list filters
type CrashFilter struct {
	CampaignID string `json:"campaign_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	UniqueOnly bool   `json:"unique_only,omitempty"`
}

// BotMetrics represents bot performance metrics
type BotMetrics struct {
	BotID            string    `json:"bot_id"`
	TotalJobsRun     int       `json:"total_jobs_run"`
	SuccessfulJobs   int       `json:"successful_jobs"`
	FailedJobs       int       `json:"failed_jobs"`
	CrashesFound     int       `json:"crashes_found"`
	UniqueCrashes    int       `json:"unique_crashes"`
	CorpusItemsAdded int       `json:"corpus_items_added"`
	CPUTime          float64   `json:"cpu_time_hours"`
	LastActive       time.Time `json:"last_active"`
}

// JobStats represents job statistics
type JobStats struct {
	JobID            string        `json:"job_id"`
	CrashesFound     int           `json:"crashes_found"`
	UniqueCrashes    int           `json:"unique_crashes"`
	CorpusSize       int           `json:"corpus_size"`
	CoveragePercent  float64       `json:"coverage_percent"`
	ExecutionsTotal  int64         `json:"executions_total"`
	ExecutionsPerSec float64       `json:"executions_per_sec"`
	Duration         time.Duration `json:"duration"`
	StartTime        time.Time     `json:"start_time"`
	EndTime          *time.Time    `json:"end_time,omitempty"`
}

// QueueStats represents queue statistics for asynq mode
type QueueStats struct {
	TotalJobs       int           `json:"total_jobs"`
	PendingJobs     int           `json:"pending_jobs"`
	RunningJobs     int           `json:"running_jobs"`
	CompletedJobs   int           `json:"completed_jobs"`
	FailedJobs      int           `json:"failed_jobs"`
	EnqueuedCount   int           `json:"enqueued_count"`
	ProcessedCount  int           `json:"processed_count"`
	FailedCount     int           `json:"failed_count"`
	RetryCount      int           `json:"retry_count"`
	AverageWaitTime time.Duration `json:"average_wait_time"`
	AverageExecTime time.Duration `json:"average_exec_time"`
	WorkersActive   int           `json:"workers_active"`
	WorkersTotal    int           `json:"workers_total"`
	LastProcessedAt time.Time     `json:"last_processed_at"`
}

// CoverageReport represents a coverage report for a job
type CoverageReport struct {
	JobID            string    `json:"job_id"`
	Format           string    `json:"format"`            // lcov, html, json
	TotalLines       int       `json:"total_lines"`       // Total lines in coverage
	CoveredLines     int       `json:"covered_lines"`     // Lines covered
	CoveragePercent  float64   `json:"coverage_percent"`  // Coverage percentage
	TotalFunctions   int       `json:"total_functions"`   // Total functions
	CoveredFunctions int       `json:"covered_functions"` // Functions covered
	TotalBranches    int       `json:"total_branches"`    // Total branches
	CoveredBranches  int       `json:"covered_branches"`  // Branches covered
	Files            []string  `json:"files"`             // List of coverage files
	GeneratedAt      time.Time `json:"generated_at"`      // When report was generated
}
