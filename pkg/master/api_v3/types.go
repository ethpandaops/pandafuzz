package api_v3

import (
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// Request types

// BotRegisterRequest represents a bot registration request
type BotRegisterRequest struct {
	Hostname     string   `json:"hostname" validate:"required,max=255"`
	Name         string   `json:"name,omitempty" validate:"max=100"`
	Capabilities []string `json:"capabilities" validate:"required,min=1,max=10"`
	APIEndpoint  string   `json:"api_endpoint" validate:"required,url,max=500"`
}

// BotRegisterResponse represents a bot registration response
type BotRegisterResponse struct {
	BotID     string    `json:"bot_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
}

// BotHeartbeatRequest represents a bot heartbeat request
type BotHeartbeatRequest struct {
	Status       common.BotStatus `json:"status" validate:"required,oneof=registering idle busy timed_out failed"`
	CurrentJob   *string          `json:"current_job,omitempty" validate:"omitempty,uuid"`
	LastActivity time.Time        `json:"last_activity" validate:"required"`
}

// BotHeartbeatResponse represents a bot heartbeat response
type BotHeartbeatResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
}

// JobRequest represents a job creation request
type JobRequest struct {
	Name              string           `json:"name" validate:"required,max=100"`
	Target            string           `json:"target" validate:"required,max=500"`
	Fuzzer            string           `json:"fuzzer" validate:"required,oneof=afl++ libfuzzer honggfuzz"`
	Duration          time.Duration    `json:"duration,omitempty"`
	Config            common.JobConfig `json:"config,omitempty"`
	CampaignID        string           `json:"campaign_id,omitempty" validate:"omitempty,uuid"`
	CorpusID          string           `json:"corpus_id,omitempty" validate:"omitempty,uuid"`
	CollectionID      string           `json:"collection_id,omitempty" validate:"omitempty,uuid"`
	UseCampaignCorpus bool             `json:"use_campaign_corpus,omitempty"`
}

// JobCompleteRequest represents a job completion request
type JobCompleteRequest struct {
	Success   bool      `json:"success" validate:"required"`
	Timestamp time.Time `json:"timestamp" validate:"required"`
	Message   string    `json:"message,omitempty"`
}

// JobCompleteResponse represents a job completion response
type JobCompleteResponse struct {
	Acknowledged bool      `json:"acknowledged"`
	JobID        string    `json:"job_id"`
	Message      string    `json:"message"`
	Status       string    `json:"status,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// CampaignRequest represents a campaign creation request
type CampaignRequest struct {
	Name         string           `json:"name" validate:"required"`
	Description  string           `json:"description,omitempty"`
	TargetBinary string           `json:"target_binary" validate:"required"`
	AutoRestart  bool             `json:"auto_restart,omitempty"`
	MaxDuration  time.Duration    `json:"max_duration,omitempty"`
	MaxJobs      int              `json:"max_jobs,omitempty" validate:"min=0"`
	JobTemplate  common.JobConfig `json:"job_template,omitempty"`
	SharedCorpus bool             `json:"shared_corpus,omitempty"`
	Tags         []string         `json:"tags,omitempty"`
}

// CorpusSyncRequest represents a corpus sync request
type CorpusSyncRequest struct {
	SourceCampaignID string `json:"source_campaign_id" validate:"required,uuid"`
	TargetCampaignID string `json:"target_campaign_id" validate:"required,uuid"`
	FilterCoverage   bool   `json:"filter_coverage,omitempty"`
	MinCoverage      int64  `json:"min_coverage,omitempty" validate:"min=0"`
}

// CorpusPromotionRequest represents a request to promote a crash to corpus
type CorpusPromotionRequest struct {
	CrashID    string `json:"crash_id" validate:"required,uuid"`
	CampaignID string `json:"campaign_id" validate:"required,uuid"`
	Reason     string `json:"reason" validate:"required"`
	Priority   int    `json:"priority,omitempty" validate:"min=1,max=10"`
}

// ReproductionRequestCreate represents a request to create a reproduction request
type ReproductionRequestCreate struct {
	CrashID     string           `json:"crash_id" validate:"required,uuid"`
	Priority    int              `json:"priority" validate:"required,min=1,max=10"`
	MaxAttempts int              `json:"max_attempts,omitempty" validate:"min=1,max=10"`
	Config      common.JobConfig `json:"config,omitempty"`
}

// BatchResultRequest represents a batch of results
type BatchResultRequest struct {
	BotID    string                  `json:"bot_id" validate:"required,uuid"`
	JobID    string                  `json:"job_id" validate:"required,uuid"`
	Crashes  []common.CrashResult    `json:"crashes,omitempty"`
	Coverage []common.CoverageResult `json:"coverage,omitempty"`
	Corpus   []common.CorpusUpdate   `json:"corpus,omitempty"`
}

// BatchResultResponse represents a batch result response
type BatchResultResponse struct {
	Status    string `json:"status"`
	BotID     string `json:"bot_id"`
	JobID     string `json:"job_id"`
	Processed struct {
		Crashes  int `json:"crashes"`
		Coverage int `json:"coverage"`
		Corpus   int `json:"corpus"`
	} `json:"processed"`
	Timestamp      time.Time `json:"timestamp"`
	Errors         []string  `json:"errors,omitempty"`
	PartialSuccess bool      `json:"partial_success,omitempty"`
}

// MaintenanceRequest represents a maintenance trigger request
type MaintenanceRequest struct {
	Type   string            `json:"type" validate:"required,oneof=cleanup optimize backup recovery vacuum"`
	Target string            `json:"target,omitempty"`
	Force  bool              `json:"force,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error     string                 `json:"error"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	RequestID string                 `json:"request_id,omitempty"`
}

// PaginationParams represents pagination parameters
type PaginationParams struct {
	Page      int
	Limit     int
	Offset    int
	SortBy    string
	SortOrder string
}

// Error types

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// NotFoundError represents a not found error
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with ID '%s' not found", e.Resource, e.ID)
}

// ConflictError represents a conflict error
type ConflictError struct {
	Resource string
	Message  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict on %s: %s", e.Resource, e.Message)
}

// APIError represents a generic API error
type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
