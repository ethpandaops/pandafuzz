package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/sse"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/quarantine"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/sync"
	corpusTypes "github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// CorpusAdapter implements the corpus-related endpoints of the generated ServerInterface
type CorpusAdapter struct {
	repository  repository.CorpusRepository
	syncService *sync.Service
	quarantine  *quarantine.Service
	sse         *sse.Manager
	logger      logrus.FieldLogger
}

// NewCorpusAdapter creates a new corpus adapter
func NewCorpusAdapter(
	repository repository.CorpusRepository,
	syncService *sync.Service,
	quarantine *quarantine.Service,
	sse *sse.Manager,
	logger logrus.FieldLogger,
) *CorpusAdapter {
	return &CorpusAdapter{
		repository:  repository,
		syncService: syncService,
		quarantine:  quarantine,
		sse:         sse,
		logger:      logger.WithField("component", "corpus_adapter"),
	}
}

// ListCorpus retrieves corpus entries with filtering and pagination
func (a *CorpusAdapter) ListCorpus(w http.ResponseWriter, r *http.Request, params generated.ListCorpusParams) {
	ctx := r.Context()

	// Set defaults for pagination
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	// Build filter
	filter := repository.CorpusFilter{
		Limit:  limit,
		Offset: offset,
	}

	if params.CampaignId != nil {
		campaignID := params.CampaignId.String()
		filter.CampaignID = &campaignID
	}

	if params.JobId != nil {
		jobID := params.JobId.String()
		filter.JobID = &jobID
	}

	if params.MinCoverage != nil {
		filter.MinCoverage = params.MinCoverage
	}

	// Get corpus entries from repository
	entries, total, err := a.repository.List(ctx, filter)
	if err != nil {
		a.logger.WithError(err).Error("failed to list corpus entries")
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve corpus entries", err)
		return
	}

	// Convert to API types
	apiEntries := make([]generated.CorpusEntry, len(entries))
	for i, entry := range entries {
		apiEntries[i] = a.convertCorpusEntryToAPI(entry)
	}

	// Create pagination info
	hasMore := offset+len(apiEntries) < total
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   total,
		HasMore: hasMore,
	}

	response := generated.CorpusListResponse{
		Data:       apiEntries,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// UploadCorpus handles corpus file uploads
func (a *CorpusAdapter) UploadCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		a.writeError(w, http.StatusBadRequest, "INVALID_FORM", "Failed to parse multipart form", err)
		return
	}

	// Get campaign ID
	campaignID := r.FormValue("campaign_id")
	if campaignID == "" {
		a.writeError(w, http.StatusBadRequest, "MISSING_CAMPAIGN_ID", "Campaign ID is required", nil)
		return
	}

	// Validate campaign ID format
	if _, err := uuid.Parse(campaignID); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_CAMPAIGN_ID", "Invalid campaign ID format", err)
		return
	}

	// Get optional job ID
	var jobID *string
	if jid := r.FormValue("job_id"); jid != "" {
		if _, err := uuid.Parse(jid); err != nil {
			a.writeError(w, http.StatusBadRequest, "INVALID_JOB_ID", "Invalid job ID format", err)
			return
		}
		jobID = &jid
	}

	// Get optional tags
	var tags []string
	if tagsStr := r.FormValue("tags"); tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	// Process uploaded files
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		a.writeError(w, http.StatusBadRequest, "NO_FILES", "No files provided", nil)
		return
	}

	uploadResult := a.processCorpusUpload(ctx, campaignID, jobID, tags, files)

	// Publish SSE event
	event := sse.NewCorpusEvent("corpus.uploaded", map[string]any{
		"campaign_id":      campaignID,
		"job_id":           jobID,
		"uploaded_count":   uploadResult.UploadedCount,
		"duplicate_count":  uploadResult.DuplicateCount,
		"total_size_bytes": uploadResult.TotalSizeBytes,
		"timestamp":        time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast corpus uploaded event")
	}

	a.writeJSONResponse(w, http.StatusCreated, uploadResult)
}

