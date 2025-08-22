package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/deduplication"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/minimizer"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	crashTypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// CrashAdapter implements the crash-related endpoints of the generated ServerInterface
type CrashAdapter struct {
	repository repository.CrashRepository
	dedup      *deduplication.Service
	minimizer  minimizer.Interface
	sse        *sse.Manager
	logger     logrus.FieldLogger
}

// NewCrashAdapter creates a new crash adapter
func NewCrashAdapter(
	repository repository.CrashRepository,
	dedup *deduplication.Service,
	minimizer minimizer.Interface,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CrashAdapter {
	return &CrashAdapter{
		repository: repository,
		dedup:      dedup,
		minimizer:  minimizer,
		sse:        sse,
		logger:     logger.WithField("component", "crash_adapter"),
	}
}

// ListCrashes retrieves crashes with filtering and pagination
func (a *CrashAdapter) ListCrashes(w http.ResponseWriter, r *http.Request, params generated.ListCrashesParams) {
	ctx := r.Context()

	// Set defaults for pagination
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	// Build filter
	filter := repository.CrashFilter{
		Limit:  limit,
		Offset: offset,
	}

	if params.CampaignId != nil {
		campaignID := params.CampaignId.String()
		filter.CampaignID = &campaignID
	}

	if params.JobId != nil {
		jobID := params.JobId.String()
		filter.JobID = &jobID
	}

	if params.UniqueOnly != nil && *params.UniqueOnly {
		filter.UniqueOnly = true
	}

	if params.CrashType != nil {
		crashType := crashTypeToString(*params.CrashType)
		filter.CrashType = &crashType
	}

	if params.Severity != nil {
		severity := crashSeverityToString(*params.Severity)
		filter.Severity = &severity
	}

	// Get crashes from repository
	crashes, total, err := a.repository.List(ctx, filter)
	if err != nil {
		a.logger.WithError(err).Error("failed to list crashes")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve crashes", err)
		return
	}

	// Convert to API types
	apiCrashes := make([]generated.Crash, len(crashes))
	for i, crash := range crashes {
		apiCrashes[i] = a.convertCrashToAPI(crash)
	}

	// Create pagination info
	hasMore := offset+len(apiCrashes) < total
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: hasMore,
	}

	response := generated.CrashListResponse{
		Data:       apiCrashes,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// GetCrash retrieves a specific crash by ID
func (a *CrashAdapter) GetCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam, params generated.GetCrashParams) {
	ctx := r.Context()

	crash, err := a.repository.FindByID(ctx, crashId.String())
	if err != nil {
		a.logger.WithError(err).WithField("crash_id", crashId).Error("failed to get crash")
		a.writeError(w, http.StatusNotFound, "CRASH_NOT_FOUND", "Crash not found", err)
		return
	}

	apiCrash := a.convertCrashToAPI(crash)
	a.writeJSONResponse(w, http.StatusOK, apiCrash)
}

// DeduplicateCrash performs crash deduplication
func (a *CrashAdapter) DeduplicateCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	ctx := r.Context()

	var req generated.DeduplicateCrashJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if no body provided
		req = generated.DeduplicateCrashJSONBody{
			Algorithm: &[]generated.DeduplicateCrashJSONBodyAlgorithm{generated.HashBased}[0],
			Threshold: &[]float32{0.8}[0],
		}
	}

	// Get crash to deduplicate
	crash, err := a.repository.FindByID(ctx, crashId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CRASH_NOT_FOUND", "Crash not found", err)
		return
	}

	startTime := time.Now()

	// Perform deduplication
	result := a.performDeduplication(ctx, crash, req)

	processingTime := float32(time.Since(startTime).Seconds())
	result.ProcessingTimeSeconds = processingTime

	// Update crash in repository if it's a duplicate
	if !result.IsUnique && result.DuplicateOf != nil {
		crash.DuplicateOf = result.DuplicateOf
		crash.IsUnique = false
		if err := a.repository.Update(ctx, crash); err != nil {
			a.logger.WithError(err).Warn("failed to update crash deduplication status")
		}
	}

	// Publish SSE event
	crashUUID := uuid.MustParse(crashId.String())
	jobUUID := uuid.MustParse(crash.JobID)
	campaignUUID := uuid.MustParse(crash.CampaignID)
	event := sse.NewCrashEvent("crash.deduplicated", crashUUID, jobUUID, campaignUUID, map[string]any{
		"crash_id":     crashId.String(),
		"is_unique":    result.IsUnique,
		"duplicate_of": result.DuplicateOf,
		"algorithm":    result.AlgorithmUsed,
		"timestamp":    time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast crash deduplicated event")
	}

	a.writeJSONResponse(w, http.StatusOK, result)
}

