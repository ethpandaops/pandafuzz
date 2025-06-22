package master

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// LogPushRequest represents a log push request from bot
type LogPushRequest struct {
	JobID   string `json:"job_id"`
	BotID   string `json:"bot_id"`
	Content string `json:"content"` // Base64 encoded log content for JSON transport
}

// handleLogPush handles log push from bots
func (s *Server) handleLogPush(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Verify job exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Parse request based on content type
	contentType := r.Header.Get("Content-Type")
	
	var logContent []byte
	var botID string

	if contentType == "application/json" {
		// JSON request with base64 encoded content
		var req LogPushRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", err)
			return
		}
		botID = req.BotID
		// Decode base64 content
		logContent = []byte(req.Content)
	} else {
		// Raw log file upload
		botID = r.Header.Get("X-Bot-ID")
		if botID == "" {
			s.writeErrorResponse(w, http.StatusBadRequest, "X-Bot-ID header is required", nil)
			return
		}
		
		// Read raw content
		content, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeErrorResponse(w, http.StatusBadRequest, "Failed to read request body", err)
			return
		}
		logContent = content
	}

	// Verify bot is assigned to this job
	if job.AssignedBot == nil || *job.AssignedBot != botID {
		s.writeErrorResponse(w, http.StatusForbidden, "Bot is not assigned to this job", nil)
		return
	}

	// Create log storage directory
	logDir := filepath.Join(s.config.Storage.BasePath, "logs", jobID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create log directory", err)
		return
	}

	// Write log file
	logPath := filepath.Join(logDir, "job.log")
	if err := os.WriteFile(logPath, logContent, 0644); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to write log file", err)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"bot_id": botID,
		"log_size": len(logContent),
		"log_path": logPath,
	}).Info("Job logs pushed successfully")

	response := map[string]interface{}{
		"status": "success",
		"job_id": jobID,
		"size": len(logContent),
	}

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, response)
}

// handleLogExists checks if logs exist for a job
func (s *Server) handleLogExists(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Check if logs exist in storage
	logPath := filepath.Join(s.config.Storage.BasePath, "logs", jobID, "job.log")
	_, err := os.Stat(logPath)
	exists := err == nil

	response := map[string]interface{}{
		"job_id": jobID,
		"exists": exists,
		"path": logPath,
	}

	s.writeJSONResponse(w, response)
}