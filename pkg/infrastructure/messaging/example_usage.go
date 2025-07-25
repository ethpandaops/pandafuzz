package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/events"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/messaging/handlers"
)

// ExampleUsage demonstrates how to use the event infrastructure
func ExampleUsage() {
	// Create logger
	logger := logrus.New().WithField("component", "event-example")

	// Create event bus with configuration
	busConfig := events.BusConfig{
		BufferSize:     1000,
		Workers:        5,
		MaxRetries:     3,
		RetryDelay:     100 * time.Millisecond,
		HandlerTimeout: 10 * time.Second,
		Logger:         logger,
	}
	bus := events.NewBus(busConfig)

	// Start the bus
	ctx := context.Background()
	if err := bus.Start(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to start event bus")
	}

	// Create handler registry
	registry := handlers.NewRegistry(logger)

	// Add interceptors
	registry.AddInterceptor(handlers.NewLoggingInterceptor(logger))
	metricsInterceptor := handlers.NewMetricsInterceptor()
	registry.AddInterceptor(metricsInterceptor)

	// Register handlers using the registry
	registerHandlers(registry, logger)

	// Subscribe registry handlers to the bus
	subscribeHandlers(bus, registry)

	// Publish some events
	publishExampleEvents(ctx, bus)

	// Wait for events to be processed
	time.Sleep(2 * time.Second)

	// Print metrics
	fmt.Println("Handler Metrics:", metricsInterceptor.GetMetrics())

	// Stop the bus
	if err := bus.Stop(); err != nil {
		logger.WithError(err).Error("Failed to stop event bus")
	}
}

// registerHandlers registers various event handlers
func registerHandlers(registry *handlers.Registry, logger logrus.FieldLogger) {
	// Job started handler
	jobStartedHandler := func(ctx context.Context, event events.Event) error {
		jobEvent, ok := event.(*JobStartedEvent)
		if !ok {
			return fmt.Errorf("invalid event type")
		}

		logger.WithFields(logrus.Fields{
			"job_id":      jobEvent.JobID,
			"job_name":    jobEvent.JobName,
			"fuzzer_type": jobEvent.FuzzerType,
			"worker_id":   jobEvent.WorkerID,
		}).Info("Job started")

		return nil
	}

	registry.Register("job.started",
		events.NewHandlerWrapper("job.started", jobStartedHandler),
		handlers.WithPriority(10),
		handlers.WithDescription("Handles job start events"),
	)

	// Job completed handler
	jobCompletedHandler := func(ctx context.Context, event events.Event) error {
		jobEvent, ok := event.(*JobCompletedEvent)
		if !ok {
			return fmt.Errorf("invalid event type")
		}

		logger.WithFields(logrus.Fields{
			"job_id":         jobEvent.JobID,
			"job_name":       jobEvent.JobName,
			"crash_count":    jobEvent.CrashCount,
			"execution_time": jobEvent.ExecutionTime,
			"success":        jobEvent.Success,
		}).Info("Job completed")

		return nil
	}

	registry.Register("job.completed",
		events.NewHandlerWrapper("job.completed", jobCompletedHandler),
		handlers.WithPriority(10),
		handlers.WithDescription("Handles job completion events"),
	)

	// Crash detected handler
	crashHandler := func(ctx context.Context, event events.Event) error {
		crashEvent, ok := event.(*CrashDetectedEvent)
		if !ok {
			return fmt.Errorf("invalid event type")
		}

		logger.WithFields(logrus.Fields{
			"job_id":     crashEvent.JobID,
			"crash_id":   crashEvent.CrashID,
			"crash_path": crashEvent.CrashPath,
			"signal":     crashEvent.Signal,
			"severity":   crashEvent.Severity,
		}).Warn("Crash detected")

		// Simulate crash analysis
		time.Sleep(50 * time.Millisecond)

		return nil
	}

	registry.Register("crash.detected",
		events.NewHandlerWrapper("crash.detected", crashHandler),
		handlers.WithPriority(20), // Higher priority for crash handling
		handlers.WithDescription("Handles crash detection events"),
	)

	// Wildcard handler that logs all events
	wildcardHandler := func(ctx context.Context, event events.Event) error {
		logger.WithFields(logrus.Fields{
			"event_type":   event.Type(),
			"aggregate_id": event.AggregateID(),
			"timestamp":    event.Timestamp(),
		}).Debug("Event received")
		return nil
	}

	registry.Register("*",
		events.NewHandlerWrapper("*", wildcardHandler),
		handlers.WithPriority(0), // Lowest priority
		handlers.WithDescription("Logs all events"),
	)
}

