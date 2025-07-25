package common

import (
	"errors"
	"strings"
	"time"
)

// Common errors
var (
	ErrNotImplemented = errors.New("not implemented")
)

// VersionInfo contains build version information
type VersionInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

// Bot management
type Bot struct {
	ID           string    `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	Hostname     string    `json:"hostname" db:"hostname"`
	Status       BotStatus `json:"status" db:"status"`
	LastSeen     time.Time `json:"last_seen" db:"last_seen"`
	RegisteredAt time.Time `json:"registered_at" db:"registered_at"`
	CurrentJob   *string   `json:"current_job" db:"current_job"`
	Capabilities []string  `json:"capabilities" db:"capabilities"`
	TimeoutAt    time.Time `json:"timeout_at" db:"timeout_at"`
	IsOnline     bool      `json:"is_online" db:"is_online"`
	FailureCount int       `json:"failure_count" db:"failure_count"`
	APIEndpoint  string    `json:"api_endpoint" db:"api_endpoint"` // Bot's API endpoint for polling
}

type BotStatus string

const (
	BotStatusRegistering BotStatus = "registering"
	BotStatusIdle        BotStatus = "idle"
	BotStatusBusy        BotStatus = "busy"
	BotStatusTimedOut    BotStatus = "timed_out"
	BotStatusFailed      BotStatus = "failed"
)

// BotOperationalConfig holds bot operational configuration
type BotOperationalConfig struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	JobTimeout        time.Duration `json:"job_timeout" yaml:"job_timeout"`
	MaxFailures       int           `json:"max_failures" yaml:"max_failures"`
	WorkDirectory     string        `json:"work_directory" yaml:"work_directory"`
}

// Job management
type Job struct {
	ID                string                 `json:"id" db:"id"`
	Name              string                 `json:"name" db:"name"`
	Target            string                 `json:"target" db:"target"`
	Fuzzer            string                 `json:"fuzzer" db:"fuzzer"` // "afl++", "libfuzzer", "minimizer"
	Type              JobType                `json:"type" db:"type"`     // "fuzzing", "minimization", "reproduction"
	Status            JobStatus              `json:"status" db:"status"`
	AssignedBot       *string                `json:"assigned_bot" db:"assigned_bot"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at"`
	StartedAt         *time.Time             `json:"started_at" db:"started_at"`
	CompletedAt       *time.Time             `json:"completed_at" db:"completed_at"`
	TimeoutAt         time.Time              `json:"timeout_at" db:"timeout_at"`
	WorkDir           string                 `json:"work_dir" db:"work_dir"`
	Config            JobConfig              `json:"config" db:"config"`
	Progress          int                    `json:"progress" db:"progress"` // Job progress percentage (0-100)
	CampaignID        *string                `json:"campaign_id" db:"campaign_id"`
	CollectionID      *string                `json:"collection_id" db:"collection_id"` // Corpus collection ID
	UseCampaignCorpus bool                   `json:"use_campaign_corpus" db:"use_campaign_corpus"`
	Metadata          map[string]interface{} `json:"metadata" db:"metadata"` // For job-specific data (e.g., crashID for minimization)
	Priority          int                    `json:"priority" db:"priority"` // Job priority (0-100, higher is more important)
}

type JobType string

const (
	JobTypeFuzzing      JobType = "fuzzing"
	JobTypeMinimization JobType = "minimization"
	JobTypeReproduction JobType = "reproduction"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusAssigned  JobStatus = "assigned"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusTimedOut  JobStatus = "timed_out"
	JobStatusCancelled JobStatus = "cancelled"
)

type JobConfig struct {
	Duration    time.Duration `json:"duration" yaml:"duration"`         // Maximum runtime
	MemoryLimit int64         `json:"memory_limit" yaml:"memory_limit"` // Memory limit in bytes
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`           // Execution timeout
	Dictionary  string        `json:"dictionary" yaml:"dictionary"`     // Optional dictionary file
	SeedCorpus  []string      `json:"seed_corpus" yaml:"seed_corpus"`   // Initial corpus files
	OutputDir   string        `json:"output_dir" yaml:"output_dir"`     // Job-specific output directory
}

// Results and findings
type CoverageResult struct {
	ID        string    `json:"id" db:"id"`
	JobID     string    `json:"job_id" db:"job_id"`
	BotID     string    `json:"bot_id" db:"bot_id"`
	Edges     int       `json:"edges" db:"edges"`         // Total edges hit
	NewEdges  int       `json:"new_edges" db:"new_edges"` // New edges this run
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	ExecCount int64     `json:"exec_count" db:"exec_count"` // Total executions
}

// Persistent storage structures
type JobAssignment struct {
	JobID     string    `json:"job_id" db:"job_id"`
	BotID     string    `json:"bot_id" db:"bot_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Status    string    `json:"status" db:"status"` // "assigned", "started", "completed"
}

