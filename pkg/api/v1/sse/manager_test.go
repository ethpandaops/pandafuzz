package sse

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Lifecycle(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce noise during tests

	config := DefaultConfig()
	config.HeartbeatInterval = 100 * time.Millisecond
	config.CleanupInterval = 100 * time.Millisecond

	manager := NewManager(config, logger)

	// Test starting the manager
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Test starting already running manager
	err = manager.Start(ctx)
	assert.Equal(t, ErrManagerAlreadyRunning, err)

	// Test stopping the manager
	err = manager.Stop()
	require.NoError(t, err)

	// Test stopping already stopped manager
	err = manager.Stop()
	assert.Equal(t, ErrManagerNotRunning, err)
}

func TestManager_ClientRegistration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	config := DefaultConfig()
	config.MaxClients = 2 // Limit for testing

	manager := NewManager(config, logger)
	ctx := context.Background()
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop()

	// Create mock HTTP response writer
	w1 := httptest.NewRecorder()
	w2 := httptest.NewRecorder()
	w3 := httptest.NewRecorder()

	req := httptest.NewRequest("GET", "/events", nil)

	// Create clients
	clientConfig := DefaultClientConfig()
	client1 := NewClient("client1", w1, req, clientConfig, logger)
	client2 := NewClient("client2", w2, req, clientConfig, logger)
	client3 := NewClient("client3", w3, req, clientConfig, logger)

	// Register first client
	err := manager.Register(client1)
	assert.NoError(t, err)

	// Register second client
	err = manager.Register(client2)
	assert.NoError(t, err)

	// Wait for registration to be processed
	time.Sleep(10 * time.Millisecond)

	// Check client count
	assert.Equal(t, int64(2), manager.metrics.GetActiveClients())

	// Try to register third client (should fail due to limit)
	err = manager.Register(client3)
	assert.Equal(t, ErrMaxClientsReached, err)

	// Unregister first client
	err = manager.Unregister(client1)
	assert.NoError(t, err)

	// Wait for unregistration to be processed
	time.Sleep(10 * time.Millisecond)

	// Check client count
	assert.Equal(t, int64(1), manager.metrics.GetActiveClients())

	// Now third client should be able to register
	err = manager.Register(client3)
	assert.NoError(t, err)
}

func TestManager_Broadcasting(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	manager := NewManager(DefaultConfig(), logger)
	ctx := context.Background()
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop()

	// Create a test event
	botID := openapi_types.UUID(uuid.New())
	event := CreateBotRegisteredEvent(botID, "test-bot", []string{"fuzzing"})

	// Test broadcasting without clients
	err := manager.Broadcast(event)
	assert.NoError(t, err)

	// Add a client
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/events", nil)
	clientConfig := DefaultClientConfig()
	client := NewClient("test-client", w, req, clientConfig, logger)

	err = manager.Register(client)
	require.NoError(t, err)

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Test broadcasting with client
	err = manager.Broadcast(event)
	assert.NoError(t, err)

	// Verify metrics
	assert.Equal(t, int64(1), manager.metrics.GetActiveClients())
}

func TestManager_TopicSubscription(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	manager := NewManager(DefaultConfig(), logger)
	ctx := context.Background()
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop()

	// Create clients
	w1 := httptest.NewRecorder()
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/events", nil)
	clientConfig := DefaultClientConfig()

	client1 := NewClient("client1", w1, req, clientConfig, logger)
	client2 := NewClient("client2", w2, req, clientConfig, logger)

	// Register clients
	require.NoError(t, manager.Register(client1))
	require.NoError(t, manager.Register(client2))

	// Wait for registration
	time.Sleep(10 * time.Millisecond)

	// Subscribe clients to different topics
	err := manager.Subscribe("client1", "bot-events")
	assert.NoError(t, err)

	err = manager.Subscribe("client2", "job-events")
	assert.NoError(t, err)

	// Subscribe client1 to both topics
	err = manager.Subscribe("client1", "job-events")
	assert.NoError(t, err)

	// Test broadcasting to specific topics
	botEvent := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{"fuzzing"})
	err = manager.BroadcastToTopic("bot-events", botEvent)
	assert.NoError(t, err)

	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{
		"fuzzer": "libfuzzer",
	})
	err = manager.BroadcastToTopic("job-events", jobEvent)
	assert.NoError(t, err)

	// Test unsubscribing
	err = manager.Unsubscribe("client1", "bot-events")
	assert.NoError(t, err)

	// Test subscribing to non-existent client
	err = manager.Subscribe("non-existent", "test-topic")
	assert.Equal(t, ErrClientNotFound, err)
}

