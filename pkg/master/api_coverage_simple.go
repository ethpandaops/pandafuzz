package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// CoverageReportRequest represents a coverage report submission from a bot
type CoverageReportRequest struct {
	JobID            string                 `json:"job_id"`
	BotID            string                 `json:"bot_id"`
	ReportID         string                 `json:"report_id"`
	Format           string                 `json:"format"`
	CoverageData     map[string]interface{} `json:"coverage_data"`
	LineCoverage     float64                `json:"line_coverage"`
	FunctionCoverage float64                `json:"function_coverage"`
	BranchCoverage   float64                `json:"branch_coverage"`
	CollectedAt      time.Time              `json:"collected_at"`
	StoragePath      string                 `json:"storage_path"`
	Size             int64                  `json:"size"`
}

// CoverageReportResponse represents a coverage report in responses
type CoverageReportResponse struct {
	JobID            string    `json:"job_id"`
	ReportID         string    `json:"report_id"`
	Format           string    `json:"format"`
	LineCoverage     float64   `json:"line_coverage"`
	FunctionCoverage float64   `json:"function_coverage"`
	BranchCoverage   float64   `json:"branch_coverage"`
	CoveragePercent  float64   `json:"coverage_percent"`
	CollectedAt      time.Time `json:"collected_at"`
	HasData          bool      `json:"has_data"`
}

