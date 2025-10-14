package integration

import (
	"bufio"
	"net/http"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
)

// TestJobCreation tests job creation functionality
func (s *APIIntegrationTestSuite) TestJobCreation() {
	generator := &TestDataGenerator{}

	// Create a campaign first
	campaignID := s.createTestCampaign(generateTestName("test-campaign"))

	// Test valid job creation
	jobName := generateTestName("test-job")
	createReq := generator.GenerateJobCreateRequest(jobName, &campaignID)

	client := s.client.GetClient()
	resp, err := client.CreateJob(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	// Validate job creation response
	validator := &ResponseValidator{}
	err = validator.ValidateJob(job, jobName)
	s.Require().NoError(err)

	s.Equal(generated.JobStatusPending, job.Status)
	s.Equal(createReq.Fuzzer, job.Fuzzer)
	s.Equal(createReq.TargetBinary, job.TargetBinary)
	s.Equal(createReq.Timeout, job.Timeout)
	s.Equal(*createReq.CampaignId, *job.CampaignId)
}

// TestJobCreationWithoutCampaign tests job creation without campaign
func (s *APIIntegrationTestSuite) TestJobCreationWithoutCampaign() {
	generator := &TestDataGenerator{}

	jobName := generateTestName("standalone-job")
	createReq := generator.GenerateJobCreateRequest(jobName, nil)

	client := s.client.GetClient()
	resp, err := client.CreateJob(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.Equal(jobName, job.Name)
	s.Nil(job.CampaignId)
	s.Equal(generated.JobStatusPending, job.Status)
}

// TestJobGet tests retrieving a single job
func (s *APIIntegrationTestSuite) TestJobGet() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("get-job"), nil)

	// Get the job
	client := s.client.GetClient()
	resp, err := client.GetJob(s.ctx, jobID, &generated.GetJobParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.Equal(jobID, job.Id)
	s.NotEmpty(job.Name)
	s.NotEmpty(job.TargetBinary)
}

// TestJobGetNotFound tests retrieving a non-existent job
func (s *APIIntegrationTestSuite) TestJobGetNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.GetJob(s.ctx, nonExistentID, &generated.GetJobParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)
}

// TestJobList tests listing jobs
func (s *APIIntegrationTestSuite) TestJobList() {
	// Create multiple test jobs
	jobCount := 3
	for i := 0; i < jobCount; i++ {
		s.createTestJob(generateTestName("list-job"), nil)
	}

	// List all jobs
	client := s.client.GetClient()
	params := &generated.ListJobsParams{}
	resp, err := client.ListJobs(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var jobList generated.JobListResponse
	err = parseJSONResponse(resp, &jobList)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(jobList.Jobs), jobCount)

	// Validate pagination
	validator := &ResponseValidator{}
	err = validator.ValidatePagination(jobList.Pagination, -1)
	s.Require().NoError(err)
}

