package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// corpusService implements the CorpusService interface
type corpusService struct {
	storage       common.Storage
	fileStorage   common.FileStorage
	logger        logrus.FieldLogger
	corpusDir     string // Base directory for storing corpus files
	quarantine    *CorpusQuarantine
	enableMetrics bool // Flag to enable/disable metrics tracking
}

// NewCorpusService creates a new corpus service instance
func NewCorpusService(storage common.Storage, fileStorage common.FileStorage, corpusDir string, logger logrus.FieldLogger) (common.CorpusService, error) {
	// Validate dependencies
	if storage == nil {
		return nil, fmt.Errorf("corpus service: storage is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("corpus service: logger is required")
	}

	// corpusDir is now used as a prefix for S3 keys when using backend storage
	cs := &corpusService{
		storage:       storage,
		fileStorage:   fileStorage,
		logger:        logger.WithField("service", "corpus"),
		corpusDir:     corpusDir, // This becomes S3 key prefix for backend storage
		enableMetrics: true,      // Enable metrics by default
	}

	// Initialize quarantine manager
	cs.quarantine = NewCorpusQuarantine(storage, fileStorage, logger)

	return cs, nil
}

// AddFile adds a new corpus file to the campaign
func (cs *corpusService) AddFile(ctx context.Context, file *common.CorpusFile) error {
	// Validate input
	if file.CampaignID == "" {
		return fmt.Errorf("campaign ID is required")
	}
	if file.Filename == "" {
		return fmt.Errorf("filename is required")
	}

	// Generate ID if not provided
	if file.ID == "" {
		file.ID = "corpus-" + uuid.New().String()
	}

	// Set timestamp
	if file.CreatedAt.IsZero() {
		file.CreatedAt = time.Now()
	}

	// Check if file is quarantined
	if cs.quarantine != nil {
		quarantined, err := cs.quarantine.GetQuarantinedFile(ctx, file.ID)
		if err == nil && quarantined != nil && quarantined.Resolution == nil {
			cs.logger.WithFields(logrus.Fields{
				"file_id":     file.ID,
				"campaign_id": file.CampaignID,
				"reason":      quarantined.Reason,
			}).Warn("Attempted to add quarantined file to corpus")
			return fmt.Errorf("file is quarantined: %s", quarantined.Reason)
		}
	}

	// Check if file already exists by hash
	existing, err := cs.storage.GetCorpusFileByHash(ctx, file.Hash)
	if err == nil && existing != nil {
		// File already exists, check if it's for the same campaign
		if existing.CampaignID == file.CampaignID {
			cs.logger.WithFields(logrus.Fields{
				"file_hash":   file.Hash,
				"campaign_id": file.CampaignID,
			}).Debug("Corpus file already exists in campaign")
			return common.ErrDuplicateCorpusFile
		}

		// File exists in another campaign - this is a duplicate across campaigns
		// Since the hash is globally unique in the database, we treat this as a duplicate
		cs.logger.WithFields(logrus.Fields{
			"file_hash":         file.Hash,
			"source_campaign":   existing.CampaignID,
			"target_campaign":   file.CampaignID,
			"coverage_increase": file.NewCoverage,
		}).Info("Corpus file with same hash exists in another campaign")
		return common.ErrDuplicateCorpusFile
	}

	// Store file metadata in database
	if err := cs.storage.AddCorpusFile(ctx, file); err != nil {
		return fmt.Errorf("failed to store corpus file metadata: %w", err)
	}

	// Note: File content should be stored separately using StoreFileContent method
	// This method only handles metadata

	// Track evolution if this file increased coverage
	if file.NewCoverage > 0 {
		if err := cs.trackEvolution(ctx, file.CampaignID); err != nil {
			cs.logger.WithError(err).Error("Failed to track corpus evolution")
		}
	}

	// Initialize metrics for new file if metrics tracking is enabled
	if cs.enableMetrics && cs.quarantine != nil {
		if err := cs.quarantine.UpdateMetrics(ctx, file.ID, func(m *common.CorpusFileMetrics) {
			m.FileID = file.ID
			m.LastExecuted = time.Now()
			m.ExecCount = 0
		}); err != nil {
			cs.logger.WithError(err).Warn("Failed to initialize corpus file metrics")
		}
	}

	cs.logger.WithFields(logrus.Fields{
		"file_id":      file.ID,
		"campaign_id":  file.CampaignID,
		"job_id":       file.JobID,
		"coverage":     file.Coverage,
		"new_coverage": file.NewCoverage,
		"generation":   file.Generation,
		"is_seed":      file.IsSeed,
	}).Info("Added corpus file")

	return nil
}

// StoreFileContent stores the actual content of a corpus file
func (cs *corpusService) StoreFileContent(ctx context.Context, campaignID, hash string, data []byte) error {
	startTime := time.Now()

	// Check context cancellation
	select {
	case <-ctx.Done():
		cs.logger.WithError(ctx.Err()).Warn("Context cancelled during store file content")
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	if cs.fileStorage == nil {
		err := common.NewStorageError("store_file_content", fmt.Errorf("file storage not configured"))
		cs.logger.WithError(err).Error("File storage not configured")
		return err
	}

	// Validate input
	if campaignID == "" {
		err := common.NewValidationError("store_file_content", fmt.Errorf("campaign ID is required"))
		cs.logger.WithError(err).Error("Validation failed: missing campaign ID")
		return err
	}
	if hash == "" {
		err := common.NewValidationError("store_file_content", fmt.Errorf("hash is required"))
		cs.logger.WithError(err).Error("Validation failed: missing hash")
		return err
	}
	if len(data) == 0 {
		err := common.NewValidationError("store_file_content", fmt.Errorf("data is empty"))
		cs.logger.WithError(err).Error("Validation failed: empty data")
		return err
	}

	filePath := cs.getCorpusFilePath(campaignID, hash)

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"hash":        hash,
		"size":        len(data),
		"path":        filePath,
	}).Debug("Starting corpus file content storage")

	if err := cs.fileStorage.SaveFile(ctx, filePath, data); err != nil {
		storageErr := common.NewStorageError("store_file_content", err)
		cs.logger.WithError(storageErr).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"hash":        hash,
			"size":        len(data),
			"path":        filePath,
		}).Error("Failed to store corpus file content")
		return storageErr
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"hash":        hash,
		"size":        len(data),
		"path":        filePath,
		"duration":    time.Since(startTime).Seconds(),
	}).Info("Successfully stored corpus file content")

	return nil
}

