package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/monitoring"
	"github.com/sirupsen/logrus"
)

// MonitoringService handles metrics collection and monitoring
type MonitoringService interface {
	// Start begins collecting metrics
	Start(ctx context.Context) error
	
	// GetCollector returns the metrics collector
	GetCollector() *monitoring.Collector
	
	// UpdateSystemMetrics updates system-wide metrics
	UpdateSystemMetrics(ctx context.Context) error
}

// monitoringService implements MonitoringService interface
type monitoringService struct {
	collector       *monitoring.Collector
	stateStore      StateStore
	logger          *logrus.Logger
	updateInterval  time.Duration
}

// Compile-time interface compliance check
var _ MonitoringService = (*monitoringService)(nil)

// NewMonitoringService creates a new monitoring service
func NewMonitoringService(
	stateStore StateStore,
	logger *logrus.Logger,
) MonitoringService {
	return &monitoringService{
		collector:      monitoring.NewCollector(logger),
		stateStore:     stateStore,
		logger:         logger,
		updateInterval: 30 * time.Second,
	}
}

// Start begins collecting metrics
func (s *monitoringService) Start(ctx context.Context) error {
	// Start periodic metrics updates
	ticker := time.NewTicker(s.updateInterval)
	defer ticker.Stop()
	
	// Initial update
	if err := s.UpdateSystemMetrics(ctx); err != nil {
		s.logger.WithError(err).Error("Failed to update initial metrics")
	}
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.UpdateSystemMetrics(ctx); err != nil {
				s.logger.WithError(err).Error("Failed to update metrics")
			}
		}
	}
}

// GetCollector returns the metrics collector
func (s *monitoringService) GetCollector() *monitoring.Collector {
	return s.collector
}

// UpdateSystemMetrics updates system-wide metrics
func (s *monitoringService) UpdateSystemMetrics(ctx context.Context) error {
	// Update bot metrics
	if err := s.updateBotMetrics(); err != nil {
		return err
	}
	
	// Update job metrics
	if err := s.updateJobMetrics(); err != nil {
		return err
	}
	
	return nil
}

// updateBotMetrics updates bot-related metrics
func (s *monitoringService) updateBotMetrics() error {
	bots, err := s.stateStore.ListBots()
	if err != nil {
		return err
	}
	
	// Count bots by status
	statusCounts := make(map[string]int)
	for _, bot := range bots {
		status := string(bot.Status)
		statusCounts[status]++
	}
	
	s.collector.UpdateBotGauge(statusCounts)
	return nil
}

// updateJobMetrics updates job-related metrics
func (s *monitoringService) updateJobMetrics() error {
	jobs, err := s.stateStore.ListJobs()
	if err != nil {
		return err
	}
	
	// Count jobs by status and fuzzer
	activeByFuzzer := make(map[string]int)
	queueSize := 0
	
	for _, job := range jobs {
		if job.Status == common.JobStatusRunning {
			activeByFuzzer[job.Fuzzer]++
		} else if job.Status == common.JobStatusPending {
			queueSize++
		}
	}
	
	s.collector.UpdateActiveJobs(activeByFuzzer)
	s.collector.UpdateJobQueueSize(queueSize)
	
	return nil
}

// MonitoringAwareBotService wraps BotService with monitoring
type MonitoringAwareBotService struct {
	BotService
	collector *monitoring.Collector
}

// Compile-time interface compliance check
var _ BotService = (*MonitoringAwareBotService)(nil)

// NewMonitoringAwareBotService creates a bot service with monitoring
func NewMonitoringAwareBotService(base BotService, collector *monitoring.Collector) BotService {
	return &MonitoringAwareBotService{
		BotService: base,
		collector:  collector,
	}
}

// Start delegates to the wrapped service
func (s *MonitoringAwareBotService) Start(ctx context.Context) error {
	return s.BotService.Start(ctx)
}

// Stop delegates to the wrapped service
func (s *MonitoringAwareBotService) Stop() error {
	return s.BotService.Stop()
}

// UpdateHeartbeat with monitoring
func (s *MonitoringAwareBotService) UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	// Get bot's last seen time before update
	bot, err := s.GetBot(ctx, botID)
	if err != nil {
		return err
	}
	
	// Update heartbeat
	err = s.BotService.UpdateHeartbeat(ctx, botID, status, currentJob)
	if err != nil {
		return err
	}
	
	// Record metrics
	s.collector.RecordBotHeartbeat(botID, string(status), bot.LastSeen)
	
	return nil
}

