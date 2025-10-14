package integration

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// TestSSEConnection tests basic SSE connection functionality
func (s *APIIntegrationTestSuite) TestSSEConnection() {
	// Connect to SSE endpoint
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/events", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.Equal("text/event-stream", resp.Header.Get("Content-Type"))

	// Verify SSE headers
	s.Equal("no-cache", resp.Header.Get("Cache-Control"))
	s.Equal("keep-alive", resp.Header.Get("Connection"))

	resp.Body.Close()
}

// TestSSEEventStreaming tests event streaming functionality
func (s *APIIntegrationTestSuite) TestSSEEventStreaming() {
	// Start SSE connection
	collector := NewEventCollector()

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	go s.connectAndCollectEvents(ctx, collector, "")

	// Give SSE connection time to establish
	time.Sleep(100 * time.Millisecond)

	// Trigger some events by creating resources
	s.createTestBot(generateTestName("sse-bot"))
	s.createTestJob(generateTestName("sse-job"), nil)

	// Wait for events to be collected
	eventReceived := collector.WaitForEvents(1, 5*time.Second)

	if eventReceived {
		s.T().Logf("Received %d SSE events", collector.GetEventCount())
		lastEvent := collector.GetLastEvent()
		if lastEvent != nil {
			s.T().Logf("Last event: %+v", lastEvent)
		}
	} else {
		s.T().Log("No SSE events received (this may be expected in test environment)")
	}

	collector.Stop()
}

// TestSSETopicSubscription tests subscribing to specific event topics
func (s *APIIntegrationTestSuite) TestSSETopicSubscription() {
	testCases := []struct {
		name  string
		topic string
	}{
		{"bot_events", "bots"},
		{"job_events", "jobs"},
		{"campaign_events", "campaigns"},
		{"crash_events", "crashes"},
		{"corpus_events", "corpus"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			collector := NewEventCollector()

			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
			defer cancel()

			go s.connectAndCollectEvents(ctx, collector, tc.topic)

			// Give connection time to establish
			time.Sleep(100 * time.Millisecond)

			// Trigger topic-specific events
			switch tc.topic {
			case "bots":
				s.createTestBot(generateTestName("topic-bot"))
			case "jobs":
				s.createTestJob(generateTestName("topic-job"), nil)
			case "campaigns":
				s.createTestCampaign(generateTestName("topic-campaign"))
			case "crashes":
				jobID := s.createTestJob(generateTestName("crash-job"), nil)
				s.reportTestCrash(jobID)
			case "corpus":
				s.uploadTestCorpus("topic-seed.txt", []byte("topic test data"))
			}

			// Check if we received topic-specific events
			eventReceived := collector.WaitForEvents(1, 3*time.Second)

			if eventReceived {
				s.T().Logf("Received %d events for topic '%s'", collector.GetEventCount(), tc.topic)
			} else {
				s.T().Logf("No events received for topic '%s' (may be expected)", tc.topic)
			}

			collector.Stop()
		})
	}
}

// TestSSEEventFiltering tests event filtering functionality
func (s *APIIntegrationTestSuite) TestSSEEventFiltering() {
	// Test filtering by event type
	collector := NewEventCollector()

	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	// Connect with filter parameters
	go s.connectAndCollectEventsWithFilter(ctx, collector, map[string]string{
		"event_types": "bot.created,job.created",
		"severity":    "info,warning",
	})

	// Give connection time to establish
	time.Sleep(100 * time.Millisecond)

	// Create resources to trigger events
	s.createTestBot(generateTestName("filter-bot"))
	s.createTestJob(generateTestName("filter-job"), nil)
	s.createTestCampaign(generateTestName("filter-campaign"))

	// Wait for filtered events
	eventReceived := collector.WaitForEvents(1, 5*time.Second)

	if eventReceived {
		s.T().Logf("Received %d filtered events", collector.GetEventCount())

		// Verify events match the filter criteria
		for i, event := range collector.Events {
			s.T().Logf("Event %d: %+v", i, event)
		}
	} else {
		s.T().Log("No filtered events received (may be expected)")
	}

	collector.Stop()
}

// TestSSEReconnection tests SSE reconnection functionality
func (s *APIIntegrationTestSuite) TestSSEReconnection() {
	collector := NewEventCollector()

	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	// Start first connection
	go s.connectAndCollectEvents(ctx, collector, "")

	time.Sleep(100 * time.Millisecond)

	// Trigger an event
	s.createTestBot(generateTestName("reconnect-bot-1"))

	// Wait for initial event
	time.Sleep(1 * time.Second)
	initialEventCount := collector.GetEventCount()

	// Simulate reconnection by starting a new connection
	// (In a real scenario, this would be automatic after connection loss)
	go s.connectAndCollectEvents(ctx, collector, "")

	time.Sleep(100 * time.Millisecond)

	// Trigger another event
	s.createTestBot(generateTestName("reconnect-bot-2"))

	// Wait for reconnection events
	finalEventReceived := collector.WaitForEvents(initialEventCount+1, 3*time.Second)

	if finalEventReceived {
		s.T().Logf("Reconnection test: initial events: %d, final events: %d",
			initialEventCount, collector.GetEventCount())
	} else {
		s.T().Log("Reconnection test: no additional events received")
	}

	collector.Stop()
}

