package integration

import (
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
)

// TestBotRegistration tests bot registration functionality
func (s *APIIntegrationTestSuite) TestBotRegistration() {
	generator := &TestDataGenerator{}

	// Test valid bot creation
	botName := generateTestName("test-bot")
	createReq := generator.GenerateBotCreateRequest(botName)

	resp, err := s.client.CreateBot(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)

	var bot generated.Bot
	err = parseJSONResponse(resp, &bot)
	s.Require().NoError(err)

	// Validate bot creation response
	validator := &ResponseValidator{}
	err = validator.ValidateBot(bot, botName)
	s.Require().NoError(err)

	s.Equal(generated.BotStatusIdle, bot.Status)
	s.Equal(createReq.Hostname, bot.Hostname)
	s.Equal(len(createReq.Capabilities), len(bot.Capabilities))
	s.NotNil(bot.ResourceUsage)
}

// TestBotRegistrationDuplicateName tests handling of duplicate bot names
func (s *APIIntegrationTestSuite) TestBotRegistrationDuplicateName() {
	generator := &TestDataGenerator{}

	botName := generateTestName("duplicate-bot")
	createReq := generator.GenerateBotCreateRequest(botName)

	// Create first bot
	resp, err := s.client.CreateBot(s.ctx, createReq)
	s.Require().NoError(err)
	s.Equal(http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Try to create bot with same name
	resp, err = s.client.CreateBot(s.ctx, createReq)
	s.Require().NoError(err)

	// Should handle duplicate appropriately (either error or allow)
	if resp.StatusCode == http.StatusConflict {
		s.T().Log("API properly rejects duplicate bot names")
		problemDetails, err := parseErrorResponse(resp)
		s.Require().NoError(err)
		s.Contains(problemDetails.Detail, "already exists")
	} else if resp.StatusCode == http.StatusCreated {
		s.T().Log("API allows duplicate bot names")
	}
	resp.Body.Close()
}

// TestBotGet tests retrieving a single bot
func (s *APIIntegrationTestSuite) TestBotGet() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("get-bot"))

	// Get the bot
	resp, err := s.client.GetBot(s.ctx, botID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var bot generated.Bot
	err = parseJSONResponse(resp, &bot)
	s.Require().NoError(err)

	s.Equal(botID, bot.Id)
	s.NotEmpty(bot.Name)
	s.NotEmpty(bot.Hostname)
}

// TestBotGetNotFound tests retrieving a non-existent bot
func (s *APIIntegrationTestSuite) TestBotGetNotFound() {
	nonExistentID := generateTestUUID()

	resp, err := s.client.GetBot(s.ctx, nonExistentID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)
}

// TestBotList tests listing bots
func (s *APIIntegrationTestSuite) TestBotList() {
	// Create multiple test bots
	botCount := 3
	for i := 0; i < botCount; i++ {
		s.createTestBot(generateTestName("list-bot"))
	}

	// List all bots
	params := &generated.ListBotsParams{}
	resp, err := s.client.ListBots(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var botList generated.BotListResponse
	err = parseJSONResponse(resp, &botList)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(botList.Bots), botCount)

	// Validate pagination
	validator := &ResponseValidator{}
	err = validator.ValidatePagination(botList.Pagination, -1)
	s.Require().NoError(err)
}

