package common

import (
	"time"
)

// CorpusFile represents a file in the fuzzing corpus
type CorpusFile struct {
	ID          string     `json:"id" db:"id"`
	CampaignID  string     `json:"campaign_id" db:"campaign_id"`
	JobID       string     `json:"job_id" db:"job_id"`
	BotID       string     `json:"bot_id" db:"bot_id"`
	Filename    string     `json:"filename" db:"filename"`
	Hash        string     `json:"hash" db:"hash"`
	Size        int64      `json:"size" db:"size"`
	Coverage    int64      `json:"coverage" db:"coverage"`         // Edges covered
	NewCoverage int64      `json:"new_coverage" db:"new_coverage"` // New edges this file found
	ParentHash  string     `json:"parent_hash" db:"parent_hash"`   // File this was mutated from
	Generation  int        `json:"generation" db:"generation"`     // Mutation generation
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	SyncedAt    *time.Time `json:"synced_at" db:"synced_at"`
	IsSeed      bool       `json:"is_seed" db:"is_seed"`
}

// CorpusUpdate represents new corpus files from a fuzzing job
type CorpusUpdate struct {
	ID        string    `json:"id" db:"id"`
	JobID     string    `json:"job_id" db:"job_id"`
	BotID     string    `json:"bot_id" db:"bot_id"`
	Files     []string  `json:"files" db:"files"` // New corpus files
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	TotalSize int64     `json:"total_size" db:"total_size"`
}

// CorpusMetadata represents metadata about a job's corpus (persisted)
type CorpusMetadata struct {
	JobID       string            `json:"job_id" db:"job_id"`
	FileCount   int               `json:"file_count" db:"file_count"`
	TotalSize   int64             `json:"total_size" db:"total_size"`
	LastUpdated time.Time         `json:"last_updated" db:"last_updated"`
	FileHashes  map[string]string `json:"file_hashes" db:"file_hashes"` // filename -> hash
}

// CorpusEvolution tracks corpus growth over time
type CorpusEvolution struct {
	CampaignID    string    `json:"campaign_id" db:"campaign_id"`
	Timestamp     time.Time `json:"timestamp" db:"timestamp"`
	TotalFiles    int       `json:"total_files" db:"total_files"`
	TotalSize     int64     `json:"total_size" db:"total_size"`
	TotalCoverage int64     `json:"total_coverage" db:"total_coverage"`
	NewFiles      int       `json:"new_files" db:"new_files"`
	NewCoverage   int64     `json:"new_coverage" db:"new_coverage"`
}

// CorpusPromotionRequest represents a request to promote a crash to the corpus
type CorpusPromotionRequest struct {
	ID            string     `json:"id" db:"id"`
	CrashID       string     `json:"crash_id" db:"crash_id"`
	CampaignID    string     `json:"campaign_id" db:"campaign_id"`
	RequestedBy   string     `json:"requested_by" db:"requested_by"` // Bot or user ID
	Reason        string     `json:"reason" db:"reason"`             // Why promote this crash
	Status        string     `json:"status" db:"status"`             // pending, approved, rejected, completed
	Priority      int        `json:"priority" db:"priority"`
	NewCoverage   int64      `json:"new_coverage" db:"new_coverage"`     // Expected new edges
	MinimizedSize int64      `json:"minimized_size" db:"minimized_size"` // Size after minimization
	CorpusFileID  *string    `json:"corpus_file_id" db:"corpus_file_id"` // Created corpus file ID
	RequestedAt   time.Time  `json:"requested_at" db:"requested_at"`
	ReviewedAt    *time.Time `json:"reviewed_at" db:"reviewed_at"`
	CompletedAt   *time.Time `json:"completed_at" db:"completed_at"`
	ReviewNotes   string     `json:"review_notes" db:"review_notes"` // Notes from review
}

// QuarantinedFile represents a quarantined corpus file
type QuarantinedFile struct {
	ID            string             `json:"id" db:"id"`
	FileID        string             `json:"file_id" db:"file_id"`
	CampaignID    string             `json:"campaign_id" db:"campaign_id"`
	Hash          string             `json:"hash" db:"hash"`
	Reason        string             `json:"reason" db:"reason"`
	Details       string             `json:"details" db:"details"`
	QuarantinedAt time.Time          `json:"quarantined_at" db:"quarantined_at"`
	QuarantinedBy string             `json:"quarantined_by" db:"quarantined_by"` // "system" or user ID
	ReviewedAt    *time.Time         `json:"reviewed_at" db:"reviewed_at"`
	ReviewedBy    *string            `json:"reviewed_by" db:"reviewed_by"`
	Resolution    *string            `json:"resolution" db:"resolution"` // "restored", "deleted", "permanent"
	Metrics       *CorpusFileMetrics `json:"metrics" db:"metrics"`
}

// CorpusFileMetrics tracks metrics for a corpus file
type CorpusFileMetrics struct {
	FileID         string        `json:"file_id" db:"file_id"`
	CrashCount     int           `json:"crash_count" db:"crash_count"`           // Number of crashes caused
	TimeoutCount   int           `json:"timeout_count" db:"timeout_count"`       // Number of timeouts caused
	AvgExecTime    time.Duration `json:"avg_exec_time" db:"avg_exec_time"`       // Average execution time
	MaxMemoryUsage int64         `json:"max_memory_usage" db:"max_memory_usage"` // Maximum memory usage in bytes
	LastExecuted   time.Time     `json:"last_executed" db:"last_executed"`       // Last time file was executed
	ExecCount      int64         `json:"exec_count" db:"exec_count"`             // Total number of executions
}

// CorpusCollection represents a collection of corpus files that can be reused across jobs
type CorpusCollection struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	FileCount   int       `json:"file_count" db:"file_count"`
	TotalSize   int64     `json:"total_size" db:"total_size"`
	Tags        []string  `json:"tags"`
}

// CorpusCollectionFile represents a file in a corpus collection
type CorpusCollectionFile struct {
	ID           string    `json:"id" db:"id"`
	CollectionID string    `json:"collection_id" db:"collection_id"`
	Filename     string    `json:"filename" db:"filename"`
	Hash         string    `json:"hash" db:"hash"`
	Size         int64     `json:"size" db:"size"`
	UploadedAt   time.Time `json:"uploaded_at" db:"uploaded_at"`
}
