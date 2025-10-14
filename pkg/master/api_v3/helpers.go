package api_v3

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/google/uuid"
)

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return uuid.New().String()
}

// parsePaginationParams parses pagination parameters from request
func parsePaginationParams(r *http.Request) PaginationParams {
	params := PaginationParams{
		Page:      1,
		Limit:     50,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			params.Page = parsed
		}
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			params.Limit = parsed
		}
	}

	if s := r.URL.Query().Get("sortBy"); s != "" {
		params.SortBy = s
	}

	if o := r.URL.Query().Get("sortOrder"); o != "" {
		if o == "asc" || o == "desc" {
			params.SortOrder = o
		}
	}

	// Calculate offset
	params.Offset = (params.Page - 1) * params.Limit

	return params
}

// parseJobFilters parses job filters from request
func parseJobFilters(r *http.Request) *service.JobFilter {
	filters := &service.JobFilter{}

	if status := r.URL.Query().Get("status"); status != "" {
		jobStatus := common.JobStatus(status)
		filters.Status = &jobStatus
	}

	if fuzzer := r.URL.Query().Get("fuzzer"); fuzzer != "" {
		filters.Fuzzer = &fuzzer
	}

	// TODO: Campaign filtering needs to be handled separately
	// as JobFilter doesn't have CampaignID field

	return filters
}

// parseCampaignFilters parses campaign filters from request
func parseCampaignFilters(r *http.Request) *common.CampaignFilters {
	filters := &common.CampaignFilters{
		Limit:  50,
		Offset: 0,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if tags := r.URL.Query().Get("tags"); tags != "" {
		filters.Tags = strings.Split(tags, ",")
	}

	if binaryHash := r.URL.Query().Get("binaryHash"); binaryHash != "" {
		filters.BinaryHash = binaryHash
	}

	return filters
}

// parseCrashFilters parses crash filters from request

// Bot filtering and pagination

func filterBotsByStatus(bots []*common.Bot, status string) []*common.Bot {
	if status == "" {
		return bots
	}

	var filtered []*common.Bot
	for _, bot := range bots {
		if string(bot.Status) == status {
			filtered = append(filtered, bot)
		}
	}
	return filtered
}

func paginateBots(bots []*common.Bot, page, limit int) []*common.Bot {
	start := (page - 1) * limit
	if start >= len(bots) {
		return []*common.Bot{}
	}

	end := start + limit
	if end > len(bots) {
		end = len(bots)
	}

	return bots[start:end]
}

// Job sorting and pagination

func sortJobs(jobs []*common.Job, sortBy, sortOrder string) {
	sort.Slice(jobs, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "name":
			less = jobs[i].Name < jobs[j].Name
		case "status":
			less = string(jobs[i].Status) < string(jobs[j].Status)
		case "fuzzer":
			less = jobs[i].Fuzzer < jobs[j].Fuzzer
		case "started_at":
			if jobs[i].StartedAt == nil && jobs[j].StartedAt == nil {
				less = false
			} else if jobs[i].StartedAt == nil {
				less = false
			} else if jobs[j].StartedAt == nil {
				less = true
			} else {
				less = jobs[i].StartedAt.Before(*jobs[j].StartedAt)
			}
		case "completed_at":
			if jobs[i].CompletedAt == nil && jobs[j].CompletedAt == nil {
				less = false
			} else if jobs[i].CompletedAt == nil {
				less = false
			} else if jobs[j].CompletedAt == nil {
				less = true
			} else {
				less = jobs[i].CompletedAt.Before(*jobs[j].CompletedAt)
			}
		default: // created_at
			less = jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
		}

		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}

func paginateJobs(jobs []*common.Job, page, limit int) []*common.Job {
	start := (page - 1) * limit
	if start >= len(jobs) {
		return []*common.Job{}
	}

	end := start + limit
	if end > len(jobs) {
		end = len(jobs)
	}

	return jobs[start:end]
}

// Campaign pagination

func paginateCampaigns(campaigns []*common.Campaign, page, limit int) []*common.Campaign {
	start := (page - 1) * limit
	if start >= len(campaigns) {
		return []*common.Campaign{}
	}

	end := start + limit
	if end > len(campaigns) {
		end = len(campaigns)
	}

	return campaigns[start:end]
}

// Crash pagination

func paginateCrashes(crashes []*common.CrashResult, page, limit int) []*common.CrashResult {
	start := (page - 1) * limit
	if start >= len(crashes) {
		return []*common.CrashResult{}
	}

	end := start + limit
	if end > len(crashes) {
		end = len(crashes)
	}

	return crashes[start:end]
}

// Corpus file pagination

func paginateCorpusFiles(files []*common.CorpusFile, page, limit int) []*common.CorpusFile {
	start := (page - 1) * limit
	if start >= len(files) {
		return []*common.CorpusFile{}
	}

	end := start + limit
	if end > len(files) {
		end = len(files)
	}

	return files[start:end]
}
