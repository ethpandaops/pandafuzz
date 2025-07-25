package crash

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/model"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Handler handles crash-related HTTP requests
type Handler struct {
	resultService service.ResultService
	storage       common.Storage
	logger        logrus.FieldLogger
}

// NewHandler creates a new crash handler
func NewHandler(
	resultService service.ResultService,
	storage common.Storage,
	logger logrus.FieldLogger,
) *Handler {
	return &Handler{
		resultService: resultService,
		storage:       storage,
		logger:        logger.WithField("component", "crash_handler"),
	}
}

// SubmitCrash handles crash result submission
func (h *Handler) SubmitCrash(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("Received crash result submission request")

	var crash common.CrashResult
	if err := json.NewDecoder(r.Body).Decode(&crash); err != nil {
		h.logger.WithError(err).Error("Failed to decode crash result")
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid crash result", err)
		return
	}

	// Convert InputBase64 to Input if provided
	if crash.InputBase64 != "" {
		decodedInput, err := base64.StdEncoding.DecodeString(crash.InputBase64)
		if err != nil {
			h.logger.WithError(err).Error("Failed to decode InputBase64")
			h.writeErrorResponse(w, http.StatusBadRequest, "Invalid base64 encoded input", err)
			return
		}
		crash.Input = decodedInput
	}

	// Log crash details
	h.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"job_id":   crash.JobID,
		"bot_id":   crash.BotID,
		"hash":     crash.Hash,
		"size":     crash.Size,
	}).Debug("Processing crash submission")

	// Validate crash result
	if crash.JobID == "" || crash.BotID == "" {
		h.logger.Error("Crash result missing required fields")
		h.writeErrorResponse(w, http.StatusBadRequest, "Job ID and Bot ID are required", nil)
		return
	}

	// Set timestamp if not provided
	if crash.Timestamp.IsZero() {
		crash.Timestamp = time.Now()
	}

	// Process crash result
	if err := h.resultService.ProcessCrashResult(r.Context(), &crash); err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to process crash result", err)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"is_unique": crash.IsUnique,
	}).Info("Crash result processed")

	response := map[string]interface{}{
		"status":    "processed",
		"crash_id":  crash.ID,
		"is_unique": crash.IsUnique,
		"timestamp": time.Now(),
	}

	h.writeJSONResponse(w, response)
}

// List retrieves all crashes
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	// Get query parameters
	limit := 100
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Parse sorting parameters
	sortBy := r.URL.Query().Get("sort_by")
	sortOrder := r.URL.Query().Get("sort_order")

	// Validate sort parameters using the model types
	var sortField model.CrashSortField
	switch sortBy {
	case string(model.CrashSortFieldTimestamp):
		sortField = model.CrashSortFieldTimestamp
	case string(model.CrashSortFieldType):
		sortField = model.CrashSortFieldType
	case string(model.CrashSortFieldSignal):
		sortField = model.CrashSortFieldSignal
	case string(model.CrashSortFieldSize):
		sortField = model.CrashSortFieldSize
	case string(model.CrashSortFieldJobID):
		sortField = model.CrashSortFieldJobID
	case string(model.CrashSortFieldBotID):
		sortField = model.CrashSortFieldBotID
	default:
		sortField = model.CrashSortFieldTimestamp // Default sort
	}

	var order model.SortOrder
	switch sortOrder {
	case string(model.SortOrderAsc):
		order = model.SortOrderAsc
	case string(model.SortOrderDesc):
		order = model.SortOrderDesc
	default:
		order = model.SortOrderDesc // Default order
	}

	// Get crashes from storage
	crashes, err := h.getCrashesSorted(r.Context(), limit, offset, string(sortField), string(order))
	if err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{
			"limit":      limit,
			"offset":     offset,
			"sort_by":    string(sortField),
			"sort_order": string(order),
		}).Error("Failed to retrieve crashes from database")
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crashes", err)
		return
	}

	// Ensure we have a valid slice even if no crashes found
	if crashes == nil {
		crashes = make([]*common.CrashResult, 0)
	}

	response := CrashListResponse{
		Crashes:   crashes,
		Count:     len(crashes),
		Limit:     limit,
		Offset:    offset,
		SortBy:    string(sortField),
		SortOrder: string(order),
	}

	h.writeJSONResponse(w, response)
}

