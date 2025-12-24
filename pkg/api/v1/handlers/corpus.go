package handlers

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// HandleListCorpus handles GET /api/v1/corpus
func (h *Handlers) HandleListCorpus(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListCorpusParams{}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			limitVal := limit
			params.Limit = &limitVal
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			offsetVal := offset
			params.Offset = &offsetVal
		}
	}

	if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
		if parsedUUID, err := uuid.Parse(campaignId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.CampaignId = &uuidVal
		}
	}

	if jobId := r.URL.Query().Get("job_id"); jobId != "" {
		if parsedUUID, err := uuid.Parse(jobId); err == nil {
			uuidVal := openapi_types.UUID(parsedUUID)
			params.JobId = &uuidVal
		}
	}

	// TODO: Add these parameters to OpenAPI spec
	// if source := r.URL.Query().Get("source"); source != "" {
	// 	sourceVal := generated.CorpusSource(source)
	// 	params.Source = &sourceVal
	// }

	// if minSize := r.URL.Query().Get("min_size"); minSize != "" {
	// 	if size, err := strconv.Atoi(minSize); err == nil {
	// 		sizeVal := size
	// 		params.MinSize = &sizeVal
	// 	}
	// }

	// if maxSize := r.URL.Query().Get("max_size"); maxSize != "" {
	// 	if size, err := strconv.Atoi(maxSize); err == nil {
	// 		sizeVal := size
	// 		params.MaxSize = &sizeVal
	// 	}
	// }

	// if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
	// 	params.SortBy = &sortBy
	// }

	// if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
	// 	sortOrderVal := generated.SortOrder(sortOrder)
	// 	params.SortOrder = &sortOrderVal
	// }

	// if since := r.URL.Query().Get("since"); since != "" {
	// 	params.Since = &since
	// }

	// if until := r.URL.Query().Get("until"); until != "" {
	// 	params.Until = &until
	// }

	// TODO: Add includeContent parameter to OpenAPI spec
	// if includeContent := r.URL.Query().Get("include_content"); includeContent != "" {
	// 	if include, err := strconv.ParseBool(includeContent); err == nil {
	// 		params.IncludeContent = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.ListCorpus(w, r, params)
}

// HandleUploadCorpus handles POST /api/v1/corpus
func (h *Handlers) HandleUploadCorpus(w http.ResponseWriter, r *http.Request) {
	h.adapter.UploadCorpus(w, r)
}

// HandleSyncCorpus handles POST /api/v1/corpus/sync
func (h *Handlers) HandleSyncCorpus(w http.ResponseWriter, r *http.Request) {
	h.adapter.SyncCorpus(w, r)
}

// HandleSelectCorpus handles POST /api/v1/corpus/select
func (h *Handlers) HandleSelectCorpus(w http.ResponseWriter, r *http.Request) {
	h.adapter.SelectCorpus(w, r)
}

// HandleListQuarantinedCorpus handles GET /api/v1/corpus/quarantine
func (h *Handlers) HandleListQuarantinedCorpus(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListQuarantinedCorpusParams{}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			limitVal := limit
			params.Limit = &limitVal
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			offsetVal := offset
			params.Offset = &offsetVal
		}
	}

	// Check if the generated types exist and use them if available
	if reason := r.URL.Query().Get("reason"); reason != "" {
		reasonVal := generated.ListQuarantinedCorpusParamsReason(reason)
		params.Reason = &reasonVal
	}

	// TODO: Add these parameters to OpenAPI spec
	// if severity := r.URL.Query().Get("severity"); severity != "" {
	// 	severityVal := generated.QuarantineSeverity(severity)
	// 	params.Severity = &severityVal
	// }

	// if campaignId := r.URL.Query().Get("campaign_id"); campaignId != "" {
	// 	params.CampaignId = &campaignId
	// }

	// if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
	// 	params.SortBy = &sortBy
	// }

	// if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
	// 	sortOrderVal := generated.SortOrder(sortOrder)
	// 	params.SortOrder = &sortOrderVal
	// }

	// if since := r.URL.Query().Get("since"); since != "" {
	// 	params.Since = &since
	// }

	// if until := r.URL.Query().Get("until"); until != "" {
	// 	params.Until = &until
	// }

	// if includeContent := r.URL.Query().Get("include_content"); includeContent != "" {
	// 	if include, err := strconv.ParseBool(includeContent); err == nil {
	// 		params.IncludeContent = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.ListQuarantinedCorpus(w, r, params)
}