// handleSubmitCoverageReport handles detailed coverage report submission
func (s *Server) handleSubmitCoverageReport(w http.ResponseWriter, r *http.Request) {
	var req CoverageReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid coverage report", err)
		return
	}

	// Validate required fields
	if req.JobID == "" || req.BotID == "" || req.ReportID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID, Bot ID, and Report ID are required", nil)
		return
	}

	// Set default format if not provided
	if req.Format == "" {
		req.Format = "json"
	}

	// Set collection time if not provided
	if req.CollectedAt.IsZero() {
		req.CollectedAt = time.Now()
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Extract metrics from coverage_data
	edges := int64(0)
	newEdges := int64(0)
	execCount := int64(0)
	coveragePercent := 0.0
	totalEdges := int64(0)
	pathsTotal := int64(0)
	pathsPending := int64(0)
	pathsFavored := int64(0)

	if req.CoverageData != nil {
		// Extract edge metrics
		if val, ok := req.CoverageData["edges"].(float64); ok {
			edges = int64(val)
		}
		if val, ok := req.CoverageData["new_edges"].(float64); ok {
			newEdges = int64(val)
		}
		if val, ok := req.CoverageData["total_edges"].(float64); ok {
			totalEdges = int64(val)
		}
		if val, ok := req.CoverageData["total_executions"].(float64); ok {
			execCount = int64(val)
		}
		if val, ok := req.CoverageData["coverage_percent"].(float64); ok {
			coveragePercent = val
		}

		// Extract path metrics
		if val, ok := req.CoverageData["paths_total"].(float64); ok {
			pathsTotal = int64(val)
		}
		if val, ok := req.CoverageData["paths_pending"].(float64); ok {
			pathsPending = int64(val)
		}
		if val, ok := req.CoverageData["paths_favored"].(float64); ok {
			pathsFavored = int64(val)
		}

		// Extract line/function/branch coverage if provided in coverage_data
		if val, ok := req.CoverageData["line_coverage"].(float64); ok && req.LineCoverage == 0 {
			req.LineCoverage = val
		}
		if val, ok := req.CoverageData["function_coverage"].(float64); ok && req.FunctionCoverage == 0 {
			req.FunctionCoverage = val
		}
		if val, ok := req.CoverageData["branch_coverage"].(float64); ok && req.BranchCoverage == 0 {
			req.BranchCoverage = val
		}
	}

	// Store in coverage table (main metrics)
	coverageRecord := map[string]interface{}{
		"id":         req.ReportID,
		"job_id":     req.JobID,
		"bot_id":     req.BotID,
		"edges":      edges,
		"new_edges":  newEdges,
		"timestamp":  req.CollectedAt.Unix(),
		"exec_count": execCount,
	}

	// Store the coverage record
	coverageKey := fmt.Sprintf("coverage:%s", req.ReportID)
	if err := s.state.db.Store(ctx, coverageKey, coverageRecord); err != nil {
		s.logger.WithError(err).Error("Failed to store coverage record")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to store coverage record", err)
		return
	}

	// Store in coverage_reports table (file metadata)
	if req.StoragePath != "" || req.Size > 0 {
		reportRecord := map[string]interface{}{
			"id":           fmt.Sprintf("%s-%s-%d", req.ReportID, req.Format, time.Now().Unix()),
			"job_id":       req.JobID,
			"format":       req.Format,
			"storage_path": req.StoragePath,
			"size":         req.Size,
			"created_at":   req.CollectedAt,
		}

		reportKey := fmt.Sprintf("coverage:report:%s", reportRecord["id"])
		if err := s.state.db.Store(ctx, reportKey, reportRecord); err != nil {
			s.logger.WithError(err).Warn("Failed to store coverage report record")
		}
	}

	// Store in coverage_metadata table (detailed metrics)
	metadataRecord := map[string]interface{}{
		"id":                fmt.Sprintf("meta-%s-%d", req.ReportID, time.Now().Unix()),
		"job_id":            req.JobID,
		"report_id":         req.ReportID,
		"line_coverage":     req.LineCoverage,
		"function_coverage": req.FunctionCoverage,
		"branch_coverage":   req.BranchCoverage,
		"total_lines":       0, // Would need to be extracted from coverage_data if available
		"covered_lines":     0, // Would need to be extracted from coverage_data if available
		"total_functions":   0,
		"covered_functions": 0,
		"collected_at":      req.CollectedAt,
		"coverage_percent":  coveragePercent,
		"edges":             edges,
		"total_edges":       totalEdges,
		"paths_total":       pathsTotal,
		"paths_pending":     pathsPending,
		"paths_favored":     pathsFavored,
	}

	// Extract additional metadata if available
	if req.CoverageData != nil {
		if val, ok := req.CoverageData["total_lines"].(float64); ok {
			metadataRecord["total_lines"] = int64(val)
		}
		if val, ok := req.CoverageData["covered_lines"].(float64); ok {
			metadataRecord["covered_lines"] = int64(val)
		}
		if val, ok := req.CoverageData["total_functions"].(float64); ok {
			metadataRecord["total_functions"] = int64(val)
		}
		if val, ok := req.CoverageData["covered_functions"].(float64); ok {
			metadataRecord["covered_functions"] = int64(val)
		}
	}

	metadataKey := fmt.Sprintf("coverage:metadata:%s", metadataRecord["id"])
	if err := s.state.db.Store(ctx, metadataKey, metadataRecord); err != nil {
		s.logger.WithError(err).Warn("Failed to store coverage metadata")
	}

	// Store a reference from job to coverage report
	jobCoverageKey := fmt.Sprintf("job:coverage:%s:%s", req.JobID, req.ReportID)
	if err := s.state.db.Store(ctx, jobCoverageKey, req.ReportID); err != nil {
		s.logger.WithError(err).Warn("Failed to store job coverage reference")
	}

	// Generate coverage files with actual data
	if err := s.generateCoverageFiles(ctx, req.JobID, req.CoverageData); err != nil {
		s.logger.WithError(err).Warn("Failed to generate coverage files")
	}

	// Update coverage stats counter
	s.state.stats.CoverageReports++

	s.logger.WithFields(logrus.Fields{
		"report_id":         req.ReportID,
		"job_id":            req.JobID,
		"bot_id":            req.BotID,
		"edges":             edges,
		"coverage_percent":  coveragePercent,
		"line_coverage":     req.LineCoverage,
		"function_coverage": req.FunctionCoverage,
		"branch_coverage":   req.BranchCoverage,
	}).Info("Coverage report submitted successfully")

	// Return success response with coverage metrics
	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, map[string]interface{}{
		"status":           "success",
		"message":          "Coverage report processed",
		"coverage_id":      req.ReportID,
		"edges":            edges,
		"coverage_percent": coveragePercent,
		"timestamp":        time.Now(),
	})
}

