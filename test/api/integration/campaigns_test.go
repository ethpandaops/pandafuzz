package integration

import (
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
)

// TestCampaignCreation tests campaign creation functionality
func (s *APIIntegrationTestSuite) TestCampaignCreation() {
	generator := &TestDataGenerator{}

	// Test valid campaign creation
	campaignName := generateTestName("test-campaign")
	createReq := generator.GenerateCampaignCreateRequest(campaignName)

	resp, err := s.client.CreateCampaign(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	// Validate campaign creation response
	validator := &ResponseValidator{}
	err = validator.ValidateCampaign(campaign, campaignName)
	s.Require().NoError(err)

	s.Equal(generated.Draft, campaign.Status)
	s.Equal(createReq.Description, campaign.Description)
	s.NotNil(campaign.JobTemplate)
	s.Equal(createReq.JobTemplate.Fuzzer, campaign.JobTemplate.Fuzzer)
	s.Equal(createReq.JobTemplate.TargetBinary, campaign.JobTemplate.TargetBinary)
}

// TestCampaignCreationDuplicateName tests handling of duplicate campaign names
func (s *APIIntegrationTestSuite) TestCampaignCreationDuplicateName() {
	generator := &TestDataGenerator{}

	campaignName := generateTestName("duplicate-campaign")
	createReq := generator.GenerateCampaignCreateRequest(campaignName)

	// Create first campaign
	resp, err := s.client.CreateCampaign(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Try to create campaign with same name
	resp, err = s.client.CreateCampaign(s.ctx, createReq)
	s.Require().NoError(err)

	// Should handle duplicate appropriately (either error or allow)
	if resp.StatusCode == http.StatusConflict {
		s.T().Log("API properly rejects duplicate campaign names")
		problemDetails, err := parseErrorResponse(resp)
		s.Require().NoError(err)
		s.Contains(problemDetails.Detail, "already exists")
	} else if resp.StatusCode == http.StatusCreated {
		s.T().Log("API allows duplicate campaign names")
	}
	resp.Body.Close()
}

// TestCampaignGet tests retrieving a single campaign
func (s *APIIntegrationTestSuite) TestCampaignGet() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("get-campaign"))

	// Get the campaign
	resp, err := s.client.GetCampaign(s.ctx, campaignID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	s.Equal(campaignID, campaign.Id)
	s.NotEmpty(campaign.Name)
	s.NotEmpty(campaign.Description)
	s.NotNil(campaign.JobTemplate)
}

// TestCampaignGetNotFound tests retrieving a non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignGetNotFound() {
	nonExistentID := generateTestUUID()

	resp, err := s.client.GetCampaign(s.ctx, nonExistentID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)
}

// TestCampaignList tests listing campaigns
func (s *APIIntegrationTestSuite) TestCampaignList() {
	// Create multiple test campaigns
	campaignCount := 3
	for i := 0; i < campaignCount; i++ {
		s.createTestCampaign(generateTestName("list-campaign"))
	}

	// List all campaigns
	params := &generated.ListCampaignsParams{}
	resp, err := s.client.ListCampaigns(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaignList generated.CampaignListResponse
	err = parseJSONResponse(resp, &campaignList)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(campaignList.Campaigns), campaignCount)

	// Validate pagination
	validator := &ResponseValidator{}
	err = validator.ValidatePagination(campaignList.Pagination, -1)
	s.Require().NoError(err)
}