// Get retrieves a specific crash by ID
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	if crashID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	crash, err := h.storage.GetCrash(r.Context(), crashID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crash", err)
		return
	}

	if crash == nil {
		h.writeErrorResponse(w, http.StatusNotFound, "Crash not found", nil)
		return
	}

	h.writeJSONResponse(w, crash)
}

// GetJobCrashes retrieves all crashes for a specific job
func (h *Handler) GetJobCrashes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Verify job exists
	job, err := h.storage.GetJob(r.Context(), jobID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job", err)
		return
	}

	if job == nil {
		h.writeErrorResponse(w, http.StatusNotFound, "Job not found", nil)
		return
	}

	// Get crashes for this job
	crashes, err := h.resultService.GetCrashResults(r.Context(), jobID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve job crashes", err)
		return
	}

	response := map[string]interface{}{
		"job_id":  jobID,
		"crashes": crashes,
		"count":   len(crashes),
	}

	h.writeJSONResponse(w, response)
}

// GetCrashInput retrieves the input file for a specific crash
func (h *Handler) GetCrashInput(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	if crashID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Get crash to retrieve input data
	crash, err := h.storage.GetCrash(r.Context(), crashID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve crash", err)
		return
	}

	if crash == nil || len(crash.Input) == 0 {
		h.writeErrorResponse(w, http.StatusNotFound, "Crash input not found", nil)
		return
	}

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"crash_%s.bin\"", crashID[:8]))
	w.Header().Set("Content-Length", strconv.Itoa(len(crash.Input)))

	// Write the binary data
	if _, err := w.Write(crash.Input); err != nil {
		h.logger.WithError(err).Error("Failed to write crash input response")
	}
}

// GetCrashGroups retrieves deduplicated crash groups for a campaign
func (h *Handler) GetCrashGroups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	if campaignID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID is required", nil)
		return
	}

	// Get query parameters for filtering
	severityFilter := r.URL.Query().Get("severity")
	minCount := 1
	if minCountStr := r.URL.Query().Get("min_count"); minCountStr != "" {
		if mc, err := strconv.Atoi(minCountStr); err == nil && mc > 0 {
			minCount = mc
		}
	}

	// Get crash groups from storage
	groups, err := h.storage.ListCrashGroups(r.Context(), campaignID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get crash groups", err)
		return
	}

	// Apply filters
	var filtered []*common.CrashGroup
	for _, group := range groups {
		// Filter by severity
		if severityFilter != "" && group.Severity != severityFilter {
			continue
		}

		// Filter by minimum count
		if group.Count < minCount {
			continue
		}

		filtered = append(filtered, group)
	}

	// Calculate statistics
	uniqueCrashes := len(filtered)
	totalCrashes := 0
	severityCounts := make(map[string]int)

	for _, group := range filtered {
		totalCrashes += group.Count
		severityCounts[group.Severity]++
	}

	response := CrashGroupsResponse{
		CampaignID:    campaignID,
		CrashGroups:   filtered,
		UniqueCrashes: uniqueCrashes,
		TotalCrashes:  totalCrashes,
		Severities:    severityCounts,
		Filters: map[string]interface{}{
			"severity":  severityFilter,
			"min_count": minCount,
		},
	}

	h.writeJSONResponse(w, response)
}