// GetEvolution retrieves corpus evolution history for a campaign
func (cs *corpusService) GetEvolution(ctx context.Context, campaignID string) ([]*common.CorpusEvolution, error) {
	// Get evolution records (last 1000 entries)
	evolution, err := cs.storage.GetCorpusEvolution(ctx, campaignID, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get corpus evolution: %w", err)
	}

	// If no evolution records exist, create initial snapshot
	if len(evolution) == 0 {
		if err := cs.trackEvolution(ctx, campaignID); err != nil {
			cs.logger.WithError(err).Error("Failed to create initial evolution snapshot")
		}

		// Retry getting evolution
		evolution, err = cs.storage.GetCorpusEvolution(ctx, campaignID, 1)
		if err != nil {
			return nil, err
		}
	}

	return evolution, nil
}

// SyncCorpus synchronizes corpus files for a bot
func (cs *corpusService) SyncCorpus(ctx context.Context, campaignID string, botID string) ([]*common.CorpusFile, error) {
	// Get unsynced files for this bot
	files, err := cs.storage.GetUnsyncedCorpusFiles(ctx, campaignID, botID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unsynced corpus files: %w", err)
	}

	if len(files) == 0 {
		return []*common.CorpusFile{}, nil
	}

	// Filter out quarantined files
	if cs.quarantine != nil {
		var activeFiles []*common.CorpusFile
		for _, file := range files {
			quarantined, err := cs.quarantine.GetQuarantinedFile(ctx, file.ID)
			if err != nil || quarantined == nil || quarantined.Resolution != nil {
				// File is not quarantined or has been resolved
				activeFiles = append(activeFiles, file)
			} else {
				cs.logger.WithFields(logrus.Fields{
					"file_id":     file.ID,
					"campaign_id": campaignID,
					"bot_id":      botID,
					"reason":      quarantined.Reason,
				}).Debug("Skipping quarantined file in sync")
			}
		}
		files = activeFiles
	}

	// Mark files as synced
	fileIDs := make([]string, len(files))
	for i, file := range files {
		fileIDs[i] = file.ID
	}

	if err := cs.storage.MarkCorpusFilesSynced(ctx, fileIDs, botID); err != nil {
		cs.logger.WithError(err).Error("Failed to mark corpus files as synced")
		// Continue anyway, bot will retry sync later
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"bot_id":      botID,
		"file_count":  len(files),
	}).Info("Synced corpus files to bot")

	return files, nil
}