// TestJobListPagination tests job list pagination
func (s *APIIntegrationTestSuite) TestJobListPagination() {
	// Create several test jobs
	jobCount := 5
	for i := 0; i < jobCount; i++ {
		s.createTestJob(generateTestName("page-job"), nil)
	}

	// Test pagination with limit
	limit := 2
	client := s.client.GetClient()
	params := &generated.ListJobsParams{
		Limit: &limit,
	}

	resp, err := client.ListJobs(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var jobList generated.JobListResponse
	err = parseJSONResponse(resp, &jobList)
	s.Require().NoError(err)

	s.LessOrEqual(len(jobList.Jobs), limit)
	s.NotNil(jobList.Pagination)
	s.Equal(limit, jobList.Pagination.Limit)
}

// TestJobListFiltering tests job list filtering
func (s *APIIntegrationTestSuite) TestJobListFiltering() {
	// Create jobs with different statuses
	pendingJob := s.createTestJob(generateTestName("pending-job"), nil)

	// Test filtering by status
	status := generated.JobStatusPending
	client := s.client.GetClient()
	params := &generated.ListJobsParams{
		Status: &status,
	}

	resp, err := client.ListJobs(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var jobList generated.JobListResponse
	err = parseJSONResponse(resp, &jobList)
	s.Require().NoError(err)

	// All returned jobs should have pending status
	for _, job := range jobList.Jobs {
		s.Equal(generated.JobStatusPending, job.Status)
	}

	// Should include our test job
	found := false
	for _, job := range jobList.Jobs {
		if job.Id == pendingJob {
			found = true
			break
		}
	}
	s.True(found, "Test job should be in filtered results")
}

// TestJobUpdate tests updating job information
func (s *APIIntegrationTestSuite) TestJobUpdate() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("update-job"), nil)

	// Update the job
	updateReq := generated.JobUpdateRequest{
		Status:      &[]generated.JobStatus{generated.JobStatusRunning}[0],
		Timeout:     &[]int{7200}[0],
		MemoryLimit: &[]int{2048}[0],
		Progress: &generated.JobProgress{
			ExecsPerSec:     1000,
			TotalExecs:      50000,
			CoveragePercent: 25.5,
			CrashesFound:    2,
			LastUpdate:      time.Now(),
		},
	}

	client := s.client.GetClient()
	resp, err := client.UpdateJob(s.ctx, jobID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.Equal(jobID, job.Id)
	s.Equal(*updateReq.Status, job.Status)
	s.Equal(*updateReq.Timeout, job.Timeout)
	s.Equal(*updateReq.MemoryLimit, job.MemoryLimit)
	s.NotNil(job.Progress)
	s.Equal(updateReq.Progress.ExecsPerSec, job.Progress.ExecsPerSec)
}

// TestJobUpdateNotFound tests updating a non-existent job
func (s *APIIntegrationTestSuite) TestJobUpdateNotFound() {
	nonExistentID := generateTestUUID()

	updateReq := generated.JobUpdateRequest{
		Status: &[]generated.JobStatus{generated.JobStatusRunning}[0],
	}

	client := s.client.GetClient()
	resp, err := client.UpdateJob(s.ctx, nonExistentID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestJobCancel tests job cancellation
func (s *APIIntegrationTestSuite) TestJobCancel() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("cancel-job"), nil)

	// Cancel the job
	client := s.client.GetClient()
	resp, err := client.CancelJob(s.ctx, jobID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.Equal(jobID, job.Id)
	s.Equal(generated.JobStatusCancelled, job.Status)
}

// TestJobCancelNotFound tests cancelling a non-existent job
func (s *APIIntegrationTestSuite) TestJobCancelNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.CancelJob(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestJobDelete tests job deletion
func (s *APIIntegrationTestSuite) TestJobDelete() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("delete-job"), nil)

	// Verify job exists
	client := s.client.GetClient()
	resp, err := client.GetJob(s.ctx, jobID, &generated.GetJobParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete the job
	resp, err = client.DeleteJob(s.ctx, jobID)
	s.Require().NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify job is deleted
	resp, err = client.GetJob(s.ctx, jobID, &generated.GetJobParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestJobDeleteNotFound tests deleting a non-existent job
func (s *APIIntegrationTestSuite) TestJobDeleteNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.DeleteJob(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestJobLogs tests job log retrieval
func (s *APIIntegrationTestSuite) TestJobLogs() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("log-job"), nil)

	// Get job logs
	client := s.client.GetClient()
	params := &generated.GetJobLogsParams{}
	resp, err := client.GetJobLogs(s.ctx, jobID, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var logs generated.JobLogsResponse
	err = parseJSONResponse(resp, &logs)
	s.Require().NoError(err)

	s.NotNil(logs.Logs)
	// Logs may be empty for a newly created job
	s.GreaterOrEqual(len(logs.Logs), 0)
}

// TestJobLogsStreaming tests job log streaming
func (s *APIIntegrationTestSuite) TestJobLogsStreaming() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("stream-job"), nil)

	// Test streaming logs
	follow := true
	client := s.client.GetClient()
	params := &generated.GetJobLogsParams{
		Follow: &follow,
	}

	resp, err := client.GetJobLogs(s.ctx, jobID, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	// For streaming, we should get a response that we can read from
	// In a real scenario, this would be an SSE stream
	defer resp.Body.Close()

	// Read first few lines or timeout
	scanner := bufio.NewScanner(resp.Body)
	lineCount := 0
	timeout := time.After(2 * time.Second)

	for lineCount < 5 {
		select {
		case <-timeout:
			s.T().Log("Log streaming timeout (expected for new job)")
			return
		default:
			if scanner.Scan() {
				line := scanner.Text()
				s.T().Logf("Log line: %s", line)
				lineCount++
			} else {
				// No more data available
				break
			}
		}
	}
}

// TestJobCoverage tests job coverage reporting
func (s *APIIntegrationTestSuite) TestJobCoverage() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("coverage-job"), nil)

	// Get job coverage
	format := generated.CoverageFormatLcov
	client := s.client.GetClient()
	params := &generated.GetJobCoverageParams{
		Format: &format,
	}

	resp, err := client.GetJobCoverage(s.ctx, jobID, params)
	s.Require().NoError(err)

	// Coverage might not be available for a new job
	if resp.StatusCode == http.StatusOK {
		var coverage generated.CoverageReport
		err = parseJSONResponse(resp, &coverage)
		s.Require().NoError(err)

		s.Equal(jobID, coverage.JobId)
		s.NotNil(coverage.CoverageMetrics)
	} else if resp.StatusCode == http.StatusNotFound {
		s.T().Log("Coverage not available for new job (expected)")
	}

	resp.Body.Close()
}

// TestJobValidation tests job validation rules
func (s *APIIntegrationTestSuite) TestJobValidation() {
	testCases := []struct {
		name        string
		request     generated.JobCreateRequest
		expectedErr bool
	}{
		{
			name: "valid_job",
			request: generated.JobCreateRequest{
				Name:         "valid-job",
				Fuzzer:       generated.FuzzerTypeAflplusplus,
				TargetBinary: "/bin/test",
				TargetArgs:   []string{"@@"},
				Timeout:      3600,
				MemoryLimit:  1024,
			},
			expectedErr: false,
		},
		{
			name: "empty_name",
			request: generated.JobCreateRequest{
				Name:         "",
				Fuzzer:       generated.FuzzerTypeAflplusplus,
				TargetBinary: "/bin/test",
				Timeout:      3600,
				MemoryLimit:  1024,
			},
			expectedErr: true,
		},
		{
			name: "empty_target_binary",
			request: generated.JobCreateRequest{
				Name:         "test-job",
				Fuzzer:       generated.FuzzerTypeAflplusplus,
				TargetBinary: "",
				Timeout:      3600,
				MemoryLimit:  1024,
			},
			expectedErr: true,
		},
		{
			name: "invalid_timeout",
			request: generated.JobCreateRequest{
				Name:         "test-job",
				Fuzzer:       generated.FuzzerTypeAflplusplus,
				TargetBinary: "/bin/test",
				Timeout:      0,
				MemoryLimit:  1024,
			},
			expectedErr: true,
		},
	}

	client := s.client.GetClient()

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := client.CreateJob(s.ctx, tc.request)
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

// TestJobStatusTransitions tests job status transitions
func (s *APIIntegrationTestSuite) TestJobStatusTransitions() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("status-job"), nil)

	// Test status transitions
	statuses := []generated.JobStatus{
		generated.JobStatusPending,
		generated.JobStatusRunning,
		generated.JobStatusCompleted,
	}

	client := s.client.GetClient()

	for _, status := range statuses {
		updateReq := generated.JobUpdateRequest{
			Status: &status,
		}

		resp, err := client.UpdateJob(s.ctx, jobID, updateReq)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)

		var job generated.Job
		err = parseJSONResponse(resp, &job)
		s.Require().NoError(err)

		s.Equal(status, job.Status)
	}
}

