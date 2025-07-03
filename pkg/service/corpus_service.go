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
	storage     common.Storage
	fileStorage common.FileStorage
	logger      logrus.FieldLogger
	corpusDir   string // Base directory for storing corpus files
}

// NewCorpusService creates a new corpus service instance
func NewCorpusService(storage common.Storage, fileStorage common.FileStorage, corpusDir string, logger logrus.FieldLogger) common.CorpusService {
	return &corpusService{
		storage:     storage,
		fileStorage: fileStorage,
		logger:      logger.WithField("service", "corpus"),
		corpusDir:   corpusDir,
	}
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

		// File exists in another campaign, this is OK for cross-campaign sharing
		cs.logger.WithFields(logrus.Fields{
			"file_hash":         file.Hash,
			"source_campaign":   existing.CampaignID,
			"target_campaign":   file.CampaignID,
			"coverage_increase": file.NewCoverage,
		}).Info("Sharing corpus file between campaigns")
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
	if cs.fileStorage == nil {
		return fmt.Errorf("file storage not configured")
	}

	filePath := cs.getCorpusFilePath(campaignID, hash)
	if err := cs.fileStorage.SaveFile(ctx, filePath, data); err != nil {
		return fmt.Errorf("failed to store corpus file content: %w", err)
	}

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
	// Use first 2 chars of hash for directory sharding
	if len(hash) >= 2 {
		return filepath.Join(cs.corpusDir, campaignID, hash[:2], hash)
	}
	return filepath.Join(cs.corpusDir, campaignID, hash)
}

// CalculateFileHash calculates SHA256 hash of file content
func (cs *corpusService) CalculateFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// LoadCorpusFile loads the actual content of a corpus file
func (cs *corpusService) LoadCorpusFile(ctx context.Context, campaignID, hash string) ([]byte, error) {
	if cs.fileStorage == nil {
		return nil, fmt.Errorf("file storage not configured")
	}

	filePath := cs.getCorpusFilePath(campaignID, hash)

	// Try to load from file storage
	data, err := cs.fileStorage.ReadFile(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load corpus file: %w", err)
	}

	// Verify hash matches
	actualHash := cs.CalculateFileHash(data)
	if actualHash != hash {
		return nil, fmt.Errorf("corpus file hash mismatch: expected %s, got %s", hash, actualHash)
	}

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
