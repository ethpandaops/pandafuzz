package campaign

import (
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CampaignCreateRequest represents a campaign creation request
type CampaignCreateRequest struct {
	Name             string           `json:"name" validate:"required"`
	Description      string           `json:"description,omitempty"`
	TargetBinary     string           `json:"target_binary" validate:"required"`
	BinaryHash       string           `json:"binary_hash,omitempty"`
	MaxJobs          int              `json:"max_jobs,omitempty"`
	MaxDuration      time.Duration    `json:"max_duration,omitempty"`
	AutoRestart      bool             `json:"auto_restart,omitempty"`
	SharedCorpus     bool             `json:"shared_corpus,omitempty"`
	JobTemplate      common.JobConfig `json:"job_template" validate:"required"`
	Tags             []string         `json:"tags,omitempty"`
	StartAfterCreate bool             `json:"start_after_create,omitempty"`
}

// CampaignUpdateRequest represents a campaign update request
type CampaignUpdateRequest struct {
	Name         *string        `json:"name,omitempty"`
	Description  *string        `json:"description,omitempty"`
	Status       *string        `json:"status,omitempty"`
	MaxJobs      *int           `json:"max_jobs,omitempty"`
	MaxDuration  *time.Duration `json:"max_duration,omitempty"`
	AutoRestart  *bool          `json:"auto_restart,omitempty"`
	SharedCorpus *bool          `json:"shared_corpus,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
}

// CampaignListResponse represents a campaign list response
type CampaignListResponse struct {
	Campaigns []*common.Campaign `json:"campaigns"`
	Count     int                `json:"count"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
}

// CampaignStatsResponse represents campaign statistics
type CampaignStatsResponse struct {
	CampaignID      string                    `json:"campaign_id"`
	Name            string                    `json:"name"`
	Status          common.CampaignStatus     `json:"status"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	CompletedAt     *time.Time                `json:"completed_at,omitempty"`
	Statistics      *common.CampaignStats     `json:"statistics"`
	CrashGroups     []*common.CrashGroup      `json:"crash_groups"`
	CorpusEvolution []*common.CorpusEvolution `json:"corpus_evolution"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message,omitempty"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