// ShareCorpus shares coverage-increasing corpus files between campaigns
func (cs *corpusService) ShareCorpus(ctx context.Context, fromCampaign, toCampaign string) error {
	// Validate campaigns exist
	fromCamp, err := cs.storage.GetCampaign(ctx, fromCampaign)
	if err != nil {
		return fmt.Errorf("source campaign not found: %w", err)
	}

	toCamp, err := cs.storage.GetCampaign(ctx, toCampaign)
	if err != nil {
		return fmt.Errorf("target campaign not found: %w", err)
	}

	// Check if campaigns have compatible binaries
	if fromCamp.BinaryHash != toCamp.BinaryHash {
		cs.logger.WithFields(logrus.Fields{
			"from_campaign": fromCampaign,
			"to_campaign":   toCampaign,
			"from_hash":     fromCamp.BinaryHash,
			"to_hash":       toCamp.BinaryHash,
		}).Warn("Campaigns have different binary hashes, corpus sharing may not be effective")
	}

	// Find coverage-increasing files from source campaign
	coverageFiles, err := cs.findCoverageIncreasingFiles(ctx, fromCampaign)
	if err != nil {
		return fmt.Errorf("failed to find coverage files: %w", err)
	}

	// Get existing files in target campaign to avoid duplicates
	existingFiles, err := cs.storage.GetCorpusFiles(ctx, toCampaign)
	if err != nil {
		return fmt.Errorf("failed to get target campaign files: %w", err)
	}

	existingHashes := make(map[string]bool)
	for _, file := range existingFiles {
		existingHashes[file.Hash] = true
	}

	// Share files that don't already exist in target
	sharedCount := 0
	for _, file := range coverageFiles {
		if existingHashes[file.Hash] {
			continue
		}

		// Create new corpus file entry for target campaign
		sharedFile := &common.CorpusFile{
			ID:          "corpus-" + uuid.New().String(),
			CampaignID:  toCampaign,
			JobID:       file.JobID,
			BotID:       file.BotID,
			Filename:    file.Filename,
			Hash:        file.Hash,
			Size:        file.Size,
			Coverage:    file.Coverage,
			NewCoverage: 0,         // Will be determined when executed in new campaign
			ParentHash:  file.Hash, // Original file becomes parent
			Generation:  file.Generation + 1,
			CreatedAt:   time.Now(),
			IsSeed:      false,
		}

		if err := cs.AddFile(ctx, sharedFile); err != nil {
			if err == common.ErrDuplicateCorpusFile {
				continue
			}
			cs.logger.WithError(err).WithField("file_hash", file.Hash).Error("Failed to share corpus file")
			continue
		}

		sharedCount++
	}

	cs.logger.WithFields(logrus.Fields{
		"from_campaign":  fromCampaign,
		"to_campaign":    toCampaign,
		"shared_files":   sharedCount,
		"coverage_files": len(coverageFiles),
	}).Info("Shared corpus files between campaigns")

	return nil
}

