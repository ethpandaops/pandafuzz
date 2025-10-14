package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// JobStatus represents the status of a fuzzing job
type JobStatus string

const (
	// StatusPending indicates the job is created but not yet queued
	StatusPending JobStatus = "pending"

	// StatusQueued indicates the job is queued and waiting to be executed
	StatusQueued JobStatus = "queued"

	// StatusStarting indicates the job has been ACKed and is starting
	StatusStarting JobStatus = "starting"

	// StatusRunning indicates the job is currently executing
	StatusRunning JobStatus = "running"

	// StatusCompleted indicates the job finished successfully
	StatusCompleted JobStatus = "completed"

	// StatusFailed indicates the job failed with an error
	StatusFailed JobStatus = "failed"

	// StatusCancelled indicates the job was cancelled
	StatusCancelled JobStatus = "cancelled"

	// StatusPaused indicates the job is temporarily paused
	StatusPaused JobStatus = "paused"
)

// String returns the string representation of JobStatus
func (s JobStatus) String() string {
	return string(s)
}

// IsValid checks if the job status is valid
func (s JobStatus) IsValid() bool {
	switch s {
	case StatusPending, StatusQueued, StatusStarting, StatusRunning, StatusCompleted,
		StatusFailed, StatusCancelled, StatusPaused:
		return true
	default:
		return false
	}
}

// IsTerminal checks if the status represents a terminal state
func (s JobStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// CanTransitionTo checks if transition to the target status is allowed
func (s JobStatus) CanTransitionTo(target JobStatus) bool {
	// Define valid state transitions
	transitions := map[JobStatus][]JobStatus{
		StatusPending:  {StatusQueued, StatusStarting, StatusCancelled},
		StatusQueued:   {StatusStarting, StatusRunning, StatusCancelled},
		StatusStarting: {StatusRunning, StatusFailed, StatusCancelled},
		StatusRunning:  {StatusCompleted, StatusFailed, StatusCancelled, StatusPaused},
		StatusPaused:   {StatusRunning, StatusCancelled},
		// Terminal states cannot transition
		StatusCompleted: {},
		StatusFailed:    {},
		StatusCancelled: {},
	}

	allowedTransitions, exists := transitions[s]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == target {
			return true
		}
	}

	return false
}

// MarshalJSON implements json.Marshaler interface
func (s JobStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// UnmarshalJSON implements json.Unmarshaler interface
func (s *JobStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	status := JobStatus(str)
	if !status.IsValid() {
		return fmt.Errorf("invalid job status: %s", str)
	}

	*s = status
	return nil
}

// ParseJobStatus parses a string into a JobStatus
func ParseJobStatus(s string) (JobStatus, error) {
	status := JobStatus(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid job status: %s", s)
	}
	return status, nil
}

// JobStatusTransition represents a state transition event
type JobStatusTransition struct {
	From      JobStatus `json:"from"`
	To        JobStatus `json:"to"`
	Timestamp int64     `json:"timestamp"`
	Reason    string    `json:"reason,omitempty"`
}

// NewJobStatusTransition creates a new status transition
func NewJobStatusTransition(from, to JobStatus, reason string) (*JobStatusTransition, error) {
	if !from.CanTransitionTo(to) {
		return nil, fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	return &JobStatusTransition{
		From:      from,
		To:        to,
		Timestamp: timeNow().Unix(),
		Reason:    reason,
	}, nil
}

// Validate ensures the transition is valid
func (t *JobStatusTransition) Validate() error {
	if t.From == "" || t.To == "" {
		return errors.New("transition must have both from and to states")
	}
	if !t.From.IsValid() {
		return fmt.Errorf("invalid from status: %s", t.From)
	}
	if !t.To.IsValid() {
		return fmt.Errorf("invalid to status: %s", t.To)
	}
	if !t.From.CanTransitionTo(t.To) {
		return fmt.Errorf("invalid transition from %s to %s", t.From, t.To)
	}
	if t.Timestamp <= 0 {
		return errors.New("transition must have a valid timestamp")
	}
	return nil
}

// timeNow is a variable to allow mocking in tests
var timeNow = func() time.Time {
	return time.Now().UTC()
}
