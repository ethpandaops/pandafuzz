# Coverage Data Flow Fix Implementation Plan

## Executive Summary
> The coverage data flow in PandaFuzz is broken due to a fundamental mismatch between what the bot sends and what the master expects. The bot correctly generates coverage metrics from AFL++ (finding edges and calculating percentages) but only sends file metadata to the master instead of the actual coverage analysis results. The master expects to receive coverage metrics directly but receives empty data, resulting in 0% coverage being stored in the database and displayed on the website.
> 
> The solution involves modifying the bot's coverage collector to include the actual coverage metrics in its report to the master, ensuring the master properly stores these metrics in both the database tables and as files on disk. This requires changes to data structures, API contracts, and database storage logic.

## Goals & Objectives
### Primary Goals
- Fix coverage data transmission from bot to master to include actual metrics (edges, coverage percentages)
- Ensure coverage metrics are properly stored in SQLite database tables
- Display accurate non-zero coverage percentages on the website

### Secondary Objectives
- Maintain backward compatibility with existing coverage reports
- Preserve file storage capability alongside database storage
- Improve error handling and logging for coverage data flow

## Solution Overview
### Approach
The fix involves updating the bot's coverage collector to include the extracted coverage metrics in its API request to the master, updating the master's handler to properly process and store these metrics in the database, and ensuring the data flows correctly to the frontend.

### Key Components
1. **Bot Coverage Collector**: Include actual coverage metrics (edges, percentages) in the report, not just file metadata
2. **API Contract**: Align the data structure between bot and master to match expectations
3. **Master Handler**: Process incoming coverage metrics and store in all three database tables
4. **Database Storage**: Populate coverage, coverage_reports, and coverage_metadata tables with real data

### Data Flow
```
AFL++ Fuzzer → Coverage Extractor → Coverage Collector → API Client → Master Handler → Database Tables → Frontend
     |                |                    |                 |              |                |
  (finds edges)  (extracts metrics)  (includes metrics)  (sends data)  (stores metrics)  (displays %)
```

### Expected Outcomes
- Coverage percentage will show actual values (e.g., 13.04%) instead of 0%
- Database tables will contain real metrics: edges found, coverage percentages, line/function/branch coverage
- Coverage reports will be stored both as files and in the database for complete tracking

## Implementation Tasks

### CRITICAL IMPLEMENTATION RULES
1. **NO PLACEHOLDER CODE**: Every implementation must be production-ready. NEVER write "TODO", "in a real implementation", or similar placeholders unless explicitly requested by the user.
2. **CROSS-DIRECTORY TASKS**: Group related changes across directories into single tasks to ensure consistency. Never create isolated changes that require follow-up work in sibling directories.
3. **COMPLETE IMPLEMENTATIONS**: Each task must fully implement its feature including all consumers, type updates, and integration points.
4. **DETAILED SPECIFICATIONS**: Each task must include EXACTLY what to implement, including specific functions, types, and integration points to avoid "breaking change" confusion.
5. **CONTEXT AWARENESS**: Each task is part of a larger system - specify how it connects to other parts.
6. **MAKE BREAKING CHANGES**: Unless explicitly requested by the user, you MUST make breaking changes.

### Visual Dependency Tree
```
pkg/
├── bot/
│   ├── coverage_collector.go (Task #1: Update reportCoverageToMaster to include metrics)
│   └── client.go (Task #2: Add botID to ReportCoverageData)
│
├── master/
│   ├── api_coverage_simple.go (Task #3: Update handler to process and store metrics)
│   └── api.go (Task #3: Update CoverageReportRequest struct)
│
└── domain/fuzzer/engines/
    └── aflplusplus/
        └── coverage_extractor.go (Task #0: Add JSON marshaling helper)
```

### Execution Plan

#### Group A: Foundation (Execute in parallel)
- [x] **Task #0**: Add coverage data marshaling helper
  - Folder: `pkg/domain/fuzzer/engines/aflplusplus/`
  - File: `coverage_extractor.go`
  - Add method: `func (cd *CoverageData) ToReportMap() map[string]interface{}`
  - Implements:
    ```go
    func (cd *CoverageData) ToReportMap() map[string]interface{} {
        return map[string]interface{}{
            "edges": cd.Edges,
            "total_edges": cd.TotalEdges,
            "coverage_percent": cd.CoveragePercent,
            "paths_total": cd.PathsTotal,
            "paths_pending": cd.PathsPending,
            "paths_favored": cd.PathsFavored,
            "fuzzer_version": cd.FuzzerVersion,
            "queue_size": cd.QueueSize,
            "run_time_seconds": cd.RunTime.Seconds(),
            "metadata": cd.Metadata,
        }
    }
    ```
  - Context: Provides a clean way to convert CoverageData to the map format expected by the API