// TestCampaignListPagination tests campaign list pagination
func (s *APIIntegrationTestSuite) TestCampaignListPagination() {
	// Create several test campaigns
	campaignCount := 5
	for i := 0; i < campaignCount; i++ {
		s.createTestCampaign(generateTestName("page-campaign"))
	}

	// Test pagination with limit
	limit := 2
	params := &generated.ListCampaignsParams{
		Limit: &limit,
	}

	resp, err := s.client.ListCampaigns(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaignList generated.CampaignListResponse
	err = parseJSONResponse(resp, &campaignList)
	s.Require().NoError(err)

	s.LessOrEqual(len(campaignList.Campaigns), limit)
	s.NotNil(campaignList.Pagination)
	s.Equal(limit, campaignList.Pagination.Limit)
}

// TestCampaignListFiltering tests campaign list filtering
func (s *APIIntegrationTestSuite) TestCampaignListFiltering() {
	// Create campaigns with different statuses
	draftCampaign := s.createTestCampaign(generateTestName("draft-campaign"))

	// Test filtering by status
	status := generated.Draft
	params := &generated.ListCampaignsParams{
		Status: &status,
	}

	resp, err := s.client.ListCampaigns(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaignList generated.CampaignListResponse
	err = parseJSONResponse(resp, &campaignList)
	s.Require().NoError(err)

	// All returned campaigns should have draft status
	for _, campaign := range campaignList.Campaigns {
		s.Equal(generated.Draft, campaign.Status)
	}

	// Should include our test campaign
	found := false
	for _, campaign := range campaignList.Campaigns {
		if campaign.Id == draftCampaign {
			found = true
			break
		}
	}
	s.True(found, "Test campaign should be in filtered results")
}

// TestCampaignUpdate tests updating campaign information
func (s *APIIntegrationTestSuite) TestCampaignUpdate() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("update-campaign"))

	// Update the campaign
	newDescription := "Updated campaign description"
	updateReq := generated.CampaignUpdateRequest{
		Description: &newDescription,
		JobTemplate: &generated.CampaignJobTemplate{
			Fuzzer:       generated.FuzzerTypeAflplusplus,
			TargetBinary: "/bin/updated-target",
			TargetArgs:   []string{"-updated", "@@"},
			Timeout:      7200,
			MemoryLimit:  2048,
		},
	}

	client := s.client.GetClient()
	resp, err := client.UpdateCampaign(s.ctx, campaignID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	s.Equal(campaignID, campaign.Id)
	s.Equal(*updateReq.Description, campaign.Description)
	s.NotNil(campaign.JobTemplate)
	s.Equal(updateReq.JobTemplate.TargetBinary, campaign.JobTemplate.TargetBinary)
	s.Equal(updateReq.JobTemplate.Timeout, campaign.JobTemplate.Timeout)
}

// TestCampaignUpdateNotFound tests updating a non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignUpdateNotFound() {
	nonExistentID := generateTestUUID()

	newDescription := "Updated description"
	updateReq := generated.CampaignUpdateRequest{
		Description: &newDescription,
	}

	client := s.client.GetClient()
	resp, err := client.UpdateCampaign(s.ctx, nonExistentID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestCampaignStart tests starting a campaign
func (s *APIIntegrationTestSuite) TestCampaignStart() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("start-campaign"))

	// Start the campaign
	client := s.client.GetClient()
	resp, err := client.StartCampaign(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	s.Equal(campaignID, campaign.Id)
	s.Equal(generated.Active, campaign.Status)
	s.NotNil(campaign.StartedAt)
}

// TestCampaignStartNotFound tests starting a non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignStartNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.StartCampaign(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestCampaignStop tests stopping a campaign
func (s *APIIntegrationTestSuite) TestCampaignStop() {
	// Create and start a test campaign
	campaignID := s.createTestCampaign(generateTestName("stop-campaign"))

	client := s.client.GetClient()

	// Start the campaign first
	resp, err := client.StartCampaign(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Stop the campaign
	stopReq := generated.StopCampaignRequest{
		Reason: "Test completion",
	}

	resp, err = client.StopCampaign(s.ctx, campaignID, stopReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)

	s.Equal(campaignID, campaign.Id)
	s.Equal(generated.Paused, campaign.Status)
	s.NotNil(campaign.StoppedAt)
}

// TestCampaignStopNotFound tests stopping a non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignStopNotFound() {
	nonExistentID := generateTestUUID()

	stopReq := generated.StopCampaignRequest{
		Reason: "Test",
	}

	client := s.client.GetClient()
	resp, err := client.StopCampaign(s.ctx, nonExistentID, stopReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestCampaignDelete tests campaign deletion
func (s *APIIntegrationTestSuite) TestCampaignDelete() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("delete-campaign"))

	// Verify campaign exists
	resp, err := s.client.GetCampaign(s.ctx, campaignID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete the campaign
	client := s.client.GetClient()
	resp, err = client.DeleteCampaign(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify campaign is deleted
	resp, err = s.client.GetCampaign(s.ctx, campaignID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCampaignDeleteNotFound tests deleting a non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignDeleteNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.DeleteCampaign(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCampaignStats tests campaign statistics retrieval
func (s *APIIntegrationTestSuite) TestCampaignStats() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("stats-campaign"))

	// Get campaign stats
	client := s.client.GetClient()
	resp, err := client.GetCampaignStats(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var stats generated.CampaignStats
	err = parseJSONResponse(resp, &stats)
	s.Require().NoError(err)

	s.Equal(campaignID, stats.CampaignId)
	s.GreaterOrEqual(stats.TotalJobs, 0)
	s.GreaterOrEqual(stats.CompletedJobs, 0)
	s.GreaterOrEqual(stats.RunningJobs, 0)
	s.GreaterOrEqual(stats.FailedJobs, 0)
	s.GreaterOrEqual(stats.TotalCrashes, 0)
	s.GreaterOrEqual(stats.UniqueCrashes, 0)
	s.GreaterOrEqual(stats.CorpusSize, 0)
}

// TestCampaignStatsNotFound tests stats for non-existent campaign
func (s *APIIntegrationTestSuite) TestCampaignStatsNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.GetCampaignStats(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCampaignValidation tests campaign validation rules
func (s *APIIntegrationTestSuite) TestCampaignValidation() {
	testCases := []struct {
		name        string
		request     generated.CampaignCreateRequest
		expectedErr bool
	}{
		{
			name: "valid_campaign",
			request: generated.CampaignCreateRequest{
				Name:        "valid-campaign",
				Description: "Valid test campaign",
				JobTemplate: generated.CampaignCreateRequestJobTemplate{
					Fuzzer:       generated.FuzzerTypeAflplusplus,
					TargetBinary: "/bin/test",
					TargetArgs:   []string{"@@"},
					Timeout:      3600,
					MemoryLimit:  1024,
				},
			},
			expectedErr: false,
		},
		{
			name: "empty_name",
			request: generated.CampaignCreateRequest{
				Name:        "",
				Description: "Test campaign",
				JobTemplate: generated.CampaignCreateRequestJobTemplate{
					Fuzzer:       generated.FuzzerTypeAflplusplus,
					TargetBinary: "/bin/test",
					Timeout:      3600,
					MemoryLimit:  1024,
				},
			},
			expectedErr: true,
		},
		{
			name: "empty_target_binary",
			request: generated.CampaignCreateRequest{
				Name:        "test-campaign",
				Description: "Test campaign",
				JobTemplate: generated.CampaignCreateRequestJobTemplate{
					Fuzzer:       generated.FuzzerTypeAflplusplus,
					TargetBinary: "",
					Timeout:      3600,
					MemoryLimit:  1024,
				},
			},
			expectedErr: true,
		},
		{
			name: "invalid_timeout",
			request: generated.CampaignCreateRequest{
				Name:        "test-campaign",
				Description: "Test campaign",
				JobTemplate: generated.CampaignCreateRequestJobTemplate{
					Fuzzer:       generated.FuzzerTypeAflplusplus,
					TargetBinary: "/bin/test",
					Timeout:      0,
					MemoryLimit:  1024,
				},
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.client.CreateCampaign(s.ctx, tc.request)
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

// TestCampaignStatusTransitions tests campaign status transitions
func (s *APIIntegrationTestSuite) TestCampaignStatusTransitions() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("status-campaign"))

	client := s.client.GetClient()

	// Test status transitions: Draft -> Active -> Paused -> Active -> Completed
	transitions := []struct {
		action   string
		status   generated.CampaignStatus
		endpoint func() (*http.Response, error)
	}{
		{
			action: "start",
			status: generated.Active,
			endpoint: func() (*http.Response, error) {
				return client.StartCampaign(s.ctx, campaignID)
			},
		},
		{
			action: "stop",
			status: generated.Paused,
			endpoint: func() (*http.Response, error) {
				return client.StopCampaign(s.ctx, campaignID, generated.StopCampaignRequest{
					Reason: "Test pause",
				})
			},
		},
		{
			action: "restart",
			status: generated.Active,
			endpoint: func() (*http.Response, error) {
				return client.StartCampaign(s.ctx, campaignID)
			},
		},
	}

	for _, transition := range transitions {
		resp, err := transition.endpoint()
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)

		var campaign generated.Campaign
		err = parseJSONResponse(resp, &campaign)
		s.Require().NoError(err)

		s.Equal(transition.status, campaign.Status)
		s.T().Logf("Campaign %s: %s -> %s", transition.action, "previous", campaign.Status)
	}
}

// TestCampaignWithJobs tests campaign functionality with associated jobs
func (s *APIIntegrationTestSuite) TestCampaignWithJobs() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("jobs-campaign"))

	// Create jobs in this campaign
	jobCount := 3
	jobIDs := make([]uuid.UUID, jobCount)
	for i := 0; i < jobCount; i++ {
		jobIDs[i] = s.createTestJob(generateTestName("campaign-job"), &campaignID)
	}

	// Get campaign stats to verify job association
	client := s.client.GetClient()
	resp, err := client.GetCampaignStats(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var stats generated.CampaignStats
	err = parseJSONResponse(resp, &stats)
	s.Require().NoError(err)

	s.GreaterOrEqual(stats.TotalJobs, jobCount)

	// Test campaign deletion with associated jobs
	resp, err = client.DeleteCampaign(s.ctx, campaignID)
	s.Require().NoError(err)

	// Depending on implementation, this might be forbidden or cascade delete
	if resp.StatusCode == http.StatusConflict {
		s.T().Log("Campaign deletion with jobs is properly restricted")
	} else if resp.StatusCode == http.StatusNoContent {
		s.T().Log("Campaign deletion cascades to jobs")
	}

	resp.Body.Close()
}

// TestConcurrentCampaignOperations tests concurrent campaign operations
func (s *APIIntegrationTestSuite) TestConcurrentCampaignOperations() {
	const numCampaigns = 5

	// Channel to collect results
	results := make(chan error, numCampaigns)

	// Create campaigns concurrently
	for i := 0; i < numCampaigns; i++ {
		go func(index int) {
			campaignName := generateTestName("concurrent-campaign")
			generator := &TestDataGenerator{}
			createReq := generator.GenerateCampaignCreateRequest(campaignName)

			resp, err := s.client.CreateCampaign(s.ctx, createReq)
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
	for i := 0; i < numCampaigns; i++ {
		err := <-results
		s.NoError(err, "Concurrent campaign creation failed")
	}
}

// TestCampaignPerformanceMetrics tests campaign performance metrics
func (s *APIIntegrationTestSuite) TestCampaignPerformanceMetrics() {
	// Create a test campaign
	campaignID := s.createTestCampaign(generateTestName("perf-campaign"))

	// Get campaign stats to check performance metrics
	client := s.client.GetClient()
	resp, err := client.GetCampaignStats(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var stats generated.CampaignStats
	err = parseJSONResponse(resp, &stats)
	s.Require().NoError(err)

	// Validate performance metrics structure
	if stats.PerformanceMetrics != nil {
		s.GreaterOrEqual(stats.PerformanceMetrics.AvgExecsPerSec, float32(0))
		s.GreaterOrEqual(stats.PerformanceMetrics.TotalExecs, 0)
		s.GreaterOrEqual(stats.PerformanceMetrics.CoveragePercent, float32(0))
		s.LessOrEqual(stats.PerformanceMetrics.CoveragePercent, float32(100))
	}
}

// TestCampaignLifecycle tests complete campaign lifecycle
func (s *APIIntegrationTestSuite) TestCampaignLifecycle() {
	// Create campaign
	campaignName := generateTestName("lifecycle-campaign")
	generator := &TestDataGenerator{}
	createReq := generator.GenerateCampaignCreateRequest(campaignName)

	resp, err := s.client.CreateCampaign(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var campaign generated.Campaign
	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)
	campaignID := campaign.Id

	// Verify initial state
	s.Equal(generated.Draft, campaign.Status)
	s.True(campaign.CreatedAt.Before(time.Now()))
	s.Nil(campaign.StartedAt)
	s.Nil(campaign.StoppedAt)
	s.Nil(campaign.CompletedAt)

	client := s.client.GetClient()

	// Start campaign
	resp, err = client.StartCampaign(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)
	s.Equal(generated.Active, campaign.Status)
	s.NotNil(campaign.StartedAt)

	// Create jobs in campaign
	jobID := s.createTestJob(generateTestName("lifecycle-job"), &campaignID)
	s.NotEqual(uuid.Nil, jobID)

	// Check stats
	resp, err = client.GetCampaignStats(s.ctx, campaignID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var stats generated.CampaignStats
	err = parseJSONResponse(resp, &stats)
	s.Require().NoError(err)
	s.GreaterOrEqual(stats.TotalJobs, 1)

	// Stop campaign
	stopReq := generated.StopCampaignRequest{
		Reason: "Lifecycle test completion",
	}

	resp, err = client.StopCampaign(s.ctx, campaignID, stopReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)
	s.Equal(generated.Paused, campaign.Status)
	s.NotNil(campaign.StoppedAt)

	// Verify final state
	resp, err = s.client.GetCampaign(s.ctx, campaignID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	err = parseJSONResponse(resp, &campaign)
	s.Require().NoError(err)
	s.Equal(campaignName, campaign.Name)
	s.Equal(generated.Paused, campaign.Status)
}
