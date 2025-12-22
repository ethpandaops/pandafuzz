package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
)

// CrashAdapter handles crash-related API requests
type CrashAdapter struct {
	crashRepo       repository.CrashRepository
	storage         common.Storage // Add storage layer for CreateCrash
	deduplication   common.DeduplicationService
	minimizer       common.CrashMinimizerService
	reproducibility common.ReproducibilityService
	sse             *sse.Manager
	logger          logrus.FieldLogger
}

// NewCrashAdapter creates a new crash adapter
func NewCrashAdapter(
	crashRepo repository.CrashRepository,
	storage common.Storage,
	deduplication common.DeduplicationService,
	minimizer common.CrashMinimizerService,
	reproducibility common.ReproducibilityService,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CrashAdapter {
	return &CrashAdapter{
		crashRepo:       crashRepo,
		storage:         storage,
		deduplication:   deduplication,
		minimizer:       minimizer,
		reproducibility: reproducibility,
		sse:             sse,
		logger:          logger.WithField("adapter", "crash"),
	}
}

// ListCrashes returns a list of crashes
func (a *CrashAdapter) ListCrashes(w http.ResponseWriter, r *http.Request, params generated.ListCrashesParams) {
	a.logger.Debug("listing crashes")

	// Mock implementation - replace with actual service calls
	groupId := "group_001"
	stackTrace := "SEGV at 0x00007fff...\nBacktrace:\n#0 0x00007fff..."
	metadata1 := generated.Metadata{
		"fuzzer":     "libfuzzer",
		"iterations": 1000000,
	}

	crashes := []generated.Crash{
		{
			Id:             openapi_types.UUID(uuid.New()),
			JobId:          openapi_types.UUID(uuid.New()),
			CampaignId:     openapi_types.UUID(uuid.New()),
			BotId:          openapi_types.UUID(uuid.New()),
			Hash:           "sha256:crash1234...",
			InputSizeBytes: 512,
			StackTrace:     &stackTrace,
			Signal:         &[]int{11}[0], // SIGSEGV
			ExitCode:       &[]int{139}[0],
			DiscoveredAt:   time.Now().Add(-2 * time.Hour),
			Severity:       generated.CrashSeverityCritical,
			Type:           generated.CrashTypeSegfault,
			GroupId:        &groupId,
			IsUnique:       &[]bool{true}[0],
			Triaged:        &[]bool{false}[0],
			Tags:           &[]string{"heap-overflow", "asan"},
			Metadata:       &metadata1,
		},
		{
			Id:             openapi_types.UUID(uuid.New()),
			JobId:          openapi_types.UUID(uuid.New()),
			CampaignId:     openapi_types.UUID(uuid.New()),
			BotId:          openapi_types.UUID(uuid.New()),
			Hash:           "sha256:crash5678...",
			InputSizeBytes: 1024,
			StackTrace:     &[]string{"Assertion failed: index < size\nBacktrace:\n#0 0x00007fff..."}[0],
			Signal:         &[]int{6}[0], // SIGABRT
			ExitCode:       &[]int{134}[0],
			DiscoveredAt:   time.Now().Add(-1 * time.Hour),
			Severity:       generated.CrashSeverityHigh,
			Type:           generated.CrashTypeAssertion,
			GroupId:        &[]string{"group_002"}[0],
			IsUnique:       &[]bool{true}[0],
			Triaged:        &[]bool{true}[0],
			Tags:           &[]string{"assertion", "bounds-check"},
		},
	}

	// Apply filtering
	filteredCrashes := crashes
	if params.Severity != nil {
		filtered := []generated.Crash{}
		for _, crash := range filteredCrashes {
			if crash.Severity == *params.Severity {
				filtered = append(filtered, crash)
			}
		}
		filteredCrashes = filtered
	}

	// Status filtering removed since Status field doesn't exist in Crash type

	// Apply pagination
	limit := 10
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Ensure we don't go out of bounds
	start := offset
	end := offset + limit
	if start > len(filteredCrashes) {
		start = len(filteredCrashes)
	}
	if end > len(filteredCrashes) {
		end = len(filteredCrashes)
	}

	paginatedCrashes := filteredCrashes[start:end]

	response := generated.CrashListResponse{
		Data: paginatedCrashes,
		Pagination: generated.Pagination{
			Total:  len(filteredCrashes),
			Limit:  limit,
			Offset: offset,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetCrash retrieves a single crash
func (a *CrashAdapter) GetCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam, params generated.GetCrashParams) {
	a.logger.WithField("crash_id", crashId).Debug("getting crash")

	// Mock implementation
	stackTrace := "SEGV at 0x00007fff...\nBacktrace:\n#0 0x00007fff..."
	groupId := "group_001"
	metadata := generated.Metadata{
		"fuzzer":     "libfuzzer",
		"iterations": 1000000,
	}

	crash := generated.Crash{
		Id:             openapi_types.UUID(crashId),
		JobId:          openapi_types.UUID(uuid.New()),
		CampaignId:     openapi_types.UUID(uuid.New()),
		BotId:          openapi_types.UUID(uuid.New()),
		Hash:           "sha256:crash1234...",
		InputSizeBytes: 512,
		StackTrace:     &stackTrace,
		Signal:         &[]int{11}[0], // SIGSEGV
		ExitCode:       &[]int{139}[0],
		DiscoveredAt:   time.Now().Add(-2 * time.Hour),
		Severity:       generated.CrashSeverityCritical,
		Type:           generated.CrashTypeSegfault,
		GroupId:        &groupId,
		IsUnique:       &[]bool{true}[0],
		Triaged:        &[]bool{false}[0],
		Tags:           &[]string{"heap-overflow", "asan"},
		Metadata:       &metadata,
	}

	// Analysis feature not available in current generated types
	// TODO: Re-enable when CrashAnalysis type is added to OpenAPI spec

	a.writeJSONResponse(w, http.StatusOK, crash)
}

// DeduplicateCrash marks a crash as duplicate
func (a *CrashAdapter) DeduplicateCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.logger.WithField("crash_id", crashId).Debug("deduplicating crash")

	// TODO: DeduplicateRequest and DeduplicationResponse types not available in generated types
	// This endpoint needs to be re-implemented when types are added to OpenAPI spec

	// For now, return a simple success response
	response := map[string]interface{}{
		"message":  "Crash deduplication recorded",
		"crash_id": crashId,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// MinimizeCrash minimizes a crash input
func (a *CrashAdapter) MinimizeCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.logger.WithField("crash_id", crashId).Debug("minimizing crash")

	// TODO: MinimizeRequest type not available in generated types
	// For now, read the request body as a generic map
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Mock implementation - MinimizationResponse type not available
	response := map[string]interface{}{
		"crash_id":       crashId,
		"status":         "in_progress",
		"original_size":  512,
		"minimized_size": 64,
		"reduction":      87.5,
		"strategy":       req["strategy"],
		"started_at":     time.Now(),
		"estimated_time": 300, // 5 minutes
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCrashEvent(
			"crash.minimization.started",
			openapi_types.UUID(crashId),
			openapi_types.UUID(uuid.New()), // jobId
			openapi_types.UUID(uuid.New()), // campaignId
			map[string]interface{}{
				"strategy": req["strategy"],
			},
		)
		a.sse.BroadcastToTopic("crash", event)
	}

	a.writeJSONResponse(w, http.StatusAccepted, response)
}

// ReproduceCrash attempts to reproduce a crash
func (a *CrashAdapter) ReproduceCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.logger.WithField("crash_id", crashId).Debug("reproducing crash")

	// TODO: ReproduceRequest and ReproductionResponse types not available in generated types
	// For now, read the request body as a generic map
	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Mock implementation - ReproductionResponse type not available
	response := map[string]interface{}{
		"crash_id":     crashId,
		"reproducible": true,
		"attempts":     3,
		"successful":   3,
		"environment": map[string]interface{}{
			"fuzzer":  req["fuzzer_type"],
			"timeout": req["timeout"],
			"args":    req["fuzzer_args"],
		},
		"consistent_stack_trace": true,
		"consistent_signal":      true,
		"execution_time":         150, // milliseconds
		"logs": []string{
			"Attempt 1: Reproduced successfully",
			"Attempt 2: Reproduced successfully",
			"Attempt 3: Reproduced successfully",
		},
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCrashEvent(
			"crash.reproduced",
			openapi_types.UUID(crashId),
			openapi_types.UUID(uuid.New()), // jobId
			openapi_types.UUID(uuid.New()), // campaignId
			map[string]interface{}{
				"reproducible": response["reproducible"],
				"attempts":     response["attempts"],
				"successful":   response["successful"],
			},
		)
		a.sse.BroadcastToTopic("crash", event)
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (a *CrashAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *CrashAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
	problem := generated.ProblemDetails{
		Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
		Title:     title,
		Status:    statusCode,
		Timestamp: &[]time.Time{time.Now()}[0],
	}

	if err != nil {
		detail := err.Error()
		problem.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		a.logger.WithError(encodeErr).Error("failed to encode error response")
	}
}

// CreateCrash handles crash creation requests
func (a *CrashAdapter) CreateCrash(w http.ResponseWriter, r *http.Request) {
	// For now, we'll use a simple struct until code generation runs
	var req struct {
		JobId      string `json:"job_id"`
		BotId      string `json:"bot_id"`
		CrashType  string `json:"crash_type"`
		Signal     int    `json:"signal"`
		ExitCode   int    `json:"exit_code"`
		InputData  string `json:"input_data"`
		StackTrace string `json:"stack_trace"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid Request", err)
		return
	}

	// Validate required fields
	if req.JobId == "" {
		a.writeError(w, http.StatusBadRequest, "missing_field", "Missing Required Field", fmt.Errorf("job_id is required"))
		return
	}
	if req.BotId == "" {
		a.writeError(w, http.StatusBadRequest, "missing_field", "Missing Required Field", fmt.Errorf("bot_id is required"))
		return
	}

	// Validate input_data size (max 10MB)
	const maxInputSize = 10 * 1024 * 1024 // 10MB
	if len(req.InputData) > maxInputSize {
		a.writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Payload Too Large", fmt.Errorf("input_data exceeds maximum size of 10MB"))
		return
	}

	// Create crash record
	crash := &common.CrashResult{
		ID:         uuid.New().String(),
		JobID:      req.JobId,
		BotID:      req.BotId,
		Hash:       generateCrashHash(req.InputData),
		Type:       req.CrashType,
		Signal:     req.Signal,
		ExitCode:   req.ExitCode,
		Timestamp:  time.Now(),
		Size:       int64(len(req.InputData)),
		IsUnique:   true, // Will be determined by deduplication later
		Input:      []byte(req.InputData),
		StackTrace: req.StackTrace,
	}

	// Store crash in database - using the existing storage layer
	// Use storage.CreateCrash which takes common.CrashResult
	if err := a.storage.CreateCrash(r.Context(), crash); err != nil {
		// Check if it's a duplicate error
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			// Return existing crash info
			resp := struct {
				CrashId               string    `json:"crash_id"`
				IsUnique              bool      `json:"is_unique"`
				DuplicateOf           *string   `json:"duplicate_of,omitempty"`
				ProcessedAt           time.Time `json:"processed_at"`
				AnalysisScheduled     bool      `json:"analysis_scheduled"`
				MinimizationScheduled bool      `json:"minimization_scheduled"`
			}{
				CrashId:               crash.ID,
				IsUnique:              false,
				DuplicateOf:           &crash.ID, // TODO: Get actual original crash ID
				ProcessedAt:           time.Now(),
				AnalysisScheduled:     false,
				MinimizationScheduled: false,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(resp)
			return
		}
		a.writeError(w, http.StatusInternalServerError, "storage_error", "Storage Error", fmt.Errorf("failed to store crash: %w", err))
		return
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewBaseEvent("crash.created", map[string]interface{}{
			"crash_id": crash.ID,
			"job_id":   crash.JobID,
			"bot_id":   crash.BotID,
			"type":     crash.Type,
			"signal":   crash.Signal,
		})
		a.sse.Broadcast(event)
	}

	// Create response
	resp := struct {
		CrashId               string    `json:"crash_id"`
		IsUnique              bool      `json:"is_unique"`
		DuplicateOf           *string   `json:"duplicate_of,omitempty"`
		ProcessedAt           time.Time `json:"processed_at"`
		AnalysisScheduled     bool      `json:"analysis_scheduled"`
		MinimizationScheduled bool      `json:"minimization_scheduled"`
	}{
		CrashId:               crash.ID,
		IsUnique:              crash.IsUnique,
		ProcessedAt:           time.Now(),
		AnalysisScheduled:     true,
		MinimizationScheduled: true,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// generateCrashHash generates a SHA256 hash from the input data
func generateCrashHash(input string) string {
	h := sha256.New()
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

// GetCrashInput returns the raw input data for a crash (from v3)
func (a *CrashAdapter) GetCrashInput(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	a.logger.WithField("crash_id", crashId).Debug("getting crash input")

	// In production, this would fetch from storage
	// Mock implementation returns sample data
	inputData := []byte("Mock crash input data for " + crashId.String())

	// Set headers for binary download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"crash_%s.bin\"", crashId.String()[:8]))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(inputData)))
	w.Header().Set("X-Crash-ID", crashId.String())
	w.Header().Set("X-Crash-Hash", generateCrashHash(string(inputData)))

	w.WriteHeader(http.StatusOK)
	w.Write(inputData)
}
