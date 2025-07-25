package scheduler

import (
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/sirupsen/logrus"
)

// QueueBackend represents the type of queue backend
type QueueBackend string

const (
	// BackendMemory uses in-memory queue (legacy)
	BackendMemory QueueBackend = "memory"

	// BackendAsynq uses Redis-backed asynq queue
	BackendAsynq QueueBackend = "asynq"
)

// CreateQueue creates a queue based on the specified backend
func CreateQueue(backend string, config Config, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) (Queue, error) {
	switch QueueBackend(backend) {
	case BackendMemory:
		log.Info("Creating in-memory queue")
		return NewMemoryQueue(config, repo, processor, log), nil

	case BackendAsynq:
		log.Info("Creating asynq queue")
		return NewAsynqQueue(config, repo, processor, log)

	default:
		return nil, fmt.Errorf("unknown queue backend: %s", backend)
	}
}

// NewMemoryQueue creates the legacy in-memory queue
func NewMemoryQueue(config Config, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) Queue {
	// This is the existing implementation, just renamed
	return NewQueue(config, repo, processor, log)
}

// CreateQueueWithDefaults creates a queue with default configuration
func CreateQueueWithDefaults(backend string, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) (Queue, error) {
	config := DefaultConfig()
	return CreateQueue(backend, config, repo, processor, log)
}

// ValidateBackend checks if the backend is valid
func ValidateBackend(backend string) error {
	switch QueueBackend(backend) {
	case BackendMemory, BackendAsynq:
		return nil
	default:
		return fmt.Errorf("invalid queue backend: %s (must be 'memory' or 'asynq')", backend)
	}
}

// GetAvailableBackends returns the list of available queue backends
func GetAvailableBackends() []string {
	return []string{
		string(BackendMemory),
		string(BackendAsynq),
	}
}

// IsAsynqBackend returns true if the backend is asynq
func IsAsynqBackend(backend string) bool {
	return QueueBackend(backend) == BackendAsynq
}

// IsMemoryBackend returns true if the backend is memory
func IsMemoryBackend(backend string) bool {
	return QueueBackend(backend) == BackendMemory
}