// generateCoverageFiles creates coverage files on disk with actual data
func (s *Server) generateCoverageFiles(ctx context.Context, jobID string, coverageData map[string]interface{}) error {
	// Create coverage directory
	coverageDir := filepath.Join("/app/data/coverage", jobID)
	if err := os.MkdirAll(coverageDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory: %w", err)
	}

	timestamp := time.Now().Unix()

	// Generate JSON file with actual coverage data
	jsonPath := filepath.Join(coverageDir, fmt.Sprintf("coverage-%d.json", timestamp))
	jsonData, err := json.MarshalIndent(coverageData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal coverage data: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	// Generate LCOV file if applicable (basic format)
	lcovPath := filepath.Join(coverageDir, fmt.Sprintf("coverage-%d.lcov", timestamp))
	lcovContent := s.generateLCOVFromMetrics(coverageData)
	if err := os.WriteFile(lcovPath, []byte(lcovContent), 0644); err != nil {
		s.logger.WithError(err).Warn("Failed to write LCOV file")
	}

	s.logger.WithFields(logrus.Fields{
		"job_id":    jobID,
		"json_file": jsonPath,
		"lcov_file": lcovPath,
	}).Debug("Generated coverage files")

	return nil
}

// generateLCOVFromMetrics creates basic LCOV content from coverage metrics
func (s *Server) generateLCOVFromMetrics(coverageData map[string]interface{}) string {
	// Generate basic LCOV format from the metrics
	lcov := "TN:PandaFuzz Coverage Report\n"
	lcov += "SF:fuzzer_target\n"

	// Add line coverage if available
	if edges, ok := coverageData["edges"].(float64); ok && edges > 0 {
		for i := 1; i <= int(edges); i++ {
			lcov += fmt.Sprintf("DA:%d,1\n", i)
		}
		lcov += fmt.Sprintf("LF:%d\n", int(edges))
		lcov += fmt.Sprintf("LH:%d\n", int(edges))
	}

	// Add function coverage if available
	if funcCov, ok := coverageData["function_coverage"].(float64); ok && funcCov > 0 {
		funcCount := int(funcCov * 100 / 100) // Estimate function count
		if funcCount > 0 {
			for i := 1; i <= funcCount; i++ {
				lcov += fmt.Sprintf("FN:%d,func_%d\n", i*10, i)
			}
			lcov += fmt.Sprintf("FNF:%d\n", funcCount)
			lcov += fmt.Sprintf("FNH:%d\n", funcCount)
		}
	}

	lcov += "end_of_record\n"
	return lcov
}

// handleGetCoverageReport retrieves coverage reports for a job
func (s *Server) handleGetCoverageReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
	defer cancel()

	// Retrieve all coverage reports for this job
	reports := []CoverageReportResponse{}

	// Check for recent coverage reports (last 24 hours)
	now := time.Now().Unix()
	for ts := now; ts > now-86400 && len(reports) < 100; ts-- {
		// Try various report ID patterns
		for _, pattern := range []string{
			fmt.Sprintf("coverage_%s_%d", jobID, ts),
			fmt.Sprintf("coverage-%s-%d", jobID, ts),
		} {
			jobCoverageKey := fmt.Sprintf("job:coverage:%s:%s", jobID, pattern)

			var storedReportID string
			if err := s.state.db.Get(ctx, jobCoverageKey, &storedReportID); err == nil && storedReportID != "" {
				// Retrieve the actual coverage record
				coverageKey := fmt.Sprintf("coverage:%s", storedReportID)
				var coverageRecord map[string]interface{}
				if err := s.state.db.Get(ctx, coverageKey, &coverageRecord); err == nil {
					// Extract coverage percentage
					reportCoveragePercent := 0.0
					if edges, ok := coverageRecord["edges"].(float64); ok && edges > 0 {
						// Simple calculation if we don't have total edges
						reportCoveragePercent = float64(edges) / 65536.0 * 100.0 // Assuming AFL++ bitmap size
					}

					// Retrieve metadata if available
					metadataKey := fmt.Sprintf("coverage:metadata:meta-%s-*", storedReportID)
					var metadataRecord map[string]interface{}
					s.state.db.Get(ctx, metadataKey, &metadataRecord) // Ignore error

					lineCov := 0.0
					funcCov := 0.0
					branchCov := 0.0
					if metadataRecord != nil {
						if val, ok := metadataRecord["line_coverage"].(float64); ok {
							lineCov = val
						}
						if val, ok := metadataRecord["function_coverage"].(float64); ok {
							funcCov = val
						}
						if val, ok := metadataRecord["branch_coverage"].(float64); ok {
							branchCov = val
						}
						if val, ok := metadataRecord["coverage_percent"].(float64); ok && val > 0 {
							reportCoveragePercent = val
						}
					}

					reports = append(reports, CoverageReportResponse{
						JobID:            jobID,
						ReportID:         storedReportID,
						Format:           "json",
						LineCoverage:     lineCov,
						FunctionCoverage: funcCov,
						BranchCoverage:   branchCov,
						CoveragePercent:  reportCoveragePercent,
						CollectedAt:      time.Unix(ts, 0),
						HasData:          true,
					})
					break // Found a report for this timestamp
				}
			}
		}
	}

	// Build response
	response := map[string]interface{}{
		"job_id":  jobID,
		"reports": reports,
		"count":   len(reports),
	}

	s.writeJSONResponse(w, response)
}
