package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CorpusAdapter handles corpus-related API requests
type CorpusAdapter struct {
	corpusService common.CorpusService
	storage       common.Storage
	fileStorage   common.FileStorage
	sse           *sse.Manager
	logger        logrus.FieldLogger
	maxFileSize   int64
	allowedExts   []string
}

// CorpusAdapterOptions configures corpus upload validation.
type CorpusAdapterOptions struct {
	MaxFileSize int64
	AllowedExts []string
}

// NewCorpusAdapter creates a new corpus adapter
func NewCorpusAdapter(
	corpusService common.CorpusService,
	storage common.Storage,
	fileStorage common.FileStorage,
	sse *sse.Manager,
	logger logrus.FieldLogger,
	options CorpusAdapterOptions,
) *CorpusAdapter {
	return &CorpusAdapter{
		corpusService: corpusService,
		storage:       storage,
		fileStorage:   fileStorage,
		sse:           sse,
		logger:        logger.WithField("adapter", "corpus"),
		maxFileSize:   options.MaxFileSize,
		allowedExts:   normalizeExtensions(options.AllowedExts),
	}
}

// ListCorpus returns a list of corpus entries
func (a *CorpusAdapter) ListCorpus(w http.ResponseWriter, r *http.Request, params generated.ListCorpusParams) {
	ctx := r.Context()
	a.logger.Debug("listing corpus entries")

	// Apply pagination
	limit := 50
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Get campaign ID filter if provided
	var campaignID string
	if params.CampaignId != nil {
		campaignID = params.CampaignId.String()
	}

	// Get corpus files from storage
	var corpusFiles []*common.CorpusFile
	var err error

	if a.storage != nil {
		if campaignID != "" {
			corpusFiles, err = a.storage.GetCorpusFiles(ctx, campaignID)
		} else {
			// Get all corpus files - list campaigns first, then get files for each
			// For simplicity, return empty if no campaign filter
			a.logger.Debug("no campaign filter provided, returning empty list")
			corpusFiles = []*common.CorpusFile{}
		}

		if err != nil {
			a.logger.WithError(err).Error("failed to get corpus files from storage")
			a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list corpus files", err)
			return
		}
	} else {
		a.logger.Warn("storage not available, returning empty list")
		corpusFiles = []*common.CorpusFile{}
	}

	// Convert to API response format
	entries := make([]generated.CorpusEntry, 0, len(corpusFiles))
	for _, cf := range corpusFiles {
		entry := a.corpusFileToEntry(cf)
		entries = append(entries, entry)
	}

	// Calculate total before pagination
	total := len(entries)

	// Apply pagination
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedEntries := entries[start:end]

	response := generated.CorpusListResponse{
		Data: paginatedEntries,
		Pagination: generated.Pagination{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// UploadCorpus handles corpus file upload
func (a *CorpusAdapter) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a.logger.Debug("uploading corpus files")

	if a.corpusService == nil {
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Corpus service not configured", nil)
		return
	}
	if a.fileStorage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Parse multipart form
	err := r.ParseMultipartForm(32 << 20) // 32MB max memory
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_FORM", "Failed to parse multipart form", err)
		return
	}

	// Get campaign ID from form
	campaignID := r.FormValue("campaign_id")
	if campaignID == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_CAMPAIGN_ID", "Campaign ID is required", nil)
		return
	}
	campaignUUID, err := uuid.Parse(campaignID)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_CAMPAIGN_ID", "Campaign ID must be a valid UUID", err)
		return
	}

	// Process uploaded files
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		a.writeError(w, http.StatusBadRequest, "NO_FILES", "No files provided", nil)
		return
	}

	uploadedEntries := []generated.CorpusEntry{}
	duplicateCount := 0
	totalSize := 0

	for _, fileHeader := range files {
		if !isAllowedExtension(fileHeader.Filename, a.allowedExts) {
			a.writeError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "File extension not allowed", nil)
			return
		}
		if a.maxFileSize > 0 && fileHeader.Size > a.maxFileSize {
			a.writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "Corpus file exceeds size limit", nil)
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to open uploaded file")
			continue
		}

		tempPath, size, hash, err := streamToTempFile(file, a.maxFileSize, true)
		file.Close()
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errUploadTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			a.writeError(w, status, "UPLOAD_FAILED", "Failed to stream uploaded file", err)
			return
		}

		tempFile, err := os.Open(tempPath)
		if err != nil {
			os.Remove(tempPath)
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to open temp file")
			continue
		}

		cleanupTemp := func() {
			tempFile.Close()
			os.Remove(tempPath)
		}
		fileID := uuid.New().String()
		now := time.Now()

		// Create corpus file for storage
		corpusFile := &common.CorpusFile{
			ID:         fileID,
			CampaignID: campaignID,
			Filename:   fileHeader.Filename,
			Hash:       hash,
			Size:       size,
			CreatedAt:  now,
			IsSeed:     true, // Uploaded files are seed corpus
		}

		if err := a.corpusService.AddFile(ctx, corpusFile); err != nil {
			// Check if it's a duplicate (various error message formats)
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "duplicate") ||
				strings.Contains(errStr, "already exists") ||
				strings.Contains(errStr, "unique constraint") ||
				strings.Contains(errStr, "corpus file already exists") {
				duplicateCount++
				a.logger.WithField("filename", fileHeader.Filename).Debug("skipping duplicate corpus file")
				cleanupTemp()
				continue
			}
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to add corpus file")
			cleanupTemp()
			continue
		}

		filePath := common.CorpusFilePath(campaignID, hash)
		if err := a.fileStorage.SaveFileStream(ctx, filePath, tempFile, size); err != nil {
			a.logger.WithError(err).WithFields(logrus.Fields{
				"filename":  fileHeader.Filename,
				"file_path": filePath,
			}).Error("failed to store corpus file content")
			cleanupTemp()
			if a.storage != nil {
				if deleteErr := a.storage.DeleteCorpusFile(ctx, corpusFile.ID); deleteErr != nil {
					a.logger.WithError(deleteErr).WithField("file_id", corpusFile.ID).Warn("failed to delete corpus metadata after save failure")
				}
			}
			continue
		}
		cleanupTemp()

		// Create response entry
		entry := generated.CorpusEntry{
			Id:         openapi_types.UUID(uuid.MustParse(fileID)),
			CampaignId: openapi_types.UUID(campaignUUID),
			Filename:   fileHeader.Filename,
			SizeBytes:  int(size),
			Hash:       hash,
			CreatedAt:  now,
			Tags:       &[]string{"uploaded", "seed"},
		}

		uploadedEntries = append(uploadedEntries, entry)
		totalSize += int(size)

		// Publish SSE event
		if a.sse != nil {
			event := sse.NewCorpusEvent("corpus.uploaded", map[string]interface{}{
				"entry_id":    entry.Id,
				"campaign_id": entry.CampaignId,
				"filename":    entry.Filename,
				"size":        entry.SizeBytes,
				"hash":        hash,
			})
			a.sse.BroadcastToTopic("corpus", event)
		}

		a.logger.WithFields(logrus.Fields{
			"filename":    fileHeader.Filename,
			"size":        size,
			"hash":        hash,
			"campaign_id": campaignID,
		}).Info("corpus file uploaded successfully")
	}

	response := generated.CorpusUploadResponse{
		UploadId:       openapi_types.UUID(uuid.New()),
		UploadedCount:  len(uploadedEntries),
		DuplicateCount: duplicateCount,
		TotalSizeBytes: totalSize,
	}

	a.writeJSONResponse(w, http.StatusCreated, response)
}

