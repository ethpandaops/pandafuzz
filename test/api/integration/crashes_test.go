package integration

import (
	"bytes"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
)

// TestCrashReporting tests crash reporting functionality
func (s *APIIntegrationTestSuite) TestCrashReporting() {
	// Create a test job first
	jobID := s.createTestJob(generateTestName("crash-job"), nil)

	// Create test crash data
	generator := &TestDataGenerator{}
	crash := generator.GenerateCrash(jobID)

	// Report the crash
	client := s.client.GetClient()
	resp, err := client.ReportCrash(s.ctx, crash)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var reportedCrash generated.Crash
	err = parseJSONResponse(resp, &reportedCrash)
	s.Require().NoError(err)

	s.Equal(jobID, reportedCrash.JobId)
	s.Equal(crash.Type, reportedCrash.Type)
	s.Equal(crash.Severity, reportedCrash.Severity)
	s.NotEmpty(reportedCrash.Hash)
	s.False(reportedCrash.Timestamp.IsZero())
}

// TestCrashGet tests retrieving a specific crash
func (s *APIIntegrationTestSuite) TestCrashGet() {
	// Create and report a test crash
	jobID := s.createTestJob(generateTestName("get-crash-job"), nil)
	crashID := s.reportTestCrash(jobID)

	// Get the crash
	client := s.client.GetClient()
	resp, err := client.GetCrash(s.ctx, crashID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crash generated.Crash
	err = parseJSONResponse(resp, &crash)
	s.Require().NoError(err)

	s.Equal(crashID, crash.Id)
	s.Equal(jobID, crash.JobId)
	s.NotEmpty(crash.Hash)
	s.NotEmpty(crash.Type)
	s.NotEmpty(crash.Severity)
}

// TestCrashGetNotFound tests retrieving a non-existent crash
func (s *APIIntegrationTestSuite) TestCrashGetNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.GetCrash(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)
}

// TestCrashList tests listing crashes
func (s *APIIntegrationTestSuite) TestCrashList() {
	// Create test job and report multiple crashes
	jobID := s.createTestJob(generateTestName("list-crash-job"), nil)
	crashCount := 3
	for i := 0; i < crashCount; i++ {
		s.reportTestCrash(jobID)
	}

	// List all crashes
	client := s.client.GetClient()
	params := &generated.ListCrashesParams{}
	resp, err := client.ListCrashes(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crashList generated.CrashListResponse
	err = parseJSONResponse(resp, &crashList)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(crashList.Crashes), crashCount)

	// Validate pagination
	validator := &ResponseValidator{}
	err = validator.ValidatePagination(crashList.Pagination, -1)
	s.Require().NoError(err)
}

