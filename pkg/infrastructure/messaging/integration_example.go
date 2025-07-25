package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/events"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/handlers"
)

// IntegrationExample shows how to integrate the event infrastructure with domain services
type IntegrationExample struct {
	bus      events.Bus
	registry *handlers.Registry
	logger   logrus.FieldLogger
}

// NewIntegrationExample creates a new integration example
func NewIntegrationExample(logger logrus.FieldLogger) (*IntegrationExample, error) {
	if logger == nil {
		logger = logrus.New().WithField("component", "event-integration")
	}

	// Create event bus
	busConfig := events.BusConfig{
		BufferSize:     1000,
		Workers:        5,
		MaxRetries:     3,
		RetryDelay:     100,
		HandlerTimeout: 30,
		Logger:         logger,
	}
	bus := events.NewBus(busConfig)

	// Create handler registry
	registry := handlers.NewRegistry(logger)

	// Add interceptors
	registry.AddInterceptor(handlers.NewLoggingInterceptor(logger))

	return &IntegrationExample{
		bus:      bus,
		registry: registry,
		logger:   logger,
	}, nil
}

// Start starts the event system
func (e *IntegrationExample) Start(ctx context.Context) error {
	// Register domain event handlers
	if err := e.registerHandlers(); err != nil {
		return fmt.Errorf("failed to register handlers: %w", err)
	}

	// Subscribe handlers to the bus
	if err := e.subscribeHandlers(); err != nil {
		return fmt.Errorf("failed to subscribe handlers: %w", err)
	}

	// Start the event bus
	if err := e.bus.Start(ctx); err != nil {
		return fmt.Errorf("failed to start event bus: %w", err)
	}

	e.logger.Info("Event system started successfully")
	return nil
}

// Stop stops the event system
func (e *IntegrationExample) Stop() error {
	return e.bus.Stop()
}

// GetBus returns the event bus for publishing events
func (e *IntegrationExample) GetBus() events.Bus {
	return e.bus
}

// registerHandlers registers all domain event handlers
func (e *IntegrationExample) registerHandlers() error {
	// Job lifecycle handlers
	e.registry.Register("job.started",
		events.NewHandlerWrapper("job.started", e.handleJobStarted),
		handlers.WithPriority(10),
		handlers.WithDescription("Handles job start events"),
	)

	e.registry.Register("job.completed",
		events.NewHandlerWrapper("job.completed", e.handleJobCompleted),
		handlers.WithPriority(10),
		handlers.WithDescription("Handles job completion events"),
	)

	// Crash detection handlers
	e.registry.Register("crash.detected",
		events.NewHandlerWrapper("crash.detected", e.handleCrashDetected),
		handlers.WithPriority(20), // Higher priority for crashes
		handlers.WithDescription("Handles crash detection events"),
	)

	// Corpus update handlers
	e.registry.Register("corpus.updated",
		events.NewHandlerWrapper("corpus.updated", e.handleCorpusUpdated),
		handlers.WithPriority(5),
		handlers.WithDescription("Handles corpus update events"),
	)

	// Quarantine handlers
	e.registry.Register("quarantine.entry_added",
		events.NewHandlerWrapper("quarantine.entry_added", e.handleQuarantineEntry),
		handlers.WithPriority(15),
		handlers.WithDescription("Handles quarantine entries"),
	)

	return nil
}

// subscribeHandlers subscribes registry handlers to the bus
func (e *IntegrationExample) subscribeHandlers() error {
	allHandlers := e.registry.ListHandlers()

	for eventType := range allHandlers {
		handler := e.registry.CreateBusHandler(eventType)
		if err := e.bus.Subscribe(eventType, handler); err != nil {
			return fmt.Errorf("failed to subscribe handler for %s: %w", eventType, err)
		}
	}

	return nil
}

// Event handlers

func (e *IntegrationExample) handleJobStarted(ctx context.Context, event events.Event) error {
	jobEvent, ok := event.(*JobStartedEvent)
	if !ok {
		return fmt.Errorf("invalid event type for job.started")
	}

	e.logger.WithFields(logrus.Fields{
		"job_id":      jobEvent.JobID,
		"job_name":    jobEvent.JobName,
		"fuzzer_type": jobEvent.FuzzerType,
		"worker_id":   jobEvent.WorkerID,
	}).Info("Processing job started event")

	// Here you would typically:
	// - Update job status in database
	// - Notify monitoring systems
	// - Initialize metrics collection
	// - Send notifications to interested parties

	return nil
}

func (e *IntegrationExample) handleJobCompleted(ctx context.Context, event events.Event) error {
	jobEvent, ok := event.(*JobCompletedEvent)
	if !ok {
		return fmt.Errorf("invalid event type for job.completed")
	}

	e.logger.WithFields(logrus.Fields{
		"job_id":         jobEvent.JobID,
		"job_name":       jobEvent.JobName,
		"crash_count":    jobEvent.CrashCount,
		"execution_time": jobEvent.ExecutionTime,
		"success":        jobEvent.Success,
	}).Info("Processing job completed event")

	// Here you would typically:
	// - Update job status and statistics
	// - Archive job results
	// - Trigger post-processing workflows
	// - Clean up resources
	// - Send completion notifications

	return nil
}