func TestEvents_Creation(t *testing.T) {
	// Test Bot Event
	botID := openapi_types.UUID(uuid.New())
	botEvent := CreateBotRegisteredEvent(botID, "test-bot", []string{"fuzzing", "coverage"})

	assert.Equal(t, EventBotRegistered, botEvent.GetType())
	assert.NotEmpty(t, botEvent.GetID())
	assert.Contains(t, botEvent.GetData(), botID.String())

	// Test Job Event
	jobID := openapi_types.UUID(uuid.New())
	campaignID := openapi_types.UUID(uuid.New())
	jobEvent := CreateJobProgressEvent(jobID, campaignID, 50.0, map[string]interface{}{
		"executions": 1000,
	})

	assert.Equal(t, EventJobProgress, jobEvent.GetType())
	assert.Contains(t, jobEvent.GetData(), "50")
	assert.Contains(t, jobEvent.GetData(), "1000")

	// Test Crash Event
	crashID := openapi_types.UUID(uuid.New())
	crashEvent := CreateCrashDetectedEvent(crashID, jobID, campaignID, "segfault", "stack trace")

	assert.Equal(t, EventCrashDetected, crashEvent.GetType())
	assert.Contains(t, crashEvent.GetData(), "segfault")

	// Test System Event
	systemEvent := CreateSystemAlertEvent("test-component", "error", "test message", map[string]interface{}{
		"code": 500,
	})

	assert.Equal(t, EventSystemAlert, systemEvent.GetType())
	assert.Contains(t, systemEvent.GetData(), "test message")
}

func TestFilters_TypeFilter(t *testing.T) {
	// Create type filter
	filter := NewTypeFilter([]string{"job.started", "job.completed"})

	// Test matching event
	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})
	assert.True(t, filter.Matches(jobEvent))

	// Test non-matching event
	botEvent := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{})
	assert.False(t, filter.Matches(botEvent))

	// Test empty filter (should match all)
	emptyFilter := NewTypeFilter([]string{})
	assert.True(t, emptyFilter.Matches(jobEvent))
	assert.True(t, emptyFilter.Matches(botEvent))
}

func TestFilters_ResourceFilter(t *testing.T) {
	botID := openapi_types.UUID(uuid.New())

	// Create resource filter for specific bot
	filter := NewResourceFilter("bot", botID.String())

	// Test matching bot event
	botEvent := CreateBotRegisteredEvent(botID, "test-bot", []string{})
	assert.True(t, filter.Matches(botEvent))

	// Test non-matching bot event
	otherBotEvent := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "other-bot", []string{})
	assert.False(t, filter.Matches(otherBotEvent))

	// Test non-bot event
	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})
	assert.False(t, filter.Matches(jobEvent))
}

func TestFilters_PatternFilter(t *testing.T) {
	// Create pattern filter for job and crash events
	filter, err := NewPatternFilter("^(job|crash)\\.")
	require.NoError(t, err)

	// Test matching events
	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})
	assert.True(t, filter.Matches(jobEvent))

	crashEvent := CreateCrashDetectedEvent(openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), "segfault", "")
	assert.True(t, filter.Matches(crashEvent))

	// Test non-matching event
	botEvent := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{})
	assert.False(t, filter.Matches(botEvent))
}

func TestFilters_CompoundFilter(t *testing.T) {
	typeFilter := NewTypeFilter([]string{"job.started", "job.completed"})
	patternFilter, _ := NewPatternFilter("^job\\.")

	// Test AND filter
	andFilter := NewCompoundFilter("AND", typeFilter, patternFilter)

	jobEvent := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})
	assert.True(t, andFilter.Matches(jobEvent))

	botEvent := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{})
	assert.False(t, andFilter.Matches(botEvent))

	// Test OR filter
	orFilter := NewCompoundFilter("OR", typeFilter, patternFilter)

	jobProgress := NewJobEvent("job.progress", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})
	assert.True(t, orFilter.Matches(jobProgress)) // Matches pattern but not type filter

	assert.False(t, orFilter.Matches(botEvent)) // Matches neither
}