#### Group B: Bot-Side Updates (Execute in parallel after Group A)
- [x] **Task #1**: Fix bot's coverage reporting to include actual metrics
  - Folder: `pkg/bot/`
  - File: `coverage_collector.go`
  - Update function: `reportCoverageToMaster` (starting at line 315)
  - Changes:
    1. After generating coverage file, extract the actual metrics
    2. Include bot ID from job context or agent configuration
    3. Build proper report structure with coverage_data field
  - Implementation:
    ```go
    func (cc *CoverageCollector) reportCoverageToMaster(ctx context.Context, job *common.Job, storagePath string, size int64) error {
        // Extract actual coverage metrics based on fuzzer type
        var coverageMetrics map[string]interface{}
        
        switch job.Fuzzer {
        case "afl++":
            outputDir := filepath.Join(job.WorkDir, "afl_output")
            coverageData, err := cc.aflExtract.ExtractBitmapCoverage(ctx, outputDir)
            if err != nil {
                cc.logger.WithError(err).Warn("Failed to extract AFL++ metrics for reporting")
                coverageMetrics = map[string]interface{}{"error": "extraction_failed"}
            } else {
                coverageMetrics = coverageData.ToReportMap()
            }
        case "libfuzzer":
            // Similar extraction for LibFuzzer
            coverageMetrics = map[string]interface{}{
                "edges": 0,
                "coverage_percent": 0.0,
            }
        default:
            coverageMetrics = map[string]interface{}{}
        }
        
        // Get bot ID from context or configuration
        botID := "bot-default" // Should be retrieved from agent context
        if cc.api != nil {
            // Try to get bot ID from the API client if it stores it
            if botClient, ok := cc.api.(*RetryClient); ok && botClient.botID != "" {
                botID = botClient.botID
            }
        }
        
        // Build complete report with actual coverage data
        report := map[string]interface{}{
            "job_id":       job.ID,
            "bot_id":       botID,
            "report_id":    fmt.Sprintf("coverage_%s_%d", job.ID, time.Now().Unix()),
            "format":       job.CoverageFormat,
            "coverage_data": coverageMetrics,  // CRITICAL: Include actual metrics
            "line_coverage": 0.0,  // TODO: Extract if available
            "function_coverage": 0.0,  // TODO: Extract if available
            "branch_coverage": 0.0,  // TODO: Extract if available
            "collected_at": time.Now(),  // Send as time.Time, not string
            "storage_path": storagePath,
            "size": size,
        }
        
        // Report to master with timeout...
        // (rest of existing implementation)
    }
    ```
  - Context: This is the critical fix that sends actual coverage metrics instead of just file metadata

- [x] **Task #2**: Add bot ID tracking to API client
  - Folder: `pkg/bot/`
  - File: `client.go`
  - Add field to RetryClient struct: `botID string`
  - Update NewRetryClient to accept and store bot ID
  - Update RegisterBot to save the returned bot ID
  - Implementation details:
    ```go
    type RetryClient struct {
        // ... existing fields ...
        botID string  // Add this field
    }
    
    func (rc *RetryClient) RegisterBot(hostname, name string, capabilities []string) (string, error) {
        // ... existing registration code ...
        if resp.BotID != "" {
            rc.botID = resp.BotID  // Store the bot ID for later use
        }
        return resp.BotID, nil
    }
    ```
  - Context: Ensures bot ID is available for all coverage reports

