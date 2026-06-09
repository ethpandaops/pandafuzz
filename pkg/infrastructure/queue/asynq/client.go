package asynq

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/queue"
	"github.com/hibiken/asynq"
	"github.com/sirupsen/logrus"
)

// Client wraps asynq client with pandafuzz-specific functionality
type Client struct {
	client    *asynq.Client
	inspector *asynq.Inspector
	logger    logrus.FieldLogger
	config    *config.QueueConfig
}

// NewClient creates a new asynq client wrapper
func NewClient(redisCfg *config.RedisConfig, queueCfg *config.QueueConfig, logger logrus.FieldLogger) (*Client, error) {
	redisOpt, err := NewRedisOpt(redisCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis options: %w", err)
	}

	client := asynq.NewClient(redisOpt)

	inspectorOpt, err := NewInspectorOpt(redisCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create inspector options: %w", err)
	}

	inspector := asynq.NewInspector(inspectorOpt)

	return &Client{
		client:    client,
		inspector: inspector,
		logger:    logger.WithField("component", "asynq-client"),
		config:    queueCfg,
	}, nil
}

// EnqueueJob enqueues a pandafuzz job as an asynq task
func (c *Client) EnqueueJob(job *types.Job, opts ...asynq.Option) error {
	task, err := NewFuzzingTask(job)
	if err != nil {
		return fmt.Errorf("failed to create task from job: %w", err)
	}

	// Merge default options with provided options
	defaultOpts := GetTaskOptions(job)
	allOpts := append(defaultOpts, opts...)

	info, err := c.client.Enqueue(task, allOpts...)
	if err != nil {
		c.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to enqueue job")
		return fmt.Errorf("failed to enqueue job %s: %w", job.ID, err)
	}

	c.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"task_id":     info.ID,
		"queue":       info.Queue,
		"max_retry":   info.MaxRetry,
		"retry_count": info.Retried,
	}).Info("Job enqueued successfully")

	return nil
}

// GetQueueStats returns statistics for a specific queue
func (c *Client) GetQueueStats(queueName string) (*queue.QueueStats, error) {
	info, err := c.inspector.GetQueueInfo(queueName)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue info: %w", err)
	}

	return &queue.QueueStats{
		Pending:   int64(info.Pending),
		Active:    int64(info.Active),
		Scheduled: int64(info.Scheduled),
		Retry:     int64(info.Retry),
		Archived:  int64(info.Archived),
		Processed: int64(info.Processed),
		Failed:    int64(info.Failed),
		QueueSizes: map[string]int64{
			queueName: int64(info.Size),
		},
	}, nil
}

// GetAllQueueStats returns combined statistics for all queues
func (c *Client) GetAllQueueStats() (*queue.QueueStats, error) {
	stats := &queue.QueueStats{
		QueueSizes: make(map[string]int64),
	}

	for queueName := range c.config.Queues {
		info, err := c.inspector.GetQueueInfo(queueName)
		if err != nil {
			c.logger.WithError(err).WithField("queue", queueName).Warn("Failed to get queue info")
			continue
		}

		stats.Pending += int64(info.Pending)
		stats.Active += int64(info.Active)
		stats.Scheduled += int64(info.Scheduled)
		stats.Retry += int64(info.Retry)
		stats.Archived += int64(info.Archived)
		stats.Processed += int64(info.Processed)
		stats.Failed += int64(info.Failed)
		stats.QueueSizes[queueName] = int64(info.Size)
	}

	return stats, nil
}

// CancelTask cancels a task by ID
func (c *Client) CancelTask(taskID string) error {
	if err := c.inspector.CancelProcessing(taskID); err != nil {
		return fmt.Errorf("failed to cancel task %s: %w", taskID, err)
	}

	c.logger.WithField("task_id", taskID).Info("Task cancelled")
	return nil
}

// DeleteTask removes a task from queue
func (c *Client) DeleteTask(queue, taskID string) error {
	if err := c.inspector.DeleteTask(queue, taskID); err != nil {
		return fmt.Errorf("failed to delete task %s from queue %s: %w", taskID, queue, err)
	}

	c.logger.WithFields(logrus.Fields{
		"task_id": taskID,
		"queue":   queue,
	}).Info("Task deleted")

	return nil
}

// PauseQueue pauses processing of a queue
func (c *Client) PauseQueue(queueName string) error {
	if err := c.inspector.PauseQueue(queueName); err != nil {
		return fmt.Errorf("failed to pause queue %s: %w", queueName, err)
	}

	c.logger.WithField("queue", queueName).Info("Queue paused")
	return nil
}

// UnpauseQueue resumes processing of a queue
func (c *Client) UnpauseQueue(queueName string) error {
	if err := c.inspector.UnpauseQueue(queueName); err != nil {
		return fmt.Errorf("failed to unpause queue %s: %w", queueName, err)
	}

	c.logger.WithField("queue", queueName).Info("Queue unpaused")
	return nil
}

// ListPendingTasks returns pending tasks from a queue
func (c *Client) ListPendingTasks(queueName string, page, pageSize int) ([]*asynq.TaskInfo, error) {
	tasks, err := c.inspector.ListPendingTasks(queueName, asynq.PageSize(pageSize), asynq.Page(page))
	if err != nil {
		return nil, fmt.Errorf("failed to list pending tasks: %w", err)
	}

	return tasks, nil
}

// ListActiveTasks returns currently processing tasks
func (c *Client) ListActiveTasks(queue string, page, pageSize int) ([]*asynq.TaskInfo, error) {
	tasks, err := c.inspector.ListActiveTasks(queue)
	if err != nil {
		return nil, fmt.Errorf("failed to list active tasks: %w", err)
	}

	return tasks, nil
}

// ListScheduledTasks returns scheduled tasks
func (c *Client) ListScheduledTasks(queue string, page, pageSize int) ([]*asynq.TaskInfo, error) {
	tasks, err := c.inspector.ListScheduledTasks(queue)
	if err != nil {
		return nil, fmt.Errorf("failed to list scheduled tasks: %w", err)
	}

	return tasks, nil
}

// GetTaskInfo returns information about a specific task
func (c *Client) GetTaskInfo(queue, taskID string) (*asynq.TaskInfo, error) {
	info, err := c.inspector.GetTaskInfo(queue, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}

	return info, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close client: %w", err)
	}

	c.logger.Info("Client closed")
	return nil
}

// ScheduleTask schedules a task for future execution
func (c *Client) ScheduleTask(ctx context.Context, task queue.Task, processAt time.Time) error {
	// Convert to asynq task
	asynqTask := asynq.NewTask(task.Type(), task.Payload())

	opts := []asynq.Option{
		asynq.ProcessAt(processAt),
	}

	info, err := c.client.EnqueueContext(ctx, asynqTask, opts...)
	if err != nil {
		return fmt.Errorf("failed to schedule task: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"task_id":    info.ID,
		"task_type":  task.Type(),
		"process_at": processAt,
	}).Info("Task scheduled")

	return nil
}

// Ping checks if Redis is reachable
func (c *Client) Ping() error {
	// Use inspector to check Redis connection
	_, err := c.inspector.Servers()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}