// GetCorpusEntry retrieves a specific corpus entry by ID
func (a *CorpusAdapter) GetCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam, params generated.GetCorpusEntryParams) {
	ctx := r.Context()

	entry, err := a.repository.FindByID(ctx, entryId.String())
	if err != nil {
		a.logger.WithError(err).WithField("entry_id", entryId).Error("failed to get corpus entry")
		a.writeError(w, http.StatusNotFound, "ENTRY_NOT_FOUND", "Corpus entry not found", err)
		return
	}

	apiEntry := a.convertCorpusEntryToAPI(entry)
	a.writeJSONResponse(w, http.StatusOK, apiEntry)
}

// DeleteCorpusEntry deletes a corpus entry
func (a *CorpusAdapter) DeleteCorpusEntry(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	ctx := r.Context()

	// Check if entry exists
	entry, err := a.repository.FindByID(ctx, entryId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "ENTRY_NOT_FOUND", "Corpus entry not found", err)
		return
	}

	// Delete entry
	if err := a.repository.Delete(ctx, entryId.String()); err != nil {
		a.logger.WithError(err).Error("failed to delete corpus entry")
		a.writeError(w, http.StatusInternalServerError, "DELETE_FAILED", "Failed to delete corpus entry", err)
		return
	}

	// Publish SSE event
	event := sse.NewCorpusEvent("corpus.entry.deleted", map[string]any{
		"entry_id":    entryId.String(),
		"campaign_id": entry.CampaignID,
		"timestamp":   time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast corpus entry deleted event")
	}

	w.WriteHeader(http.StatusNoContent)
}

// DownloadCorpusFile downloads a corpus entry file
func (a *CorpusAdapter) DownloadCorpusFile(w http.ResponseWriter, r *http.Request, entryId generated.CorpusEntryIdParam) {
	ctx := r.Context()

	// Get corpus entry
	entry, err := a.repository.FindByID(ctx, entryId.String())
	if err != nil {
		a.writeError(w, http.StatusNotFound, "ENTRY_NOT_FOUND", "Corpus entry not found", err)
		return
	}

	// Get file data from storage
	fileData, err := a.getCorpusFileData(ctx, entry)
	if err != nil {
		a.logger.WithError(err).Error("failed to get corpus file data")
		a.writeError(w, http.StatusInternalServerError, "FILE_ACCESS_FAILED", "Failed to access file", err)
		return
	}

	// Set download headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", entry.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fileData)))
	w.WriteHeader(http.StatusOK)
	w.Write(fileData)
}

// SelectCorpus performs corpus selection based on criteria
func (a *CorpusAdapter) SelectCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req generated.CorpusSelectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Perform corpus selection
	selection := a.performCorpusSelection(ctx, &req)

	// Publish SSE event
	event := sse.NewCorpusEvent("corpus.selected", map[string]any{
		"selection_id":   selection.SelectionId.String(),
		"campaign_id":    req.CampaignId.String(),
		"selected_count": len(selection.SelectedEntries),
		"total_coverage": selection.TotalCoverage,
		"strategy_used":  selection.StrategyUsed,
		"timestamp":      time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast corpus selected event")
	}

	a.writeJSONResponse(w, http.StatusOK, selection)
}

// SyncCorpus synchronizes corpus between campaigns
func (a *CorpusAdapter) SyncCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req generated.CorpusSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err)
		return
	}

	// Perform corpus sync
	syncResult := a.performCorpusSync(ctx, &req)

	// Publish SSE event
	event := sse.NewCorpusEvent("corpus.synced", map[string]any{
		"sync_id":          syncResult.SyncId.String(),
		"source_campaign":  req.SourceCampaignId.String(),
		"target_campaign":  req.TargetCampaignId.String(),
		"synced_files":     syncResult.SyncedFiles,
		"total_size_bytes": syncResult.TotalSizeBytes,
		"strategy_used":    syncResult.StrategyUsed,
		"timestamp":        time.Now(),
	})
	if err := a.sse.Broadcast(event); err != nil {
		a.logger.WithError(err).Warn("failed to broadcast corpus synced event")
	}

	a.writeJSONResponse(w, http.StatusOK, syncResult)
}

