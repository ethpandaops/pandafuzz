package common

import (
	"time"
)

// CrashResult represents a crash found during fuzzing
type CrashResult struct {
	ID           string                 `json:"id" db:"id"`
	JobID        string                 `json:"job_id" db:"job_id"`
	BotID        string                 `json:"bot_id" db:"bot_id"`
	CampaignID   string                 `json:"campaign_id" db:"campaign_id"` // Campaign this crash belongs to
	Hash         string                 `json:"hash" db:"hash"`               // SHA256 for deduplication
	FilePath     string                 `json:"file_path" db:"file_path"`     // Relative to job work dir
	Type         string                 `json:"type" db:"type"`               // "segfault", "assertion", "timeout"
	Signal       int                    `json:"signal" db:"signal"`           // Signal number if applicable
	ExitCode     int                    `json:"exit_code" db:"exit_code"`
	Timestamp    time.Time              `json:"timestamp" db:"timestamp"`
	Size         int64                  `json:"size" db:"size"`                 // Crash input size
	IsUnique     bool                   `json:"is_unique" db:"is_unique"`       // Not a duplicate
	Input        []byte                 `json:"-" db:"-"`                       // Raw crash input (not persisted)
	InputBase64  string                 `json:"input_base64,omitempty" db:"-"`  // Base64 encoded crash input for JSON serialization
	Output       string                 `json:"output" db:"output"`             // Crash output/stderr
	StackTrace   string                 `json:"stack_trace" db:"stack_trace"`   // Raw stack trace
	Reproducible bool                   `json:"reproducible" db:"reproducible"` // Whether crash is reproducible
	Minimized    bool                   `json:"minimized" db:"minimized"`       // Whether crash has been minimized
	Metadata     map[string]interface{} `json:"metadata" db:"metadata"`         // Additional metadata
}

// StackFrame represents a single frame in a stack trace
type StackFrame struct {
	Function string `json:"function" db:"function"`
	File     string `json:"file" db:"file"`
	Line     int    `json:"line" db:"line"`
	Offset   uint64 `json:"offset" db:"offset"`
}

// StackTrace represents a parsed stack trace for crash deduplication
type StackTrace struct {
	Frames   []StackFrame `json:"frames" db:"frames"`
	TopNHash string       `json:"top_n_hash" db:"top_n_hash"` // Hash of top N frames
	FullHash string       `json:"full_hash" db:"full_hash"`   // Hash of complete trace
	RawTrace string       `json:"raw_trace" db:"raw_trace"`
}

// CrashGroup represents a group of similar crashes for deduplication
type CrashGroup struct {
	ID           string       `json:"id" db:"id"`
	CampaignID   string       `json:"campaign_id" db:"campaign_id"`
	StackHash    string       `json:"stack_hash" db:"stack_hash"`
	FirstSeen    time.Time    `json:"first_seen" db:"first_seen"`
	LastSeen     time.Time    `json:"last_seen" db:"last_seen"`
	Count        int          `json:"count" db:"count"`
	Severity     string       `json:"severity" db:"severity"`
	StackFrames  []StackFrame `json:"stack_frames" db:"stack_frames"`
	ExampleCrash string       `json:"example_crash" db:"example_crash"` // ID of representative crash
}

// ReproducibilityStatus represents the reproducibility status of a crash
type ReproducibilityStatus string

const (
	ReproducibilityStatusUnknown   ReproducibilityStatus = "unknown"
	ReproducibilityStatusTesting   ReproducibilityStatus = "testing"
	ReproducibilityStatusConfirmed ReproducibilityStatus = "confirmed"
	ReproducibilityStatusFlaky     ReproducibilityStatus = "flaky"
	ReproducibilityStatusFailed    ReproducibilityStatus = "failed"
)

// ReproductionRequest represents a request to reproduce a crash
type ReproductionRequest struct {
	ID           string                `json:"id" db:"id"`
	CrashID      string                `json:"crash_id" db:"crash_id"`
	CampaignID   string                `json:"campaign_id" db:"campaign_id"`
	JobID        string                `json:"job_id" db:"job_id"`
	BotID        *string               `json:"bot_id" db:"bot_id"` // Bot assigned to reproduce
	Status       ReproducibilityStatus `json:"status" db:"status"`
	Priority     int                   `json:"priority" db:"priority"`           // Higher priority = more important
	AttemptCount int                   `json:"attempt_count" db:"attempt_count"` // Number of reproduction attempts
	MaxAttempts  int                   `json:"max_attempts" db:"max_attempts"`   // Maximum allowed attempts
	RequestedAt  time.Time             `json:"requested_at" db:"requested_at"`
	StartedAt    *time.Time            `json:"started_at" db:"started_at"`
	CompletedAt  *time.Time            `json:"completed_at" db:"completed_at"`
	TimeoutAt    time.Time             `json:"timeout_at" db:"timeout_at"`
	Config       JobConfig             `json:"config" db:"config"` // Config for reproduction
}

// ReproductionResult represents the result of a crash reproduction attempt
type ReproductionResult struct {
	ID              string                `json:"id" db:"id"`
	RequestID       string                `json:"request_id" db:"request_id"`
	CrashID         string                `json:"crash_id" db:"crash_id"`
	BotID           string                `json:"bot_id" db:"bot_id"`
	AttemptNumber   int                   `json:"attempt_number" db:"attempt_number"`
	Status          ReproducibilityStatus `json:"status" db:"status"`
	Reproduced      bool                  `json:"reproduced" db:"reproduced"`         // Whether crash was reproduced
	ExecutionTime   time.Duration         `json:"execution_time" db:"execution_time"` // Time taken to execute
	Signal          int                   `json:"signal" db:"signal"`                 // Signal if crash reproduced
	ExitCode        int                   `json:"exit_code" db:"exit_code"`
	Output          string                `json:"output" db:"output"` // Stdout/stderr from execution
	StackTrace      string                `json:"stack_trace" db:"stack_trace"`
	StackHash       string                `json:"stack_hash" db:"stack_hash"`             // For comparing with original
	MatchesOriginal bool                  `json:"matches_original" db:"matches_original"` // Stack matches original crash
	EnvironmentInfo map[string]string     `json:"environment_info" db:"environment_info"` // Bot env details
	Timestamp       time.Time             `json:"timestamp" db:"timestamp"`
}

// MinimizationResult represents the result of a crash minimization attempt
type MinimizationResult struct {
	ID               string        `json:"id" db:"id"`
	CrashID          string        `json:"crash_id" db:"crash_id"`
	JobID            string        `json:"job_id" db:"job_id"`
	BotID            string        `json:"bot_id" db:"bot_id"`
	Strategy         string        `json:"strategy" db:"strategy"` // "delta_debug", "coverage_guided", etc.
	OriginalSize     int64         `json:"original_size" db:"original_size"`
	MinimizedSize    int64         `json:"minimized_size" db:"minimized_size"`
	ReductionPercent float64       `json:"reduction_percent" db:"reduction_percent"`
	Iterations       int           `json:"iterations" db:"iterations"`
	ExecutionTime    time.Duration `json:"execution_time" db:"execution_time"`
	Success          bool          `json:"success" db:"success"`
	MinimizedPath    string        `json:"minimized_path" db:"minimized_path"`
	MinimizedHash    string        `json:"minimized_hash" db:"minimized_hash"`
	StillReproduces  bool          `json:"still_reproduces" db:"still_reproduces"`
	Error            string        `json:"error,omitempty" db:"error"`
	Timestamp        time.Time     `json:"timestamp" db:"timestamp"`
}
