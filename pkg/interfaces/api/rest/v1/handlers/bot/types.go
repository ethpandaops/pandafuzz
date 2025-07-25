package bot

import (
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// LogEntry represents a log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

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

// JobCompleteRequest represents a job completion request
type JobCompleteRequest struct {
	JobID          string     `json:"job_id" validate:"required"`
	Status         string     `json:"status" validate:"required,oneof=completed failed"`
	Error          string     `json:"error,omitempty"`
	TotalExecs     int64      `json:"total_execs"`
	CrashesFound   int        `json:"crashes_found"`
	Coverage       *float64   `json:"coverage,omitempty"`
	CorpusCount    int        `json:"corpus_count"`
	LastNewFinding *time.Time `json:"last_new_finding,omitempty"`
	Logs           []LogEntry `json:"logs,omitempty"`
}

// AcknowledgmentResponse represents an acknowledgment response
type AcknowledgmentResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// BotResourceMetrics represents resource usage metrics
type BotResourceMetrics struct {
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsage int64     `json:"memory_usage"`
	MemoryLimit int64     `json:"memory_limit"`
	DiskUsage   int64     `json:"disk_usage"`
	NetworkSent int64     `json:"network_sent"`
	NetworkRecv int64     `json:"network_recv"`
}

// ResourceMetricsResponse represents a resource metrics response
type ResourceMetricsResponse struct {
	BotID   string              `json:"bot_id"`
	Metrics *BotResourceMetrics `json:"metrics"`
}
