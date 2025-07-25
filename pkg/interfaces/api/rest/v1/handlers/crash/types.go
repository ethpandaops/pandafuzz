package crash

import (
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CrashListResponse represents a crash list response
type CrashListResponse struct {
	Crashes   []*common.CrashResult `json:"crashes"`
	Count     int                   `json:"count"`
	Limit     int                   `json:"limit"`
	Offset    int                   `json:"offset"`
	SortBy    string                `json:"sort_by"`
	SortOrder string                `json:"sort_order"`
}

// CrashGroupsResponse represents crash groups response
type CrashGroupsResponse struct {
	CampaignID    string                 `json:"campaign_id"`
	CrashGroups   []*common.CrashGroup   `json:"crash_groups"`
	UniqueCrashes int                    `json:"unique_crashes"`
	TotalCrashes  int                    `json:"total_crashes"`
	Severities    map[string]int         `json:"severities"`
	Filters       map[string]interface{} `json:"filters"`
}

// StackTraceResponse represents a stack trace response
type StackTraceResponse struct {
	CrashID      string                 `json:"crash_id"`
	StackTrace   *common.StackTrace     `json:"stack_trace"`
	CrashDetails map[string]interface{} `json:"crash_details,omitempty"`
}

// BatchResultRequest represents a batch of results from collector
type BatchResultRequest struct {
	BotID    string                  `json:"bot_id" validate:"required"`
	JobID    string                  `json:"job_id" validate:"required"`
	Crashes  []common.CrashResult    `json:"crashes,omitempty"`
	Coverage []common.CoverageResult `json:"coverage,omitempty"`
	Corpus   []common.CorpusUpdate   `json:"corpus,omitempty"`
}

// BatchResultResponse represents a batch result response
type BatchResultResponse struct {
	Status         string         `json:"status"`
	BotID          string         `json:"bot_id"`
	JobID          string         `json:"job_id"`
	Processed      map[string]int `json:"processed"`
	Timestamp      time.Time      `json:"timestamp"`
	Errors         []string       `json:"errors,omitempty"`
	PartialSuccess bool           `json:"partial_success,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message,omitempty"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