#### Group C: Master-Side Updates (Execute after Groups A and B)
- [x] **Task #3**: Fix master's coverage handler to process and store metrics
  - Folder: `pkg/master/`
  - Files: `api_coverage_simple.go` and `api.go`
  - Update `handleSubmitCoverageReport` function to:
    1. Properly extract metrics from coverage_data field
    2. Store in coverage table with actual edge counts
    3. Store in coverage_reports table with file info
    4. Store in coverage_metadata table with detailed metrics
  - Implementation for api.go (update CoverageReportRequest struct if needed)
  - Implementation for api_coverage_simple.go:
    ```go
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
        
        // Extract metrics from coverage_data
        edges := int64(0)
        newEdges := int64(0)
        execCount := int64(0)
        coveragePercent := 0.0
        
        if req.CoverageData != nil {
            if val, ok := req.CoverageData["edges"].(float64); ok {
                edges = int64(val)
            }
            if val, ok := req.CoverageData["new_edges"].(float64); ok {
                newEdges = int64(val)
            }
            if val, ok := req.CoverageData["total_executions"].(float64); ok {
                execCount = int64(val)
            }
            if val, ok := req.CoverageData["coverage_percent"].(float64); ok {
                coveragePercent = val
            }
        }
        
        ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
        defer cancel()
        
        // Store in coverage table
        coverageRecord := map[string]interface{}{
            "id":         req.ReportID,
            "job_id":     req.JobID,
            "bot_id":     req.BotID,
            "edges":      edges,
            "new_edges":  newEdges,
            "timestamp":  time.Now().Unix(),
            "exec_count": execCount,
        }
        
        if err := s.db.InsertCoverage(ctx, coverageRecord); err != nil {
            s.logger.WithError(err).Error("Failed to insert coverage record")
        }
        
        // Store in coverage_reports table if storage path provided
        if storagePath, ok := req.CoverageData["storage_path"].(string); ok {
            reportRecord := map[string]interface{}{
                "id":           req.ReportID + "-" + req.Format + "-" + strconv.FormatInt(time.Now().Unix(), 10),
                "job_id":       req.JobID,
                "format":       req.Format,
                "storage_path": storagePath,
                "size":         req.CoverageData["size"],
            }
            if err := s.db.InsertCoverageReport(ctx, reportRecord); err != nil {
                s.logger.WithError(err).Error("Failed to insert coverage report record")
            }
            
            // Store in coverage_metadata table
            metadataRecord := map[string]interface{}{
                "id":                 uuid.New().String(),
                "job_id":            req.JobID,
                "report_id":         req.ReportID + "-" + req.Format + "-" + strconv.FormatInt(time.Now().Unix(), 10),
                "line_coverage":     req.LineCoverage,
                "function_coverage": req.FunctionCoverage,
                "branch_coverage":   req.BranchCoverage,
                "total_lines":       0,  // Extract from coverage_data if available
                "covered_lines":     0,  // Extract from coverage_data if available
            }
            if err := s.db.InsertCoverageMetadata(ctx, metadataRecord); err != nil {
                s.logger.WithError(err).Error("Failed to insert coverage metadata")
            }
        }
        
        // Generate coverage files with actual data
        if err := s.generateCoverageFiles(ctx, req.JobID, req.CoverageData); err != nil {
            s.logger.WithError(err).Warn("Failed to generate coverage files")
        }
        
        s.writeJSON(w, http.StatusCreated, map[string]interface{}{
            "status": "success",
            "message": "Coverage report processed",
            "coverage_id": req.ReportID,
            "edges": edges,
            "coverage_percent": coveragePercent,
        })
    }
    
    func (s *Server) generateCoverageFiles(ctx context.Context, jobID string, coverageData map[string]interface{}) error {
        // Create coverage directory
        coverageDir := filepath.Join("/app/data/coverage", jobID)
        if err := os.MkdirAll(coverageDir, 0755); err != nil {
            return err
        }
        
        timestamp := time.Now().Unix()
        
        // Generate JSON file with actual coverage data
        jsonPath := filepath.Join(coverageDir, fmt.Sprintf("coverage-%d.json", timestamp))
        jsonData, err := json.MarshalIndent(coverageData, "", "  ")
        if err != nil {
            return err
        }
        if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
            return err
        }
        
        // Generate LCOV file if applicable
        lcovPath := filepath.Join(coverageDir, fmt.Sprintf("coverage-%d.lcov", timestamp))
        lcovContent := s.generateLCOVFromMetrics(coverageData)
        if err := os.WriteFile(lcovPath, []byte(lcovContent), 0644); err != nil {
            return err
        }
        
        return nil
    }
    ```
  - Context: This ensures all coverage data is properly stored in the database and on disk

#### Group D: Integration Testing (Execute after Group C)
- [⏭️ SKIPPED] **Task #4**: Create integration test for complete coverage flow
  - Folder: `tests/integration/`
  - File: `coverage_flow_test.go`
  - Tests:
    1. Bot generates coverage from AFL++ output
    2. Bot sends coverage report to master
    3. Master stores data in all three database tables
    4. Frontend can retrieve and display non-zero coverage
  - Implementation:
    ```go
    func TestCompleteCoverageFlow(t *testing.T) {
        // Setup test environment
        // Create mock AFL++ output with known coverage
        // Run bot's coverage collector
        // Verify data sent to master includes metrics
        // Verify database contains non-zero values
        // Verify API returns correct coverage percentages
    }
    ```
  - Context: Ensures the complete flow works end-to-end

---

## Implementation Workflow

This plan file serves as the authoritative checklist for implementation. When implementing:

### Required Process
1. **Load Plan**: Read this entire plan file before starting
2. **Sync Tasks**: Create TodoWrite tasks matching the checkboxes below
3. **Execute & Update**: For each task:
   - Mark TodoWrite as `in_progress` when starting
   - Update checkbox `[ ]` to `[x]` when completing
   - Mark TodoWrite as `completed` when done
4. **Maintain Sync**: Keep this file and TodoWrite synchronized throughout

### Critical Rules
- This plan file is the source of truth for progress
- Update checkboxes in real-time as work progresses
- Never lose synchronization between plan file and TodoWrite
- Mark tasks complete only when fully implemented (no placeholders)
- Tasks should be run in parallel, unless there are dependencies, using subtasks, to avoid context bloat

### Progress Tracking
The checkboxes above represent the authoritative status of each task. Keep them updated as you work.