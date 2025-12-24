package adapters

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	sse           *sse.Manager
	logger        logrus.FieldLogger
}

// NewCorpusAdapter creates a new corpus adapter
func NewCorpusAdapter(
	corpusService common.CorpusService,
	storage common.Storage,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CorpusAdapter {
	return &CorpusAdapter{
		corpusService: corpusService,
		storage:       storage,
		sse:           sse,
		logger:        logger.WithField("adapter", "corpus"),
	}
}

// ListCorpus returns a list of corpus entries
func (a *CorpusAdapter) ListCorpus(w http.ResponseWriter, r *http.Request, params generated.ListCorpusParams) {
	a.logger.Debug("listing corpus entries")

	// Mock implementation - replace with actual service calls
	botId1 := openapi_types.UUID(uuid.New())
	botId2 := openapi_types.UUID(uuid.New())
	metadata1 := generated.Metadata{
		"coverage": 85.5,
		"edges":    1200,
	}

	entries := []generated.CorpusEntry{
		{
			Id:         openapi_types.UUID(uuid.New()),
			CampaignId: openapi_types.UUID(uuid.New()),
			JobId:      openapi_types.UUID(uuid.New()),
			Filename:   "input_001.bin",
			SizeBytes:  1024,
			Hash:       "sha256:abcd1234...",
			CreatedAt:  time.Now(),
			BotId:      &botId1,
			Tags:       &[]string{"seed", "interesting"},
			Metadata:   &metadata1,
		},
		{
			Id:         openapi_types.UUID(uuid.New()),
			CampaignId: openapi_types.UUID(uuid.New()),
			JobId:      openapi_types.UUID(uuid.New()),
			Filename:   "input_002.bin",
			SizeBytes:  2048,
			Hash:       "sha256:efgh5678...",
			CreatedAt:  time.Now(),
			BotId:      &botId2,
			Tags:       &[]string{"generated"},
		},
	}

	// Apply pagination
	limit := 10
	offset := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}

	// Ensure we don't go out of bounds
	start := offset
	end := offset + limit
	if start > len(entries) {
		start = len(entries)
	}
	if end > len(entries) {
		end = len(entries)
	}

	paginatedEntries := entries[start:end]

	response := generated.CorpusListResponse{
		Data: paginatedEntries,
		Pagination: generated.Pagination{
			Total:  len(entries),
			Limit:  limit,
			Offset: offset,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// UploadCorpus handles corpus file upload
func (a *CorpusAdapter) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	a.logger.Debug("uploading corpus files")

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

	// Process uploaded files
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		a.writeError(w, http.StatusBadRequest, "NO_FILES", "No files provided", nil)
		return
	}

	uploadedEntries := []generated.CorpusEntry{}
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			a.logger.WithError(err).Warn("failed to open uploaded file")
			continue
		}
		defer file.Close()

		// Read file content
		content, err := io.ReadAll(file)
		if err != nil {
			a.logger.WithError(err).Warn("failed to read file content")
			continue
		}

		// Create corpus entry
		entry := generated.CorpusEntry{
			Id:         openapi_types.UUID(uuid.New()),
			CampaignId: openapi_types.UUID(uuid.MustParse(campaignID)),
			JobId:      openapi_types.UUID(uuid.New()),
			Filename:   fileHeader.Filename,
			SizeBytes:  int(fileHeader.Size),
			Hash:       fmt.Sprintf("sha256:%x", content[:min(32, len(content))]), // Mock hash
			CreatedAt:  time.Now(),
			Tags:       &[]string{"uploaded"},
		}

		uploadedEntries = append(uploadedEntries, entry)

		// Publish SSE event
		if a.sse != nil {
			event := sse.NewCorpusEvent("corpus.uploaded", map[string]interface{}{
				"entry_id":    entry.Id,
				"campaign_id": entry.CampaignId,
				"filename":    entry.Filename,
				"size":        entry.SizeBytes,
			})
			a.sse.BroadcastToTopic("corpus", event)
		}
	}

	response := generated.CorpusUploadResponse{
		UploadId:       openapi_types.UUID(uuid.New()),
		UploadedCount:  len(uploadedEntries),
		DuplicateCount: 0,
		TotalSizeBytes: calculateTotalSize(uploadedEntries),
	}

	a.writeJSONResponse(w, http.StatusCreated, response)
}