// trackEvolution records current corpus state for a campaign
func (cs *corpusService) trackEvolution(ctx context.Context, campaignID string) error {
	// Get all corpus files for the campaign
	files, err := cs.storage.GetCorpusFiles(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get corpus files: %w", err)
	}

	// Calculate metrics
	totalFiles := len(files)
	var totalSize, totalCoverage, newCoverage int64
	filesSince := 0

	// Get last evolution record to calculate deltas
	lastEvolution, err := cs.storage.GetCorpusEvolution(ctx, campaignID, 1)
	if err == nil && len(lastEvolution) > 0 {
		lastTimestamp := lastEvolution[0].Timestamp
		for _, file := range files {
			totalSize += file.Size
			totalCoverage += file.Coverage
			if file.CreatedAt.After(lastTimestamp) {
				filesSince++
				newCoverage += file.NewCoverage
			}
		}
	} else {
		// First evolution record
		for _, file := range files {
			totalSize += file.Size
			totalCoverage += file.Coverage
			newCoverage += file.NewCoverage
		}
		filesSince = totalFiles
	}

	// Create evolution record
	evolution := &common.CorpusEvolution{
		CampaignID:    campaignID,
		Timestamp:     time.Now(),
		TotalFiles:    totalFiles,
		TotalSize:     totalSize,
		TotalCoverage: totalCoverage,
		NewFiles:      filesSince,
		NewCoverage:   newCoverage,
	}

	if err := cs.storage.RecordCorpusEvolution(ctx, evolution); err != nil {
		return fmt.Errorf("failed to record corpus evolution: %w", err)
	}

	return nil
}

// findCoverageIncreasingFiles finds files that increased coverage
func (cs *corpusService) findCoverageIncreasingFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	// Get all files for the campaign
	files, err := cs.storage.GetCorpusFiles(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	// Filter files that increased coverage
	var coverageFiles []*common.CorpusFile
	for _, file := range files {
		if file.NewCoverage > 0 {
			coverageFiles = append(coverageFiles, file)
		}
	}

	// Sort by coverage contribution (highest first)
	// In a real implementation, you'd sort these
	return coverageFiles, nil
}

// getCorpusFilePath returns the storage path for a corpus file
func (cs *corpusService) getCorpusFilePath(campaignID, hash string) string {
	return common.CorpusFilePath(campaignID, hash)
}

// CalculateFileHash calculates SHA256 hash of file content
func (cs *corpusService) CalculateFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// LoadCorpusFile loads the actual content of a corpus file
func (cs *corpusService) LoadCorpusFile(ctx context.Context, campaignID, hash string) ([]byte, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
	}

	if cs.fileStorage == nil {
		return nil, common.NewStorageError("load_corpus_file", fmt.Errorf("file storage not configured"))
	}

	// Validate input
	if campaignID == "" {
		return nil, common.NewValidationError("load_corpus_file", fmt.Errorf("campaign ID is required"))
	}
	if hash == "" {
		return nil, common.NewValidationError("load_corpus_file", fmt.Errorf("hash is required"))
	}

	filePath := cs.getCorpusFilePath(campaignID, hash)

	// Try to load from file storage
	data, err := cs.fileStorage.ReadFile(ctx, filePath)
	if err != nil {
		return nil, common.NewStorageError("load_corpus_file", err)
	}

	// Verify hash matches
	actualHash := cs.CalculateFileHash(data)
	if actualHash != hash {
		return nil, common.NewValidationError("load_corpus_file",
			fmt.Errorf("corpus file hash mismatch: expected %s, got %s", hash, actualHash))
	}

	cs.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"hash":        hash,
		"size":        len(data),
		"path":        filePath,
	}).Debug("Loaded corpus file content")

	return data, nil
}