// System configuration persisted to disk
type SystemConfig struct {
	MasterID          string        `json:"master_id" yaml:"master_id"`
	BotTimeout        time.Duration `json:"bot_timeout" yaml:"bot_timeout"`
	JobTimeout        time.Duration `json:"job_timeout" yaml:"job_timeout"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	MaxConcurrentJobs int           `json:"max_concurrent_jobs" yaml:"max_concurrent_jobs"`
	StoragePath       string        `json:"storage_path" yaml:"storage_path"`
}

// Retry policy configuration
type RetryPolicy struct {
	MaxRetries      int           `json:"max_retries" yaml:"max_retries"`
	InitialDelay    time.Duration `json:"initial_delay" yaml:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay" yaml:"max_delay"`
	Multiplier      float64       `json:"multiplier" yaml:"multiplier"`
	Jitter          bool          `json:"jitter" yaml:"jitter"`
	RetryableErrors []string      `json:"retryable_errors" yaml:"retryable_errors"`
}

// Resource limits configuration
type ResourceLimits struct {
	MaxCorpusSize     int64         `json:"max_corpus_size" yaml:"max_corpus_size"`         // Maximum corpus size per job
	MaxCrashSize      int64         `json:"max_crash_size" yaml:"max_crash_size"`           // Maximum crash file size
	MaxCrashCount     int           `json:"max_crash_count" yaml:"max_crash_count"`         // Maximum crashes per job
	MaxJobDuration    time.Duration `json:"max_job_duration" yaml:"max_job_duration"`       // Maximum job runtime
	MaxConcurrentJobs int           `json:"max_concurrent_jobs" yaml:"max_concurrent_jobs"` // Maximum concurrent jobs
	MaxCacheSize      int           `json:"max_cache_size" yaml:"max_cache_size"`           // Maximum cache size per type
}

// Error types for consistent error handling
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeDatabase   ErrorType = "database"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeStorage    ErrorType = "storage"
	ErrorTypeSystem     ErrorType = "system"
)

// PandaFuzzError with context
type PandaFuzzError struct {
	Type    ErrorType      `json:"type"`
	Op      string         `json:"operation"`
	Err     error          `json:"error"`
	Context map[string]any `json:"context"`
}

func (e *PandaFuzzError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Type) + " error in " + e.Op
}

func (e *PandaFuzzError) Unwrap() error {
	return e.Err
}

// Constructor functions for common errors
func NewValidationError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeValidation,
		Op:   op,
		Err:  err,
	}
}

func NewDatabaseError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeDatabase,
		Op:   op,
		Err:  err,
	}
}

func NewTimeoutError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeTimeout,
		Op:   op,
		Err:  err,
	}
}

func NewNetworkError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeNetwork,
		Op:   op,
		Err:  err,
	}
}

func NewStorageError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeStorage,
		Op:   op,
		Err:  err,
	}
}

func NewSystemError(op string, err error) *PandaFuzzError {
	return &PandaFuzzError{
		Type: ErrorTypeSystem,
		Op:   op,
		Err:  err,
	}
}

// IsNotFoundError checks if an error indicates a resource not found
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific database error
	if err == ErrKeyNotFound {
		return true
	}

	// Check if it's our error type
	if e, ok := err.(*PandaFuzzError); ok {
		return e.Type == "not_found"
	}

	// Check error message
	errStr := err.Error()
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no such") ||
		strings.Contains(errStr, "does not exist")
}

// Fuzzer event types
type FuzzerEventType string

const (
	FuzzerEventStarted      FuzzerEventType = "started"
	FuzzerEventStopped      FuzzerEventType = "stopped"
	FuzzerEventCrashFound   FuzzerEventType = "crash_found"
	FuzzerEventCorpusUpdate FuzzerEventType = "corpus_update"
	FuzzerEventCoverage     FuzzerEventType = "coverage"
	FuzzerEventStats        FuzzerEventType = "stats"
	FuzzerEventError        FuzzerEventType = "error"
	FuzzerEventTimeout      FuzzerEventType = "timeout"
)

// FuzzerEvent represents a fuzzer lifecycle or status event
type FuzzerEvent struct {
	Type      FuzzerEventType        `json:"type" db:"type"`
	Timestamp time.Time              `json:"timestamp" db:"timestamp"`
	JobID     string                 `json:"job_id" db:"job_id"`
	Data      map[string]interface{} `json:"data" db:"data"` // Event-specific data
}

// ResourceMetrics represents system resource usage metrics
type ResourceMetrics struct {
	CPU          float64   `json:"cpu" db:"cpu"`                     // CPU usage percentage
	Memory       int64     `json:"memory" db:"memory"`               // Memory usage in bytes
	Disk         int64     `json:"disk" db:"disk"`                   // Disk usage in bytes
	ProcessCount int       `json:"process_count" db:"process_count"` // Number of active processes
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
}

// CleanupPolicy defines resource cleanup policies and thresholds
type CleanupPolicy struct {
	MaxJobAge       time.Duration `json:"max_job_age" yaml:"max_job_age"`           // Maximum age for job data retention
	MaxCrashAge     time.Duration `json:"max_crash_age" yaml:"max_crash_age"`       // Maximum age for crash data retention
	MaxCorpusSize   int64         `json:"max_corpus_size" yaml:"max_corpus_size"`   // Maximum corpus size in bytes
	MaxDiskUsage    int64         `json:"max_disk_usage" yaml:"max_disk_usage"`     // Maximum disk usage in bytes
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"` // How often cleanup runs
}

