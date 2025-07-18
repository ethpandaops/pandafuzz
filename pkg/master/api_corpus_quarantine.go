package master

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// QuarantineRequest represents a request to quarantine a corpus file
type QuarantineRequest struct {
	FileID  string `json:"file_id"`
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

// RestoreRequest represents a request to restore a quarantined file
type RestoreRequest struct {
	FileID string `json:"file_id"`
	Notes  string `json:"notes"`
}

// DeleteQuarantineRequest represents a request to permanently delete a quarantined file
type DeleteQuarantineRequest struct {
	FileID string `json:"file_id"`
	Reason string `json:"reason"`
}

// QuarantineRuleRequest represents a request to enable/disable a quarantine rule
type QuarantineRuleRequest struct {
	RuleName string `json:"rule_name"`
	Enabled  bool   `json:"enabled"`
}

// QuarantineThresholdsRequest represents a request to update quarantine thresholds
type QuarantineThresholdsRequest struct {
	CrashThreshold   *int           `json:"crash_threshold,omitempty"`
	TimeoutThreshold *int           `json:"timeout_threshold,omitempty"`
	MemoryThreshold  *int64         `json:"memory_threshold,omitempty"`
	PerfThreshold    *time.Duration `json:"perf_threshold,omitempty"`
}

// QuarantineResponse represents the response for quarantine operations
type QuarantineResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	FileID  string `json:"file_id,omitempty"`
}

// handleQuarantineCorpusFile handles requests to quarantine a corpus file
func (s *Server) handleQuarantineCorpusFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["campaignID"]

	// Validate campaign exists
	storage, ok := s.state.db.(common.Storage)
	if !ok {
		s.logger.Error("Storage interface not available")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	campaign, err := storage.GetCampaign(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get campaign")
		http.Error(w, "Campaign not found", http.StatusNotFound)
		return
	}

	var req QuarantineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.FileID == "" {
		http.Error(w, "file_id is required", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context (if authentication is implemented)
	userID := "api_user" // TODO: Get from auth context

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		QuarantineFile(ctx interface{}, fileID string, reason string, details string) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Quarantine the file
	if err := corpusService.QuarantineFile(r.Context(), req.FileID, req.Reason, req.Details); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"file_id":     req.FileID,
			"reason":      req.Reason,
		}).Error("Failed to quarantine corpus file")
		http.Error(w, "Failed to quarantine file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaign.ID,
		"file_id":     req.FileID,
		"reason":      req.Reason,
		"user_id":     userID,
	}).Info("Corpus file quarantined via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(QuarantineResponse{
		Success: true,
		Message: "File quarantined successfully",
		FileID:  req.FileID,
	})
}

// handleRestoreQuarantinedFile handles requests to restore a quarantined file
func (s *Server) handleRestoreQuarantinedFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["campaignID"]

	// Validate campaign exists
	storage, ok := s.state.db.(common.Storage)
	if !ok {
		s.logger.Error("Storage interface not available")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	_, err := storage.GetCampaign(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get campaign")
		http.Error(w, "Campaign not found", http.StatusNotFound)
		return
	}

	var req RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.FileID == "" {
		http.Error(w, "file_id is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context
	userID := "api_user" // TODO: Get from auth context

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		RestoreQuarantinedFile(ctx interface{}, fileID string, restoredBy string, notes string) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Restore the file
	if err := corpusService.RestoreQuarantinedFile(r.Context(), req.FileID, userID, req.Notes); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"file_id":     req.FileID,
		}).Error("Failed to restore quarantined file")
		http.Error(w, "Failed to restore file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"file_id":     req.FileID,
		"user_id":     userID,
	}).Info("Quarantined file restored via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(QuarantineResponse{
		Success: true,
		Message: "File restored successfully",
		FileID:  req.FileID,
	})
}

// handleGetQuarantinedFiles handles requests to list quarantined files for a campaign
func (s *Server) handleGetQuarantinedFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["campaignID"]

	// Validate campaign exists
	storage, ok := s.state.db.(common.Storage)
	if !ok {
		s.logger.Error("Storage interface not available")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	_, err := storage.GetCampaign(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get campaign")
		http.Error(w, "Campaign not found", http.StatusNotFound)
		return
	}

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		GetQuarantinedFiles(ctx interface{}, campaignID string) ([]*common.QuarantinedFile, error)
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Get quarantined files
	files, err := corpusService.GetQuarantinedFiles(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).WithField("campaign_id", campaignID).Error("Failed to get quarantined files")
		http.Error(w, "Failed to get quarantined files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"campaign_id":       campaignID,
		"quarantined_files": files,
		"count":             len(files),
	})
}

// handleDeleteQuarantinedFile handles requests to permanently delete a quarantined file
func (s *Server) handleDeleteQuarantinedFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["campaignID"]

	// Validate campaign exists
	storage, ok := s.state.db.(common.Storage)
	if !ok {
		s.logger.Error("Storage interface not available")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	_, err := storage.GetCampaign(r.Context(), campaignID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get campaign")
		http.Error(w, "Campaign not found", http.StatusNotFound)
		return
	}

	var req DeleteQuarantineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.FileID == "" {
		http.Error(w, "file_id is required", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context
	userID := "api_user" // TODO: Get from auth context

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		DeleteQuarantinedFile(ctx interface{}, fileID string, deletedBy string, reason string) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Delete the file
	if err := corpusService.DeleteQuarantinedFile(r.Context(), req.FileID, userID, req.Reason); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"file_id":     req.FileID,
		}).Error("Failed to delete quarantined file")
		http.Error(w, "Failed to delete file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"file_id":     req.FileID,
		"user_id":     userID,
		"reason":      req.Reason,
	}).Info("Quarantined file permanently deleted via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(QuarantineResponse{
		Success: true,
		Message: "File deleted successfully",
		FileID:  req.FileID,
	})
}

