package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// QuarantineReason represents why a corpus file was quarantined
type QuarantineReason string

const (
	// Automatic quarantine reasons
	QuarantineReasonCrashLoop    QuarantineReason = "crash_loop"       // File causes repeated crashes
	QuarantineReasonTimeout      QuarantineReason = "timeout"          // File causes consistent timeouts
	QuarantineReasonMemoryExcess QuarantineReason = "memory_excess"    // File uses excessive memory
	QuarantineReasonPerformance  QuarantineReason = "performance"      // File significantly slows fuzzing
	QuarantineReasonValidation   QuarantineReason = "validation_error" // File fails validation checks
	QuarantineReasonMalformed    QuarantineReason = "malformed"        // File has invalid format
	QuarantineReasonDuplicate    QuarantineReason = "duplicate"        // Duplicate with different hash

	// Manual quarantine reasons
	QuarantineReasonManual        QuarantineReason = "manual"        // Manually quarantined by user
	QuarantineReasonSecurity      QuarantineReason = "security"      // Security concerns
	QuarantineReasonInvestigation QuarantineReason = "investigation" // Under investigation
)

// QuarantineRule defines a rule for automatic quarantine
type QuarantineRule struct {
	Name        string
	Description string
	Check       func(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) (bool, QuarantineReason, string)
	Enabled     bool
}

// CorpusQuarantine manages quarantined corpus files
type CorpusQuarantine struct {
	storage     common.Storage
	fileStorage common.FileStorage
	logger      logrus.FieldLogger
	rules       []QuarantineRule
	mu          sync.RWMutex

	// Configuration
	crashThreshold   int           // Max crashes before quarantine
	timeoutThreshold int           // Max timeouts before quarantine
	memoryThreshold  int64         // Max memory usage in bytes
	perfThreshold    time.Duration // Max execution time
}

// NewCorpusQuarantine creates a new corpus quarantine manager
func NewCorpusQuarantine(storage common.Storage, fileStorage common.FileStorage, logger logrus.FieldLogger) *CorpusQuarantine {
	cq := &CorpusQuarantine{
		storage:          storage,
		fileStorage:      fileStorage,
		logger:           logger.WithField("component", "corpus_quarantine"),
		crashThreshold:   5,                 // Default: 5 crashes
		timeoutThreshold: 10,                // Default: 10 timeouts
		memoryThreshold:  1024 * 1024 * 500, // Default: 500MB
		perfThreshold:    30 * time.Second,  // Default: 30 seconds
	}

	// Initialize default rules
	cq.initializeRules()

	return cq
}

// initializeRules sets up the default quarantine rules
func (cq *CorpusQuarantine) initializeRules() {
	cq.rules = []QuarantineRule{
		{
			Name:        "crash_loop_detection",
			Description: "Quarantine files causing repeated crashes",
			Enabled:     true,
			Check: func(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) (bool, QuarantineReason, string) {
				if metrics.CrashCount >= cq.crashThreshold {
					return true, QuarantineReasonCrashLoop,
						fmt.Sprintf("File caused %d crashes (threshold: %d)", metrics.CrashCount, cq.crashThreshold)
				}
				return false, "", ""
			},
		},
		{
			Name:        "timeout_detection",
			Description: "Quarantine files causing excessive timeouts",
			Enabled:     true,
			Check: func(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) (bool, QuarantineReason, string) {
				if metrics.TimeoutCount >= cq.timeoutThreshold {
					return true, QuarantineReasonTimeout,
						fmt.Sprintf("File caused %d timeouts (threshold: %d)", metrics.TimeoutCount, cq.timeoutThreshold)
				}
				return false, "", ""
			},
		},
		{
			Name:        "memory_excess_detection",
			Description: "Quarantine files using excessive memory",
			Enabled:     true,
			Check: func(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) (bool, QuarantineReason, string) {
				if metrics.MaxMemoryUsage > cq.memoryThreshold {
					return true, QuarantineReasonMemoryExcess,
						fmt.Sprintf("File used %d bytes of memory (threshold: %d)", metrics.MaxMemoryUsage, cq.memoryThreshold)
				}
				return false, "", ""
			},
		},
		{
			Name:        "performance_detection",
			Description: "Quarantine files with poor performance",
			Enabled:     true,
			Check: func(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) (bool, QuarantineReason, string) {
				if metrics.AvgExecTime > cq.perfThreshold && metrics.ExecCount > 10 {
					return true, QuarantineReasonPerformance,
						fmt.Sprintf("File avg execution time %v exceeds threshold %v", metrics.AvgExecTime, cq.perfThreshold)
				}
				return false, "", ""
			},
		},
	}
}