// ListQuarantinedCorpus lists quarantined corpus entries
func (a *CorpusAdapter) ListQuarantinedCorpus(w http.ResponseWriter, r *http.Request, params generated.ListQuarantinedCorpusParams) {
	ctx := r.Context()

	// Set defaults for pagination
	limit := 50
	offset := 0

	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		if limit > 1000 {
			limit = 1000
		}
	}

	if params.Offset != nil && *params.Offset >= 0 {
		offset = *params.Offset
	}

	// Get quarantined entries
	entries := a.getQuarantinedEntries(ctx, params.Reason, limit, offset)

	// Create pagination info
	pagination := generated.Pagination{
		Limit:   limit,
		Offset:  offset,
		Total:   len(entries),
		HasMore: len(entries) == limit,
	}

	response := generated.CorpusListResponse{
		Data:       entries,
		Pagination: pagination,
	}

	a.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (a *CorpusAdapter) convertCorpusEntryToAPI(entry *corpusTypes.Entry) generated.CorpusEntry {
	apiEntry := generated.CorpusEntry{
		Id:         uuid.MustParse(entry.ID),
		CampaignId: uuid.MustParse(entry.CampaignID),
		JobId:      uuid.MustParse(entry.JobID),
		Filename:   entry.Filename,
		Hash:       entry.Hash,
		SizeBytes:  entry.SizeBytes,
		CreatedAt:  entry.CreatedAt,
	}

	if entry.BotID != nil {
		botID := uuid.MustParse(*entry.BotID)
		apiEntry.BotId = &botID
	}

	if entry.IsSeed {
		apiEntry.IsSeed = &entry.IsSeed
	}

	if entry.IsMinimized {
		apiEntry.IsMinimized = &entry.IsMinimized
	}

	if len(entry.Tags) > 0 {
		apiEntry.Tags = &entry.Tags
	}

	// Set coverage info if available
	if entry.CoverageEdges > 0 {
		coverageInfo := struct {
			BlocksCovered *int `json:"blocks_covered,omitempty"`
			EdgesCovered  *int `json:"edges_covered,omitempty"`
			NewEdges      *int `json:"new_edges,omitempty"`
		}{
			EdgesCovered: &entry.CoverageEdges,
		}
		apiEntry.CoverageInfo = &coverageInfo
	}

	return apiEntry
}

func (a *CorpusAdapter) processCorpusUpload(ctx context.Context, campaignID string, jobID *string, tags []string, files []*http.UploadedFile) generated.CorpusUploadResponse {
	uploadID := uuid.New()
	uploadedCount := 0
	duplicateCount := 0
	totalSizeBytes := 0
	var errors []struct {
		Code     *string `json:"code,omitempty"`
		Error    *string `json:"error,omitempty"`
		Filename *string `json:"filename,omitempty"`
	}

	startTime := time.Now()

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			error := struct {
				Code     *string `json:"code,omitempty"`
				Error    *string `json:"error,omitempty"`
				Filename *string `json:"filename,omitempty"`
			}{
				Code:     &[]string{"FILE_OPEN_ERROR"}[0],
				Error:    &[]string{err.Error()}[0],
				Filename: &fileHeader.Filename,
			}
			errors = append(errors, error)
			continue
		}
		defer file.Close()

		// Read file data
		data, err := io.ReadAll(file)
		if err != nil {
			error := struct {
				Code     *string `json:"code,omitempty"`
				Error    *string `json:"error,omitempty"`
				Filename *string `json:"filename,omitempty"`
			}{
				Code:     &[]string{"FILE_READ_ERROR"}[0],
				Error:    &[]string{err.Error()}[0],
				Filename: &fileHeader.Filename,
			}
			errors = append(errors, error)
			continue
		}

		// Create corpus entry
		entry, err := corpusTypes.NewEntry(campaignID, *jobID, fileHeader.Filename, data)
		if err != nil {
			error := struct {
				Code     *string `json:"code,omitempty"`
				Error    *string `json:"error,omitempty"`
				Filename *string `json:"filename,omitempty"`
			}{
				Code:     &[]string{"ENTRY_CREATION_ERROR"}[0],
				Error:    &[]string{err.Error()}[0],
				Filename: &fileHeader.Filename,
			}
			errors = append(errors, error)
			continue
		}

		// Set tags
		entry.Tags = tags

		// Check for duplicates
		existing, err := a.repository.FindByHash(ctx, entry.Hash)
		if err == nil && existing != nil {
			duplicateCount++
			continue
		}

		// Save entry
		if err := a.repository.Create(ctx, entry); err != nil {
			error := struct {
				Code     *string `json:"code,omitempty"`
				Error    *string `json:"error,omitempty"`
				Filename *string `json:"filename,omitempty"`
			}{
				Code:     &[]string{"SAVE_ERROR"}[0],
				Error:    &[]string{err.Error()}[0],
				Filename: &fileHeader.Filename,
			}
			errors = append(errors, error)
			continue
		}

		uploadedCount++
		totalSizeBytes += entry.SizeBytes
	}

	processingTime := float32(time.Since(startTime).Seconds())
	errorCount := len(errors)

	response := generated.CorpusUploadResponse{
		UploadId:              uploadID,
		UploadedCount:         uploadedCount,
		DuplicateCount:        duplicateCount,
		TotalSizeBytes:        totalSizeBytes,
		ProcessingTimeSeconds: &processingTime,
	}

	if errorCount > 0 {
		response.ErrorCount = &errorCount
		response.Errors = &errors
	}

	return response
}