// handleGetQuarantineRules handles requests to get quarantine rules
func (s *Server) handleGetQuarantineRules(w http.ResponseWriter, r *http.Request) {
	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		GetQuarantineRules() ([]service.QuarantineRule, error)
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Get rules
	rules, err := corpusService.GetQuarantineRules()
	if err != nil {
		s.logger.WithError(err).Error("Failed to get quarantine rules")
		http.Error(w, "Failed to get rules: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules": rules,
		"count": len(rules),
	})
}

// handleSetQuarantineRule handles requests to enable/disable a quarantine rule
func (s *Server) handleSetQuarantineRule(w http.ResponseWriter, r *http.Request) {
	var req QuarantineRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.RuleName == "" {
		http.Error(w, "rule_name is required", http.StatusBadRequest)
		return
	}

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		SetQuarantineRule(ruleName string, enabled bool) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Set rule
	if err := corpusService.SetQuarantineRule(req.RuleName, req.Enabled); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"rule_name": req.RuleName,
			"enabled":   req.Enabled,
		}).Error("Failed to set quarantine rule")
		http.Error(w, "Failed to set rule: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"rule_name": req.RuleName,
		"enabled":   req.Enabled,
	}).Info("Quarantine rule updated via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "Rule updated successfully",
		"rule_name": req.RuleName,
		"enabled":   req.Enabled,
	})
}

// handleSetQuarantineThresholds handles requests to update quarantine thresholds
func (s *Server) handleSetQuarantineThresholds(w http.ResponseWriter, r *http.Request) {
	var req QuarantineThresholdsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		SetQuarantineThresholds(crashes, timeouts int, memory int64, perfDuration time.Duration) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support quarantine operations")
		http.Error(w, "Quarantine not supported", http.StatusNotImplemented)
		return
	}

	// Extract values with defaults
	crashes := 0
	if req.CrashThreshold != nil {
		crashes = *req.CrashThreshold
	}
	timeouts := 0
	if req.TimeoutThreshold != nil {
		timeouts = *req.TimeoutThreshold
	}
	memory := int64(0)
	if req.MemoryThreshold != nil {
		memory = *req.MemoryThreshold
	}
	perfDuration := time.Duration(0)
	if req.PerfThreshold != nil {
		perfDuration = *req.PerfThreshold
	}

	// Set thresholds
	if err := corpusService.SetQuarantineThresholds(crashes, timeouts, memory, perfDuration); err != nil {
		s.logger.WithError(err).Error("Failed to set quarantine thresholds")
		http.Error(w, "Failed to set thresholds: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithFields(logrus.Fields{
		"crash_threshold":   crashes,
		"timeout_threshold": timeouts,
		"memory_threshold":  memory,
		"perf_threshold":    perfDuration,
	}).Info("Quarantine thresholds updated via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Thresholds updated successfully",
		"thresholds": map[string]interface{}{
			"crash_threshold":   crashes,
			"timeout_threshold": timeouts,
			"memory_threshold":  memory,
			"perf_threshold":    perfDuration.String(),
		},
	})
}

// handleUpdateCorpusFileMetrics handles requests to update corpus file metrics
func (s *Server) handleUpdateCorpusFileMetrics(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID := vars["fileID"]

	var metrics common.CorpusFileMetrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Ensure file ID matches
	metrics.FileID = fileID

	// Get corpus service
	corpusService, ok := s.services.Corpus.(interface {
		UpdateCorpusFileMetrics(ctx interface{}, fileID string, update func(*common.CorpusFileMetrics)) error
	})
	if !ok {
		s.logger.Error("Corpus service does not support metrics operations")
		http.Error(w, "Metrics not supported", http.StatusNotImplemented)
		return
	}

	// Update metrics
	if err := corpusService.UpdateCorpusFileMetrics(r.Context(), fileID, func(m *common.CorpusFileMetrics) {
		// Update fields that were provided
		if metrics.CrashCount > 0 {
			m.CrashCount = metrics.CrashCount
		}
		if metrics.TimeoutCount > 0 {
			m.TimeoutCount = metrics.TimeoutCount
		}
		if metrics.AvgExecTime > 0 {
			m.AvgExecTime = metrics.AvgExecTime
		}
		if metrics.MaxMemoryUsage > 0 {
			m.MaxMemoryUsage = metrics.MaxMemoryUsage
		}
		if metrics.ExecCount > 0 {
			m.ExecCount = metrics.ExecCount
		}
		m.LastExecuted = time.Now()
	}); err != nil {
		s.logger.WithError(err).WithField("file_id", fileID).Error("Failed to update corpus file metrics")
		http.Error(w, "Failed to update metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.WithField("file_id", fileID).Info("Corpus file metrics updated via API")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Metrics updated successfully",
		"file_id": fileID,
	})
}