// ImportSeedCorpus imports a directory of seed files into a campaign
func (cs *corpusService) ImportSeedCorpus(ctx context.Context, campaignID string, seedDir string) error {
	// Walk the seed directory
	err := filepath.Walk(seedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			cs.logger.WithError(err).WithField("file", path).Warn("Failed to read seed file")
			return nil // Continue with other files
		}

		// Calculate hash
		hash := cs.CalculateFileHash(content)

		// Create corpus file entry
		corpusFile := &common.CorpusFile{
			ID:          "corpus-" + uuid.New().String(),
			CampaignID:  campaignID,
			Filename:    filepath.Base(path),
			Hash:        hash,
			Size:        info.Size(),
			Coverage:    0, // Will be determined when executed
			NewCoverage: 0,
			Generation:  0,
			CreatedAt:   time.Now(),
			IsSeed:      true,
		}

		// Add to campaign
		if err := cs.AddFile(ctx, corpusFile); err != nil {
			if err == common.ErrDuplicateCorpusFile {
				cs.logger.WithField("file", path).Debug("Seed file already exists")
				return nil
			}
			cs.logger.WithError(err).WithField("file", path).Error("Failed to import seed file")
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk seed directory: %w", err)
	}

	// Track evolution after import
	if err := cs.trackEvolution(ctx, campaignID); err != nil {
		cs.logger.WithError(err).Error("Failed to track evolution after seed import")
	}

	return nil
}

// CleanupOrphanedFiles removes corpus files that are no longer referenced
func (cs *corpusService) CleanupOrphanedFiles(ctx context.Context, campaignID string) error {
	if cs.fileStorage == nil {
		return nil // Nothing to clean if no file storage
	}

	// Get all corpus files from database
	dbFiles, err := cs.storage.GetCorpusFiles(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get corpus files from database: %w", err)
	}

	// Create hash set for quick lookup
	validHashes := make(map[string]bool)
	for _, file := range dbFiles {
		validHashes[file.Hash] = true
	}

	// Walk campaign corpus directory
	corpusPath := filepath.Join(cs.corpusDir, campaignID)
	removedCount := 0

	err = filepath.Walk(corpusPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Extract hash from filename (last component)
		hash := filepath.Base(path)

		// Check if file is still referenced
		if !validHashes[hash] {
			cs.logger.WithField("file", path).Debug("Removing orphaned corpus file")
			if err := os.Remove(path); err != nil {
				cs.logger.WithError(err).WithField("file", path).Warn("Failed to remove orphaned file")
			} else {
				removedCount++
			}
		}

		return nil
	})

	if err != nil {
		cs.logger.WithError(err).Error("Failed to walk corpus directory during cleanup")
	}

	if removedCount > 0 {
		cs.logger.WithFields(logrus.Fields{
			"campaign_id":   campaignID,
			"removed_files": removedCount,
		}).Info("Cleaned up orphaned corpus files")
	}

	return nil
}

