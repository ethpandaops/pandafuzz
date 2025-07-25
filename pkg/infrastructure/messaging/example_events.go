package messaging

import (
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/events"
)

// Example domain events that can be used with the messaging infrastructure

// JobStartedEvent is published when a fuzzing job starts
type JobStartedEvent struct {
	events.BaseEvent
	JobID      string `json:"job_id"`
	JobName    string `json:"job_name"`
	FuzzerType string `json:"fuzzer_type"`
	WorkerID   string `json:"worker_id"`
}

// NewJobStartedEvent creates a new job started event
func NewJobStartedEvent(jobID, jobName, fuzzerType, workerID string) *JobStartedEvent {
	return &JobStartedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "job.started",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:      jobID,
		JobName:    jobName,
		FuzzerType: fuzzerType,
		WorkerID:   workerID,
	}
}

// JobCompletedEvent is published when a fuzzing job completes
type JobCompletedEvent struct {
	events.BaseEvent
	JobID         string        `json:"job_id"`
	JobName       string        `json:"job_name"`
	CrashCount    uint64        `json:"crash_count"`
	ExecutionTime time.Duration `json:"execution_time"`
	Success       bool          `json:"success"`
	ErrorMessage  string        `json:"error_message,omitempty"`
}

// NewJobCompletedEvent creates a new job completed event
func NewJobCompletedEvent(jobID, jobName string, crashCount uint64, executionTime time.Duration, success bool, errorMessage string) *JobCompletedEvent {
	return &JobCompletedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "job.completed",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:         jobID,
		JobName:       jobName,
		CrashCount:    crashCount,
		ExecutionTime: executionTime,
		Success:       success,
		ErrorMessage:  errorMessage,
	}
}

// CrashDetectedEvent is published when a crash is detected
type CrashDetectedEvent struct {
	events.BaseEvent
	JobID      string `json:"job_id"`
	CrashID    string `json:"crash_id"`
	CrashPath  string `json:"crash_path"`
	Signal     int    `json:"signal"`
	Severity   string `json:"severity"`
	Stacktrace string `json:"stacktrace,omitempty"`
	InputHash  string `json:"input_hash"`
}

// NewCrashDetectedEvent creates a new crash detected event
func NewCrashDetectedEvent(jobID, crashID, crashPath string, signal int, severity, stacktrace, inputHash string) *CrashDetectedEvent {
	return &CrashDetectedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "crash.detected",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:      jobID,
		CrashID:    crashID,
		CrashPath:  crashPath,
		Signal:     signal,
		Severity:   severity,
		Stacktrace: stacktrace,
		InputHash:  inputHash,
	}
}

// CorpusUpdatedEvent is published when the corpus is updated
type CorpusUpdatedEvent struct {
	events.BaseEvent
	JobID            string  `json:"job_id"`
	NewSamplesCount  int     `json:"new_samples_count"`
	TotalSamples     int     `json:"total_samples"`
	CoverageIncrease float64 `json:"coverage_increase"`
}

// NewCorpusUpdatedEvent creates a new corpus updated event
func NewCorpusUpdatedEvent(jobID string, newSamplesCount, totalSamples int, coverageIncrease float64) *CorpusUpdatedEvent {
	return &CorpusUpdatedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "corpus.updated",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:            jobID,
		NewSamplesCount:  newSamplesCount,
		TotalSamples:     totalSamples,
		CoverageIncrease: coverageIncrease,
	}
}

// WorkerStatusChangedEvent is published when a worker status changes
type WorkerStatusChangedEvent struct {
	events.BaseEvent
	WorkerID       string `json:"worker_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	Reason         string `json:"reason,omitempty"`
}

// NewWorkerStatusChangedEvent creates a new worker status changed event
func NewWorkerStatusChangedEvent(workerID, previousStatus, newStatus, reason string) *WorkerStatusChangedEvent {
	return &WorkerStatusChangedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "worker.status_changed",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: workerID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		WorkerID:       workerID,
		PreviousStatus: previousStatus,
		NewStatus:      newStatus,
		Reason:         reason,
	}
}

// JobQueuedEvent is published when a job is queued
type JobQueuedEvent struct {
	events.BaseEvent
	JobID    string `json:"job_id"`
	JobName  string `json:"job_name"`
	Priority int    `json:"priority"`
	Position int    `json:"position"`
}

// NewJobQueuedEvent creates a new job queued event
func NewJobQueuedEvent(jobID, jobName string, priority, position int) *JobQueuedEvent {
	return &JobQueuedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "job.queued",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:    jobID,
		JobName:  jobName,
		Priority: priority,
		Position: position,
	}
}

// QuarantineEntryAddedEvent is published when an entry is added to quarantine
type QuarantineEntryAddedEvent struct {
	events.BaseEvent
	EntryID      string    `json:"entry_id"`
	FilePath     string    `json:"file_path"`
	Reason       string    `json:"reason"`
	Severity     string    `json:"severity"`
	SourceJobID  string    `json:"source_job_id"`
	QuarantineAt time.Time `json:"quarantine_at"`
}

// NewQuarantineEntryAddedEvent creates a new quarantine entry added event
func NewQuarantineEntryAddedEvent(entryID, filePath, reason, severity, sourceJobID string) *QuarantineEntryAddedEvent {
	return &QuarantineEntryAddedEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "quarantine.entry_added",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: entryID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		EntryID:      entryID,
		FilePath:     filePath,
		Reason:       reason,
		Severity:     severity,
		SourceJobID:  sourceJobID,
		QuarantineAt: time.Now().UTC(),
	}
}