// ListQuarantinedCorpus returns quarantined corpus entries
func (a *CorpusAdapter) ListQuarantinedCorpus(w http.ResponseWriter, r *http.Request, params generated.ListQuarantinedCorpusParams) {
	a.logger.Debug("listing quarantined corpus entries")

	// Mock implementation - using regular CorpusEntry since QuarantinedCorpusEntry doesn't exist
	entries := []generated.CorpusEntry{
		{
			Id:         openapi_types.UUID(uuid.New()),
			CampaignId: openapi_types.UUID(uuid.New()),
			JobId:      openapi_types.UUID(uuid.New()),
			Filename:   "suspicious_001.bin",
			SizeBytes:  4096,
			Hash:       "sha256:ijkl9012...",
			CreatedAt:  time.Now(),
			Metadata: &generated.Metadata{
				"memory_peak":       "8GB",
				"cpu_usage":         "400%",
				"quarantine_reason": "Excessive memory consumption",
			},
		},
	}

	// Use regular CorpusListResponse since QuarantinedCorpusListResponse doesn't exist
	response := generated.CorpusListResponse{
		Data: entries,
		Pagination: generated.Pagination{
			Total:  len(entries),
			Limit:  10,
			Offset: 0,
		},
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// SelectCorpus selects corpus entries for a campaign
func (a *CorpusAdapter) SelectCorpus(w http.ResponseWriter, r *http.Request) {
	a.logger.Debug("selecting corpus entries")

	var req generated.CorpusSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Mock implementation
	selectedEntries := []generated.CorpusEntry{
		{
			Id:         openapi_types.UUID(uuid.New()),
			CampaignId: req.CampaignId,
			JobId:      openapi_types.UUID(uuid.New()),
			Filename:   "selected_001.bin",
			SizeBytes:  512,
			Hash:       "sha256:mnop3456...",
			CreatedAt:  time.Now(),
			Tags:       &[]string{"selected"},
		},
	}

	// Extract entry IDs for the response
	selectedIDs := make([]openapi_types.UUID, len(selectedEntries))
	for i, entry := range selectedEntries {
		selectedIDs[i] = entry.Id
	}

	totalSize := calculateTotalSize(selectedEntries)
	response := generated.CorpusSelectionResponse{
		SelectionId:          openapi_types.UUID(uuid.New()),
		SelectedEntries:      selectedIDs,
		TotalCoverage:        1000, // Mock coverage value
		TotalSizeBytes:       &totalSize,
		SelectionTimeSeconds: 0.5,
		StrategyUsed:         &[]string{"diversity-based"}[0],
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.selected", map[string]interface{}{
			"campaign_id":   req.CampaignId,
			"selection_id":  response.SelectionId,
			"selected":      len(response.SelectedEntries),
			"strategy_used": response.StrategyUsed,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// SyncCorpus synchronizes corpus between campaigns
func (a *CorpusAdapter) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	a.logger.Debug("synchronizing corpus")

	var req generated.CorpusSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Mock implementation
	response := generated.CorpusSyncResponse{
		SyncId:          openapi_types.UUID(uuid.New()),
		SyncedFiles:     10,
		SkippedFiles:    &[]int{2}[0],
		TotalSizeBytes:  10240,
		DurationSeconds: 0.5,
		StrategyUsed:    "incremental",
		Summary: &struct {
			CoverageImprovement *float32 `json:"coverage_improvement,omitempty"`
			SourceTotalFiles    *int     `json:"source_total_files,omitempty"`
			TargetFilesAfter    *int     `json:"target_files_after,omitempty"`
			TargetFilesBefore   *int     `json:"target_files_before,omitempty"`
		}{
			CoverageImprovement: &[]float32{5.2}[0],
			SourceTotalFiles:    &[]int{100}[0],
			TargetFilesBefore:   &[]int{50}[0],
			TargetFilesAfter:    &[]int{60}[0],
		},
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.sync.started", map[string]interface{}{
			"sync_id":         response.SyncId,
			"source_campaign": req.SourceCampaignId,
			"target_campaign": req.TargetCampaignId,
			"synced_files":    response.SyncedFiles,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	a.writeJSONResponse(w, http.StatusAccepted, response)
}

// DeleteCorpusEntry deletes a corpus entry
func (a *CorpusAdapter) DeleteCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	a.logger.WithField("entry_id", entryId).Debug("deleting corpus entry")

	// Mock implementation - replace with actual service call
	// In production, this would call the corpus service to delete the entry

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.deleted", map[string]interface{}{
			"entry_id": entryId,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetCorpusEntry retrieves a single corpus entry
func (a *CorpusAdapter) GetCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam, params generated.GetCorpusEntryParams) {
	a.logger.WithField("entry_id", entryId).Debug("getting corpus entry")

	// Mock implementation
	metadata := generated.Metadata{
		"coverage": 85.5,
		"edges":    1200,
	}
	botId := openapi_types.UUID(uuid.New())

	entry := generated.CorpusEntry{
		Id:         openapi_types.UUID(entryId),
		CampaignId: openapi_types.UUID(uuid.New()),
		JobId:      openapi_types.UUID(uuid.New()),
		Filename:   "input_001.bin",
		SizeBytes:  1024,
		Hash:       "sha256:abcd1234...",
		CreatedAt:  time.Now(),
		BotId:      &botId,
		Tags:       &[]string{"seed", "interesting"},
		Metadata:   &metadata,
	}

	a.writeJSONResponse(w, http.StatusOK, entry)
}

// DownloadCorpusFile downloads a corpus file
func (a *CorpusAdapter) DownloadCorpusFile(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	a.logger.WithField("entry_id", entryId).Debug("downloading corpus file")

	// Mock implementation - in production, this would stream the actual file
	content := []byte("Mock corpus file content")

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\"corpus_file.bin\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))

	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// Helper methods

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

	// Mock implementation - would call corpus service
	entryID := uuid.New()
	entry := generated.CorpusEntry{
		Id:         openapi_types.UUID(entryID),
		CampaignId: openapi_types.UUID(uuid.MustParse(req.CampaignID)),
		JobId:      openapi_types.UUID(uuid.New()),
		Filename:   fmt.Sprintf("crash_%s.bin", req.CrashID[:8]),
		SizeBytes:  512,
		Hash:       "sha256:promoted_" + req.CrashID[:8],
		CreatedAt:  time.Now(),
		Tags:       &[]string{"promoted", "crash"},
	}

	if len(req.Tags) > 0 {
		allTags := append(*entry.Tags, req.Tags...)
		entry.Tags = &allTags
	}

	// Publish SSE event
	if a.sse != nil {
		event := sse.NewCorpusEvent("corpus.promoted", map[string]interface{}{
			"entry_id":    entry.Id,
			"crash_id":    req.CrashID,
			"campaign_id": entry.CampaignId,
		})
		a.sse.BroadcastToTopic("corpus", event)
	}

	response := map[string]interface{}{
		"success":  true,
		"entry_id": entryID.String(),
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
		file, err := fileHeader.Open()
		if err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to open uploaded file")
			continue
		}
		defer file.Close()

		// Read file content
		content, err := io.ReadAll(file)
		if err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to read uploaded file")
			continue
		}

		// Calculate proper hash
		hash := fmt.Sprintf("sha256:%x", sha256.Sum256(content))

		// Create corpus collection file record
		collectionFile := &common.CorpusCollectionFile{
			ID:           uuid.New().String(),
			CollectionID: collectionID,
			Filename:     fileHeader.Filename,
			Size:         int64(len(content)),
			Hash:         hash,
			UploadedAt:   time.Now(),
		}

		if err := a.storage.AddCorpusCollectionFile(ctx, collectionFile); err != nil {
			a.logger.WithError(err).WithField("filename", fileHeader.Filename).Warn("failed to add corpus collection file")
			continue
		}

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