// TestCrashListPagination tests crash list pagination
func (s *APIIntegrationTestSuite) TestCrashListPagination() {
	// Create test job and report several crashes
	jobID := s.createTestJob(generateTestName("page-crash-job"), nil)
	crashCount := 5
	for i := 0; i < crashCount; i++ {
		s.reportTestCrash(jobID)
	}

	// Test pagination with limit
	limit := 2
	client := s.client.GetClient()
	params := &generated.ListCrashesParams{
		Limit: &limit,
	}

	resp, err := client.ListCrashes(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crashList generated.CrashListResponse
	err = parseJSONResponse(resp, &crashList)
	s.Require().NoError(err)

	s.LessOrEqual(len(crashList.Crashes), limit)
	s.NotNil(crashList.Pagination)
	s.Equal(limit, crashList.Pagination.Limit)
}

// TestCrashListFiltering tests crash list filtering
func (s *APIIntegrationTestSuite) TestCrashListFiltering() {
	// Create test job and report crashes with different severities
	jobID := s.createTestJob(generateTestName("filter-crash-job"), nil)

	// Report a high severity crash
	generator := &TestDataGenerator{}
	highSeverityCrash := generator.GenerateCrash(jobID)
	highSeverityCrash.Severity = generated.CrashSeverityHigh

	client := s.client.GetClient()
	resp, err := client.ReportCrash(s.ctx, highSeverityCrash)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Test filtering by severity
	severity := generated.CrashSeverityHigh
	params := &generated.ListCrashesParams{
		Severity: &severity,
	}

	resp, err = client.ListCrashes(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crashList generated.CrashListResponse
	err = parseJSONResponse(resp, &crashList)
	s.Require().NoError(err)

	// All returned crashes should have high severity
	for _, crash := range crashList.Crashes {
		s.Equal(generated.CrashSeverityHigh, crash.Severity)
	}
}

// TestCrashFilterByJob tests filtering crashes by job
func (s *APIIntegrationTestSuite) TestCrashFilterByJob() {
	// Create two test jobs
	jobID1 := s.createTestJob(generateTestName("crash-job-1"), nil)
	jobID2 := s.createTestJob(generateTestName("crash-job-2"), nil)

	// Report crashes for both jobs
	s.reportTestCrash(jobID1)
	s.reportTestCrash(jobID2)

	// Filter crashes by job
	client := s.client.GetClient()
	params := &generated.ListCrashesParams{
		JobId: &jobID1,
	}

	resp, err := client.ListCrashes(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crashList generated.CrashListResponse
	err = parseJSONResponse(resp, &crashList)
	s.Require().NoError(err)

	// All returned crashes should belong to job1
	for _, crash := range crashList.Crashes {
		s.Equal(jobID1, crash.JobId)
	}
}

// TestCrashUpdate tests updating crash information
func (s *APIIntegrationTestSuite) TestCrashUpdate() {
	// Create and report a test crash
	jobID := s.createTestJob(generateTestName("update-crash-job"), nil)
	crashID := s.reportTestCrash(jobID)

	// Update the crash
	newSeverity := generated.CrashSeverityMedium
	reproInfo := &generated.CrashReproductionInfo{
		Reproducible:    true,
		ReproAttempts:   5,
		SuccessfulRepro: 4,
		LastReproduced:  &[]time.Time{time.Now()}[0],
	}

	updateReq := generated.UpdateCrashRequest{
		Severity:         &newSeverity,
		ReproductionInfo: reproInfo,
	}

	client := s.client.GetClient()
	resp, err := client.UpdateCrash(s.ctx, crashID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var crash generated.Crash
	err = parseJSONResponse(resp, &crash)
	s.Require().NoError(err)

	s.Equal(crashID, crash.Id)
	s.Equal(newSeverity, crash.Severity)
	s.NotNil(crash.ReproductionInfo)
	s.Equal(reproInfo.Reproducible, crash.ReproductionInfo.Reproducible)
	s.Equal(reproInfo.ReproAttempts, crash.ReproductionInfo.ReproAttempts)
}

// TestCrashUpdateNotFound tests updating a non-existent crash
func (s *APIIntegrationTestSuite) TestCrashUpdateNotFound() {
	nonExistentID := generateTestUUID()

	newSeverity := generated.CrashSeverityLow
	updateReq := generated.UpdateCrashRequest{
		Severity: &newSeverity,
	}

	client := s.client.GetClient()
	resp, err := client.UpdateCrash(s.ctx, nonExistentID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestCrashDelete tests crash deletion
func (s *APIIntegrationTestSuite) TestCrashDelete() {
	// Create and report a test crash
	jobID := s.createTestJob(generateTestName("delete-crash-job"), nil)
	crashID := s.reportTestCrash(jobID)

	// Verify crash exists
	client := s.client.GetClient()
	resp, err := client.GetCrash(s.ctx, crashID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete the crash
	resp, err = client.DeleteCrash(s.ctx, crashID)
	s.Require().NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify crash is deleted
	resp, err = client.GetCrash(s.ctx, crashID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCrashDeleteNotFound tests deleting a non-existent crash
func (s *APIIntegrationTestSuite) TestCrashDeleteNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.DeleteCrash(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCrashDeduplication tests crash deduplication functionality
func (s *APIIntegrationTestSuite) TestCrashDeduplication() {
	// Create test job
	jobID := s.createTestJob(generateTestName("dedup-crash-job"), nil)

	// Create multiple crashes with similar characteristics
	generator := &TestDataGenerator{}

	// Report first crash
	crash1 := generator.GenerateCrash(jobID)
	crash1.Hash = "duplicate-crash-hash"

	client := s.client.GetClient()
	resp, err := client.ReportCrash(s.ctx, crash1)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var reportedCrash1 generated.Crash
	err = parseJSONResponse(resp, &reportedCrash1)
	s.Require().NoError(err)
	crashID1 := reportedCrash1.Id

	// Request deduplication for this crash
	dedupReq := generated.DeduplicateCrashRequest{
		SimilarityThreshold: 0.8,
		MaxResults:          10,
	}

	resp, err = client.DeduplicateCrash(s.ctx, crashID1, dedupReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var dedupResp generated.CrashDeduplicationResponse
	err = parseJSONResponse(resp, &dedupResp)
	s.Require().NoError(err)

	s.Equal(crashID1, dedupResp.CrashId)
	s.NotNil(dedupResp.SimilarCrashes)
	s.GreaterOrEqual(len(dedupResp.SimilarCrashes), 0)
}

// TestCrashDeduplicationNotFound tests deduplication for non-existent crash
func (s *APIIntegrationTestSuite) TestCrashDeduplicationNotFound() {
	nonExistentID := generateTestUUID()

	dedupReq := generated.DeduplicateCrashRequest{
		SimilarityThreshold: 0.8,
		MaxResults:          10,
	}

	client := s.client.GetClient()
	resp, err := client.DeduplicateCrash(s.ctx, nonExistentID, dedupReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCrashMinimization tests crash minimization functionality
func (s *APIIntegrationTestSuite) TestCrashMinimization() {
	// Create and report a test crash
	jobID := s.createTestJob(generateTestName("minimize-crash-job"), nil)
	crashID := s.reportTestCrash(jobID)

	// Request crash minimization
	minimizeReq := generated.MinimizeCrashRequest{
		Strategy:    "binary_search",
		MaxAttempts: 10,
		Timeout:     300,
	}

	client := s.client.GetClient()
	resp, err := client.MinimizeCrash(s.ctx, crashID, minimizeReq)
	s.Require().NoError(err)

	// Minimization is typically async, so accept either immediate result or async response
	if resp.StatusCode == http.StatusOK {
		// Immediate result
		var minimizedCrash generated.Crash
		err = parseJSONResponse(resp, &minimizedCrash)
		s.Require().NoError(err)
		s.Equal(crashID, minimizedCrash.Id)
	} else if resp.StatusCode == http.StatusAccepted {
		// Async operation started
		var asyncResp generated.MinimizeCrash202Response
		err = parseJSONResponse(resp, &asyncResp)
		s.Require().NoError(err)
		s.NotEmpty(asyncResp.TaskId)
		s.T().Logf("Async minimization started with task ID: %s", asyncResp.TaskId)
	}
}

// TestCrashMinimizationNotFound tests minimization for non-existent crash
func (s *APIIntegrationTestSuite) TestCrashMinimizationNotFound() {
	nonExistentID := generateTestUUID()

	minimizeReq := generated.MinimizeCrashRequest{
		Strategy:    "binary_search",
		MaxAttempts: 5,
		Timeout:     300,
	}

	client := s.client.GetClient()
	resp, err := client.MinimizeCrash(s.ctx, nonExistentID, minimizeReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCrashReproduction tests crash reproduction functionality
func (s *APIIntegrationTestSuite) TestCrashReproduction() {
	// Create and report a test crash
	jobID := s.createTestJob(generateTestName("repro-crash-job"), nil)
	crashID := s.reportTestCrash(jobID)

	// Request crash reproduction
	reproReq := generated.ReproduceCrashRequest{
		Attempts: 3,
		Timeout:  300,
		Environment: map[string]interface{}{
			"timeout":      10,
			"memory_limit": 1024,
		},
	}

	client := s.client.GetClient()
	resp, err := client.ReproduceCrash(s.ctx, crashID, reproReq)
	s.Require().NoError(err)

	// Reproduction is typically async, so accept either immediate result or async response
	if resp.StatusCode == http.StatusOK {
		// Immediate result
		var reproResult generated.Crash
		err = parseJSONResponse(resp, &reproResult)
		s.Require().NoError(err)
		s.Equal(crashID, reproResult.Id)
		s.NotNil(reproResult.ReproductionInfo)
	} else if resp.StatusCode == http.StatusAccepted {
		// Async operation started
		var asyncResp generated.ReproduceCrash202Response
		err = parseJSONResponse(resp, &asyncResp)
		s.Require().NoError(err)
		s.NotEmpty(asyncResp.TaskId)
		s.T().Logf("Async reproduction started with task ID: %s", asyncResp.TaskId)
	}
}

// TestCrashReproductionNotFound tests reproduction for non-existent crash
func (s *APIIntegrationTestSuite) TestCrashReproductionNotFound() {
	nonExistentID := generateTestUUID()

	reproReq := generated.ReproduceCrashRequest{
		Attempts: 3,
		Timeout:  300,
	}

	client := s.client.GetClient()
	resp, err := client.ReproduceCrash(s.ctx, nonExistentID, reproReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCrashValidation tests crash validation rules
func (s *APIIntegrationTestSuite) TestCrashValidation() {
	jobID := s.createTestJob(generateTestName("validation-crash-job"), nil)

	testCases := []struct {
		name        string
		crash       generated.Crash
		expectedErr bool
	}{
		{
			name: "valid_crash",
			crash: generated.Crash{
				JobId:     jobID,
				Hash:      "valid-crash-hash",
				Timestamp: time.Now(),
				Type:      generated.CrashTypeSegmentationFault,
				Severity:  generated.CrashSeverityMedium,
				Input:     []byte("valid input"),
				Output:    "valid output",
				CrashInfo: generated.CrashCrashInfo{
					Signal:     11,
					ExitCode:   139,
					StackTrace: "valid stack trace",
				},
			},
			expectedErr: false,
		},
		{
			name: "empty_hash",
			crash: generated.Crash{
				JobId:     jobID,
				Hash:      "", // Invalid: empty hash
				Timestamp: time.Now(),
				Type:      generated.CrashTypeSegmentationFault,
				Severity:  generated.CrashSeverityMedium,
				Input:     []byte("input"),
				Output:    "output",
			},
			expectedErr: true,
		},
		{
			name: "empty_input",
			crash: generated.Crash{
				JobId:     jobID,
				Hash:      "test-hash",
				Timestamp: time.Now(),
				Type:      generated.CrashTypeSegmentationFault,
				Severity:  generated.CrashSeverityMedium,
				Input:     []byte{}, // Invalid: empty input
				Output:    "output",
			},
			expectedErr: true,
		},
	}

	client := s.client.GetClient()

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := client.ReportCrash(s.ctx, tc.crash)
			s.Require().NoError(err)

			if tc.expectedErr {
				s.Equal(http.StatusBadRequest, resp.StatusCode)
			} else {
				s.Equal(http.StatusCreated, resp.StatusCode)
			}

			resp.Body.Close()
		})
	}
}

// TestCrashTypeClassification tests crash type classification
func (s *APIIntegrationTestSuite) TestCrashTypeClassification() {
	jobID := s.createTestJob(generateTestName("type-crash-job"), nil)

	// Test different crash types
	crashTypes := []generated.CrashType{
		generated.CrashTypeSegmentationFault,
		generated.CrashTypeAbort,
		generated.CrashTypeAssertionFailure,
		generated.CrashTypeBufferOverflow,
		generated.CrashTypeNullPointerDereference,
	}

	generator := &TestDataGenerator{}
	client := s.client.GetClient()

	for _, crashType := range crashTypes {
		crash := generator.GenerateCrash(jobID)
		crash.Type = crashType
		crash.Hash = fmt.Sprintf("%s-hash", string(crashType))

		resp, err := client.ReportCrash(s.ctx, crash)
		s.Require().NoError(err)
		s.Equal(http.StatusCreated, resp.StatusCode)

		var reportedCrash generated.Crash
		err = parseJSONResponse(resp, &reportedCrash)
		s.Require().NoError(err)

		s.Equal(crashType, reportedCrash.Type)
	}
}

// TestCrashSeverityLevels tests crash severity classification
func (s *APIIntegrationTestSuite) TestCrashSeverityLevels() {
	jobID := s.createTestJob(generateTestName("severity-crash-job"), nil)

	// Test different severity levels
	severities := []generated.CrashSeverity{
		generated.CrashSeverityLow,
		generated.CrashSeverityMedium,
		generated.CrashSeverityHigh,
		generated.CrashSeverityCritical,
	}

	generator := &TestDataGenerator{}
	client := s.client.GetClient()

	for _, severity := range severities {
		crash := generator.GenerateCrash(jobID)
		crash.Severity = severity
		crash.Hash = fmt.Sprintf("%s-severity-hash", string(severity))

		resp, err := client.ReportCrash(s.ctx, crash)
		s.Require().NoError(err)
		s.Equal(http.StatusCreated, resp.StatusCode)

		var reportedCrash generated.Crash
		err = parseJSONResponse(resp, &reportedCrash)
		s.Require().NoError(err)

		s.Equal(severity, reportedCrash.Severity)
	}
}

// TestConcurrentCrashOperations tests concurrent crash operations
func (s *APIIntegrationTestSuite) TestConcurrentCrashOperations() {
	const numCrashes = 5

	// Create test job
	jobID := s.createTestJob(generateTestName("concurrent-crash-job"), nil)

	// Channel to collect results
	results := make(chan error, numCrashes)

	// Report crashes concurrently
	for i := 0; i < numCrashes; i++ {
		go func(index int) {
			generator := &TestDataGenerator{}
			crash := generator.GenerateCrash(jobID)
			crash.Hash = fmt.Sprintf("concurrent-crash-%d-hash", index)

			client := s.client.GetClient()
			resp, err := client.ReportCrash(s.ctx, crash)
			if err != nil {
				results <- err
				return
			}

			if resp.StatusCode != http.StatusCreated {
				results <- fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				return
			}

			resp.Body.Close()
			results <- nil
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numCrashes; i++ {
		err := <-results
		s.NoError(err, "Concurrent crash reporting failed")
	}
}

// TestCrashStackTraceAnalysis tests stack trace analysis functionality
func (s *APIIntegrationTestSuite) TestCrashStackTraceAnalysis() {
	// Create and report a test crash with detailed stack trace
	jobID := s.createTestJob(generateTestName("stack-crash-job"), nil)

	generator := &TestDataGenerator{}
	crash := generator.GenerateCrash(jobID)
	crash.CrashInfo.StackTrace = `#0 0x0000000000400123 in vulnerable_function() at test.c:42
#1 0x0000000000400456 in main() at test.c:10
#2 0x00007f0123456789 in __libc_start_main() at libc.c:308`

	client := s.client.GetClient()
	resp, err := client.ReportCrash(s.ctx, crash)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var reportedCrash generated.Crash
	err = parseJSONResponse(resp, &reportedCrash)
	s.Require().NoError(err)

	s.Equal(crash.CrashInfo.StackTrace, reportedCrash.CrashInfo.StackTrace)
	s.Equal(crash.CrashInfo.Signal, reportedCrash.CrashInfo.Signal)
	s.Equal(crash.CrashInfo.ExitCode, reportedCrash.CrashInfo.ExitCode)
}

// Helper methods

// reportTestCrash reports a test crash and returns its ID
func (s *APIIntegrationTestSuite) reportTestCrash(jobID uuid.UUID) uuid.UUID {
	generator := &TestDataGenerator{}
	crash := generator.GenerateCrash(jobID)

	client := s.client.GetClient()
	resp, err := client.ReportCrash(s.ctx, crash)
	s.Require().NoError(err)
	s.Require().Equal(http.StatusCreated, resp.StatusCode)

	var reportedCrash generated.Crash
	err = parseJSONResponse(resp, &reportedCrash)
	s.Require().NoError(err)

	return reportedCrash.Id
}
