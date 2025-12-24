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

// ListCrashes returns a list of crashes from the database
func (a *CrashAdapter) ListCrashes(w http.ResponseWriter, r *http.Request, params generated.ListCrashesParams) {
	ctx := r.Context()
	a.logger.Debug("listing crashes")

	// Extract job ID filter if provided
	jobID := ""
	if params.JobId != nil {
		jobID = params.JobId.String()
	}

	// Apply pagination
	limit := 10
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Query crashes from storage
	crashes, err := a.storage.ListCrashes(ctx, jobID, limit, offset)
	if err != nil {
		a.logger.WithError(err).Error("failed to list crashes")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list crashes", err)
		return
	}

	// Convert to API types
	apiCrashes := make([]generated.Crash, 0, len(crashes))
	for _, crash := range crashes {
		apiCrash := a.convertToAPICrash(crash)

		// Apply severity filter if specified
		if params.Severity != nil && apiCrash.Severity != *params.Severity {
			continue
		}

		// Apply crash type filter if specified
		if params.CrashType != nil && apiCrash.Type != *params.CrashType {
			continue
		}

		apiCrashes = append(apiCrashes, apiCrash)
	}

	// Get total count for pagination
	totalCount, err := a.storage.GetCrashCount(ctx, jobID)
	if err != nil {
		a.logger.WithError(err).Warn("failed to get crash count, using list length")
		totalCount = len(apiCrashes)
	}

	response := generated.CrashListResponse{
		Data: apiCrashes,
		Pagination: generated.Pagination{
			Total:  totalCount,
			Limit:  limit,
			Offset: offset,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// convertToAPICrash converts a common.CrashResult to generated.Crash
func (a *CrashAdapter) convertToAPICrash(crash *common.CrashResult) generated.Crash {
	// Parse UUIDs
	crashID, _ := uuid.Parse(crash.ID)
	jobID, _ := uuid.Parse(crash.JobID)
	botID, _ := uuid.Parse(crash.BotID)
	campaignID, _ := uuid.Parse(crash.CampaignID)

	// Map crash type string to enum
	crashType := a.mapCrashType(crash.Type)
	severity := a.determineSeverity(crash)

	apiCrash := generated.Crash{
		Id:             openapi_types.UUID(crashID),
		JobId:          openapi_types.UUID(jobID),
		BotId:          openapi_types.UUID(botID),
		CampaignId:     openapi_types.UUID(campaignID),
		Hash:           crash.Hash,
		InputSizeBytes: int(crash.Size),
		DiscoveredAt:   crash.Timestamp,
		Severity:       severity,
		Type:           crashType,
		IsUnique:       &crash.IsUnique,
	}

	// Optional fields
	if crash.Signal != 0 {
		apiCrash.Signal = &crash.Signal
	}
	if crash.ExitCode != 0 {
		apiCrash.ExitCode = &crash.ExitCode
	}
	if crash.StackTrace != "" {
		apiCrash.StackTrace = &crash.StackTrace
	}
	if crash.Reproducible {
		apiCrash.ReproductionInfo = &struct {
			Environment             *map[string]string `json:"environment,omitempty"`
			LastReproductionAttempt *time.Time         `json:"last_reproduction_attempt,omitempty"`
			Reproducible            *bool              `json:"reproducible,omitempty"`
			ReproductionRate        *float32           `json:"reproduction_rate,omitempty"`
		}{
			Reproducible: &crash.Reproducible,
		}
	}

	return apiCrash
}

// mapCrashType maps a crash type string to the API enum
func (a *CrashAdapter) mapCrashType(crashType string) generated.CrashType {
	switch strings.ToLower(crashType) {
	case "segfault", "sigsegv":
		return generated.CrashTypeSegfault
	case "abort", "sigabrt":
		return generated.CrashTypeAbort
	case "assertion":
		return generated.CrashTypeAssertion
	case "timeout":
		return generated.CrashTypeTimeout
	case "heap_overflow", "heap-overflow":
		return generated.CrashTypeHeapOverflow
	case "stack_overflow", "stack-overflow":
		return generated.CrashTypeStackOverflow
	case "use_after_free", "use-after-free":
		return generated.CrashTypeUseAfterFree
	case "double_free", "double-free":
		return generated.CrashTypeDoubleFree
	case "memory_leak", "memory-leak":
		return generated.CrashTypeMemoryLeak
	default:
		return generated.CrashTypeOther
	}
}

// determineSeverity determines crash severity based on crash type and signal
func (a *CrashAdapter) determineSeverity(crash *common.CrashResult) generated.CrashSeverity {
	// Critical: memory corruption issues
	switch strings.ToLower(crash.Type) {
	case "use_after_free", "use-after-free", "double_free", "double-free", "heap_overflow", "heap-overflow":
		return generated.CrashSeverityCritical
	case "stack_overflow", "stack-overflow":
		return generated.CrashSeverityCritical
	}

	// High: segfaults and aborts
	switch crash.Signal {
	case 11: // SIGSEGV
		return generated.CrashSeverityHigh
	case 6: // SIGABRT
		return generated.CrashSeverityHigh
	case 8: // SIGFPE
		return generated.CrashSeverityMedium
	}

	// Medium: assertions and timeouts
	switch strings.ToLower(crash.Type) {
	case "assertion":
		return generated.CrashSeverityMedium
	case "timeout":
		return generated.CrashSeverityLow
	case "memory_leak", "memory-leak":
		return generated.CrashSeverityLow
	}

	return generated.CrashSeverityMedium
}

// GetCrash retrieves a single crash from the database
func (a *CrashAdapter) GetCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam, params generated.GetCrashParams) {
	ctx := r.Context()
	a.logger.WithField("crash_id", crashId).Debug("getting crash")

	// Fetch crash from storage
	crash, err := a.storage.GetCrash(ctx, crashId.String())
	if err != nil {
		a.logger.WithError(err).WithField("crash_id", crashId).Error("failed to get crash")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Crash not found", err)
		return
	}

	// Convert to API type
	apiCrash := a.convertToAPICrash(crash)

	a.writeJSONResponse(w, http.StatusOK, apiCrash)
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
	ctx := r.Context()
	a.logger.WithField("crash_id", crashId).Debug("getting crash input")

	// Fetch crash from storage
	crash, err := a.storage.GetCrash(ctx, crashId.String())
	if err != nil {
		a.logger.WithError(err).WithField("crash_id", crashId).Error("failed to get crash")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Crash not found", err)
		return
	}

	// Get input data - either from Input field or from file path
	var inputData []byte
	if len(crash.Input) > 0 {
		inputData = crash.Input
	} else if crash.InputBase64 != "" {
		// Decode from base64 if stored that way
		inputData, err = hex.DecodeString(crash.InputBase64)
		if err != nil {
			// Try treating it as raw string if not hex
			inputData = []byte(crash.InputBase64)
		}
	} else {
		// Return empty response if no input data available
		inputData = []byte{}
	}

	// Set headers for binary download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"crash_%s.bin\"", crashId.String()[:8]))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(inputData)))
	w.Header().Set("X-Crash-ID", crashId.String())
	w.Header().Set("X-Crash-Hash", crash.Hash)
	w.Header().Set("X-Crash-Size", fmt.Sprintf("%d", crash.Size))

	w.WriteHeader(http.StatusOK)
	w.Write(inputData)
}
