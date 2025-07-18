package master

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/httputil"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// ReproductionRequest represents the request body for queueing a reproduction
type ReproductionRequest struct {
	Priority int `json:"priority" validate:"min=0,max=10"`
}

// ReproductionResultRequest represents the request body for recording a result
type ReproductionResultRequest struct {
	*common.ReproductionResult
}

// ReproductionStatusResponse represents the response for getting reproduction status
type ReproductionStatusResponse struct {
	Request *common.ReproductionRequest `json:"request"`
	Score   float64                     `json:"score,omitempty"`
}

// ReproductionResultsResponse represents the response for getting reproduction results
type ReproductionResultsResponse struct {
	Results []*common.ReproductionResult `json:"results"`
	Score   float64                      `json:"score"`
	Count   int                          `json:"count"`
}

// handleCrashReproduce handles POST /api/crashes/:id/reproduce
// Queue a crash for reproduction testing
func (s *Server) handleCrashReproduce(w http.ResponseWriter, r *http.Request) {
	// Extract crash ID from URL
	vars := mux.Vars(r)
	crashID := vars["crashID"]
	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Parse request body
	var req ReproductionRequest
	if r.ContentLength > 0 {
		if err := s.decodeJSONBody(w, r, &req); err != nil {
			// Error response already written by decodeJSONBody
			return
		}
	}

	// Default priority if not specified
	if req.Priority == 0 {
		req.Priority = 5 // Medium priority
	}

	// Validate priority range
	if req.Priority < 0 || req.Priority > 10 {
		s.writeErrorResponse(w, http.StatusBadRequest, "Priority must be between 0 and 10", nil)
		return
	}

	// Check if reproducibility service is available
	if s.services.Reproducibility == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Reproducibility service not available", nil)
		return
	}

	// Queue the reproduction request
	err := s.services.Reproducibility.QueueReproduction(r.Context(), crashID, req.Priority)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"crash_id": crashID,
			"priority": req.Priority,
		}).Error("Failed to queue reproduction")
		s.responseWriter.WriteError(w, err)
		return
	}

	// Return success response
	response := httputil.NewSuccessResponse("Reproduction request queued successfully", map[string]any{
		"crash_id": crashID,
		"priority": req.Priority,
		"status":   "queued",
	})
	s.writeJSONResponse(w, response)
}

// handleGetCrashReproduction handles GET /api/crashes/:id/reproduction
// Get the current status of a reproduction task
func (s *Server) handleGetCrashReproduction(w http.ResponseWriter, r *http.Request) {
	// Extract crash ID from URL
	vars := mux.Vars(r)
	crashID := vars["crashID"]
	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Check if reproducibility service is available
	if s.services.Reproducibility == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Reproducibility service not available", nil)
		return
	}

	// Get reproduction status
	request, err := s.services.Reproducibility.GetReproductionStatus(r.Context(), crashID)
	if err != nil {
		s.logger.WithError(err).WithField("crash_id", crashID).Error("Failed to get reproduction status")
		s.responseWriter.WriteError(w, err)
		return
	}

	// Calculate score if available
	var score float64
	if request.Status == common.ReproducibilityStatusConfirmed ||
		request.Status == common.ReproducibilityStatusFlaky ||
		request.Status == common.ReproducibilityStatusFailed {
		score, _ = s.services.Reproducibility.CalculateReproducibilityScore(r.Context(), crashID)
	}

	// Return response
	response := ReproductionStatusResponse{
		Request: request,
		Score:   score,
	}
	s.writeJSONResponse(w, response)
}

// handleSubmitReproductionResult handles POST /api/reproduction/results
// Record the result of a reproduction attempt
func (s *Server) handleSubmitReproductionResult(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req ReproductionResultRequest
	if err := s.decodeJSONBody(w, r, &req); err != nil {
		// Error response already written by decodeJSONBody
		return
	}

	// Validate required fields
	if req.ReproductionResult == nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Reproduction result is required", nil)
		return
	}

	if req.CrashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	if req.RequestID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Request ID is required", nil)
		return
	}

	if req.BotID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Bot ID is required", nil)
		return
	}

	// Check if reproducibility service is available
	if s.services.Reproducibility == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Reproducibility service not available", nil)
		return
	}

	// Record the result
	err := s.services.Reproducibility.RecordReproductionResult(r.Context(), req.ReproductionResult)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"crash_id":   req.CrashID,
			"request_id": req.RequestID,
			"bot_id":     req.BotID,
		}).Error("Failed to record reproduction result")
		s.responseWriter.WriteError(w, err)
		return
	}

	// Return success response
	response := httputil.NewSuccessResponse("Reproduction result recorded successfully", map[string]any{
		"crash_id":   req.CrashID,
		"request_id": req.RequestID,
		"reproduced": req.Reproduced,
	})
	s.writeJSONResponse(w, response)
}

// handleGetReproductionResults handles GET /api/crashes/:id/reproduction/results
// Get all reproduction results for a crash
func (s *Server) handleGetReproductionResults(w http.ResponseWriter, r *http.Request) {
	// Extract crash ID from URL
	vars := mux.Vars(r)
	crashID := vars["crashID"]
	if crashID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID is required", nil)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Pagination parameters
	page := 1
	limit := 50
	if pageStr := query.Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Check if reproducibility service is available
	if s.services.Reproducibility == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Reproducibility service not available", nil)
		return
	}

	// Get reproduction results
	results, err := s.services.Reproducibility.GetReproductionResults(r.Context(), crashID)
	if err != nil {
		s.logger.WithError(err).WithField("crash_id", crashID).Error("Failed to get reproduction results")
		s.responseWriter.WriteError(w, err)
		return
	}

	// Calculate reproducibility score
	score, _ := s.services.Reproducibility.CalculateReproducibilityScore(r.Context(), crashID)

	// Apply pagination
	total := len(results)
	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginatedResults := results[start:end]

	// Return response
	if query.Get("format") == "paginated" {
		response := httputil.NewPaginatedResponse(paginatedResults, total, page, limit)
		s.writeJSONResponse(w, response)
	} else {
		response := ReproductionResultsResponse{
			Results: paginatedResults,
			Score:   score,
			Count:   total,
		}
		s.writeJSONResponse(w, response)
	}
}

// Helper validation functions

// validateReproductionResult validates a reproduction result
func (s *Server) validateReproductionResult(result *common.ReproductionResult) error {
	if result == nil {
		return errors.NewValidationError("reproduction_result", "Result cannot be nil")
	}

	if result.CrashID == "" {
		return errors.NewValidationError("crash_id", "Crash ID is required")
	}

	if result.RequestID == "" {
		return errors.NewValidationError("request_id", "Request ID is required")
	}

	if result.BotID == "" {
		return errors.NewValidationError("bot_id", "Bot ID is required")
	}

	if result.AttemptNumber <= 0 {
		return errors.NewValidationError("attempt_number", "Attempt number must be positive")
	}

	return nil
}
