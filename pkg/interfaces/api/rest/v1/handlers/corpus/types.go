package corpus

import (
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

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

// Common response types

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message,omitempty"`
	Details   string    `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