// MonitoringAwareJobService wraps JobService with monitoring
type MonitoringAwareJobService struct {
	JobService
	collector *monitoring.Collector
}

// Compile-time interface compliance check
var _ JobService = (*MonitoringAwareJobService)(nil)

// NewMonitoringAwareJobService creates a job service with monitoring
func NewMonitoringAwareJobService(base JobService, collector *monitoring.Collector) JobService {
	return &MonitoringAwareJobService{
		JobService: base,
		collector:  collector,
	}
}

// Start delegates to the wrapped service
func (s *MonitoringAwareJobService) Start(ctx context.Context) error {
	return s.JobService.Start(ctx)
}

// Stop delegates to the wrapped service
func (s *MonitoringAwareJobService) Stop() error {
	return s.JobService.Stop()
}

// CreateJob with monitoring
func (s *MonitoringAwareJobService) CreateJob(ctx context.Context, req CreateJobRequest) (*common.Job, error) {
	job, err := s.JobService.CreateJob(ctx, req)
	if err != nil {
		return nil, err
	}
	
	// Record metrics
	s.collector.RecordJobCreated(job.Fuzzer)
	
	return job, nil
}

// CompleteJob with monitoring
func (s *MonitoringAwareJobService) CompleteJob(ctx context.Context, jobID, botID string, success bool) error {
	// Get job info before completion
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	
	// Complete job
	err = s.JobService.CompleteJob(ctx, jobID, botID, success)
	if err != nil {
		return err
	}
	
	// Calculate duration
	var duration time.Duration
	if job.StartedAt != nil {
		duration = time.Since(*job.StartedAt)
	}
	
	// Record metrics
	s.collector.RecordJobCompleted(job.Fuzzer, success, duration)
	
	return nil
}

// MonitoringAwareResultService wraps ResultService with monitoring
type MonitoringAwareResultService struct {
	ResultService
	collector *monitoring.Collector
}

// Compile-time interface compliance check
var _ ResultService = (*MonitoringAwareResultService)(nil)

// NewMonitoringAwareResultService creates a result service with monitoring
func NewMonitoringAwareResultService(base ResultService, collector *monitoring.Collector) ResultService {
	return &MonitoringAwareResultService{
		ResultService: base,
		collector:     collector,
	}
}

// Start delegates to the wrapped service
func (s *MonitoringAwareResultService) Start(ctx context.Context) error {
	return s.ResultService.Start(ctx)
}

// Stop delegates to the wrapped service
func (s *MonitoringAwareResultService) Stop() error {
	return s.ResultService.Stop()
}

// ProcessCrashResult with monitoring
func (s *MonitoringAwareResultService) ProcessCrashResult(ctx context.Context, crash *common.CrashResult) error {
	err := s.ResultService.ProcessCrashResult(ctx, crash)
	if err != nil {
		return err
	}
	
	// Record metrics
	s.collector.RecordCrash(crash.JobID, crash.Type, crash.IsUnique)
	
	return nil
}

// ProcessCoverageResult with monitoring
func (s *MonitoringAwareResultService) ProcessCoverageResult(ctx context.Context, coverage *common.CoverageResult) error {
	err := s.ResultService.ProcessCoverageResult(ctx, coverage)
	if err != nil {
		return err
	}
	
	// Record metrics
	s.collector.UpdateCoverage(coverage.JobID, int64(coverage.Edges), int64(coverage.NewEdges))
	
	// Update execution rate if available
	if coverage.ExecCount > 0 && !coverage.Timestamp.IsZero() {
		// This is a simplified calculation
		s.collector.UpdateExecRate(coverage.JobID, coverage.BotID, float64(coverage.ExecCount))
	}
	
	return nil
}

// ProcessCorpusUpdate with monitoring
func (s *MonitoringAwareResultService) ProcessCorpusUpdate(ctx context.Context, corpus *common.CorpusUpdate) error {
	err := s.ResultService.ProcessCorpusUpdate(ctx, corpus)
	if err != nil {
		return err
	}
	
	// Record metrics
	s.collector.UpdateCorpusSize(corpus.JobID, corpus.TotalSize)
	
	return nil
}