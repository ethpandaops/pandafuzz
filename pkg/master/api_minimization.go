package master

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// MinimizationRequest represents a request to minimize a crash
type MinimizationRequest struct {
	CrashID  string                       `json:"crash_id"`
	Strategy service.MinimizationStrategy `json:"strategy"`
	Priority int                          `json:"priority,omitempty"`
}

// MinimizationResponse represents the response for minimization operations
type MinimizationResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message,omitempty"`
	Result  *common.MinimizationResult   `json:"result,omitempty"`
	Results []*common.MinimizationResult `json:"results,omitempty"`
	Stats   map[string]interface{}       `json:"stats,omitempty"`
}

// handleMinimizeCrash handles POST /api/v1/crashes/{id}/minimize
func (s *Server) handleMinimizeCrash(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	// Parse request body
	var req MinimizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Default to crash ID from URL if not in body
	if req.CrashID == "" {
		req.CrashID = crashID
	}

	// Validate crash ID match
	if req.CrashID != crashID {
		s.writeErrorResponse(w, http.StatusBadRequest, "Crash ID mismatch", nil)
		return
	}

	// Default strategy if not specified
	if req.Strategy == "" {
		req.Strategy = service.MinimizationStrategyDeltaDebug
	}

	// Check if crash minimizer service is available
	if s.services.CrashMinimizer == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Crash minimizer service not available", nil)
		return
	}

	// Create minimization job
	crash, err := s.state.GetCrash(r.Context(), crashID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Crash not found", err)
		return
	}

	// Create a minimization job
	minJob := service.CreateMinimizationJob(crash, req.Strategy)
	minJob.Type = common.JobTypeMinimization
	minJob.Metadata = map[string]interface{}{
		"crash_id": crashID,
		"strategy": string(req.Strategy),
		"priority": req.Priority,
	}

	// Create the job
	if err := s.state.SaveJobWithRetry(r.Context(), minJob.Job); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create minimization job", err)
		return
	}

	// Queue for execution
	// TODO: Implement job queueing
	// s.queueJob(minJob.Job)

	// Return success response
	resp := MinimizationResponse{
		Success: true,
		Message: "Minimization job created",
		Result: &common.MinimizationResult{
			ID:        minJob.ID,
			CrashID:   crashID,
			JobID:     minJob.ID,
			Strategy:  string(req.Strategy),
			Timestamp: time.Now(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

// handleGetMinimizationResult handles GET /api/v1/minimization/{id}
func (s *Server) handleGetMinimizationResult(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	resultID := vars["id"]

	// Check if service is available
	if s.services.CrashMinimizer == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Crash minimizer service not available", nil)
		return
	}

	// Get the result
	result, err := s.services.CrashMinimizer.GetMinimizationResult(r.Context(), resultID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Minimization result not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get minimization result", err)
		}
		return
	}

	resp := MinimizationResponse{
		Success: true,
		Result:  result,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleListMinimizationResults handles GET /api/v1/crashes/{id}/minimizations
func (s *Server) handleListMinimizationResults(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	// Check if service is available
	if s.services.CrashMinimizer == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Crash minimizer service not available", nil)
		return
	}

	// Get all minimization results for the crash
	results, err := s.services.CrashMinimizer.ListMinimizationResults(r.Context(), crashID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list minimization results", err)
		return
	}

	resp := MinimizationResponse{
		Success: true,
		Results: results,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleGetBestMinimization handles GET /api/v1/crashes/{id}/minimizations/best
func (s *Server) handleGetBestMinimization(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	crashID := vars["id"]

	// Check if service is available
	if s.services.CrashMinimizer == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Crash minimizer service not available", nil)
		return
	}

	// Get the best minimization
	result, err := s.services.CrashMinimizer.GetBestMinimization(r.Context(), crashID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "No successful minimization found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get best minimization", err)
		}
		return
	}

	resp := MinimizationResponse{
		Success: true,
		Result:  result,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleGetMinimizationStats handles GET /api/v1/campaigns/{id}/minimization-stats
func (s *Server) handleGetMinimizationStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]

	// Get minimization statistics from storage
	// TODO: Implement GetMinimizationStats
	stats := make(map[string]interface{})
	stats["campaign_id"] = campaignID
	stats["total_minimizations"] = 0
	stats["successful"] = 0
	stats["failed"] = 0

	resp := MinimizationResponse{
		Success: true,
		Stats:   stats,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleReportMinimizationResult handles POST /api/v1/minimization/result
func (s *Server) handleReportMinimizationResult(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var result common.MinimizationResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Get bot ID from header
	botID := r.Header.Get("X-Bot-ID")
	if botID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Missing X-Bot-ID header", nil)
		return
	}

	// Ensure bot ID matches
	if result.BotID != botID {
		s.writeErrorResponse(w, http.StatusForbidden, "Bot ID mismatch", nil)
		return
	}

	// Store the minimization result
	// TODO: Implement CreateMinimizationResult
	// if err := s.state.CreateMinimizationResult(r.Context(), &result); err != nil {
	//	s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to store minimization result", err)
	//	return
	// }

	// Update job status if job ID is provided
	if result.JobID != "" {
		status := common.JobStatusCompleted
		if !result.Success {
			status = common.JobStatusFailed
		}
		_ = map[string]interface{}{
			"status":       status,
			"completed_at": time.Now(),
		}
		// TODO: Implement UpdateJob
		// if err := s.state.UpdateJob(r.Context(), result.JobID, updates); err != nil {
		//	s.logger.WithError(err).Warn("Failed to update job status after minimization")
		// }
	}

	// Send success response
	resp := MinimizationResponse{
		Success: true,
		Message: "Minimization result recorded",
		Result:  &result,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// registerMinimizationRoutes registers all minimization-related routes
func (s *Server) registerMinimizationRoutes(v1 *mux.Router) {
	// Crash minimization endpoints
	v1.HandleFunc("/crashes/{id}/minimize", s.handleMinimizeCrash).Methods("POST")
	v1.HandleFunc("/crashes/{id}/minimizations", s.handleListMinimizationResults).Methods("GET")
	v1.HandleFunc("/crashes/{id}/minimizations/best", s.handleGetBestMinimization).Methods("GET")

	// Minimization result endpoints
	v1.HandleFunc("/minimization/{id}", s.handleGetMinimizationResult).Methods("GET")
	v1.HandleFunc("/minimization/result", s.handleReportMinimizationResult).Methods("POST")

	// Campaign minimization stats
	v1.HandleFunc("/campaigns/{id}/minimization-stats", s.handleGetMinimizationStats).Methods("GET")
}