func (e *IntegrationExample) handleCrashDetected(ctx context.Context, event events.Event) error {
	crashEvent, ok := event.(*CrashDetectedEvent)
	if !ok {
		return fmt.Errorf("invalid event type for crash.detected")
	}

	e.logger.WithFields(logrus.Fields{
		"job_id":     crashEvent.JobID,
		"crash_id":   crashEvent.CrashID,
		"crash_path": crashEvent.CrashPath,
		"severity":   crashEvent.Severity,
		"signal":     crashEvent.Signal,
	}).Warn("Processing crash detected event")

	// Here you would typically:
	// - Store crash information in database
	// - Trigger crash analysis workflows
	// - Update crash statistics
	// - Send alerts for high-severity crashes
	// - Queue crash for deduplication

	// Example: If high severity, publish additional alert event
	if crashEvent.Severity == "high" || crashEvent.Severity == "critical" {
		// This shows how handlers can publish new events
		alertEvent := NewCrashAlertEvent(crashEvent.JobID, crashEvent.CrashID, crashEvent.Severity)
		if err := e.bus.PublishAsync(ctx, alertEvent); err != nil {
			e.logger.WithError(err).Error("Failed to publish crash alert")
		}
	}

	return nil
}

func (e *IntegrationExample) handleCorpusUpdated(ctx context.Context, event events.Event) error {
	corpusEvent, ok := event.(*CorpusUpdatedEvent)
	if !ok {
		return fmt.Errorf("invalid event type for corpus.updated")
	}

	e.logger.WithFields(logrus.Fields{
		"job_id":            corpusEvent.JobID,
		"new_samples":       corpusEvent.NewSamplesCount,
		"total_samples":     corpusEvent.TotalSamples,
		"coverage_increase": corpusEvent.CoverageIncrease,
	}).Info("Processing corpus updated event")

	// Here you would typically:
	// - Update corpus statistics
	// - Trigger corpus synchronization
	// - Update coverage metrics
	// - Archive new corpus entries

	return nil
}

func (e *IntegrationExample) handleQuarantineEntry(ctx context.Context, event events.Event) error {
	quarantineEvent, ok := event.(*QuarantineEntryAddedEvent)
	if !ok {
		return fmt.Errorf("invalid event type for quarantine.entry_added")
	}

	e.logger.WithFields(logrus.Fields{
		"entry_id":   quarantineEvent.EntryID,
		"file_path":  quarantineEvent.FilePath,
		"reason":     quarantineEvent.Reason,
		"severity":   quarantineEvent.Severity,
		"source_job": quarantineEvent.SourceJobID,
	}).Warn("Processing quarantine entry event")

	// Here you would typically:
	// - Update quarantine database
	// - Notify security team
	// - Update quarantine metrics
	// - Trigger security analysis workflows

	return nil
}

// CrashAlertEvent is an example of a derived event
type CrashAlertEvent struct {
	events.BaseEvent
	JobID    string `json:"job_id"`
	CrashID  string `json:"crash_id"`
	Severity string `json:"severity"`
}

// NewCrashAlertEvent creates a new crash alert event
func NewCrashAlertEvent(jobID, crashID, severity string) *CrashAlertEvent {
	return &CrashAlertEvent{
		BaseEvent: events.BaseEvent{
			EventType:    "crash.alert",
			EventTime:    time.Now().UTC(),
			EventID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			AggregateID_: jobID,
			Version_:     1,
			Metadata_:    make(map[string]string),
		},
		JobID:    jobID,
		CrashID:  crashID,
		Severity: severity,
	}
}

// DomainEventPublisher is an interface that domain services can use to publish events
type DomainEventPublisher interface {
	PublishJobStarted(jobID, jobName, fuzzerType, workerID string) error
	PublishJobCompleted(jobID, jobName string, crashCount uint64, executionTime time.Duration, success bool, errorMessage string) error
	PublishCrashDetected(jobID, crashID, crashPath string, signal int, severity, stacktrace, inputHash string) error
	PublishCorpusUpdated(jobID string, newSamplesCount, totalSamples int, coverageIncrease float64) error
	PublishQuarantineEntryAdded(entryID, filePath, reason, severity, sourceJobID string) error
}

// domainEventPublisher implements DomainEventPublisher
type domainEventPublisher struct {
	bus    events.Publisher
	logger logrus.FieldLogger
}

// NewDomainEventPublisher creates a new domain event publisher
func NewDomainEventPublisher(bus events.Publisher, logger logrus.FieldLogger) DomainEventPublisher {
	if logger == nil {
		logger = logrus.New().WithField("component", "domain-event-publisher")
	}
	return &domainEventPublisher{
		bus:    bus,
		logger: logger,
	}
}

func (p *domainEventPublisher) PublishJobStarted(jobID, jobName, fuzzerType, workerID string) error {
	event := NewJobStartedEvent(jobID, jobName, fuzzerType, workerID)
	return p.bus.PublishAsync(context.Background(), event)
}

func (p *domainEventPublisher) PublishJobCompleted(jobID, jobName string, crashCount uint64, executionTime time.Duration, success bool, errorMessage string) error {
	event := NewJobCompletedEvent(jobID, jobName, crashCount, executionTime, success, errorMessage)
	return p.bus.PublishAsync(context.Background(), event)
}

func (p *domainEventPublisher) PublishCrashDetected(jobID, crashID, crashPath string, signal int, severity, stacktrace, inputHash string) error {
	event := NewCrashDetectedEvent(jobID, crashID, crashPath, signal, severity, stacktrace, inputHash)
	return p.bus.PublishAsync(context.Background(), event)
}

func (p *domainEventPublisher) PublishCorpusUpdated(jobID string, newSamplesCount, totalSamples int, coverageIncrease float64) error {
	event := NewCorpusUpdatedEvent(jobID, newSamplesCount, totalSamples, coverageIncrease)
	return p.bus.PublishAsync(context.Background(), event)
}

func (p *domainEventPublisher) PublishQuarantineEntryAdded(entryID, filePath, reason, severity, sourceJobID string) error {
	event := NewQuarantineEntryAddedEvent(entryID, filePath, reason, severity, sourceJobID)
	return p.bus.PublishAsync(context.Background(), event)
}