// EvaluateFile checks if a corpus file should be quarantined based on metrics
func (cq *CorpusQuarantine) EvaluateFile(ctx context.Context, file *common.CorpusFile, metrics *common.CorpusFileMetrics) error {
	cq.mu.RLock()
	rules := cq.rules
	cq.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		shouldQuarantine, reason, details := rule.Check(ctx, file, metrics)
		if shouldQuarantine {
			cq.logger.WithFields(logrus.Fields{
				"file_id":     file.ID,
				"campaign_id": file.CampaignID,
				"rule":        rule.Name,
				"reason":      reason,
			}).Info("Quarantining corpus file")

			return cq.QuarantineFile(ctx, file, reason, details, "system")
		}
	}

	return nil
}

// QuarantineFile quarantines a specific corpus file
func (cq *CorpusQuarantine) QuarantineFile(ctx context.Context, file *common.CorpusFile, reason QuarantineReason, details string, quarantinedBy string) error {
	// Check if already quarantined
	existing, err := cq.GetQuarantinedFile(ctx, file.ID)
	if err == nil && existing != nil {
		cq.logger.WithField("file_id", file.ID).Debug("File already quarantined")
		return nil
	}

	quarantined := &common.QuarantinedFile{
		ID:            "quarantine-" + file.ID,
		FileID:        file.ID,
		CampaignID:    file.CampaignID,
		Hash:          file.Hash,
		Reason:        string(reason),
		Details:       details,
		QuarantinedAt: time.Now(),
		QuarantinedBy: quarantinedBy,
	}

	// Store quarantine record
	if err := cq.storage.AddQuarantinedFile(ctx, quarantined); err != nil {
		return fmt.Errorf("failed to store quarantine record: %w", err)
	}

	// Move file to quarantine storage if file storage is available
	if cq.fileStorage != nil {
		sourcePath := cq.getCorpusFilePath(file.CampaignID, file.Hash)
		quarantinePath := cq.getQuarantineFilePath(file.CampaignID, file.Hash)

		// Read file content
		data, err := cq.fileStorage.ReadFile(ctx, sourcePath)
		if err != nil {
			cq.logger.WithError(err).Warn("Failed to read corpus file for quarantine")
		} else {
			// Save to quarantine location
			if err := cq.fileStorage.SaveFile(ctx, quarantinePath, data); err != nil {
				cq.logger.WithError(err).Warn("Failed to save file to quarantine storage")
			}

			// Delete from active corpus storage
			if err := cq.fileStorage.DeleteFile(ctx, sourcePath); err != nil {
				cq.logger.WithError(err).Warn("Failed to delete quarantined file from corpus")
			}
		}
	}

	// Update corpus file status to mark as quarantined
	updates := map[string]interface{}{
		"quarantined":    true,
		"quarantined_at": time.Now(),
	}
	if err := cq.storage.UpdateCorpusFile(ctx, file.ID, updates); err != nil {
		cq.logger.WithError(err).Warn("Failed to update corpus file quarantine status")
	}

	cq.logger.WithFields(logrus.Fields{
		"file_id":        file.ID,
		"campaign_id":    file.CampaignID,
		"reason":         reason,
		"quarantined_by": quarantinedBy,
	}).Info("Corpus file quarantined")

	return nil
}

