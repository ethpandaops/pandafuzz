package master

import (
	"net/http"
	"strconv"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// AvailableCorpusResponse represents an available corpus for job creation
type AvailableCorpusResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "campaign" or "standalone"
	FileCount   int    `json:"file_count"`
	TotalSize   int64  `json:"total_size"`
}

// handleListAvailableCorpora lists all available corpora (campaigns and standalone) for job creation
func (s *Server) handleListAvailableCorpora(w http.ResponseWriter, r *http.Request) {
	// Get query parameters
	limit := 100
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	availableCorpora := make([]AvailableCorpusResponse, 0)

	// Get all campaigns (both regular and corpus-only)
	var allCampaigns []*common.Campaign
	var err error

	if s.services != nil && s.services.Campaign != nil {
		// Get all campaigns
		allCampaigns, err = s.services.Campaign.List(r.Context(), common.CampaignFilters{
			Limit:  limit * 2, // Get more since we'll filter
			Offset: 0,
		})
	} else {
		// Fallback to direct storage
		if storage, ok := s.state.db.(common.Storage); ok {
			allCampaigns, err = storage.ListCampaigns(r.Context(), limit*2, 0, "")
		} else {
			s.logger.Warn("Storage interface not available for listing campaigns")
			allCampaigns = []*common.Campaign{}
		}
	}

	if err != nil {
		s.logger.WithError(err).Warn("Failed to list campaigns for corpus selection")
		// Continue with empty list
		allCampaigns = []*common.Campaign{}
	}

	// Process campaigns
	for _, campaign := range allCampaigns {
		// Skip campaigns that don't share corpus
		if !campaign.SharedCorpus && campaign.Status != common.CampaignStatusCorpusOnly {
			continue
		}

		corpusType := "campaign"
		if campaign.Status == common.CampaignStatusCorpusOnly {
			corpusType = "standalone"
		}

		// Get corpus statistics
		var fileCount int
		var totalSize int64

		if storage, ok := s.state.db.(common.Storage); ok {
			if files, err := storage.GetCorpusFiles(r.Context(), campaign.ID); err == nil {
				fileCount = len(files)
				for _, f := range files {
					totalSize += f.Size
				}
			}
		}

		availableCorpora = append(availableCorpora, AvailableCorpusResponse{
			ID:          campaign.ID,
			Name:        campaign.Name,
			Description: campaign.Description,
			Type:        corpusType,
			FileCount:   fileCount,
			TotalSize:   totalSize,
		})
	}

	// Apply pagination
	total := len(availableCorpora)
	if offset >= len(availableCorpora) {
		availableCorpora = []AvailableCorpusResponse{}
	} else if offset+limit > len(availableCorpora) {
		availableCorpora = availableCorpora[offset:]
	} else {
		availableCorpora = availableCorpora[offset : offset+limit]
	}

	s.logger.WithFields(logrus.Fields{
		"total_available": total,
		"returned":        len(availableCorpora),
		"limit":           limit,
		"offset":          offset,
	}).Debug("Listed available corpora for job creation")

	response := map[string]any{
		"corpora": availableCorpora,
		"count":   len(availableCorpora),
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	}

	s.writeJSONResponse(w, response)
}
