package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HandleJobAck handles POST /api/v1/jobs/{id}/ack
func (h *Handlers) HandleJobAck(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var request struct {
		BotID      string `json:"bot_id"`
		JobID      string `json:"job_id"`
		LeaseToken string `json:"lease_token"`
		Status     string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the job ID matches
	if request.JobID != jobID {
		http.Error(w, "Job ID mismatch", http.StatusBadRequest)
		return
	}

	// Forward to the job adapter
	h.adapter.AckJob(w, r, jobID, request.BotID, request.LeaseToken)
}

// HandleJobHeartbeat handles POST /api/v1/jobs/{id}/heartbeat
func (h *Handlers) HandleJobHeartbeat(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var request struct {
		BotID      string `json:"bot_id"`
		JobID      string `json:"job_id"`
		LeaseToken string `json:"lease_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate the job ID matches
	if request.JobID != jobID {
		http.Error(w, "Job ID mismatch", http.StatusBadRequest)
		return
	}

	// Forward to the job adapter
	h.adapter.JobHeartbeat(w, r, jobID, request.BotID, request.LeaseToken)
}
