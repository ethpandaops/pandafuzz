package sse

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"
)

// ExampleSSEServer demonstrates how to use the SSE infrastructure
func ExampleSSEServer() {
	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create SSE manager with custom configuration
	config := Config{
		MaxClients:          100,
		ClientTimeout:       5 * time.Minute,
		WriteTimeout:        10 * time.Second,
		HeartbeatInterval:   30 * time.Second,
		EventBufferSize:     1000,
		ClientBufferSize:    50,
		BroadcastBufferSize: 5000,
		MaxEventsPerSecond:  50,
		BurstSize:           100,
		CleanupInterval:     1 * time.Minute,
		MaxEventHistory:     1000,
	}

	manager := NewManager(config, logger)

	// Start the manager
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		logger.WithError(err).Fatal("failed to start SSE manager")
		return
	}
	defer manager.Stop()

	// Set up HTTP handler for SSE endpoint
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		handleSSEConnection(manager, w, r, logger)
	})

	// Set up example endpoints that generate events
	http.HandleFunc("/api/bot/register", func(w http.ResponseWriter, r *http.Request) {
		simulateBotRegistration(manager, w, r)
	})

	http.HandleFunc("/api/job/start", func(w http.ResponseWriter, r *http.Request) {
		simulateJobStart(manager, w, r)
	})

	http.HandleFunc("/api/crash/report", func(w http.ResponseWriter, r *http.Request) {
		simulateCrashReport(manager, w, r)
	})

	// Start generating background events
	go generateBackgroundEvents(manager, logger)

	logger.Info("SSE server starting on :8080")
	logger.Info("Connect to http://localhost:8080/events for SSE stream")
	logger.Info("Trigger events via:")
	logger.Info("  POST http://localhost:8080/api/bot/register")
	logger.Info("  POST http://localhost:8080/api/job/start")
	logger.Info("  POST http://localhost:8080/api/crash/report")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.WithError(err).Fatal("HTTP server failed")
	}
}

// handleSSEConnection handles new SSE client connections
func handleSSEConnection(manager *Manager, w http.ResponseWriter, r *http.Request, logger logrus.FieldLogger) {
	// Generate unique client ID
	clientID := fmt.Sprintf("client_%s", uuid.New().String()[:8])

	// Parse query parameters for filters
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	// Create client with filters
	clientConfig := DefaultClientConfig()
	client := NewClient(clientID, w, r, clientConfig, logger)

	// Add filters based on query parameters
	filter := ParseFiltersFromParams(queryParams)
	client.AddFilter(filter)

	// Subscribe to topics if specified
	if topics := r.URL.Query().Get("topics"); topics != "" {
		// This is handled by the type filter, but you could also implement
		// explicit topic subscriptions here
	}

	// Register client with manager
	if err := manager.Register(client); err != nil {
		logger.WithError(err).Error("failed to register SSE client")
		http.Error(w, "Failed to register client", http.StatusInternalServerError)
		return
	}

	// Ensure client is unregistered when connection ends
	defer func() {
		if err := manager.Unregister(client); err != nil {
			logger.WithError(err).Error("failed to unregister SSE client")
		}
	}()

	// Start serving SSE (this blocks until connection closes)
	client.ServeSSE()
}