func (a *CorpusAdapter) performCorpusSelection(ctx context.Context, req *generated.CorpusSelectionRequest) generated.CorpusSelectionResponse {
	startTime := time.Now()

	// Mock implementation - in reality, this would use the selection service
	selectedEntries := []uuid.UUID{
		uuid.New(),
		uuid.New(),
		uuid.New(),
	}

	selectionTime := float32(time.Since(startTime).Seconds())
	strategy := string(req.SelectionStrategy)

	response := generated.CorpusSelectionResponse{
		SelectionId:          uuid.New(),
		SelectedEntries:      selectedEntries,
		TotalCoverage:        750,
		SelectionTimeSeconds: selectionTime,
		StrategyUsed:         &strategy,
		TotalSizeBytes:       &[]int{1024 * 10}[0], // 10KB
	}

	return response
}

func (a *CorpusAdapter) performCorpusSync(ctx context.Context, req *generated.CorpusSyncRequest) generated.CorpusSyncResponse {
	startTime := time.Now()

	// Mock implementation - in reality, this would use the sync service
	syncedFiles := 25
	totalSizeBytes := 1024 * 1024 // 1MB
	strategy := "selective"
	if req.SyncStrategy != nil {
		strategy = string(*req.SyncStrategy)
	}

	durationSeconds := float32(time.Since(startTime).Seconds())

	response := generated.CorpusSyncResponse{
		SyncId:          uuid.New(),
		SyncedFiles:     syncedFiles,
		TotalSizeBytes:  totalSizeBytes,
		DurationSeconds: durationSeconds,
		StrategyUsed:    strategy,
		SkippedFiles:    &[]int{3}[0],
		Summary: &struct {
			CoverageImprovement *float32 `json:"coverage_improvement,omitempty"`
			SourceTotalFiles    *int     `json:"source_total_files,omitempty"`
			TargetFilesAfter    *int     `json:"target_files_after,omitempty"`
			TargetFilesBefore   *int     `json:"target_files_before,omitempty"`
		}{
			CoverageImprovement: &[]float32{15.5}[0],
			SourceTotalFiles:    &[]int{100}[0],
			TargetFilesAfter:    &[]int{85}[0],
			TargetFilesBefore:   &[]int{60}[0],
		},
	}

	return response
}

func (a *CorpusAdapter) getCorpusFileData(ctx context.Context, entry *corpusTypes.Entry) ([]byte, error) {
	// Mock implementation - in reality, this would fetch from storage
	return []byte(fmt.Sprintf("Mock corpus data for entry %s", entry.ID)), nil
}

func (a *CorpusAdapter) getQuarantinedEntries(ctx context.Context, reason *generated.ListQuarantinedCorpusParamsReason, limit, offset int) []generated.CorpusEntry {
	// Mock implementation - in reality, this would use the quarantine service
	entries := []generated.CorpusEntry{
		{
			Id:         uuid.New(),
			CampaignId: uuid.New(),
			JobId:      uuid.New(),
			Filename:   "suspicious_input.txt",
			Hash:       "def456abc789",
			SizeBytes:  512,
			CreatedAt:  time.Now().Add(-2 * time.Hour),
		},
	}

	return entries
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
