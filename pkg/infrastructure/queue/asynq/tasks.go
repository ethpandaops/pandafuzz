package asynq

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/hibiken/asynq"
)

// Task type constants
const (
	TypeFuzzingJob = "fuzzing:job"
)

// Queue name constants (mapped from job priorities)
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// CampaignInfo contains campaign-related information
type CampaignInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// FuzzerConfig contains fuzzer configuration
type FuzzerConfig struct {
	Type         string            `json:"type"`
	Args         []string          `json:"args"`
	Environment  map[string]string `json:"environment"`
	Timeout      time.Duration     `json:"timeout"`
	MaxDuration  time.Duration     `json:"max_duration"`
	MemoryLimit  int64             `json:"memory_limit"`
	CPULimit     float64           `json:"cpu_limit"`
	EnableCorpus bool              `json:"enable_corpus"`
}

// TargetConfig contains target binary configuration
type TargetConfig struct {
	BinaryPath        string   `json:"binary_path"`
	Arguments         []string `json:"arguments"`
	CorpusPath        string   `json:"corpus_path"`
	OutputPath        string   `json:"output_path"`
	WorkDir           string   `json:"work_dir"`
	UseCampaignCorpus bool     `json:"use_campaign_corpus"`
	CollectionID      string   `json:"collection_id,omitempty"`
}

// FuzzingTask represents a fuzzing job task
type FuzzingTask struct {
	JobID       string            `json:"job_id"`
	BotID       string            `json:"bot_id"`
	Campaign    CampaignInfo      `json:"campaign"`
	Fuzzer      FuzzerConfig      `json:"fuzzer"`
	Target      TargetConfig      `json:"target"`
	Priority    types.JobPriority `json:"priority"`
	CreatedAt   time.Time         `json:"created_at"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	MaxRetries  int               `json:"max_retries"`
	RetryDelay  time.Duration     `json:"retry_delay"`
}

// NewFuzzingTask creates an asynq task from a pandafuzz job
func NewFuzzingTask(job *types.Job) (*asynq.Task, error) {
	task := &FuzzingTask{
		JobID:    job.ID,
		Campaign: CampaignInfo{
			// Campaign info would be populated from job metadata
		},
		Fuzzer: FuzzerConfig{
			Type:         job.FuzzerType,
			Args:         job.TargetArgs,
			Environment:  job.Environment,
			Timeout:      getTimeoutFromConfig(job.FuzzerConfig),
			MaxDuration:  job.MaxDuration,
			EnableCorpus: true,
		},
		Target: TargetConfig{
			BinaryPath: job.TargetBinary,
			Arguments:  job.TargetArgs,
			CorpusPath: job.CorpusPath,
			OutputPath: job.OutputPath,
			WorkDir:    getWorkDir(job),
		},
		Priority:    job.Priority,
		CreatedAt:   job.CreatedAt,
		ScheduledAt: job.ScheduledAt,
		Metadata:    job.Metadata,
		Tags:        job.Tags,
		MaxRetries:  job.MaxRetries,
		RetryDelay:  job.RetryDelay,
	}

	// Populate campaign info from metadata if available
	if campaignID, ok := job.Metadata["campaign_id"]; ok {
		task.Campaign.ID = campaignID
	}
	if campaignName, ok := job.Metadata["campaign_name"]; ok {
		task.Campaign.Name = campaignName
	}

	// Set corpus collection info
	if collectionID, ok := job.Metadata["collection_id"]; ok {
		task.Target.CollectionID = collectionID
	}
	if useCampaignCorpus, ok := job.FuzzerConfig["use_campaign_corpus"].(bool); ok {
		task.Target.UseCampaignCorpus = useCampaignCorpus
	}

	payload, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fuzzing task: %w", err)
	}

	return asynq.NewTask(TypeFuzzingJob, payload), nil
}

// ParseFuzzingTask parses an asynq task into a FuzzingTask
func ParseFuzzingTask(task *asynq.Task) (*FuzzingTask, error) {
	var fuzzingTask FuzzingTask
	if err := json.Unmarshal(task.Payload(), &fuzzingTask); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fuzzing task: %w", err)
	}
	return &fuzzingTask, nil
}

// GetQueueNameForPriority returns the queue name for a given priority
func GetQueueNameForPriority(priority types.JobPriority) string {
	switch priority {
	case types.PriorityCritical:
		return QueueCritical
	case types.PriorityHigh:
		return QueueDefault
	case types.PriorityNormal:
		return QueueDefault
	case types.PriorityLow:
		return QueueLow
	default:
		return QueueDefault
	}
}

// GetTaskOptions returns asynq options for a job
func GetTaskOptions(job *types.Job) []asynq.Option {
	opts := []asynq.Option{
		asynq.Queue(GetQueueNameForPriority(job.Priority)),
		asynq.MaxRetry(job.MaxRetries),
	}

	if job.MaxDuration > 0 {
		opts = append(opts, asynq.Timeout(job.MaxDuration))
	}

	if job.ScheduledAt != nil && job.ScheduledAt.After(time.Now()) {
		opts = append(opts, asynq.ProcessAt(*job.ScheduledAt))
	}

	// Note: Custom retry delay is not directly supported in newer asynq versions
	// The retry delay is configured at the server level

	// Set unique task ID based on job ID to prevent duplicates
	opts = append(opts, asynq.TaskID(job.ID))

	return opts
}

// Helper functions

func getTimeoutFromConfig(config map[string]any) time.Duration {
	if timeout, ok := config["timeout"].(float64); ok {
		return time.Duration(timeout) * time.Second
	}
	if timeout, ok := config["timeout"].(int); ok {
		return time.Duration(timeout) * time.Second
	}
	if timeout, ok := config["timeout"].(time.Duration); ok {
		return timeout
	}
	return 30 * time.Minute // default timeout
}

func getWorkDir(job *types.Job) string {
	if workDir, ok := job.Metadata["work_dir"]; ok {
		return workDir
	}
	// Generate work dir based on job ID
	return fmt.Sprintf("/tmp/pandafuzz/jobs/%s", job.ID)
}