// PromoteCrashToCorpus promotes a crash input to the campaign corpus
func (cs *corpusService) PromoteCrashToCorpus(ctx context.Context, crashID, campaignID string) (*common.CorpusFile, error) {
	// Get crash details from storage
	crash, err := cs.storage.GetCrash(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to get crash: %w", err)
	}

	// Validate campaign exists
	_, err = cs.storage.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get campaign: %w", err)
	}

	// Ensure crash input data is available
	if len(crash.Input) == 0 {
		// Try to load from file storage if not in memory
		if cs.fileStorage != nil && crash.FilePath != "" {
			crashPath := filepath.Join(cs.corpusDir, "..", "crashes", crash.FilePath)
			crash.Input, err = cs.fileStorage.ReadFile(ctx, crashPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load crash input: %w", err)
			}
		} else {
			return nil, fmt.Errorf("crash input data not available")
		}
	}

	// Calculate hash of crash input
	hash := cs.CalculateFileHash(crash.Input)

	// Check if this input already exists in corpus
	existing, err := cs.storage.GetCorpusFileByHash(ctx, hash)
	if err == nil && existing != nil && existing.CampaignID == campaignID {
		cs.logger.WithFields(logrus.Fields{
			"crash_id":    crashID,
			"campaign_id": campaignID,
			"hash":        hash,
		}).Info("Crash input already exists in campaign corpus")
		return existing, nil
	}

	// Create corpus file entry
	corpusFile := &common.CorpusFile{
		ID:          "corpus-" + uuid.New().String(),
		CampaignID:  campaignID,
		JobID:       crash.JobID,
		BotID:       crash.BotID,
		Filename:    fmt.Sprintf("crash_%s", crash.Hash[:8]),
		Hash:        hash,
		Size:        int64(len(crash.Input)),
		Coverage:    0,  // Will be determined when executed
		NewCoverage: 0,  // To be updated based on coverage analysis
		ParentHash:  "", // Crash has no parent
		Generation:  0,  // First generation from crash
		CreatedAt:   time.Now(),
		IsSeed:      false,
	}

	// Store file content if file storage is available
	if cs.fileStorage != nil {
		filePath := cs.getCorpusFilePath(campaignID, hash)
		if err := cs.fileStorage.SaveFile(ctx, filePath, crash.Input); err != nil {
			return nil, fmt.Errorf("failed to store corpus file content: %w", err)
		}
	}

	// Add file to corpus
	if err := cs.AddFile(ctx, corpusFile); err != nil {
		if err == common.ErrDuplicateCorpusFile {
			// File was added by another process, return the existing one
			existing, _ := cs.storage.GetCorpusFileByHash(ctx, hash)
			return existing, nil
		}
		return nil, fmt.Errorf("failed to add corpus file: %w", err)
	}

	// Update crash with campaign reference if not already set
	if crash.CampaignID == "" {
		if err := cs.storage.UpdateCrashWithCampaign(ctx, crashID, campaignID); err != nil {
			cs.logger.WithError(err).Warn("Failed to update crash with campaign reference")
		}
	}

	// Track corpus evolution metrics
	if err := cs.trackEvolution(ctx, campaignID); err != nil {
		cs.logger.WithError(err).Error("Failed to track corpus evolution after crash promotion")
	}

	cs.logger.WithFields(logrus.Fields{
		"crash_id":       crashID,
		"campaign_id":    campaignID,
		"corpus_file_id": corpusFile.ID,
		"hash":           hash,
		"size":           corpusFile.Size,
	}).Info("Promoted crash to corpus")

	return corpusFile, nil
}

// GetCorpusForJob retrieves corpus files for a job from its associated campaign
func (cs *corpusService) GetCorpusForJob(ctx context.Context, jobID string) ([]*common.CorpusFile, error) {
	// Get job details to find associated campaign
	job, err := cs.storage.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	// Check if job has an associated campaign
	if job.CampaignID == nil || *job.CampaignID == "" {
		// Job not linked to any campaign, return empty corpus
		cs.logger.WithField("job_id", jobID).Debug("Job not linked to any campaign")
		return []*common.CorpusFile{}, nil
	}

	// Check if job should use campaign corpus
	if !job.UseCampaignCorpus {
		cs.logger.WithFields(logrus.Fields{
			"job_id":      jobID,
			"campaign_id": *job.CampaignID,
		}).Debug("Job configured to not use campaign corpus")
		return []*common.CorpusFile{}, nil
	}

	// Get corpus files from the campaign
	campaignFiles, err := cs.storage.GetCorpusFiles(ctx, *job.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to get corpus files for campaign: %w", err)
	}

	// Sort corpus files by coverage contribution (highest first)
	// This helps prioritize high-value corpus entries
	sortCorpusByCoverage(campaignFiles)

	cs.logger.WithFields(logrus.Fields{
		"job_id":            jobID,
		"campaign_id":       *job.CampaignID,
		"corpus_file_count": len(campaignFiles),
	}).Debug("Retrieved corpus files for job")

	return campaignFiles, nil
}