// MinimizeCrash performs crash input minimization
func (a *CrashAdapter) MinimizeCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	ctx := r.Context()

	var req generated.MinimizeCrashJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if no body provided
		req = generated.MinimizeCrashJSONBody{
			Strategy:       &[]generated.MinimizeCrashJSONBodyStrategy{generated.BinarySearch}[0],
			TimeoutSeconds: &[]int{300}[0], // 5 minutes
			Priority:       &[]int{5}[0],
		}
	}

	// Get crash to minimize
	crash, err := a.repository.FindByID(ctx, crashId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CRASH_NOT_FOUND", "Crash not found", err)
		return
	}

	// Start minimization process
	if err := a.startMinimization(ctx, crash, req); err != nil {
		a.logger.WithError(err).Error("failed to start crash minimization")
		a.writeError(w, http.StatusInternalServerError, "MINIMIZATION_FAILED", "Failed to start minimization", err)
		return
	}

	// Publish SSE event
	crashUUID := uuid.MustParse(crashId.String())
	jobUUID := uuid.MustParse(crash.JobID)
	campaignUUID := uuid.MustParse(crash.CampaignID)
	event := sse.NewCrashEvent("crash.minimization.started", crashUUID, jobUUID, campaignUUID, map[string]any{
		"crash_id":  crashId.String(),
		"strategy":  req.Strategy,
		"priority":  req.Priority,
		"timestamp": time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast crash minimization started event")
	}

	response := map[string]any{
		"crash_id": crashId.String(),
		"status":   "started",
		"message":  "Minimization process started",
	}

	a.writeJSONResponse(w, http.StatusAccepted, response)
}

// ReproduceCrash attempts to reproduce a crash
func (a *CrashAdapter) ReproduceCrash(w http.ResponseWriter, r *http.Request, crashId generated.CrashIdParam) {
	ctx := r.Context()

	var req generated.ReproduceCrashJSONBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if no body provided
		req = generated.ReproduceCrashJSONBody{
			Attempts:       &[]int{3}[0],
			TimeoutSeconds: &[]int{30}[0],
		}
	}

	// Get crash to reproduce
	crash, err := a.repository.FindByID(ctx, crashId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "CRASH_NOT_FOUND", "Crash not found", err)
		return
	}

	// Perform reproduction attempts
	reproductionResult := a.performReproduction(ctx, crash, req)

	// Update crash reproduction info
	crash.ReproductionRate = &reproductionResult.SuccessRate
	crash.Reproducible = reproductionResult.SuccessRate > 0
	crash.LastReproductionAttempt = &[]time.Time{time.Now()}[0]

	if err := a.repository.Update(ctx, crash); err != nil {
		a.logger.WithError(err).Warn("failed to update crash reproduction info")
	}

	// Publish SSE event
	crashUUID := uuid.MustParse(crashId.String())
	jobUUID := uuid.MustParse(crash.JobID)
	campaignUUID := uuid.MustParse(crash.CampaignID)
	event := sse.NewCrashEvent("crash.reproduced", crashUUID, jobUUID, campaignUUID, map[string]any{
		"crash_id":     crashId.String(),
		"success_rate": reproductionResult.SuccessRate,
		"attempts":     reproductionResult.Attempts,
		"reproducible": reproductionResult.SuccessRate > 0,
		"timestamp":    time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast crash reproduced event")
	}

	a.writeJSONResponse(w, http.StatusOK, reproductionResult)
}

// Helper methods

func (a *CrashAdapter) convertCrashToAPI(crash *crashTypes.Crash) generated.Crash {
	apiCrash := generated.Crash{
		Id:             uuid.MustParse(crash.ID),
		BotId:          uuid.MustParse(crash.BotID),
		CampaignId:     uuid.MustParse(crash.CampaignID),
		JobId:          uuid.MustParse(crash.JobID),
		Hash:           crash.Hash,
		Type:           stringToCrashType(crash.Type),
		Severity:       stringToCrashSeverity(crash.Severity),
		DiscoveredAt:   crash.DiscoveredAt,
		InputSizeBytes: crash.InputSizeBytes,
	}

	if crash.DuplicateOf != nil {
		duplicateOf := uuid.MustParse(*crash.DuplicateOf)
		apiCrash.DuplicateOf = &duplicateOf
	}

	if crash.GroupID != nil {
		apiCrash.GroupId = crash.GroupID
	}

	if crash.IsUnique != nil {
		apiCrash.IsUnique = crash.IsUnique
	}

	if crash.Priority != nil {
		apiCrash.Priority = crash.Priority
	}

	if crash.Triaged != nil {
		apiCrash.Triaged = crash.Triaged
	}

	if crash.ExitCode != nil {
		apiCrash.ExitCode = crash.ExitCode
	}

	if crash.Signal != nil {
		apiCrash.Signal = crash.Signal
	}

	if crash.StackTrace != nil {
		apiCrash.StackTrace = crash.StackTrace
	}

	if crash.MinimizedInputID != nil {
		minimizedInputID := uuid.MustParse(*crash.MinimizedInputID)
		apiCrash.MinimizedInputId = &minimizedInputID
	}

	if crash.ReproductionRate != nil || crash.Reproducible || crash.LastReproductionAttempt != nil {
		reproductionInfo := struct {
			Environment             *map[string]string `json:"environment,omitempty"`
			LastReproductionAttempt *time.Time         `json:"last_reproduction_attempt,omitempty"`
			Reproducible            *bool              `json:"reproducible,omitempty"`
			ReproductionRate        *float32           `json:"reproduction_rate,omitempty"`
		}{
			Reproducible:            &crash.Reproducible,
			ReproductionRate:        crash.ReproductionRate,
			LastReproductionAttempt: crash.LastReproductionAttempt,
		}
		apiCrash.ReproductionInfo = &reproductionInfo
	}

	if len(crash.Tags) > 0 {
		apiCrash.Tags = &crash.Tags
	}

	return apiCrash
}