// simulateBotRegistration simulates a bot registration and broadcasts the event
func simulateBotRegistration(manager *Manager, w http.ResponseWriter, r *http.Request) {
	botID := openapi_types.UUID(uuid.New())

	event := CreateBotRegisteredEvent(
		botID,
		"example-bot-"+botID.String()[:8],
		[]string{"fuzzing", "coverage", "analysis"},
	)

	if err := manager.Broadcast(event); err != nil {
		http.Error(w, "Failed to broadcast event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"bot_id": "%s", "status": "registered"}`, botID.String())
}

// simulateJobStart simulates a job start and broadcasts the event
func simulateJobStart(manager *Manager, w http.ResponseWriter, r *http.Request) {
	jobID := openapi_types.UUID(uuid.New())
	campaignID := openapi_types.UUID(uuid.New())

	event := NewJobEvent(EventJobStarted, jobID, campaignID, map[string]interface{}{
		"fuzzer_type": "libfuzzer",
		"timeout":     300,
		"target":      "example-target",
	})

	if err := manager.Broadcast(event); err != nil {
		http.Error(w, "Failed to broadcast event", http.StatusInternalServerError)
		return
	}

	// Simulate job progress events
	go func() {
		for progress := 10; progress <= 100; progress += 10 {
			time.Sleep(2 * time.Second)

			progressEvent := CreateJobProgressEvent(jobID, campaignID, float64(progress), map[string]interface{}{
				"executions":     progress * 100,
				"coverage_paths": progress * 5,
				"unique_crashes": progress / 20,
			})

			manager.Broadcast(progressEvent)
		}

		// Send completion event
		completionEvent := CreateJobCompletedEvent(jobID, campaignID, 20*time.Second, map[string]interface{}{
			"total_executions": 10000,
			"final_coverage":   500,
			"crashes_found":    5,
		})
		manager.Broadcast(completionEvent)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"job_id": "%s", "campaign_id": "%s", "status": "started"}`, jobID.String(), campaignID.String())
}

// simulateCrashReport simulates a crash report and broadcasts the event
func simulateCrashReport(manager *Manager, w http.ResponseWriter, r *http.Request) {
	crashID := openapi_types.UUID(uuid.New())
	jobID := openapi_types.UUID(uuid.New())
	campaignID := openapi_types.UUID(uuid.New())

	event := CreateCrashDetectedEvent(
		crashID,
		jobID,
		campaignID,
		"segmentation_fault",
		"#0 0x401234 in vulnerable_function\n#1 0x401567 in main",
	)

	if err := manager.Broadcast(event); err != nil {
		http.Error(w, "Failed to broadcast event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"crash_id": "%s", "job_id": "%s", "type": "segmentation_fault"}`,
		crashID.String(), jobID.String())
}

// generateBackgroundEvents generates periodic system events
func generateBackgroundEvents(manager *Manager, logger logrus.FieldLogger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	eventCount := 0
	for range ticker.C {
		eventCount++

		// Generate different types of events
		switch eventCount % 4 {
		case 0:
			// System health event
			event := CreateSystemAlertEvent("health_monitor", "info", "System health check completed", map[string]interface{}{
				"cpu_usage":    45.2,
				"memory_usage": 67.8,
				"disk_usage":   23.1,
			})
			manager.Broadcast(event)

		case 1:
			// Corpus sync event
			jobID := openapi_types.UUID(uuid.New())
			campaignID := openapi_types.UUID(uuid.New())
			event := CreateCorpusSyncEvent(jobID, campaignID, "sync_completed", 150)
			manager.Broadcast(event)

		case 2:
			// Campaign state change
			campaignID := openapi_types.UUID(uuid.New())
			event := CreateCampaignStateChangeEvent(campaignID, "running", "paused")
			manager.Broadcast(event)

		case 3:
			// Bot heartbeat
			botID := openapi_types.UUID(uuid.New())
			event := NewBotEvent(EventBotHeartbeat, botID, map[string]interface{}{
				"status":       "active",
				"cpu_usage":    23.4,
				"memory_usage": 45.2,
				"uptime":       "2h30m15s",
			})
			manager.Broadcast(event)
		}

		logger.WithField("event_count", eventCount).Debug("generated background event")
	}
}

// ExampleClientUsage demonstrates how to consume SSE events from a client
func ExampleClientUsage() {
	// This would typically be done in JavaScript or another client
	// Here's how you might consume the events:

	fmt.Println("Example SSE client usage (JavaScript):")
	fmt.Println(`
	const eventSource = new EventSource('/events?types=job.started,job.progress,crash.detected');

	eventSource.addEventListener('job.started', function(event) {
		const data = JSON.parse(event.data);
		console.log('Job started:', data);
	});

	eventSource.addEventListener('job.progress', function(event) {
		const data = JSON.parse(event.data);
		console.log('Job progress:', data.progress + '%');
	});

	eventSource.addEventListener('crash.detected', function(event) {
		const data = JSON.parse(event.data);
		console.log('Crash detected:', data.crash_type);
	});

	eventSource.onerror = function(event) {
		console.error('SSE error:', event);
	};
	`)
}

// ExampleFilterUsage demonstrates different filtering options
func ExampleFilterUsage() {
	logger := logrus.New()

	// Example 1: Type filter
	typeFilter := NewTypeFilter([]string{"job.started", "job.completed", "crash.detected"})

	// Example 2: Resource filter
	botID := "550e8400-e29b-41d4-a716-446655440000"
	resourceFilter := NewResourceFilter("bot", botID)

	// Example 3: Pattern filter
	patternFilter, _ := NewPatternFilter("^(job|crash)\\.")

	// Example 4: Severity filter
	severityFilter := NewSeverityFilter("warning")

	// Example 5: Compound filter (AND logic)
	compoundFilter := NewCompoundFilter("AND", typeFilter, severityFilter)

	// Example 6: Compound filter (OR logic)
	orFilter := NewCompoundFilter("OR", resourceFilter, patternFilter)

	// Test filters with sample events
	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{
		"fuzzer": "libfuzzer",
	})

	logger.WithFields(logrus.Fields{
		"type_filter":     typeFilter.Matches(jobEvent),
		"resource_filter": resourceFilter.Matches(jobEvent),
		"pattern_filter":  patternFilter.Matches(jobEvent),
		"severity_filter": severityFilter.Matches(jobEvent),
		"compound_filter": compoundFilter.Matches(jobEvent),
		"or_filter":       orFilter.Matches(jobEvent),
	}).Info("filter test results")
}