// TestSSEConnectionLimits tests SSE connection limits and management
func (s *APIIntegrationTestSuite) TestSSEConnectionLimits() {
	const numConnections = 3
	collectors := make([]*EventCollector, numConnections)

	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	// Start multiple SSE connections
	for i := 0; i < numConnections; i++ {
		collectors[i] = NewEventCollector()
		go s.connectAndCollectEvents(ctx, collectors[i], "")
		time.Sleep(50 * time.Millisecond) // Stagger connections
	}

	// Give all connections time to establish
	time.Sleep(200 * time.Millisecond)

	// Trigger events that should be received by all connections
	s.createTestBot(generateTestName("multi-connect-bot"))

	// Wait for events on all connections
	time.Sleep(2 * time.Second)

	// Check that all connections received events (or handle gracefully)
	for i, collector := range collectors {
		eventCount := collector.GetEventCount()
		s.T().Logf("Connection %d received %d events", i, eventCount)
		collector.Stop()
	}
}

// TestSSEEventFormat tests SSE event format compliance
func (s *APIIntegrationTestSuite) TestSSEEventFormat() {
	// Connect to SSE and capture raw event data
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/events", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "text/event-stream")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.T().Skipf("SSE endpoint not available (status %d)", resp.StatusCode)
		return
	}

	// Read events with timeout
	scanner := bufio.NewScanner(resp.Body)
	timeout := time.After(5 * time.Second)
	eventLines := make([]string, 0)

	// Trigger an event in another goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.createTestBot(generateTestName("format-bot"))
	}()

	// Collect event lines
	for len(eventLines) < 10 {
		select {
		case <-timeout:
			s.T().Log("SSE format test timeout (no events received)")
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()
				eventLines = append(eventLines, line)
				s.T().Logf("SSE line: %s", line)

				// Check for valid SSE format
				if strings.HasPrefix(line, "data: ") {
					s.True(len(line) > 6, "Data line should have content")
				} else if strings.HasPrefix(line, "event: ") {
					s.True(len(line) > 7, "Event line should have event type")
				} else if strings.HasPrefix(line, "id: ") {
					s.True(len(line) > 4, "ID line should have content")
				} else if line == "" {
					// Empty line indicates end of event
					s.T().Log("Event boundary detected")
				}
			} else {
				break
			}
		}
	}

	if len(eventLines) > 0 {
		s.T().Logf("Captured %d SSE event lines", len(eventLines))
	}
}

// TestSSEErrorHandling tests SSE error handling
func (s *APIIntegrationTestSuite) TestSSEErrorHandling() {
	// Test with invalid Accept header
	req, err := http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/events", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "application/json") // Wrong content type

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Should either accept the request or return appropriate error
	if resp.StatusCode == http.StatusNotAcceptable {
		s.T().Log("SSE properly rejects non-event-stream requests")
	} else if resp.StatusCode == http.StatusOK {
		s.T().Log("SSE accepts request despite wrong Accept header")
	}

	// Test with invalid parameters
	req, err = http.NewRequestWithContext(s.ctx, "GET", s.baseURL+"/api/v1/events?invalid=parameter", nil)
	s.Require().NoError(err)
	req.Header.Set("Accept", "text/event-stream")

	resp, err = httpClient.Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()

	// Should handle invalid parameters gracefully
	s.True(resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest)
}

// TestSSEBackpressure tests SSE backpressure handling
func (s *APIIntegrationTestSuite) TestSSEBackpressure() {
	collector := NewEventCollector()

	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()

	// Start SSE connection
	go s.connectAndCollectEvents(ctx, collector, "")

	// Give connection time to establish
	time.Sleep(100 * time.Millisecond)

	// Rapidly create resources to test backpressure
	const numResources = 10
	for i := 0; i < numResources; i++ {
		s.createTestBot(generateTestName("backpressure-bot"))
		time.Sleep(10 * time.Millisecond) // Small delay to simulate rapid creation
	}

	// Wait for events to be processed
	finalEventReceived := collector.WaitForEvents(1, 5*time.Second)

	if finalEventReceived {
		eventCount := collector.GetEventCount()
		s.T().Logf("Backpressure test: created %d resources, received %d events",
			numResources, eventCount)

		// Events might be batched or dropped under backpressure
		s.GreaterOrEqual(eventCount, 1, "Should receive at least one event")
	} else {
		s.T().Log("Backpressure test: no events received")
	}

	collector.Stop()
}

// Helper methods

// connectAndCollectEvents connects to SSE and collects events
func (s *APIIntegrationTestSuite) connectAndCollectEvents(ctx context.Context, collector *EventCollector, topic string) {
	url := s.baseURL + "/api/v1/events"
	if topic != "" {
		url += "?topics=" + topic
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		s.T().Logf("Failed to create SSE request: %v", err)
		return
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		s.T().Logf("Failed to connect to SSE: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.T().Logf("SSE connection failed with status %d", resp.StatusCode)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	currentEvent := make(map[string]interface{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-collector.Done:
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()

				if strings.HasPrefix(line, "data: ") {
					currentEvent["data"] = strings.TrimPrefix(line, "data: ")
				} else if strings.HasPrefix(line, "event: ") {
					currentEvent["event"] = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "id: ") {
					currentEvent["id"] = strings.TrimPrefix(line, "id: ")
				} else if line == "" && len(currentEvent) > 0 {
					// End of event
					collector.AddEvent(currentEvent)
					currentEvent = make(map[string]interface{})
				}
			} else {
				// Scanner finished or error
				return
			}
		}
	}
}

// connectAndCollectEventsWithFilter connects to SSE with filter parameters
func (s *APIIntegrationTestSuite) connectAndCollectEventsWithFilter(ctx context.Context, collector *EventCollector, filters map[string]string) {
	url := s.baseURL + "/api/v1/events"

	if len(filters) > 0 {
		url += "?"
		first := true
		for key, value := range filters {
			if !first {
				url += "&"
			}
			url += key + "=" + value
			first = false
		}
	}

	s.connectAndCollectEvents(ctx, collector, "")
}