func (a *CrashAdapter) performDeduplication(ctx context.Context, crash *crashTypes.Crash, req generated.DeduplicateCrashJSONBody) generated.CrashDeduplicationResponse {
	// Mock implementation - in reality, this would use the deduplication service
	algorithmUsed := string(*req.Algorithm)

	// Simulate finding similar crashes
	similarCrashes := []struct {
		CrashId         *uuid.UUID `json:"crash_id,omitempty"`
		SimilarityScore *float32   `json:"similarity_score,omitempty"`
	}{
		{
			CrashId:         &[]uuid.UUID{uuid.New()}[0],
			SimilarityScore: &[]float32{0.95}[0],
		},
	}

	// Determine if crash is unique based on similarity threshold
	isUnique := true
	var duplicateOf *uuid.UUID
	var similarityScore *float32

	if len(similarCrashes) > 0 && *similarCrashes[0].SimilarityScore > *req.Threshold {
		isUnique = false
		duplicateOf = similarCrashes[0].CrashId
		similarityScore = similarCrashes[0].SimilarityScore
	}

	response := generated.CrashDeduplicationResponse{
		CrashId:         uuid.MustParse(crash.ID),
		AlgorithmUsed:   algorithmUsed,
		IsUnique:        isUnique,
		DuplicateOf:     duplicateOf,
		SimilarityScore: similarityScore,
		SimilarCrashes:  &similarCrashes,
	}

	return response
}

func (a *CrashAdapter) startMinimization(ctx context.Context, crash *crashTypes.Crash, req generated.MinimizeCrashJSONBody) error {
	// Mock implementation - in reality, this would queue a minimization job
	a.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"strategy": req.Strategy,
		"priority": req.Priority,
	}).Info("starting crash minimization")

	return nil
}

func (a *CrashAdapter) performReproduction(ctx context.Context, crash *crashTypes.Crash, req generated.ReproduceCrashJSONBody) struct {
	Attempts    int     `json:"attempts"`
	Successful  int     `json:"successful"`
	SuccessRate float32 `json:"success_rate"`
} {
	// Mock implementation - in reality, this would attempt to reproduce the crash
	attempts := *req.Attempts
	successful := attempts - 1 // Assume 1 failure
	successRate := float32(successful) / float32(attempts)

	return struct {
		Attempts    int     `json:"attempts"`
		Successful  int     `json:"successful"`
		SuccessRate float32 `json:"success_rate"`
	}{
		Attempts:    attempts,
		Successful:  successful,
		SuccessRate: successRate,
	}
}

// Type conversion helpers
func crashTypeToString(crashType generated.CrashType) string {
	return string(crashType)
}

func stringToCrashType(s string) generated.CrashType {
	switch s {
	case "segfault":
		return generated.CrashTypeSegfault
	case "abort":
		return generated.CrashTypeAbort
	case "assertion":
		return generated.CrashTypeAssertion
	case "heap_overflow":
		return generated.CrashTypeHeapOverflow
	case "stack_overflow":
		return generated.CrashTypeStackOverflow
	case "use_after_free":
		return generated.CrashTypeUseAfterFree
	case "double_free":
		return generated.CrashTypeDoubleFree
	case "memory_leak":
		return generated.CrashTypeMemoryLeak
	case "timeout":
		return generated.CrashTypeTimeout
	default:
		return generated.CrashTypeOther
	}
}

func crashSeverityToString(severity generated.CrashSeverity) string {
	return string(severity)
}

func stringToCrashSeverity(s string) generated.CrashSeverity {
	switch s {
	case "low":
		return generated.CrashSeverityLow
	case "medium":
		return generated.CrashSeverityMedium
	case "high":
		return generated.CrashSeverityHigh
	case "critical":
		return generated.CrashSeverityCritical
	default:
		return generated.CrashSeverityMedium
	}
}

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