// RestoreFile restores a quarantined file back to the corpus
func (cq *CorpusQuarantine) RestoreFile(ctx context.Context, fileID string, restoredBy string, notes string) error {
	// Get quarantine record
	quarantined, err := cq.GetQuarantinedFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get quarantined file: %w", err)
	}
	if quarantined == nil {
		return fmt.Errorf("file not found in quarantine")
	}

	// Get original corpus file
	file, err := cq.storage.GetCorpusFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get corpus file: %w", err)
	}

	// Restore file in storage if available
	if cq.fileStorage != nil {
		quarantinePath := cq.getQuarantineFilePath(file.CampaignID, file.Hash)
		corpusPath := cq.getCorpusFilePath(file.CampaignID, file.Hash)

		// Read from quarantine
		data, err := cq.fileStorage.ReadFile(ctx, quarantinePath)
		if err != nil {
			cq.logger.WithError(err).Warn("Failed to read quarantined file")
		} else {
			// Save back to corpus
			if err := cq.fileStorage.SaveFile(ctx, corpusPath, data); err != nil {
				return fmt.Errorf("failed to restore file to corpus: %w", err)
			}

			// Delete from quarantine
			if err := cq.fileStorage.DeleteFile(ctx, quarantinePath); err != nil {
				cq.logger.WithError(err).Warn("Failed to delete file from quarantine storage")
			}
		}
	}

	// Update quarantine record
	now := time.Now()
	resolution := "restored"
	updates := map[string]interface{}{
		"reviewed_at":  now,
		"reviewed_by":  restoredBy,
		"resolution":   resolution,
		"review_notes": notes,
	}
	if err := cq.storage.UpdateQuarantinedFile(ctx, quarantined.ID, updates); err != nil {
		return fmt.Errorf("failed to update quarantine record: %w", err)
	}

	// Update corpus file to mark as not quarantined
	corpusUpdates := map[string]interface{}{
		"quarantined": false,
		"restored_at": now,
	}
	if err := cq.storage.UpdateCorpusFile(ctx, fileID, corpusUpdates); err != nil {
		cq.logger.WithError(err).Warn("Failed to update corpus file quarantine status")
	}

	cq.logger.WithFields(logrus.Fields{
		"file_id":     fileID,
		"campaign_id": file.CampaignID,
		"restored_by": restoredBy,
	}).Info("Corpus file restored from quarantine")

	return nil
}

// GetQuarantinedFiles retrieves all quarantined files for a campaign
func (cq *CorpusQuarantine) GetQuarantinedFiles(ctx context.Context, campaignID string) ([]*common.QuarantinedFile, error) {
	return cq.storage.GetQuarantinedFiles(ctx, campaignID)
}

// GetQuarantinedFile retrieves a specific quarantined file
func (cq *CorpusQuarantine) GetQuarantinedFile(ctx context.Context, fileID string) (*common.QuarantinedFile, error) {
	return cq.storage.GetQuarantinedFile(ctx, fileID)
}

// DeleteQuarantinedFile permanently deletes a quarantined file
func (cq *CorpusQuarantine) DeleteQuarantinedFile(ctx context.Context, fileID string, deletedBy string, reason string) error {
	// Get quarantine record
	quarantined, err := cq.GetQuarantinedFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get quarantined file: %w", err)
	}
	if quarantined == nil {
		return fmt.Errorf("file not found in quarantine")
	}

	// Delete file from quarantine storage
	if cq.fileStorage != nil {
		quarantinePath := cq.getQuarantineFilePath(quarantined.CampaignID, quarantined.Hash)
		if err := cq.fileStorage.DeleteFile(ctx, quarantinePath); err != nil {
			cq.logger.WithError(err).Warn("Failed to delete file from quarantine storage")
		}
	}

	// Update quarantine record
	now := time.Now()
	resolution := "deleted"
	updates := map[string]interface{}{
		"reviewed_at":  now,
		"reviewed_by":  deletedBy,
		"resolution":   resolution,
		"review_notes": reason,
	}
	if err := cq.storage.UpdateQuarantinedFile(ctx, quarantined.ID, updates); err != nil {
		return fmt.Errorf("failed to update quarantine record: %w", err)
	}

	// Delete corpus file record
	if err := cq.storage.DeleteCorpusFile(ctx, fileID); err != nil {
		cq.logger.WithError(err).Warn("Failed to delete corpus file record")
	}

	cq.logger.WithFields(logrus.Fields{
		"file_id":     fileID,
		"campaign_id": quarantined.CampaignID,
		"deleted_by":  deletedBy,
		"reason":      reason,
	}).Info("Quarantined file permanently deleted")

	return nil
}

