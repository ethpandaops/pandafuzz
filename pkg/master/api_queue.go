package master

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
)

// respondWithJSON writes a JSON response with the given status code
func (s *Server) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

// respondWithError writes an error response with the given status code
func (s *Server) respondWithError(w http.ResponseWriter, code int, operation string, message string) {
	s.respondWithJSON(w, code, map[string]interface{}{
		"error":     message,
		"operation": operation,
		"code":      code,
	})
}

// handleQueueStats returns overall queue statistics
func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	// Check if queue backend is enabled
	if !s.isQueueBackendEnabled() {
		s.respondWithError(w, http.StatusNotFound, "queue_stats", "Queue statistics not available in polling mode")
		return
	}

	// Get queue stats from job service
	stats, err := s.services.Job.GetQueueStats(r.Context())
	if err != nil {
		s.logger.WithError(err).Error("Failed to get queue statistics")
		s.respondWithError(w, http.StatusInternalServerError, "queue_stats", "Failed to retrieve queue statistics")
		return
	}

	s.respondWithJSON(w, http.StatusOK, stats)
}

// handleQueueStatsDetail returns detailed statistics for a specific queue
func (s *Server) handleQueueStatsDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	queueName := vars["queue"]

	if queueName == "" {
		s.respondWithError(w, http.StatusBadRequest, "queue_stats_detail", "Queue name is required")
		return
	}

	// Check if queue backend is enabled
	if !s.isQueueBackendEnabled() {
		s.respondWithError(w, http.StatusNotFound, "queue_stats_detail", "Queue statistics not available in polling mode")
		return
	}

	// For now, return a simplified response
	// In a full implementation, this would get detailed stats for the specific queue
	response := map[string]interface{}{
		"queue":   queueName,
		"message": "Detailed queue statistics not yet implemented",
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleQueuePause pauses queue processing
func (s *Server) handleQueuePause(w http.ResponseWriter, r *http.Request) {
	// Check if queue backend is enabled
	if !s.isQueueBackendEnabled() {
		s.respondWithError(w, http.StatusNotFound, "queue_pause", "Queue operations not available in polling mode")
		return
	}

	// Parse request body
	var req struct {
		Queue string `json:"queue,omitempty"` // Optional: specific queue to pause
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "queue_pause", "Invalid request body")
		return
	}

	// For now, return a success response
	// In a full implementation, this would pause the queue processing
	response := map[string]interface{}{
		"status":  "success",
		"message": "Queue pause functionality not yet implemented",
		"queue":   req.Queue,
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleQueueResume resumes queue processing
func (s *Server) handleQueueResume(w http.ResponseWriter, r *http.Request) {
	// Check if queue backend is enabled
	if !s.isQueueBackendEnabled() {
		s.respondWithError(w, http.StatusNotFound, "queue_resume", "Queue operations not available in polling mode")
		return
	}

	// Parse request body
	var req struct {
		Queue string `json:"queue,omitempty"` // Optional: specific queue to resume
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondWithError(w, http.StatusBadRequest, "queue_resume", "Invalid request body")
		return
	}

	// For now, return a success response
	// In a full implementation, this would resume the queue processing
	response := map[string]interface{}{
		"status":  "success",
		"message": "Queue resume functionality not yet implemented",
		"queue":   req.Queue,
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// handleQueuePurge purges all jobs from a specific queue
func (s *Server) handleQueuePurge(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	queueName := vars["queue"]

	if queueName == "" {
		s.respondWithError(w, http.StatusBadRequest, "queue_purge", "Queue name is required")
		return
	}

	// Check if queue backend is enabled
	if !s.isQueueBackendEnabled() {
		s.respondWithError(w, http.StatusNotFound, "queue_purge", "Queue operations not available in polling mode")
		return
	}

	// Validate queue name
	validQueues := []string{"critical", "default", "low"}
	isValid := false
	for _, q := range validQueues {
		if q == queueName {
			isValid = true
			break
		}
	}

	if !isValid {
		s.respondWithError(w, http.StatusBadRequest, "queue_purge", fmt.Sprintf("Invalid queue name. Must be one of: %v", validQueues))
		return
	}

	// For now, return a success response
	// In a full implementation, this would purge the queue
	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Queue purge functionality not yet implemented for queue: %s", queueName),
		"queue":   queueName,
	}

	s.respondWithJSON(w, http.StatusOK, response)
}

// isQueueBackendEnabled checks if the queue backend is enabled
func (s *Server) isQueueBackendEnabled() bool {
	// Check if asynq backend is configured
	return s.config.Queue.Backend == "asynq"
}

// QueueStatsResponse represents the queue statistics response
type QueueStatsResponse struct {
	Backend   string              `json:"backend"`
	Stats     *service.QueueStats `json:"stats,omitempty"`
	Error     string              `json:"error,omitempty"`
	Timestamp int64               `json:"timestamp"`
}