// subscribeHandlers subscribes registry handlers to the bus
func subscribeHandlers(bus events.Bus, registry *handlers.Registry) {
	// Get all registered event types
	allHandlers := registry.ListHandlers()

	for eventType := range allHandlers {
		// Create a composite handler for each event type
		handler := registry.CreateBusHandler(eventType)
		if err := bus.Subscribe(eventType, handler); err != nil {
			panic(fmt.Sprintf("Failed to subscribe handler: %v", err))
		}
	}
}

// publishExampleEvents publishes various example events
func publishExampleEvents(ctx context.Context, bus events.Publisher) {
	// Job started event
	jobStarted := NewJobStartedEvent("job-123", "test-fuzzing", "libfuzzer", "worker-1")
	jobStarted.SetMetadata("environment", "testing")

	if err := bus.Publish(ctx, jobStarted); err != nil {
		fmt.Printf("Failed to publish job started event: %v\n", err)
	}

	// Simulate some work
	time.Sleep(100 * time.Millisecond)

	// Crash detected event
	crashDetected := NewCrashDetectedEvent(
		"job-123",
		"crash-456",
		"/output/crashes/crash-456",
		11, // SIGSEGV
		"high",
		"stack trace here...",
		"abc123",
	)

	if err := bus.PublishAsync(ctx, crashDetected); err != nil {
		fmt.Printf("Failed to publish crash detected event: %v\n", err)
	}

	// Corpus updated event
	corpusUpdated := NewCorpusUpdatedEvent("job-123", 5, 150, 2.5)

	if err := bus.PublishAsync(ctx, corpusUpdated); err != nil {
		fmt.Printf("Failed to publish corpus updated event: %v\n", err)
	}

	// Job completed event
	jobCompleted := NewJobCompletedEvent(
		"job-123",
		"test-fuzzing",
		3,
		5*time.Minute,
		true,
		"",
	)

	if err := bus.Publish(ctx, jobCompleted); err != nil {
		fmt.Printf("Failed to publish job completed event: %v\n", err)
	}
}

// ExampleWithFilters demonstrates using event filters
func ExampleWithFilters() {
	logger := logrus.New()
	bus := events.NewBus(events.DefaultBusConfig())

	// Create a type filter to only process specific events
	typeFilter := &events.TypeFilter{
		EventTypes: []string{"job.started", "job.completed"},
	}

	// Create an aggregate filter
	aggregateFilter := &events.AggregateFilter{
		AggregateID: "job-123",
	}

	// Create a composite filter
	compositeFilter := &events.CompositeFilter{
		Filters: []events.Filter{typeFilter, aggregateFilter},
	}

	// Apply filter to a handler
	filteredHandler := &filterHandler{
		handler: events.NewHandlerWrapper("*", func(ctx context.Context, event events.Event) error {
			logger.WithField("event_type", event.Type()).Info("Filtered event received")
			return nil
		}),
		filter: compositeFilter,
	}

	// Use the filtered handler
	bus.Subscribe("*", filteredHandler)
}

// filterHandler wraps a handler with filtering
type filterHandler struct {
	handler events.Handler
	filter  events.Filter
}

func (h *filterHandler) Handle(ctx context.Context, event events.Event) error {
	if !h.filter.Apply(event) {
		return nil
	}
	return h.handler.Handle(ctx, event)
}

func (h *filterHandler) CanHandle(eventType string) bool {
	return h.handler.CanHandle(eventType)
}

// ExampleDirectPublisher demonstrates using the direct publisher
func ExampleDirectPublisher() {
	logger := logrus.New()

	// Create a direct publisher for synchronous event handling
	publisher := events.NewDirectPublisher(logger)

	// Cast to access handler methods
	directPub, ok := publisher.(*events.DirectPublisher)
	if !ok {
		// Handle the type properly in production code
		return
	}

	// Add handlers directly
	directPub.AddHandler("job.started", events.NewHandlerWrapper("job.started",
		func(ctx context.Context, event events.Event) error {
			fmt.Printf("Direct handler: %s\n", event.Type())
			return nil
		}),
	)

	// Publish event
	event := NewJobStartedEvent("job-789", "direct-test", "afl", "worker-2")
	ctx := context.Background()

	if err := publisher.Publish(ctx, event); err != nil {
		fmt.Printf("Failed to publish: %v\n", err)
	}
}