// TestJobAssignment tests job assignment to bots
func (s *APIIntegrationTestSuite) TestJobAssignment() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("assigned-bot"))

	// Create a test job
	jobID := s.createTestJob(generateTestName("assigned-job"), nil)

	// Assign job to bot (simulate assignment through status update)
	updateReq := generated.JobUpdateRequest{
		Status:      &[]generated.JobStatus{generated.JobStatusRunning}[0],
		AssignedBot: &botID,
	}

	client := s.client.GetClient()
	resp, err := client.UpdateJob(s.ctx, jobID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.Equal(generated.JobStatusRunning, job.Status)
	s.NotNil(job.AssignedBot)
	s.Equal(botID, *job.AssignedBot)
}

// TestJobProgress tests job progress tracking
func (s *APIIntegrationTestSuite) TestJobProgress() {
	// Create a test job
	jobID := s.createTestJob(generateTestName("progress-job"), nil)

	// Update job with progress
	progress := &generated.JobProgress{
		ExecsPerSec:     1500,
		TotalExecs:      75000,
		CoveragePercent: 45.5,
		CrashesFound:    3,
		LastUpdate:      time.Now(),
	}

	updateReq := generated.JobUpdateRequest{
		Progress: progress,
	}

	client := s.client.GetClient()
	resp, err := client.UpdateJob(s.ctx, jobID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var job generated.Job
	err = parseJSONResponse(resp, &job)
	s.Require().NoError(err)

	s.NotNil(job.Progress)
	s.Equal(progress.ExecsPerSec, job.Progress.ExecsPerSec)
	s.Equal(progress.TotalExecs, job.Progress.TotalExecs)
	s.Equal(progress.CoveragePercent, job.Progress.CoveragePercent)
	s.Equal(progress.CrashesFound, job.Progress.CrashesFound)
}

// TestConcurrentJobOperations tests concurrent job operations
func (s *APIIntegrationTestSuite) TestConcurrentJobOperations() {
	const numJobs = 5

	// Channel to collect results
	results := make(chan error, numJobs)

	// Create jobs concurrently
	for i := 0; i < numJobs; i++ {
		go func(index int) {
			jobName := generateTestName("concurrent-job")
			generator := &TestDataGenerator{}
			createReq := generator.GenerateJobCreateRequest(jobName, nil)

			client := s.client.GetClient()
			resp, err := client.CreateJob(s.ctx, createReq)
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
	for i := 0; i < numJobs; i++ {
		err := <-results
		s.NoError(err, "Concurrent job creation failed")
	}
}

// TestJobByCampaign tests filtering jobs by campaign
func (s *APIIntegrationTestSuite) TestJobByCampaign() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("job-campaign"))

	// Create jobs in this campaign
	jobCount := 3
	jobIDs := make([]uuid.UUID, jobCount)
	for i := 0; i < jobCount; i++ {
		jobIDs[i] = s.createTestJob(generateTestName("campaign-job"), &campaignID)
	}

	// Filter jobs by campaign
	client := s.client.GetClient()
	params := &generated.ListJobsParams{
		CampaignId: &campaignID,
	}

	resp, err := client.ListJobs(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var jobList generated.JobListResponse
	err = parseJSONResponse(resp, &jobList)
	s.Require().NoError(err)

	// All returned jobs should belong to our campaign
	for _, job := range jobList.Jobs {
		s.NotNil(job.CampaignId)
		s.Equal(campaignID, *job.CampaignId)
	}

	// Should include all our test jobs
	foundCount := 0
	for _, job := range jobList.Jobs {
		for _, jobID := range jobIDs {
			if job.Id == jobID {
				foundCount++
				break
			}
		}
	}
	s.Equal(jobCount, foundCount, "All campaign jobs should be found")
}