func TestFilters_ParseFromParams(t *testing.T) {
	params := map[string]string{
		"types":        "job.started,job.completed",
		"bot_id":       uuid.New().String(),
		"campaign_id":  uuid.New().String(),
		"pattern":      "^job\\.",
		"min_severity": "warning",
	}

	filter := ParseFiltersFromParams(params)
	assert.NotNil(t, filter)

	// Should be a compound filter since multiple filters were added
	compoundFilter, ok := filter.(*CompoundFilter)
	assert.True(t, ok)
	assert.Equal(t, "AND", compoundFilter.Logic)
	assert.Len(t, compoundFilter.Filters, 5) // type, bot, campaign, pattern, severity
}

func TestClient_Lifecycle(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/events", nil)
	clientConfig := DefaultClientConfig()

	client := NewClient("test-client", w, req, clientConfig, logger)

	// Test initial state
	assert.True(t, client.IsAlive())
	assert.Equal(t, "test-client", client.ID)

	// Test adding filters
	typeFilter := NewTypeFilter([]string{"job.started"})
	client.AddFilter(typeFilter)
	assert.Len(t, client.Filters, 1)

	// Test closing client
	client.Close()
	assert.False(t, client.IsAlive())

	// Test sending event to closed client
	event := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{})
	err := client.SendEvent(event)
	assert.Equal(t, ErrClientClosed, err)
}

func TestMetrics(t *testing.T) {
	metrics := NewMetrics()

	// Test initial state
	assert.Equal(t, int64(0), metrics.GetActiveClients())
	assert.Equal(t, int64(0), metrics.GetTotalClients())
	assert.Equal(t, int64(0), metrics.GetEventsSent())
	assert.Equal(t, int64(0), metrics.GetEventsDropped())

	// Test incrementing metrics
	metrics.IncrementActiveClients()
	assert.Equal(t, int64(1), metrics.GetActiveClients())
	assert.Equal(t, int64(1), metrics.GetTotalClients())

	metrics.IncrementEventsSent()
	assert.Equal(t, int64(1), metrics.GetEventsSent())

	metrics.IncrementEventsDropped()
	assert.Equal(t, int64(1), metrics.GetEventsDropped())

	// Test decrementing active clients
	metrics.DecrementActiveClients()
	assert.Equal(t, int64(0), metrics.GetActiveClients())
	assert.Equal(t, int64(1), metrics.GetTotalClients()) // Total should not decrease

	// Test snapshot
	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(0), snapshot.ActiveClients)
	assert.Equal(t, int64(1), snapshot.TotalClients)
	assert.Equal(t, int64(1), snapshot.EventsSent)
	assert.Equal(t, int64(1), snapshot.EventsDropped)
	assert.NotEmpty(t, snapshot.Uptime)
}

func TestEventHistory(t *testing.T) {
	history := NewEventHistory(3) // Small size for testing

	// Create test events
	event1 := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "bot1", []string{})
	event2 := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "bot2", []string{})
	event3 := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "bot3", []string{})
	event4 := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "bot4", []string{})

	// Add events
	history.Add(event1)
	history.Add(event2)
	history.Add(event3)

	// Get all events
	events := history.GetEventsAfter("")
	assert.Len(t, events, 3)

	// Add fourth event (should overwrite first)
	history.Add(event4)
	events = history.GetEventsAfter("")
	assert.Len(t, events, 3)

	// Should contain events 2, 3, 4 but not 1
	eventTypes := make([]string, len(events))
	for i, e := range events {
		eventTypes[i] = e.Event.GetType()
	}
	assert.Equal(t, []string{EventBotRegistered, EventBotRegistered, EventBotRegistered}, eventTypes)
}

// Benchmark tests
func BenchmarkManager_Broadcast(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	manager := NewManager(DefaultConfig(), logger)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	event := CreateBotRegisteredEvent(openapi_types.UUID(uuid.New()), "test-bot", []string{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Broadcast(event)
	}
}

func BenchmarkFilters_TypeFilter(b *testing.B) {
	filter := NewTypeFilter([]string{"job.started", "job.completed", "job.failed"})
	event := NewJobEvent("job.started", openapi_types.UUID(uuid.New()), openapi_types.UUID(uuid.New()), map[string]interface{}{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Matches(event)
	}
}

func BenchmarkEvent_Creation(b *testing.B) {
	botID := openapi_types.UUID(uuid.New())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CreateBotRegisteredEvent(botID, "test-bot", []string{"fuzzing"})
	}
}