// HandleGetCorpusEntry handles GET /api/v1/corpus/{id}
func (h *Handlers) HandleGetCorpusEntry(w http.ResponseWriter, r *http.Request) {
	entryId := h.extractCorpusEntryID(r)

	// Parse query parameters
	params := generated.GetCorpusEntryParams{}

	// TODO: Add these parameters to OpenAPI spec
	// if includeContent := r.URL.Query().Get("include_content"); includeContent != "" {
	// 	if include, err := strconv.ParseBool(includeContent); err == nil {
	// 		params.IncludeContent = &include
	// 	}
	// }

	// if includeMetadata := r.URL.Query().Get("include_metadata"); includeMetadata != "" {
	// 	if include, err := strconv.ParseBool(includeMetadata); err == nil {
	// 		params.IncludeMetadata = &include
	// 	}
	// }

	// if includeStats := r.URL.Query().Get("include_stats"); includeStats != "" {
	// 	if include, err := strconv.ParseBool(includeStats); err == nil {
	// 		params.IncludeStats = &include
	// 	}
	// }

	// Delegate to adapter
	h.adapter.GetCorpusEntry(w, r, entryId, params)
}

// HandleDeleteCorpusEntry handles DELETE /api/v1/corpus/{id}
func (h *Handlers) HandleDeleteCorpusEntry(w http.ResponseWriter, r *http.Request) {
	entryId := h.extractCorpusEntryID(r)
	h.adapter.DeleteCorpusEntry(w, r, entryId)
}

// HandleDownloadCorpusFile handles GET /api/v1/corpus/{id}/download
func (h *Handlers) HandleDownloadCorpusFile(w http.ResponseWriter, r *http.Request) {
	entryId := h.extractCorpusEntryID(r)
	h.adapter.DownloadCorpusFile(w, r, entryId)
}

// HandlePromoteCrashToCorpus handles POST /api/v1/corpus/promote
func (h *Handlers) HandlePromoteCrashToCorpus(w http.ResponseWriter, r *http.Request) {
	h.adapter.PromoteCrashToCorpus(w, r)
}

// extractCollectionIDAsUUID extracts collection ID from URL parameters as UUID
func (h *Handlers) extractCollectionIDAsUUID(r *http.Request) openapi_types.UUID {
	idStr := chi.URLParam(r, "collectionId")
	parsedUUID, _ := uuid.Parse(idStr)
	return openapi_types.UUID(parsedUUID)
}

// HandleListCorpusCollections handles GET /api/v1/corpus/collections
func (h *Handlers) HandleListCorpusCollections(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	params := generated.ListCorpusCollectionsParams{}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			limitVal := limit
			params.Limit = &limitVal
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			offsetVal := offset
			params.Offset = &offsetVal
		}
	}

	h.adapter.ListCorpusCollections(w, r, params)
}

// HandleCreateCorpusCollection handles POST /api/v1/corpus/collections
func (h *Handlers) HandleCreateCorpusCollection(w http.ResponseWriter, r *http.Request) {
	h.adapter.CreateCorpusCollection(w, r)
}

// HandleGetCorpusCollection handles GET /api/v1/corpus/collections/{collectionId}
func (h *Handlers) HandleGetCorpusCollection(w http.ResponseWriter, r *http.Request) {
	collectionId := h.extractCollectionIDAsUUID(r)
	h.adapter.GetCorpusCollection(w, r, collectionId)
}

// HandleUpdateCorpusCollection handles PUT /api/v1/corpus/collections/{collectionId}
func (h *Handlers) HandleUpdateCorpusCollection(w http.ResponseWriter, r *http.Request) {
	collectionId := h.extractCollectionIDAsUUID(r)
	h.adapter.UpdateCorpusCollection(w, r, collectionId)
}

// HandleDeleteCorpusCollection handles DELETE /api/v1/corpus/collections/{collectionId}
func (h *Handlers) HandleDeleteCorpusCollection(w http.ResponseWriter, r *http.Request) {
	collectionId := h.extractCollectionIDAsUUID(r)
	h.adapter.DeleteCorpusCollection(w, r, collectionId)
}

// HandleUploadCorpusCollectionFiles handles POST /api/v1/corpus/collections/{collectionId}/upload
func (h *Handlers) HandleUploadCorpusCollectionFiles(w http.ResponseWriter, r *http.Request) {
	collectionId := h.extractCollectionIDAsUUID(r)
	h.adapter.UploadCorpusCollectionFiles(w, r, collectionId)
}

// HandleListCorpusCollectionFiles handles GET /api/v1/corpus/collections/{collectionId}/files
func (h *Handlers) HandleListCorpusCollectionFiles(w http.ResponseWriter, r *http.Request) {
	collectionId := h.extractCollectionIDAsUUID(r)
	h.adapter.ListCorpusCollectionFiles(w, r, collectionId)
}