// ListQuarantinedCorpus returns quarantined corpus entries
func (a *CorpusAdapter) ListQuarantinedCorpus(w http.ResponseWriter, r *http.Request, params generated.ListQuarantinedCorpusParams) {
	ctx := r.Context()
	a.logger.Debug("listing quarantined corpus entries")

	// Apply pagination
	limit := 50
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Get reason filter if provided
	var reasonFilter string
	if params.Reason != nil {
		reasonFilter = string(*params.Reason)
	}

	// Get quarantined files from storage
	// Note: ListQuarantinedCorpus doesn't have campaign_id filter in the API spec
	// We'll need to get all quarantined files and filter by reason if specified
	var allQuarantinedFiles []*common.QuarantinedFile

	if a.storage != nil {
		// Get all quarantined files (empty campaign filter gets all)
		allQuarantinedFiles, _ = a.storage.GetQuarantinedFiles(ctx, "")
		if allQuarantinedFiles == nil {
			allQuarantinedFiles = []*common.QuarantinedFile{}
		}
	} else {
		a.logger.Warn("storage not available, returning empty list")
		allQuarantinedFiles = []*common.QuarantinedFile{}
	}

	// Filter by reason if specified
	var quarantinedFiles []*common.QuarantinedFile
	if reasonFilter != "" {
		quarantinedFiles = make([]*common.QuarantinedFile, 0, len(allQuarantinedFiles))
		for _, qf := range allQuarantinedFiles {
			if qf.Reason == reasonFilter {
				quarantinedFiles = append(quarantinedFiles, qf)
			}
		}
	} else {
		quarantinedFiles = allQuarantinedFiles
	}

	// Convert to API response format
	entries := make([]generated.CorpusEntry, 0, len(quarantinedFiles))
	for _, qf := range quarantinedFiles {
		entry := a.quarantinedFileToEntry(qf)
		entries = append(entries, entry)
	}

	// Calculate total before pagination
	total := len(entries)

	// Apply pagination
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedEntries := entries[start:end]

	response := generated.CorpusListResponse{
		Data: paginatedEntries,
		Pagination: generated.Pagination{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// SelectCorpus selects corpus entries for a campaign
func (a *CorpusAdapter) SelectCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()
	a.logger.Debug("selecting corpus entries")

	var req generated.CorpusSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	campaignID := req.CampaignId.String()

	// Get corpus files from storage
	var corpusFiles []*common.CorpusFile
	var err error

	if a.storage != nil {
		corpusFiles, err = a.storage.GetCorpusFiles(ctx, campaignID)
		if err != nil {
			a.logger.WithError(err).Error("failed to get corpus files for selection")
			a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get corpus files", err)
			return
		}
	} else {
		a.logger.Warn("storage not available, returning empty selection")
		corpusFiles = []*common.CorpusFile{}
	}

	// Apply selection strategy (default: all files, can be extended for more complex selection)
	strategy := string(req.SelectionStrategy)
	if strategy == "" {
		strategy = "all"
	}

	// Apply max size limit if specified from criteria
	maxSize := 0
	if req.Criteria != nil && req.Criteria.MaxSizeBytes != nil {
		maxSize = *req.Criteria.MaxSizeBytes
	}

	// Select corpus files based on strategy
	selectedFiles := make([]*common.CorpusFile, 0, len(corpusFiles))
	var currentSize int64

	for _, cf := range corpusFiles {
		// Apply size limit if specified
		if maxSize > 0 && int(currentSize+cf.Size) > maxSize {
			continue
		}

		// Apply selection strategy
		switch strategy {
		case "seed-only":
			if cf.IsSeed {
				selectedFiles = append(selectedFiles, cf)
				currentSize += cf.Size
			}
		case "high-coverage":
			if cf.NewCoverage > 0 || cf.Coverage > 0 {
				selectedFiles = append(selectedFiles, cf)
				currentSize += cf.Size
			}
		case "all", "":
			selectedFiles = append(selectedFiles, cf)
			currentSize += cf.Size
		}
	}

	// Convert to API format and collect IDs
	selectedIDs := make([]openapi_types.UUID, 0, len(selectedFiles))
	var totalCoverage int64
	var totalSize int64

	for _, cf := range selectedFiles {
		if id, err := uuid.Parse(cf.ID); err == nil {
			selectedIDs = append(selectedIDs, openapi_types.UUID(id))
		}
		totalCoverage += cf.Coverage
		totalSize += cf.Size
	}

	durationSeconds := time.Since(startTime).Seconds()
	totalSizeInt := int(totalSize)

	response := generated.CorpusSelectionResponse{
		SelectionId:          openapi_types.UUID(uuid.New()),
		SelectedEntries:      selectedIDs,
		TotalCoverage:        int(totalCoverage),
		TotalSizeBytes:       &totalSizeInt,
		SelectionTimeSeconds: float32(durationSeconds),
		StrategyUsed:         &strategy,
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.selected", map[string]interface{}{
			"campaign_id":   campaignID,
			"selection_id":  response.SelectionId,
			"selected":      len(response.SelectedEntries),
			"strategy_used": strategy,
			"total_size":    totalSize,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.logger.WithFields(logrus.Fields{
		"campaign_id":      campaignID,
		"selected_count":   len(selectedIDs),
		"total_size":       totalSize,
		"strategy":         strategy,
		"duration_seconds": durationSeconds,
	}).Info("corpus selection completed")

	a.writeJSONResponse(w, http.StatusOK, response)
}

// SyncCorpus synchronizes corpus between campaigns
func (a *CorpusAdapter) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()
	a.logger.Debug("synchronizing corpus")

	var req generated.CorpusSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	sourceCampaignID := req.SourceCampaignId.String()
	targetCampaignID := req.TargetCampaignId.String()

	// Get corpus counts before sync for statistics
	var sourceFilesCount, targetFilesBeforeCount int
	if a.storage != nil {
		sourceFiles, err := a.storage.GetCorpusFiles(ctx, sourceCampaignID)
		if err == nil {
			sourceFilesCount = len(sourceFiles)
		}
		targetFiles, err := a.storage.GetCorpusFiles(ctx, targetCampaignID)
		if err == nil {
			targetFilesBeforeCount = len(targetFiles)
		}
	}

	// Perform the actual sync using corpus service
	var syncedFilesCount int
	var totalSizeBytes int64
	if a.corpusService != nil {
		// ShareCorpus copies corpus from source to target campaign
		if err := a.corpusService.ShareCorpus(ctx, sourceCampaignID, targetCampaignID); err != nil {
			a.logger.WithError(err).Error("failed to sync corpus between campaigns")
			a.writeError(w, http.StatusInternalServerError, "SYNC_ERROR", "Failed to sync corpus", err)
			return
		}

		// Get updated counts after sync
		if a.storage != nil {
			targetFilesAfter, err := a.storage.GetCorpusFiles(ctx, targetCampaignID)
			if err == nil {
				syncedFilesCount = len(targetFilesAfter) - targetFilesBeforeCount
				for _, f := range targetFilesAfter {
					totalSizeBytes += f.Size
				}
			}
		}
	} else {
		a.logger.Warn("corpus service not available, sync skipped")
	}

	durationSeconds := time.Since(startTime).Seconds()
	syncID := uuid.New()

	// Get final target count
	targetFilesAfterCount := targetFilesBeforeCount + syncedFilesCount

	// Build response
	skippedFiles := sourceFilesCount - syncedFilesCount
	if skippedFiles < 0 {
		skippedFiles = 0
	}

	response := generated.CorpusSyncResponse{
		SyncId:          openapi_types.UUID(syncID),
		SyncedFiles:     syncedFilesCount,
		SkippedFiles:    &skippedFiles,
		TotalSizeBytes:  int(totalSizeBytes),
		DurationSeconds: float32(durationSeconds),
		StrategyUsed:    "copy-all",
		Summary: &struct {
			CoverageImprovement *float32 `json:"coverage_improvement,omitempty"`
			SourceTotalFiles    *int     `json:"source_total_files,omitempty"`
			TargetFilesAfter    *int     `json:"target_files_after,omitempty"`
			TargetFilesBefore   *int     `json:"target_files_before,omitempty"`
		}{
			SourceTotalFiles:  &sourceFilesCount,
			TargetFilesBefore: &targetFilesBeforeCount,
			TargetFilesAfter:  &targetFilesAfterCount,
		},
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.sync.completed", map[string]interface{}{
			"sync_id":         response.SyncId,
			"source_campaign": sourceCampaignID,
			"target_campaign": targetCampaignID,
			"synced_files":    response.SyncedFiles,
			"duration":        durationSeconds,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.logger.WithFields(logrus.Fields{
		"sync_id":         syncID.String(),
		"source_campaign": sourceCampaignID,
		"target_campaign": targetCampaignID,
		"synced_files":    syncedFilesCount,
		"duration":        durationSeconds,
	}).Info("corpus sync completed successfully")

	a.writeJSONResponse(w, http.StatusAccepted, response)
}

// DeleteCorpusEntry deletes a corpus entry
func (a *CorpusAdapter) DeleteCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	ctx := r.Context()
	a.logger.WithField("entry_id", entryId).Debug("deleting corpus entry")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Get corpus file to get metadata for deletion
	corpusFile, err := a.storage.GetCorpusFile(ctx, entryId.String())
	if err != nil {
		a.logger.WithError(err).WithField("entry_id", entryId).Error("failed to get corpus file for deletion")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus entry not found", err)
		return
	}

	// Delete from file storage if available
	if a.fileStorage != nil && corpusFile != nil {
		if corpusFile.CampaignID != "" && corpusFile.Hash != "" {
			filePath := common.CorpusFilePath(corpusFile.CampaignID, corpusFile.Hash)
			if err := a.fileStorage.DeleteFile(ctx, filePath); err != nil {
				a.logger.WithError(err).WithFields(logrus.Fields{
					"entry_id":  entryId,
					"file_path": filePath,
				}).Warn("failed to delete corpus file from storage, continuing with metadata deletion")
			}
		} else {
			a.logger.WithFields(logrus.Fields{
				"entry_id": entryId,
				"campaign": corpusFile.CampaignID,
				"hash":     corpusFile.Hash,
				"filename": corpusFile.Filename,
			}).Warn("corpus entry missing hash or campaign ID; skipping file deletion")
		}
	}

	// Delete from database
	if err := a.storage.DeleteCorpusFile(ctx, entryId.String()); err != nil {
		a.logger.WithError(err).WithField("entry_id", entryId).Error("failed to delete corpus file metadata")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to delete corpus entry", err)
		return
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.deleted", map[string]interface{}{
			"entry_id":    entryId,
			"campaign_id": corpusFile.CampaignID,
			"filename":    corpusFile.Filename,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.logger.WithField("entry_id", entryId).Info("corpus entry deleted successfully")
	w.WriteHeader(http.StatusNoContent)
}

// GetCorpusEntry retrieves a single corpus entry
func (a *CorpusAdapter) GetCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam, params generated.GetCorpusEntryParams) {
	ctx := r.Context()
	a.logger.WithField("entry_id", entryId).Debug("getting corpus entry")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Get corpus file from storage
	corpusFile, err := a.storage.GetCorpusFile(ctx, entryId.String())
	if err != nil {
		a.logger.WithError(err).WithField("entry_id", entryId).Error("failed to get corpus file")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus entry not found", err)
		return
	}

	// Convert to API response format
	entry := a.corpusFileToEntry(corpusFile)

	a.writeJSONResponse(w, http.StatusOK, entry)
}

// DownloadCorpusFile downloads a corpus file
func (a *CorpusAdapter) DownloadCorpusFile(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	ctx := r.Context()
	a.logger.WithField("entry_id", entryId).Debug("downloading corpus file")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}
	if a.fileStorage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Get corpus file metadata from storage
	corpusFile, err := a.storage.GetCorpusFile(ctx, entryId.String())
	if err != nil {
		a.logger.WithError(err).WithField("entry_id", entryId).Error("failed to get corpus file metadata")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus entry not found", err)
		return
	}

	if corpusFile.CampaignID == "" || corpusFile.Hash == "" {
		a.writeError(w, http.StatusInternalServerError, "INVALID_METADATA", "Corpus entry missing campaign ID or hash", nil)
		return
	}

	filePath := common.CorpusFilePath(corpusFile.CampaignID, corpusFile.Hash)
	content, err := a.fileStorage.ReadFile(ctx, filePath)
	if err != nil {
		a.logger.WithError(err).WithFields(logrus.Fields{
			"entry_id":  entryId,
			"file_path": filePath,
		}).Error("failed to read corpus file from storage")
		a.writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", "Failed to read corpus file", err)
		return
	}

	// Set response headers
	filename := corpusFile.Filename
	if filename == "" {
		filename = fmt.Sprintf("corpus_%s.bin", entryId.String()[:8])
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))

	// Add hash header for verification
	if corpusFile.Hash != "" {
		w.Header().Set("X-Content-Hash", corpusFile.Hash)
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(content); err != nil {
		a.logger.WithError(err).Warn("failed to write response body")
	}
}

// Helper methods

// corpusFileToEntry converts a common.CorpusFile to a generated.CorpusEntry
func (a *CorpusAdapter) corpusFileToEntry(cf *common.CorpusFile) generated.CorpusEntry {
	entry := generated.CorpusEntry{
		Filename:  cf.Filename,
		SizeBytes: int(cf.Size),
		Hash:      cf.Hash,
		CreatedAt: cf.CreatedAt,
	}

	// Parse UUIDs safely
	if id, err := uuid.Parse(cf.ID); err == nil {
		entry.Id = openapi_types.UUID(id)
	}
	if campaignID, err := uuid.Parse(cf.CampaignID); err == nil {
		entry.CampaignId = openapi_types.UUID(campaignID)
	}
	if cf.JobID != "" {
		if jobID, err := uuid.Parse(cf.JobID); err == nil {
			entry.JobId = openapi_types.UUID(jobID)
		}
	}
	if cf.BotID != "" {
		if botID, err := uuid.Parse(cf.BotID); err == nil {
			entry.BotId = &openapi_types.UUID{}
			*entry.BotId = openapi_types.UUID(botID)
		}
	}

	// Set tags based on seed status
	if cf.IsSeed {
		entry.Tags = &[]string{"seed"}
	} else {
		entry.Tags = &[]string{"generated"}
	}

	// Add coverage metadata if available
	if cf.Coverage > 0 || cf.NewCoverage > 0 {
		metadata := generated.Metadata{
			"coverage":     cf.Coverage,
			"new_coverage": cf.NewCoverage,
			"generation":   cf.Generation,
		}
		entry.Metadata = &metadata
	}

	return entry
}

func normalizeExtensions(exts []string) []string {
	if len(exts) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(exts))
	for _, ext := range exts {
		trimmed := strings.TrimSpace(strings.ToLower(ext))
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ".") {
			trimmed = "." + trimmed
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func isAllowedExtension(filename string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	for _, allowedExt := range allowed {
		if ext == allowedExt {
			return true
		}
	}
	return false
}

// quarantinedFileToEntry converts a common.QuarantinedFile to a generated.CorpusEntry
func (a *CorpusAdapter) quarantinedFileToEntry(qf *common.QuarantinedFile) generated.CorpusEntry {
	entry := generated.CorpusEntry{
		Hash:      qf.Hash,
		CreatedAt: qf.QuarantinedAt,
	}

	// Parse UUIDs safely
	if id, err := uuid.Parse(qf.ID); err == nil {
		entry.Id = openapi_types.UUID(id)
	}
	if campaignID, err := uuid.Parse(qf.CampaignID); err == nil {
		entry.CampaignId = openapi_types.UUID(campaignID)
	}

	// Add quarantine metadata
	metadata := generated.Metadata{
		"quarantine_reason":  qf.Reason,
		"quarantine_details": qf.Details,
		"quarantined_by":     qf.QuarantinedBy,
	}
	if qf.Resolution != nil {
		metadata["resolution"] = *qf.Resolution
	}
	entry.Metadata = &metadata

	entry.Tags = &[]string{"quarantined"}

	return entry
}

func (a *CorpusAdapter) writeJSONResponse(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		a.logger.WithError(err).Error("failed to encode JSON response")
	}
}

func (a *CorpusAdapter) writeError(w http.ResponseWriter, statusCode int, errorType, title string, err error) {
	problem := generated.ProblemDetails{
		Type:      fmt.Sprintf("/errors/%s", strings.ToLower(errorType)),
		Title:     title,
		Status:    statusCode,
		Timestamp: &[]time.Time{time.Now()}[0],
	}

	if err != nil {
		detail := err.Error()
		problem.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if encodeErr := json.NewEncoder(w).Encode(problem); encodeErr != nil {
		a.logger.WithError(encodeErr).Error("failed to encode error response")
	}
}

func calculateTotalSize(entries []generated.CorpusEntry) int {
	total := 0
	for _, entry := range entries {
		total += entry.SizeBytes
	}
	return total
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// PromoteCrashToCorpus promotes a crash input to the corpus (from v3)
func (a *CorpusAdapter) PromoteCrashToCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a.logger.Debug("promoting crash to corpus")

	var req struct {
		CrashID    string   `json:"crash_id"`
		CampaignID string   `json:"campaign_id"`
		Tags       []string `json:"tags,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	if req.CrashID == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_CRASH_ID", "Crash ID is required", nil)
		return
	}

	if req.CampaignID == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_CAMPAIGN_ID", "Campaign ID is required", nil)
		return
	}

	// Use corpus service to promote crash to corpus
	var corpusFile *common.CorpusFile
	var err error

	if a.corpusService != nil {
		corpusFile, err = a.corpusService.PromoteCrashToCorpus(ctx, req.CrashID, req.CampaignID)
		if err != nil {
			a.logger.WithError(err).WithFields(logrus.Fields{
				"crash_id":    req.CrashID,
				"campaign_id": req.CampaignID,
			}).Error("failed to promote crash to corpus")
			a.writeError(w, http.StatusInternalServerError, "PROMOTION_FAILED", "Failed to promote crash to corpus", err)
			return
		}
	} else {
		a.logger.Warn("corpus service not available, cannot promote crash")
		a.writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Corpus service not available", nil)
		return
	}

	// Convert to API response format
	entry := a.corpusFileToEntry(corpusFile)

	// Add promoted and crash tags
	promotedTags := []string{"promoted", "crash"}
	if len(req.Tags) > 0 {
		promotedTags = append(promotedTags, req.Tags...)
	}
	entry.Tags = &promotedTags

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.promoted", map[string]interface{}{
			"entry_id":    entry.Id,
			"crash_id":    req.CrashID,
			"campaign_id": entry.CampaignId,
			"filename":    entry.Filename,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.logger.WithFields(logrus.Fields{
		"crash_id":    req.CrashID,
		"campaign_id": req.CampaignID,
		"entry_id":    corpusFile.ID,
	}).Info("crash promoted to corpus successfully")

	response := map[string]interface{}{
		"success":  true,
		"entry_id": corpusFile.ID,
		"entry":    entry,
		"message":  "Crash promoted to corpus successfully",
	}

	a.writeJSONResponse(w, http.StatusCreated, response)
}

// ListCorpusCollections returns all corpus collections
func (a *CorpusAdapter) ListCorpusCollections(w http.ResponseWriter, r *http.Request, params generated.ListCorpusCollectionsParams) {
	ctx := r.Context()
	a.logger.Debug("listing corpus collections")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	collections, err := a.storage.GetCorpusCollections(ctx)
	if err != nil {
		a.logger.WithError(err).Error("failed to get corpus collections")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list corpus collections", err)
		return
	}

	// Apply pagination if specified
	total := len(collections)
	limit := 50
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Apply pagination to results
	end := offset + limit
	if end > total {
		end = total
	}
	if offset > total {
		offset = total
	}
	pagedCollections := collections[offset:end]

	a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"collections": pagedCollections,
		"pagination": map[string]interface{}{
			"total":    total,
			"limit":    limit,
			"offset":   offset,
			"has_more": end < total,
		},
	})
}

// CreateCorpusCollection creates a new corpus collection
func (a *CorpusAdapter) CreateCorpusCollection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a.logger.Debug("creating corpus collection")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Parse request body
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	if req.Name == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_NAME", "Collection name is required", nil)
		return
	}

	// Create collection
	collection := &common.CorpusCollection{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := a.storage.CreateCorpusCollection(ctx, collection); err != nil {
		a.logger.WithError(err).Error("failed to create corpus collection")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to create corpus collection", err)
		return
	}

	a.writeJSONResponse(w, http.StatusCreated, collection)
}

// GetCorpusCollection returns a specific corpus collection
func (a *CorpusAdapter) GetCorpusCollection(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	a.logger.WithField("collection_id", collectionID).Debug("getting corpus collection")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	collection, err := a.storage.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		a.logger.WithError(err).WithField("collection_id", collectionID).Error("failed to get corpus collection")
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus collection not found", err)
		return
	}

	a.writeJSONResponse(w, http.StatusOK, collection)
}

// UpdateCorpusCollection updates a corpus collection
func (a *CorpusAdapter) UpdateCorpusCollection(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	a.logger.WithField("collection_id", collectionID).Debug("updating corpus collection")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Get existing collection
	collection, err := a.storage.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus collection not found", err)
		return
	}

	// Parse request body for updates
	var req struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Tags        []string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Apply updates
	if req.Name != nil {
		collection.Name = *req.Name
	}
	if req.Description != nil {
		collection.Description = *req.Description
	}
	if req.Tags != nil {
		collection.Tags = req.Tags
	}
	collection.UpdatedAt = time.Now()

	if err := a.storage.UpdateCorpusCollection(ctx, collection); err != nil {
		a.logger.WithError(err).Error("failed to update corpus collection")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to update corpus collection", err)
		return
	}

	a.writeJSONResponse(w, http.StatusOK, collection)
}

// DeleteCorpusCollection deletes a corpus collection
func (a *CorpusAdapter) DeleteCorpusCollection(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	a.logger.WithField("collection_id", collectionID).Debug("deleting corpus collection")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	if err := a.storage.DeleteCorpusCollection(ctx, collectionID); err != nil {
		a.logger.WithError(err).Error("failed to delete corpus collection")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to delete corpus collection", err)
		return
	}

	a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Corpus collection deleted successfully",
	})
}

// UploadCorpusCollectionFiles uploads files to a corpus collection
func (a *CorpusAdapter) UploadCorpusCollectionFiles(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	a.logger.WithField("collection_id", collectionID).Debug("uploading files to corpus collection")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}
	if a.fileStorage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "File storage not configured", nil)
		return
	}

	// Verify collection exists
	_, err := a.storage.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus collection not found", err)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		a.writeError(w, http.StatusBadRequest, "PARSE_ERROR", "Failed to parse multipart form", err)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		a.writeError(w, http.StatusBadRequest, "NO_FILES", "No files provided", nil)
		return
	}

	uploadedFiles := make([]*common.CorpusCollectionFile, 0, len(files))
	for _, fileHeader := range files {
		if !isAllowedExtension(fileHeader.Filename, a.allowedExts) {
			a.writeError(w, http.StatusBadRequest, "INVALID_FILE_TYPE", "File extension not allowed", nil)
			return
		}
		if a.maxFileSize > 0 && fileHeader.Size > a.maxFileSize {
			a.writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "Corpus file exceeds size limit", nil)
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to open uploaded file")
			continue
		}

		tempPath, size, hash, err := streamToTempFile(file, a.maxFileSize, true)
		file.Close()
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errUploadTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			a.writeError(w, status, "UPLOAD_FAILED", "Failed to stream uploaded file", err)
			return
		}

		tempFile, err := os.Open(tempPath)
		if err != nil {
			os.Remove(tempPath)
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to open temp file")
			continue
		}

		cleanupTemp := func() {
			tempFile.Close()
			os.Remove(tempPath)
		}

		// Create corpus collection file record
		collectionFile := &common.CorpusCollectionFile{
			ID:           uuid.New().String(),
			CollectionID: collectionID,
			Filename:     fileHeader.Filename,
			Size:         size,
			Hash:         hash,
			UploadedAt:   time.Now(),
		}

		if err := a.storage.AddCorpusCollectionFile(ctx, collectionFile); err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to add corpus collection file")
			cleanupTemp()
			continue
		}

		filePath := common.CorpusCollectionFilePath(collectionID, hash)
		if err := a.fileStorage.SaveFileStream(ctx, filePath, tempFile, size); err != nil {
			a.logger.WithError(err).WithFields(logrus.Fields{
				"filename":  fileHeader.Filename,
				"file_path": filePath,
			}).Error("failed to store corpus collection file content")
			cleanupTemp()
			if deleteErr := a.storage.DeleteCorpusCollectionFile(ctx, collectionFile.ID); deleteErr != nil {
				a.logger.WithError(deleteErr).WithField("file_id", collectionFile.ID).Warn("failed to delete collection metadata after save failure")
			}
			continue
		}
		cleanupTemp()

		uploadedFiles = append(uploadedFiles, collectionFile)
	}

	a.writeJSONResponse(w, http.StatusCreated, map[string]interface{}{
		"success":        true,
		"uploaded_count": len(uploadedFiles),
		"files":          uploadedFiles,
	})
}

// ListCorpusCollectionFiles lists files in a corpus collection
func (a *CorpusAdapter) ListCorpusCollectionFiles(w http.ResponseWriter, r *http.Request, collectionID string) {
	ctx := r.Context()
	a.logger.WithField("collection_id", collectionID).Debug("listing corpus collection files")

	if a.storage == nil {
		a.writeError(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "Storage not configured", nil)
		return
	}

	// Verify collection exists
	_, err := a.storage.GetCorpusCollection(ctx, collectionID)
	if err != nil {
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "Corpus collection not found", err)
		return
	}

	files, err := a.storage.GetCorpusCollectionFiles(ctx, collectionID)
	if err != nil {
		a.logger.WithError(err).Error("failed to get corpus collection files")
		a.writeError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list corpus collection files", err)
		return
	}

	a.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"files": files,
		"total": len(files),
	})
}