// GetStackTrace retrieves detailed stack trace for a crash
func (h *Handler) GetStackTrace(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["crash_id"]

	if crashID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Get stack trace from storage
	stackTrace, err := h.storage.GetStackTrace(r.Context(), crashID)
	if err != nil {
		h.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get stack trace", err)
		return
	}

	if stackTrace == nil {
		h.writeErrorResponse(w, http.StatusNotFound, "Stack trace not found", nil)
		return
	}

	// Get crash details for context
	crash, err := h.storage.GetCrash(r.Context(), crashID)
	if err != nil {
		h.logger.WithError(err).Warn("Failed to get crash details")
		// Continue without crash details
	}

	response := StackTraceResponse{
		CrashID:    crashID,
		StackTrace: stackTrace,
	}

	if crash != nil {
		response.CrashDetails = map[string]interface{}{
			"job_id":    crash.JobID,
			"bot_id":    crash.BotID,
			"timestamp": crash.Timestamp,
			"type":      crash.Type,
			"signal":    crash.Signal,
		}
	}

	h.writeJSONResponse(w, response)
}

// BatchSubmit handles batch result submission
func (h *Handler) BatchSubmit(w http.ResponseWriter, r *http.Request) {
	var req BatchResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate request
	if req.BotID == "" || req.JobID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "Bot ID and Job ID are required", nil)
		return
	}

	// Track processing results
	processedCrashes := 0
	processedCoverage := 0
	processedCorpus := 0
	errors := []string{}

	// Process crashes
	for i := range req.Crashes {
		crash := &req.Crashes[i]
		crash.BotID = req.BotID
		crash.JobID = req.JobID
		if crash.Timestamp.IsZero() {
			crash.Timestamp = time.Now()
		}

		// Convert InputBase64 to Input if provided
		if crash.InputBase64 != "" && len(crash.Input) == 0 {
			decodedInput, err := base64.StdEncoding.DecodeString(crash.InputBase64)
			if err != nil {
				errors = append(errors, fmt.Sprintf("crash %d: failed to decode InputBase64: %v", i, err))
				continue
			}
			crash.Input = decodedInput
			crash.InputBase64 = "" // Clear after processing
		}

		if err := h.resultService.ProcessCrashResult(r.Context(), crash); err != nil {
			errors = append(errors, fmt.Sprintf("crash %d: %v", i, err))
		} else {
			processedCrashes++
		}
	}

	// Process coverage
	for i := range req.Coverage {
		coverage := &req.Coverage[i]
		coverage.BotID = req.BotID
		coverage.JobID = req.JobID
		if coverage.Timestamp.IsZero() {
			coverage.Timestamp = time.Now()
		}

		if err := h.resultService.ProcessCoverageResult(r.Context(), coverage); err != nil {
			errors = append(errors, fmt.Sprintf("coverage %d: %v", i, err))
		} else {
			processedCoverage++
		}
	}

	// Process corpus updates
	for i := range req.Corpus {
		corpus := &req.Corpus[i]
		corpus.BotID = req.BotID
		corpus.JobID = req.JobID
		if corpus.Timestamp.IsZero() {
			corpus.Timestamp = time.Now()
		}

		if err := h.resultService.ProcessCorpusUpdate(r.Context(), corpus); err != nil {
			errors = append(errors, fmt.Sprintf("corpus %d: %v", i, err))
		} else {
			processedCorpus++
		}
	}

	h.logger.WithFields(logrus.Fields{
		"bot_id":             req.BotID,
		"job_id":             req.JobID,
		"crashes_processed":  processedCrashes,
		"coverage_processed": processedCoverage,
		"corpus_processed":   processedCorpus,
		"errors":             len(errors),
	}).Info("Batch results processed")

	response := BatchResultResponse{
		Status: "processed",
		BotID:  req.BotID,
		JobID:  req.JobID,
		Processed: map[string]int{
			"crashes":  processedCrashes,
			"coverage": processedCoverage,
			"corpus":   processedCorpus,
		},
		Timestamp: time.Now(),
	}

	if len(errors) > 0 {
		response.Errors = errors
		response.PartialSuccess = true
	}

	h.writeJSONResponse(w, response)
}

// Helper methods

func (h *Handler) getCrashesSorted(ctx context.Context, limit, offset int, sortBy, sortOrder string) ([]*common.CrashResult, error) {
	// This is a simplified implementation - in production, you would have proper
	// database queries with sorting
	crashes, err := h.storage.ListCrashes(ctx, "", limit, offset)
	if err != nil {
		return nil, err
	}

	return crashes, nil
}
