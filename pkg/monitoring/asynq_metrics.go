package monitoring

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// AsynqMetricsCollector collects metrics from asynq and exports them to Prometheus
type AsynqMetricsCollector struct {
	inspector *asynq.Inspector
	logger    *logrus.Logger

	// Metrics
	queueSize      *prometheus.GaugeVec
	queueLatency   *prometheus.HistogramVec
	processingTime *prometheus.HistogramVec
	tasksProcessed *prometheus.CounterVec
	tasksFailed    *prometheus.CounterVec
	tasksRetried   *prometheus.CounterVec
	workersActive  prometheus.Gauge
	workersTotal   prometheus.Gauge
	scheduledTasks prometheus.Gauge
	retriedTasks   prometheus.Gauge
	archivedTasks  prometheus.Gauge
	pendingTasks   *prometheus.GaugeVec
	activeTasks    *prometheus.GaugeVec
	completedTasks *prometheus.CounterVec
}

// NewAsynqMetricsCollector creates a new metrics collector for asynq
func NewAsynqMetricsCollector(redisOpt asynq.RedisClientOpt, logger *logrus.Logger) *AsynqMetricsCollector {
	inspector := asynq.NewInspector(redisOpt)

	collector := &AsynqMetricsCollector{
		inspector: inspector,
		logger:    logger,

		queueSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_queue_size",
				Help: "Current number of tasks in queue",
			},
			[]string{"queue", "state"},
		),

		queueLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_queue_latency_seconds",
				Help:    "Time spent by tasks waiting in queue",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
			},
			[]string{"queue"},
		),

		processingTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_task_processing_seconds",
				Help:    "Time spent processing tasks",
				Buckets: prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"task_type", "status"},
		),

		tasksProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_tasks_processed_total",
				Help: "Total number of tasks processed",
			},
			[]string{"task_type", "status"},
		),

		tasksFailed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_tasks_failed_total",
				Help: "Total number of failed tasks",
			},
			[]string{"task_type", "reason"},
		),

		tasksRetried: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_tasks_retried_total",
				Help: "Total number of retried tasks",
			},
			[]string{"task_type"},
		),

		workersActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_workers_active",
				Help: "Number of active workers",
			},
		),

		workersTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_workers_total",
				Help: "Total number of workers",
			},
		),

		scheduledTasks: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_tasks_scheduled",
				Help: "Number of scheduled tasks",
			},
		),

		retriedTasks: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_tasks_retry",
				Help: "Number of tasks in retry queue",
			},
		),

		archivedTasks: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_tasks_archived",
				Help: "Number of archived tasks",
			},
		),

		pendingTasks: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_tasks_pending",
				Help: "Number of pending tasks by queue",
			},
			[]string{"queue"},
		),

		activeTasks: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_tasks_active",
				Help: "Number of active tasks by queue",
			},
			[]string{"queue"},
		),

		completedTasks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_tasks_completed_total",
				Help: "Total number of completed tasks by queue",
			},
			[]string{"queue"},
		),
	}

	// Register all metrics
	prometheus.MustRegister(
		collector.queueSize,
		collector.queueLatency,
		collector.processingTime,
		collector.tasksProcessed,
		collector.tasksFailed,
		collector.tasksRetried,
		collector.workersActive,
		collector.workersTotal,
		collector.scheduledTasks,
		collector.retriedTasks,
		collector.archivedTasks,
		collector.pendingTasks,
		collector.activeTasks,
		collector.completedTasks,
	)

	return collector
}

// Start begins collecting metrics periodically
func (c *AsynqMetricsCollector) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect metrics immediately
	c.collect()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Stopping asynq metrics collector")
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect gathers current metrics from asynq
func (c *AsynqMetricsCollector) collect() {
	// Get queue names
	queues, err := c.inspector.Queues()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get queue list")
		return
	}

	// Collect metrics for each queue
	for _, queue := range queues {
		info, err := c.inspector.GetQueueInfo(queue)
		if err != nil {
			c.logger.WithError(err).WithField("queue", queue).Error("Failed to get queue info")
			continue
		}

		// Update queue size metrics
		c.queueSize.WithLabelValues(queue, "pending").Set(float64(info.Pending))
		c.queueSize.WithLabelValues(queue, "active").Set(float64(info.Active))
		c.queueSize.WithLabelValues(queue, "scheduled").Set(float64(info.Scheduled))
		c.queueSize.WithLabelValues(queue, "retry").Set(float64(info.Retry))
		c.queueSize.WithLabelValues(queue, "archived").Set(float64(info.Archived))
		c.queueSize.WithLabelValues(queue, "completed").Set(float64(info.Completed))

		// Update individual queue metrics
		c.pendingTasks.WithLabelValues(queue).Set(float64(info.Pending))
		c.activeTasks.WithLabelValues(queue).Set(float64(info.Active))

		// Calculate queue latency from oldest pending task
		if info.Pending > 0 {
			// This would require inspecting individual tasks
			// For now, we'll use the queue info timestamp
			c.queueLatency.WithLabelValues(queue).Observe(float64(info.Latency.Seconds()))
		}
	}

	// Get overall stats by aggregating queue info
	var totalScheduled, totalRetry, totalArchived int
	for _, queue := range queues {
		info, err := c.inspector.GetQueueInfo(queue)
		if err != nil {
			continue
		}
		totalScheduled += info.Scheduled
		totalRetry += info.Retry
		totalArchived += info.Archived
	}

	// Update global metrics
	c.scheduledTasks.Set(float64(totalScheduled))
	c.retriedTasks.Set(float64(totalRetry))
	c.archivedTasks.Set(float64(totalArchived))
}

// RecordTaskProcessed records a processed task metric
func (c *AsynqMetricsCollector) RecordTaskProcessed(taskType string, status string, duration time.Duration) {
	c.tasksProcessed.WithLabelValues(taskType, status).Inc()
	c.processingTime.WithLabelValues(taskType, status).Observe(duration.Seconds())
}

// RecordTaskFailed records a failed task metric
func (c *AsynqMetricsCollector) RecordTaskFailed(taskType string, reason string) {
	c.tasksFailed.WithLabelValues(taskType, reason).Inc()
}

// RecordTaskRetried records a retried task metric
func (c *AsynqMetricsCollector) RecordTaskRetried(taskType string) {
	c.tasksRetried.WithLabelValues(taskType).Inc()
}

// SetWorkerCount sets the total number of workers
func (c *AsynqMetricsCollector) SetWorkerCount(count int) {
	c.workersTotal.Set(float64(count))
}

// Close closes the metrics collector
func (c *AsynqMetricsCollector) Close() error {
	return c.inspector.Close()
}