// Campaign represents a fuzzing campaign that groups related jobs
type Campaign struct {
	ID           string         `json:"id" db:"id"`
	Name         string         `json:"name" db:"name"`
	Description  string         `json:"description" db:"description"`
	Status       CampaignStatus `json:"status" db:"status"`
	TargetBinary string         `json:"target_binary" db:"target_binary"`
	BinaryHash   string         `json:"binary_hash" db:"binary_hash"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
	CompletedAt  *time.Time     `json:"completed_at" db:"completed_at"`
	AutoRestart  bool           `json:"auto_restart" db:"auto_restart"`
	MaxDuration  time.Duration  `json:"max_duration" db:"max_duration"`
	MaxJobs      int            `json:"max_jobs" db:"max_jobs"`
	JobTemplate  JobConfig      `json:"job_template" db:"job_template"`
	SharedCorpus bool           `json:"shared_corpus" db:"shared_corpus"`
	Tags         []string       `json:"tags" db:"tags"`
}

// CampaignStatus represents the status of a campaign
type CampaignStatus string

const (
	CampaignStatusPending    CampaignStatus = "pending"
	CampaignStatusRunning    CampaignStatus = "running"
	CampaignStatusCompleted  CampaignStatus = "completed"
	CampaignStatusFailed     CampaignStatus = "failed"
	CampaignStatusPaused     CampaignStatus = "paused"
	CampaignStatusCancelled  CampaignStatus = "cancelled"
	CampaignStatusCorpusOnly CampaignStatus = "corpus_only" // For standalone corpus collections
)

// CampaignStats represents aggregated statistics for a campaign
type CampaignStats struct {
	CampaignID    string    `json:"campaign_id"`
	TotalJobs     int       `json:"total_jobs"`
	CompletedJobs int       `json:"completed_jobs"`
	TotalCrashes  int       `json:"total_crashes"`
	UniqueCrashes int       `json:"unique_crashes"`
	TotalCoverage int64     `json:"total_coverage"`
	CorpusSize    int64     `json:"corpus_size"`
	LastUpdated   time.Time `json:"last_updated"`
}

// CampaignFilters for listing campaigns
type CampaignFilters struct {
	Status     string   `json:"status"`
	Tags       []string `json:"tags"`
	BinaryHash string   `json:"binary_hash"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
}

// CampaignUpdates for partial campaign updates
type CampaignUpdates struct {
	Name         *string         `json:"name"`
	Description  *string         `json:"description"`
	Status       *CampaignStatus `json:"status"`
	AutoRestart  *bool           `json:"auto_restart"`
	MaxDuration  *time.Duration  `json:"max_duration"`
	MaxJobs      *int            `json:"max_jobs"`
	SharedCorpus *bool           `json:"shared_corpus"`
	Tags         []string        `json:"tags"`
}

// CampaignMetrics for real-time monitoring
type CampaignMetrics struct {
	ExecutionsPerSecond int64     `json:"executions_per_second"`
	CoverageRate        float64   `json:"coverage_rate"`
	CrashRate           float64   `json:"crash_rate"`
	MemoryUsage         int64     `json:"memory_usage"`
	CPUUsage            float64   `json:"cpu_usage"`
	LastUpdated         time.Time `json:"last_updated"`
}

// CampaignState for state management
type CampaignState struct {
	Campaign      *Campaign        `json:"campaign"`
	ActiveJobs    map[string]*Job  `json:"active_jobs"`
	CompletedJobs map[string]*Job  `json:"completed_jobs"`
	LastUpdate    time.Time        `json:"last_update"`
	Metrics       *CampaignMetrics `json:"metrics"`
}

// WebSocket message types
type WSMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

type WSMessageType string

const (
	WSMessageTypeCampaignUpdate WSMessageType = "campaign_update"
	WSMessageTypeCrashFound     WSMessageType = "crash_found"
	WSMessageTypeCorpusUpdate   WSMessageType = "corpus_update"
	WSMessageTypeBotStatus      WSMessageType = "bot_status"
	WSMessageTypeJobStatus      WSMessageType = "job_status"
	WSMessageTypeMetrics        WSMessageType = "metrics"
)

// HongFuzzReport represents the parsed report data from HongFuzz
type HongFuzzReport struct {
	Iterations     uint64  `json:"iterations"`      // Total iterations performed
	Crashes        uint64  `json:"crashes"`         // Total crashes found
	Timeouts       uint64  `json:"timeouts"`        // Total timeouts encountered
	Coverage       float64 `json:"coverage"`        // Coverage percentage
	Speed          float64 `json:"speed"`           // Executions per second
	GuardCoverage  uint64  `json:"guard_coverage"`  // Guard coverage count
	BranchCoverage uint64  `json:"branch_coverage"` // Branch coverage count
}
