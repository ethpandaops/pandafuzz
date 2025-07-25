package v1

import (
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// Bot API types

// BotRegisterRequest represents a bot registration request
type BotRegisterRequest struct {
	Hostname     string   `json:"hostname" validate:"required"`
	Name         string   `json:"name,omitempty"`
	Capabilities []string `json:"capabilities" validate:"required"`
	APIEndpoint  string   `json:"api_endpoint" validate:"required"`
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
	Status       common.BotStatus `json:"status"`
	CurrentJob   *string          `json:"current_job,omitempty"`
	LastActivity time.Time        `json:"last_activity"`
}

// BotHeartbeatResponse represents a bot heartbeat response
type BotHeartbeatResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
}

// BotListResponse represents a bot list response
type BotListResponse struct {
	Bots  []*common.Bot `json:"bots"`
	Count int           `json:"count"`
}

// Job API types

// JobRequest represents a job creation request
type JobRequest struct {
	Name              string           `json:"name" validate:"required"`
	Target            string           `json:"target" validate:"required"`
	Fuzzer            string           `json:"fuzzer" validate:"required"`
	Duration          time.Duration    `json:"duration"`
	Config            common.JobConfig `json:"config"`
	CampaignID        string           `json:"campaign_id,omitempty"`
	CorpusID          string           `json:"corpus_id,omitempty"`
	CollectionID      string           `json:"collection_id,omitempty"`
	UseCampaignCorpus bool             `json:"use_campaign_corpus,omitempty"`
}

// JobCompleteRequest represents a job completion request
type JobCompleteRequest struct {
	Success   bool      `json:"success"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
}

// JobListResponse represents a job list response
type JobListResponse struct {
	Jobs      []*common.Job `json:"jobs"`
	Count     int           `json:"count"`
	Page      int           `json:"page"`
	Limit     int           `json:"limit"`
	Total     int           `json:"total"`
	SortBy    string        `json:"sort_by"`
	SortOrder string        `json:"sort_order"`
}

// Campaign API types

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

// Crash API types

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

// Corpus API types

// CorpusSyncRequest represents a corpus synchronization request
type CorpusSyncRequest struct {
	BotID string `json:"bot_id" validate:"required"`
}

// CorpusSyncResponse represents a corpus synchronization response
type CorpusSyncResponse struct {
	CampaignID string               `json:"campaign_id"`
	BotID      string               `json:"bot_id"`
	Files      []*common.CorpusFile `json:"files"`
	FileCount  int                  `json:"file_count"`
	TotalSize  int64                `json:"total_size"`
	Timestamp  time.Time            `json:"timestamp"`
}

// CorpusShareRequest represents a corpus sharing request
type CorpusShareRequest struct {
	ToCampaignID    string `json:"to_campaign_id" validate:"required"`
	OnlyNewCoverage bool   `json:"only_new_coverage,omitempty"`
}

// CorpusShareResponse represents a corpus sharing response
type CorpusShareResponse struct {
	Status       string    `json:"status"`
	FromCampaign string    `json:"from_campaign"`
	ToCampaign   string    `json:"to_campaign"`
	Timestamp    time.Time `json:"timestamp"`
}

// CorpusFileResponse represents a corpus file with download URL
type CorpusFileResponse struct {
	*common.CorpusFile
	DownloadURL string `json:"download_url"`
}

// CorpusListResponse represents a corpus file list response
type CorpusListResponse struct {
	CampaignID string                `json:"campaign_id"`
	Files      []*CorpusFileResponse `json:"files"`
	Count      int                   `json:"count"`
	Total      int                   `json:"total"`
	Limit      int                   `json:"limit"`
	Offset     int                   `json:"offset"`
}

// CorpusEvolutionResponse represents corpus evolution response
type CorpusEvolutionResponse struct {
	CampaignID   string                    `json:"campaign_id"`
	CampaignName string                    `json:"campaign_name"`
	Evolution    []*common.CorpusEvolution `json:"evolution"`
	DataPoints   int                       `json:"data_points"`
}

// PromoteCrashToCorpusRequest represents a request to promote a crash to corpus
type PromoteCrashToCorpusRequest struct {
	CrashID    string `json:"crash_id" validate:"required"`
	CampaignID string `json:"campaign_id" validate:"required"`
}

// PromoteCrashToCorpusResponse represents a response for crash promotion
type PromoteCrashToCorpusResponse struct {
	Status     string             `json:"status"`
	CrashID    string             `json:"crash_id"`
	CampaignID string             `json:"campaign_id"`
	CorpusFile *common.CorpusFile `json:"corpus_file"`
	Message    string             `json:"message"`
}

// Result API types

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

// System API types

// MaintenanceRequest represents a maintenance trigger request
type MaintenanceRequest struct {
	Type   string            `json:"type" validate:"required"`
	Target string            `json:"target,omitempty"`
	Force  bool              `json:"force,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

// MaintenanceResponse represents a maintenance response
type MaintenanceResponse struct {
	Status    string    `json:"status"`
	Type      string    `json:"type"`
	Target    string    `json:"target,omitempty"`
	Duration  string    `json:"duration"`
	Timestamp time.Time `json:"timestamp"`
}

// BotResourceMetrics represents resource usage metrics for a bot
type BotResourceMetrics struct {
	BotID         string    `json:"bot_id"`
	Timestamp     time.Time `json:"timestamp"`
	CPUUsage      float64   `json:"cpu_usage"`
	MemoryUsage   uint64    `json:"memory_usage"`
	DiskUsage     uint64    `json:"disk_usage"`
	NetworkTx     uint64    `json:"network_tx"`
	NetworkRx     uint64    `json:"network_rx"`
	ActiveJobs    int       `json:"active_jobs"`
	JobsCompleted int       `json:"jobs_completed"`
	Uptime        string    `json:"uptime"`
}

// ResourceMetricsResponse represents resource metrics response
type ResourceMetricsResponse struct {
	BotID    string              `json:"bot_id"`
	Hostname string              `json:"hostname"`
	Status   common.BotStatus    `json:"status"`
	Online   bool                `json:"online"`
	Metrics  *BotResourceMetrics `json:"metrics"`
}

// Common response types

// AcknowledgmentResponse represents a generic acknowledgment
type AcknowledgmentResponse struct {
	Acknowledged bool      `json:"acknowledged"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
}

// StatusResponse represents a generic status response
type StatusResponse struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