// TestBotListPagination tests bot list pagination
func (s *APIIntegrationTestSuite) TestBotListPagination() {
	// Create several test bots
	botCount := 5
	for i := 0; i < botCount; i++ {
		s.createTestBot(generateTestName("page-bot"))
	}

	// Test pagination with limit
	limit := 2
	params := &generated.ListBotsParams{
		Limit: &limit,
	}

	resp, err := s.client.ListBots(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var botList generated.BotListResponse
	err = parseJSONResponse(resp, &botList)
	s.Require().NoError(err)

	s.LessOrEqual(len(botList.Bots), limit)
	s.NotNil(botList.Pagination)
	s.Equal(limit, botList.Pagination.Limit)
}

// TestBotListFiltering tests bot list filtering
func (s *APIIntegrationTestSuite) TestBotListFiltering() {
	// Create bots with different statuses
	idleBot := s.createTestBot(generateTestName("idle-bot"))

	// Test filtering by status
	status := generated.BotStatusIdle
	params := &generated.ListBotsParams{
		Status: &status,
	}

	resp, err := s.client.ListBots(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var botList generated.BotListResponse
	err = parseJSONResponse(resp, &botList)
	s.Require().NoError(err)

	// All returned bots should have idle status
	for _, bot := range botList.Bots {
		s.Equal(generated.BotStatusIdle, bot.Status)
	}

	// Should include our test bot
	found := false
	for _, bot := range botList.Bots {
		if bot.Id == idleBot {
			found = true
			break
		}
	}
	s.True(found, "Test bot should be in filtered results")
}

// TestBotUpdate tests updating bot information
func (s *APIIntegrationTestSuite) TestBotUpdate() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("update-bot"))

	// Update the bot
	updateReq := generated.BotUpdateRequest{
		Capabilities: []generated.BotUpdateRequestCapabilities{
			generated.BotUpdateRequestCapabilitiesFuzzing,
			generated.BotUpdateRequestCapabilitiesMinimization,
		},
		ResourceUsage: &generated.BotResourceUsage{
			CpuPercent:           75.0,
			MemoryMb:             1024,
			DiskSpaceMb:          4096,
			ActiveJobs:           2,
			QueueLength:          5,
			NetworkBytesSent:     1000000,
			NetworkBytesReceived: 2000000,
		},
	}

	client := s.client.GetClient()
	resp, err := client.UpdateBot(s.ctx, botID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var bot generated.Bot
	err = parseJSONResponse(resp, &bot)
	s.Require().NoError(err)

	s.Equal(botID, bot.Id)
	s.Equal(len(updateReq.Capabilities), len(bot.Capabilities))
	s.NotNil(bot.ResourceUsage)
	s.Equal(updateReq.ResourceUsage.CpuPercent, bot.ResourceUsage.CpuPercent)
	s.Equal(updateReq.ResourceUsage.ActiveJobs, bot.ResourceUsage.ActiveJobs)
}

// TestBotUpdateNotFound tests updating a non-existent bot
func (s *APIIntegrationTestSuite) TestBotUpdateNotFound() {
	nonExistentID := generateTestUUID()

	updateReq := generated.BotUpdateRequest{
		Capabilities: []generated.BotUpdateRequestCapabilities{
			generated.BotUpdateRequestCapabilitiesFuzzing,
		},
	}

	client := s.client.GetClient()
	resp, err := client.UpdateBot(s.ctx, nonExistentID, updateReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestBotHeartbeat tests bot heartbeat functionality
func (s *APIIntegrationTestSuite) TestBotHeartbeat() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("heartbeat-bot"))

	// Send heartbeat
	heartbeatReq := generated.BotHeartbeatRequest{
		Status: generated.BotStatusIdle,
		ResourceUsage: generated.BotHeartbeatRequestResourceUsage{
			CpuPercent:           50.0,
			MemoryMb:             512,
			DiskSpaceMb:          2048,
			ActiveJobs:           1,
			QueueLength:          3,
			NetworkBytesSent:     500000,
			NetworkBytesReceived: 1000000,
		},
		Capabilities: []generated.BotCapabilities{
			generated.BotCapabilitiesFuzzing,
			generated.BotCapabilitiesCoverage,
		},
	}

	client := s.client.GetClient()
	resp, err := client.SendBotHeartbeat(s.ctx, botID, heartbeatReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var heartbeatResp generated.BotHeartbeatResponse
	err = parseJSONResponse(resp, &heartbeatResp)
	s.Require().NoError(err)

	s.False(heartbeatResp.Timestamp.IsZero())
	// Commands may be empty or contain instructions
	s.NotNil(heartbeatResp.Commands)
}

// TestBotHeartbeatNotFound tests heartbeat for non-existent bot
func (s *APIIntegrationTestSuite) TestBotHeartbeatNotFound() {
	nonExistentID := generateTestUUID()

	heartbeatReq := generated.BotHeartbeatRequest{
		Status: generated.BotStatusIdle,
		ResourceUsage: generated.BotHeartbeatRequestResourceUsage{
			CpuPercent: 25.0,
			MemoryMb:   256,
		},
	}

	client := s.client.GetClient()
	resp, err := client.SendBotHeartbeat(s.ctx, nonExistentID, heartbeatReq)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

// TestBotDelete tests bot deletion
func (s *APIIntegrationTestSuite) TestBotDelete() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("delete-bot"))

	// Verify bot exists
	resp, err := s.client.GetBot(s.ctx, botID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete the bot
	client := s.client.GetClient()
	resp, err = client.DeleteBot(s.ctx, botID)
	s.Require().NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify bot is deleted
	resp, err = s.client.GetBot(s.ctx, botID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestBotDeleteNotFound tests deleting a non-existent bot
func (s *APIIntegrationTestSuite) TestBotDeleteNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.DeleteBot(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestConcurrentBotOperations tests concurrent bot operations
func (s *APIIntegrationTestSuite) TestConcurrentBotOperations() {
	const numBots = 5

	// Channel to collect results
	results := make(chan error, numBots)

	// Create bots concurrently
	for i := 0; i < numBots; i++ {
		go func(index int) {
			botName := generateTestName("concurrent-bot")
			generator := &TestDataGenerator{}
			createReq := generator.GenerateBotCreateRequest(botName)

			resp, err := s.client.CreateBot(s.ctx, createReq)
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
	for i := 0; i < numBots; i++ {
		err := <-results
		s.NoError(err, "Concurrent bot creation failed")
	}
}

// TestBotValidation tests bot validation rules
func (s *APIIntegrationTestSuite) TestBotValidation() {
	testCases := []struct {
		name        string
		request     generated.BotCreateRequest
		expectedErr bool
	}{
		{
			name: "valid_bot",
			request: generated.BotCreateRequest{
				Name:     "valid-bot",
				Hostname: "test-host",
				Capabilities: []generated.BotCreateRequestCapabilities{
					generated.BotCreateRequestCapabilitiesFuzzing,
				},
			},
			expectedErr: false,
		},
		{
			name: "empty_name",
			request: generated.BotCreateRequest{
				Name:     "",
				Hostname: "test-host",
				Capabilities: []generated.BotCreateRequestCapabilities{
					generated.BotCreateRequestCapabilitiesFuzzing,
				},
			},
			expectedErr: true,
		},
		{
			name: "empty_hostname",
			request: generated.BotCreateRequest{
				Name:     "test-bot",
				Hostname: "",
				Capabilities: []generated.BotCreateRequestCapabilities{
					generated.BotCreateRequestCapabilitiesFuzzing,
				},
			},
			expectedErr: true,
		},
		{
			name: "no_capabilities",
			request: generated.BotCreateRequest{
				Name:         "test-bot",
				Hostname:     "test-host",
				Capabilities: []generated.BotCreateRequestCapabilities{},
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := s.client.CreateBot(s.ctx, tc.request)
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

// TestBotStatusTransitions tests bot status transitions
func (s *APIIntegrationTestSuite) TestBotStatusTransitions() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("status-bot"))

	// Test status transitions through heartbeat
	statuses := []generated.BotStatus{
		generated.BotStatusIdle,
		generated.BotStatusBusy,
		generated.BotStatusMaintenance,
		generated.BotStatusIdle,
	}

	client := s.client.GetClient()

	for _, status := range statuses {
		heartbeatReq := generated.BotHeartbeatRequest{
			Status: status,
			ResourceUsage: generated.BotHeartbeatRequestResourceUsage{
				CpuPercent: 25.0,
				MemoryMb:   256,
			},
		}

		resp, err := client.SendBotHeartbeat(s.ctx, botID, heartbeatReq)
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)
		resp.Body.Close()

		// Verify status update
		resp, err = s.client.GetBot(s.ctx, botID.String())
		s.Require().NoError(err)
		s.Equal(http.StatusOK, resp.StatusCode)

		var bot generated.Bot
		err = parseJSONResponse(resp, &bot)
		s.Require().NoError(err)

		s.Equal(status, bot.Status)
	}
}

// TestBotResourceMonitoring tests bot resource usage tracking
func (s *APIIntegrationTestSuite) TestBotResourceMonitoring() {
	// Create a test bot
	botID := s.createTestBot(generateTestName("resource-bot"))

	// Send heartbeat with resource usage
	heartbeatReq := generated.BotHeartbeatRequest{
		Status: generated.BotStatusBusy,
		ResourceUsage: generated.BotHeartbeatRequestResourceUsage{
			CpuPercent:           85.5,
			MemoryMb:             2048,
			DiskSpaceMb:          10240,
			ActiveJobs:           3,
			QueueLength:          7,
			NetworkBytesSent:     5000000,
			NetworkBytesReceived: 10000000,
		},
	}

	client := s.client.GetClient()
	resp, err := client.SendBotHeartbeat(s.ctx, botID, heartbeatReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify resource usage is tracked
	resp, err = s.client.GetBot(s.ctx, botID.String())
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var bot generated.Bot
	err = parseJSONResponse(resp, &bot)
	s.Require().NoError(err)

	s.NotNil(bot.ResourceUsage)
	s.Equal(heartbeatReq.ResourceUsage.CpuPercent, bot.ResourceUsage.CpuPercent)
	s.Equal(heartbeatReq.ResourceUsage.MemoryMb, bot.ResourceUsage.MemoryMb)
	s.Equal(heartbeatReq.ResourceUsage.ActiveJobs, bot.ResourceUsage.ActiveJobs)
}
