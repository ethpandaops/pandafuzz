package queue

import (
	"context"
	"time"
)

// TaskQueue defines the interface for enqueueing tasks
type TaskQueue interface {
	// Enqueue adds a task to the queue for immediate processing
	Enqueue(ctx context.Context, task Task) error

	// EnqueueWithOptions adds a task with additional options like delay, priority, etc.
	EnqueueWithOptions(ctx context.Context, task Task, opts ...TaskOption) error

	// Close gracefully shuts down the queue client
	Close() error
}

// Task defines the interface for queue tasks
type Task interface {
	// Type returns the task type identifier
	Type() string

	// Payload returns the serialized task data
	Payload() []byte
}

// TaskProcessor defines the interface for processing tasks
type TaskProcessor interface {
	// ProcessTask handles the execution of a task
	ProcessTask(ctx context.Context, task Task) error
}

// TaskOption defines options that can be applied when enqueueing tasks
type TaskOption interface {
	// Apply applies the option to the task configuration
	Apply(*TaskConfig)
}

// TaskConfig holds configuration for task enqueueing
type TaskConfig struct {
	// Delay specifies how long to wait before processing the task
	Delay time.Duration

	// ProcessAt specifies when to process the task
	ProcessAt time.Time

	// MaxRetry specifies the maximum number of retry attempts
	MaxRetry int

	// Timeout specifies the maximum duration for task processing
	Timeout time.Duration

	// Queue specifies which queue to send the task to
	Queue string

	// Priority specifies the task priority (higher = more important)
	Priority int

	// UniqueID ensures only one task with this ID exists in the queue
	UniqueID string

	// RetentionPeriod specifies how long to keep the task after completion
	RetentionPeriod time.Duration
}

// TaskOptionFunc is a function that implements TaskOption
type TaskOptionFunc func(*TaskConfig)

// Apply implements the TaskOption interface
func (f TaskOptionFunc) Apply(cfg *TaskConfig) {
	f(cfg)
}

// WithDelay returns an option to delay task processing
func WithDelay(d time.Duration) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.Delay = d
	})
}

// WithProcessAt returns an option to process task at specific time
func WithProcessAt(t time.Time) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.ProcessAt = t
	})
}

// WithMaxRetry returns an option to set max retry attempts
func WithMaxRetry(n int) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.MaxRetry = n
	})
}

// WithTimeout returns an option to set task processing timeout
func WithTimeout(d time.Duration) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.Timeout = d
	})
}

// WithQueue returns an option to specify the target queue
func WithQueue(q string) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.Queue = q
	})
}

// WithPriority returns an option to set task priority
func WithPriority(p int) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.Priority = p
	})
}

// WithUniqueID returns an option to ensure task uniqueness
func WithUniqueID(id string) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.UniqueID = id
	})
}

// WithRetention returns an option to set task retention period
func WithRetention(d time.Duration) TaskOption {
	return TaskOptionFunc(func(cfg *TaskConfig) {
		cfg.RetentionPeriod = d
	})
}

// QueueStats represents statistics about queue operations
type QueueStats struct {
	// Pending is the number of tasks waiting to be processed
	Pending int64

	// Active is the number of tasks currently being processed
	Active int64

	// Scheduled is the number of tasks scheduled for future processing
	Scheduled int64

	// Retry is the number of tasks waiting to be retried
	Retry int64

	// Archived is the number of completed or failed tasks
	Archived int64

	// Processed is the total number of tasks processed
	Processed int64

	// Failed is the total number of failed tasks
	Failed int64

	// QueueSizes maps queue names to their current sizes
	QueueSizes map[string]int64
}

// WorkerServer defines the interface for task processing servers
type WorkerServer interface {
	// Start begins processing tasks from queues
	Start() error

	// Stop gracefully shuts down the server
	Stop() error

	// HandleFunc registers a handler function for a task type
	HandleFunc(taskType string, handler HandlerFunc)
}

// HandlerFunc is a function that processes tasks
type HandlerFunc func(ctx context.Context, task Task) error