// LinkJobCorpus links a job to a campaign for corpus inheritance
func (cs *corpusService) LinkJobCorpus(ctx context.Context, jobID, campaignID string) error {
	// Validate job exists
	job, err := cs.storage.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Validate campaign exists
	campaign, err := cs.storage.GetCampaign(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get campaign: %w", err)
	}

	// Check if job and campaign have compatible binaries
	if campaign.BinaryHash != "" && job.Target != "" {
		// In a real implementation, we might calculate job's binary hash
		// and compare with campaign's binary hash
		cs.logger.WithFields(logrus.Fields{
			"job_id":          jobID,
			"campaign_id":     campaignID,
			"job_target":      job.Target,
			"campaign_binary": campaign.TargetBinary,
		}).Debug("Linking job to campaign corpus")
	}

	// Create the link in storage
	if err := cs.storage.LinkJobToCampaign(ctx, campaignID, jobID); err != nil {
		return fmt.Errorf("failed to link job to campaign: %w", err)
	}

	// If campaign has shared corpus enabled, mark existing corpus files for sync
	if campaign.SharedCorpus {
		corpusFiles, err := cs.storage.GetCorpusFiles(ctx, campaignID)
		if err == nil && len(corpusFiles) > 0 {
			cs.logger.WithFields(logrus.Fields{
				"job_id":      jobID,
				"campaign_id": campaignID,
				"corpus_size": len(corpusFiles),
			}).Info("Job will inherit campaign corpus")
		}
	}

	cs.logger.WithFields(logrus.Fields{
		"job_id":        jobID,
		"campaign_id":   campaignID,
		"shared_corpus": campaign.SharedCorpus,
	}).Info("Linked job to campaign corpus")

	return nil
}

// Helper function to sort corpus files by coverage contribution
func sortCorpusByCoverage(files []*common.CorpusFile) {
	// Simple bubble sort for now - in production, use sort.Slice
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			// Sort by new coverage (descending), then by total coverage
			if files[j].NewCoverage > files[i].NewCoverage ||
				(files[j].NewCoverage == files[i].NewCoverage && files[j].Coverage > files[i].Coverage) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}
}

// QuarantineFile quarantines a corpus file
func (cs *corpusService) QuarantineFile(ctx context.Context, fileID string, reason string, details string) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	// Get the corpus file
	file, err := cs.storage.GetCorpusFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("failed to get corpus file: %w", err)
	}

	// Quarantine the file
	return cs.quarantine.QuarantineFile(ctx, file, QuarantineReason(reason), details, "user")
}

// RestoreQuarantinedFile restores a quarantined file
func (cs *corpusService) RestoreQuarantinedFile(ctx context.Context, fileID string, restoredBy string, notes string) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.RestoreFile(ctx, fileID, restoredBy, notes)
}

// GetQuarantinedFiles retrieves quarantined files for a campaign
func (cs *corpusService) GetQuarantinedFiles(ctx context.Context, campaignID string) ([]*common.QuarantinedFile, error) {
	if cs.quarantine == nil {
		return nil, fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.GetQuarantinedFiles(ctx, campaignID)
}

// DeleteQuarantinedFile permanently deletes a quarantined file
func (cs *corpusService) DeleteQuarantinedFile(ctx context.Context, fileID string, deletedBy string, reason string) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.DeleteQuarantinedFile(ctx, fileID, deletedBy, reason)
}

// UpdateCorpusFileMetrics updates metrics for a corpus file
func (cs *corpusService) UpdateCorpusFileMetrics(ctx context.Context, fileID string, update func(*common.CorpusFileMetrics)) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.UpdateMetrics(ctx, fileID, update)
}

// SetQuarantineRule enables or disables a quarantine rule
func (cs *corpusService) SetQuarantineRule(ruleName string, enabled bool) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.SetRule(ruleName, enabled)
}

// GetQuarantineRules returns all configured quarantine rules
func (cs *corpusService) GetQuarantineRules() ([]QuarantineRule, error) {
	if cs.quarantine == nil {
		return nil, fmt.Errorf("quarantine manager not initialized")
	}

	return cs.quarantine.GetRules(), nil
}

// SetQuarantineThresholds updates quarantine thresholds
func (cs *corpusService) SetQuarantineThresholds(crashes, timeouts int, memory int64, perfDuration time.Duration) error {
	if cs.quarantine == nil {
		return fmt.Errorf("quarantine manager not initialized")
	}

	cs.quarantine.SetThresholds(crashes, timeouts, memory, perfDuration)
	return nil
}

// EnableMetricsTracking enables or disables metrics tracking
func (cs *corpusService) EnableMetricsTracking(enabled bool) {
	cs.enableMetrics = enabled
}