// UpdateMetrics updates the metrics for a corpus file
func (cq *CorpusQuarantine) UpdateMetrics(ctx context.Context, fileID string, update func(*common.CorpusFileMetrics)) error {
	// Get current metrics
	metrics, err := cq.storage.GetCorpusFileMetrics(ctx, fileID)
	if err != nil {
		// Create new metrics if not found
		metrics = &common.CorpusFileMetrics{
			FileID:       fileID,
			LastExecuted: time.Now(),
		}
	}

	// Apply update
	update(metrics)

	// Store updated metrics
	if err := cq.storage.UpdateCorpusFileMetrics(ctx, fileID, metrics); err != nil {
		return fmt.Errorf("failed to update corpus file metrics: %w", err)
	}

	// Check if file should be quarantined based on new metrics
	file, err := cq.storage.GetCorpusFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get corpus file: %w", err)
	}

	if err := cq.EvaluateFile(ctx, file, metrics); err != nil {
		cq.logger.WithError(err).Warn("Failed to evaluate file for quarantine")
	}

	return nil
}

// SetRule enables or disables a quarantine rule
func (cq *CorpusQuarantine) SetRule(ruleName string, enabled bool) error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	for i, rule := range cq.rules {
		if rule.Name == ruleName {
			cq.rules[i].Enabled = enabled
			cq.logger.WithFields(logrus.Fields{
				"rule":    ruleName,
				"enabled": enabled,
			}).Info("Updated quarantine rule")
			return nil
		}
	}

	return fmt.Errorf("rule not found: %s", ruleName)
}

// GetRules returns all configured quarantine rules
func (cq *CorpusQuarantine) GetRules() []QuarantineRule {
	cq.mu.RLock()
	defer cq.mu.RUnlock()

	rules := make([]QuarantineRule, len(cq.rules))
	copy(rules, cq.rules)
	return rules
}

// SetThresholds updates the quarantine thresholds
func (cq *CorpusQuarantine) SetThresholds(crashes, timeouts int, memory int64, perfDuration time.Duration) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if crashes > 0 {
		cq.crashThreshold = crashes
	}
	if timeouts > 0 {
		cq.timeoutThreshold = timeouts
	}
	if memory > 0 {
		cq.memoryThreshold = memory
	}
	if perfDuration > 0 {
		cq.perfThreshold = perfDuration
	}

	cq.logger.WithFields(logrus.Fields{
		"crash_threshold":   cq.crashThreshold,
		"timeout_threshold": cq.timeoutThreshold,
		"memory_threshold":  cq.memoryThreshold,
		"perf_threshold":    cq.perfThreshold,
	}).Info("Updated quarantine thresholds")
}

// getCorpusFilePath returns the storage path for a corpus file
func (cq *CorpusQuarantine) getCorpusFilePath(campaignID, hash string) string {
	if len(hash) >= 2 {
		return fmt.Sprintf("corpus/%s/%s/%s", campaignID, hash[:2], hash)
	}
	return fmt.Sprintf("corpus/%s/%s", campaignID, hash)
}

// getQuarantineFilePath returns the storage path for a quarantined file
func (cq *CorpusQuarantine) getQuarantineFilePath(campaignID, hash string) string {
	if len(hash) >= 2 {
		return fmt.Sprintf("quarantine/%s/%s/%s", campaignID, hash[:2], hash)
	}
	return fmt.Sprintf("quarantine/%s/%s", campaignID, hash)
}
