package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// parseJSONResponse parses a JSON response into the provided interface
func parseJSONResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("failed to unmarshal JSON response: %w", err)
	}

	return nil
}

// parseErrorResponse parses an error response
func parseErrorResponse(resp *http.Response) (*generated.ProblemDetails, error) {
	var problemDetails generated.ProblemDetails
	err := parseJSONResponse(resp, &problemDetails)
	if err != nil {
		return nil, err
	}
	return &problemDetails, nil
}

// generateTestName generates a unique test name with timestamp
func generateTestName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// generateTestUUID generates a test UUID
func generateTestUUID() uuid.UUID {
	return uuid.New()
}

// assertStatusCode checks if the response has the expected status code
func assertStatusCode(t require.TestingT, resp *http.Response, expectedCode int) {
	if resp.StatusCode != expectedCode {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		require.Failf(t, "Unexpected status code",
			"Expected: %d, Got: %d, Response: %s",
			expectedCode, resp.StatusCode, string(body))
	}
}

// createHTTPRequest creates an HTTP request with context
func createHTTPRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return req, nil
}

// waitForCondition waits for a condition to become true within timeout
func waitForCondition(condition func() bool, timeout time.Duration, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// TestDataGenerator provides methods to generate test data
type TestDataGenerator struct{}

// GenerateBotCreateRequest generates a test bot create request
func (g *TestDataGenerator) GenerateBotCreateRequest(name string) generated.BotCreateRequest {
	return generated.BotCreateRequest{
		Name:     name,
		Hostname: "test-host-" + strings.ToLower(name),
		Capabilities: []generated.BotCreateRequestCapabilities{
			generated.BotCreateRequestCapabilitiesFuzzing,
			generated.BotCreateRequestCapabilitiesCoverage,
		},
		ResourceUsage: &generated.BotResourceUsage{
			CpuPercent:           25.5,
			MemoryMb:             256,
			DiskSpaceMb:          2048,
			ActiveJobs:           0,
			QueueLength:          0,
			NetworkBytesSent:     0,
			NetworkBytesReceived: 0,
		},
	}
}

// GenerateCampaignCreateRequest generates a test campaign create request
func (g *TestDataGenerator) GenerateCampaignCreateRequest(name string) generated.CampaignCreateRequest {
	return generated.CampaignCreateRequest{
		Name:        name,
		Description: "Test campaign: " + name,
		JobTemplate: generated.CampaignCreateRequestJobTemplate{
			Fuzzer:       generated.FuzzerTypeLibfuzzer,
			TargetBinary: "/bin/test-target",
			TargetArgs:   []string{"-max_total_time=3600", "@@"},
			Timeout:      3600,
			MemoryLimit:  2048,
			Dictionary:   "/path/to/dict.txt",
			Corpus:       []string{"/corpus/seed1", "/corpus/seed2"},
		},
	}
}

// GenerateJobCreateRequest generates a test job create request
func (g *TestDataGenerator) GenerateJobCreateRequest(name string, campaignID *uuid.UUID) generated.JobCreateRequest {
	return generated.JobCreateRequest{
		Name:         name,
		CampaignId:   campaignID,
		Fuzzer:       generated.FuzzerTypeAflplusplus,
		TargetBinary: "/bin/target",
		TargetArgs:   []string{"-i", "@@"},
		Timeout:      1800,
		MemoryLimit:  1024,
		Dictionary:   "/dict/test.dict",
		Corpus:       []string{"/corpus/input1", "/corpus/input2"},
	}
}

// GenerateCorpusEntry generates a test corpus entry
func (g *TestDataGenerator) GenerateCorpusEntry(name string) generated.CorpusEntry {
	now := time.Now()
	return generated.CorpusEntry{
		Id:           generateTestUUID(),
		Name:         name,
		Hash:         "sha256:" + strings.Repeat("a", 64),
		Size:         1024,
		Path:         "/corpus/" + name,
		CreatedAt:    now,
		LastAccessed: &now,
		AccessCount:  5,
		CoverageInfo: &generated.CorpusEntryCoverageInfo{
			EdgesCovered:    150,
			NewEdges:        25,
			CoveragePercent: 75.5,
		},
		GenerationInfo: &generated.CorpusEntryGenerationInfo{
			Generation:   3,
			ParentIds:    []uuid.UUID{generateTestUUID()},
			MutationType: generated.Crossover,
		},
	}
}

// GenerateCrash generates a test crash
func (g *TestDataGenerator) GenerateCrash(jobID uuid.UUID) generated.Crash {
	now := time.Now()
	return generated.Crash{
		Id:        generateTestUUID(),
		JobId:     jobID,
		Hash:      "crash-" + strings.Repeat("b", 32),
		Timestamp: now,
		Type:      generated.CrashTypeSegmentationFault,
		Severity:  generated.CrashSeverityHigh,
		Input:     []byte("AAAA\x00\x01\x02\x03"),
		Output:    "Segmentation fault (core dumped)",
		CrashInfo: generated.CrashCrashInfo{
			Signal:     11,
			ExitCode:   139,
			StackTrace: "#0 0x0000000000400123 in main() at test.c:42\n#1 0x00007f0123456789 in __libc_start_main()",
			Registers: map[string]interface{}{
				"rax": "0x0000000000000000",
				"rbx": "0x0000000000400000",
				"rip": "0x0000000000400123",
			},
		},
		ReproductionInfo: &generated.CrashReproductionInfo{
			Reproducible:    true,
			ReproAttempts:   3,
			SuccessfulRepro: 3,
			LastReproduced:  &now,
		},
	}
}

// ResponseValidator provides methods to validate API responses
type ResponseValidator struct{}

// ValidateBot validates a bot response
func (v *ResponseValidator) ValidateBot(bot generated.Bot, expectedName string) error {
	if bot.Name != expectedName {
		return fmt.Errorf("expected bot name %s, got %s", expectedName, bot.Name)
	}

	if bot.Id == uuid.Nil {
		return fmt.Errorf("bot ID should not be nil")
	}

	if bot.Status == "" {
		return fmt.Errorf("bot status should not be empty")
	}

	if bot.CreatedAt.IsZero() {
		return fmt.Errorf("bot creation time should not be zero")
	}

	return nil
}

// ValidateCampaign validates a campaign response
func (v *ResponseValidator) ValidateCampaign(campaign generated.Campaign, expectedName string) error {
	if campaign.Name != expectedName {
		return fmt.Errorf("expected campaign name %s, got %s", expectedName, campaign.Name)
	}

	if campaign.Id == uuid.Nil {
		return fmt.Errorf("campaign ID should not be nil")
	}

	if campaign.Status == "" {
		return fmt.Errorf("campaign status should not be empty")
	}

	if campaign.CreatedAt.IsZero() {
		return fmt.Errorf("campaign creation time should not be zero")
	}

	return nil
}

// ValidateJob validates a job response
func (v *ResponseValidator) ValidateJob(job generated.Job, expectedName string) error {
	if job.Name != expectedName {
		return fmt.Errorf("expected job name %s, got %s", expectedName, job.Name)
	}

	if job.Id == uuid.Nil {
		return fmt.Errorf("job ID should not be nil")
	}

	if job.Status == "" {
		return fmt.Errorf("job status should not be empty")
	}

	if job.CreatedAt.IsZero() {
		return fmt.Errorf("job creation time should not be zero")
	}

	return nil
}

// ValidatePagination validates pagination in list responses
func (v *ResponseValidator) ValidatePagination(pagination *generated.Pagination, expectedTotal int) error {
	if pagination == nil {
		return fmt.Errorf("pagination should not be nil")
	}

	if pagination.Total < 0 {
		return fmt.Errorf("pagination total should be non-negative")
	}

	if expectedTotal >= 0 && pagination.Total != expectedTotal {
		return fmt.Errorf("expected total %d, got %d", expectedTotal, pagination.Total)
	}

	if pagination.Page < 1 {
		return fmt.Errorf("pagination page should be >= 1")
	}

	if pagination.Limit < 1 {
		return fmt.Errorf("pagination limit should be >= 1")
	}

	return nil
}

// EventCollector collects SSE events for testing
type EventCollector struct {
	Events []map[string]interface{}
	Done   chan struct{}
}

// NewEventCollector creates a new event collector
func NewEventCollector() *EventCollector {
	return &EventCollector{
		Events: make([]map[string]interface{}, 0),
		Done:   make(chan struct{}),
	}
}

// AddEvent adds an event to the collector
func (c *EventCollector) AddEvent(event map[string]interface{}) {
	c.Events = append(c.Events, event)
}

// WaitForEvents waits for a specific number of events or timeout
func (c *EventCollector) WaitForEvents(count int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(c.Events) >= count {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// GetEventCount returns the number of collected events
func (c *EventCollector) GetEventCount() int {
	return len(c.Events)
}

// GetLastEvent returns the last collected event
func (c *EventCollector) GetLastEvent() map[string]interface{} {
	if len(c.Events) == 0 {
		return nil
	}
	return c.Events[len(c.Events)-1]
}

// Stop stops the event collector
func (c *EventCollector) Stop() {
	close(c.Done)
}
