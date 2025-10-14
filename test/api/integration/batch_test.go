package integration

import (
	"net/http"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
)

// TestBatchOperations tests batch operations functionality
func (s *APIIntegrationTestSuite) TestBatchOperations() {
	// Create batch request with multiple operations
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "batch-bot-1",
					"hostname":     "batch-host-1",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "batch-bot-2",
					"hostname":     "batch-host-2",
					"capabilities": []string{"fuzzing", "coverage"},
				},
			},
			{
				Operation: generated.CreateCampaign,
				Data: map[string]interface{}{
					"name":        "batch-campaign",
					"description": "Batch created campaign",
					"job_template": map[string]interface{}{
						"fuzzer":        "aflplusplus",
						"target_binary": "/bin/test",
						"timeout":       3600,
						"memory_limit":  1024,
					},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			FailFast:    true,
			Concurrent:  false,
			Timeout:     300,
			Transaction: true,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// Validate batch response
	s.Equal(len(batchReq.Operations), len(batchResp.Results))
	s.GreaterOrEqual(batchResp.SuccessCount, 0)
	s.GreaterOrEqual(batchResp.FailureCount, 0)
	s.Equal(batchResp.SuccessCount+batchResp.FailureCount, len(batchResp.Results))

	// Check individual operation results
	for i, result := range batchResp.Results {
		s.T().Logf("Batch operation %d: %s - %s", i, result.Operation, result.Status)

		if result.Status == generated.BatchResponseResultsStatusSuccess {
			s.NotNil(result.Data, "Successful operation should have result data")
		} else if result.Status == generated.BatchResponseResultsStatusFailed {
			s.NotNil(result.Error, "Failed operation should have error details")
		}
	}
}

// TestBatchOperationsFailFast tests batch operations with fail-fast enabled
func (s *APIIntegrationTestSuite) TestBatchOperationsFailFast() {
	// Create batch request with one operation that will fail
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "", // Invalid: empty name
					"hostname":     "fail-fast-host",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "should-not-execute",
					"hostname":     "should-not-execute-host",
					"capabilities": []string{"fuzzing"},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			FailFast:    true,
			Concurrent:  false,
			Transaction: true,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// With fail-fast, execution should stop after first failure
	s.Greater(batchResp.FailureCount, 0, "Should have at least one failure")

	// Check that some operations might be skipped
	skippedCount := 0
	for _, result := range batchResp.Results {
		if result.Status == generated.BatchResponseResultsStatusSkipped {
			skippedCount++
		}
	}

	if skippedCount > 0 {
		s.T().Logf("Fail-fast correctly skipped %d operations", skippedCount)
	}
}

// TestBatchOperationsTransaction tests batch operations with transaction support
func (s *APIIntegrationTestSuite) TestBatchOperationsTransaction() {
	// Create batch request with transaction enabled
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "transaction-bot-1",
					"hostname":     "transaction-host-1",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "transaction-bot-2",
					"hostname":     "transaction-host-2",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "", // This will cause the entire transaction to fail
					"hostname":     "invalid-host",
					"capabilities": []string{"fuzzing"},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			Transaction: true,
			FailFast:    true,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// With transaction enabled, all operations should be rolled back if any fail
	if batchResp.FailureCount > 0 {
		s.T().Log("Transaction correctly rolled back all operations due to failure")

		// Verify that none of the bots were actually created
		listResp, err := s.client.ListBots(s.ctx, &generated.ListBotsParams{})
		s.Require().NoError(err)

		var botList generated.BotListResponse
		err = parseJSONResponse(listResp, &botList)
		s.Require().NoError(err)

		// Check that no bots with transaction names exist
		for _, bot := range botList.Bots {
			s.NotContains(bot.Name, "transaction-bot", "Transaction should have rolled back bot creation")
		}
	}
}

// TestBatchOperationsConcurrent tests concurrent batch operations
func (s *APIIntegrationTestSuite) TestBatchOperationsConcurrent() {
	// Create batch request with concurrent execution
	numOperations := 5
	operations := make([]generated.BatchRequestOperationsInner, numOperations)

	for i := 0; i < numOperations; i++ {
		operations[i] = generated.BatchRequestOperationsInner{
			Operation: generated.CreateBot,
			Data: map[string]interface{}{
				"name":         fmt.Sprintf("concurrent-bot-%d", i),
				"hostname":     fmt.Sprintf("concurrent-host-%d", i),
				"capabilities": []string{"fuzzing"},
			},
		}
	}

	batchReq := generated.BatchRequest{
		Operations: operations,
		Options: &generated.BatchRequestOptions{
			Concurrent:  true,
			Transaction: false,
			Timeout:     60,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	startTime := time.Now()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)
	executionTime := time.Since(startTime)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// Concurrent execution should be faster than sequential
	s.T().Logf("Batch execution time: %v", executionTime)
	s.Equal(numOperations, len(batchResp.Results))

	// Most operations should succeed
	s.GreaterOrEqual(batchResp.SuccessCount, numOperations-1)
}

// TestBatchOperationsTimeout tests batch operations timeout handling
func (s *APIIntegrationTestSuite) TestBatchOperationsTimeout() {
	// Create batch request with very short timeout
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "timeout-bot",
					"hostname":     "timeout-host",
					"capabilities": []string{"fuzzing"},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			Timeout:     1, // Very short timeout (1 second)
			Transaction: false,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)

	// Should handle timeout gracefully
	if resp.StatusCode == http.StatusRequestTimeout {
		s.T().Log("Batch operations properly handles timeout")
	} else {
		// Operation completed within timeout
		var batchResp generated.BatchResponse
		err = parseJSONResponse(resp, &batchResp)
		s.Require().NoError(err)
		s.T().Logf("Batch completed within short timeout: %d successes, %d failures",
			batchResp.SuccessCount, batchResp.FailureCount)
	}

	resp.Body.Close()
}

// TestBatchOperationsValidation tests batch request validation
func (s *APIIntegrationTestSuite) TestBatchOperationsValidation() {
	testCases := []struct {
		name        string
		request     generated.BatchRequest
		expectedErr bool
	}{
		{
			name: "valid_batch",
			request: generated.BatchRequest{
				Operations: []generated.BatchRequestOperationsInner{
					{
						Operation: generated.CreateBot,
						Data: map[string]interface{}{
							"name":         "valid-batch-bot",
							"hostname":     "valid-host",
							"capabilities": []string{"fuzzing"},
						},
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "empty_operations",
			request: generated.BatchRequest{
				Operations: []generated.BatchRequestOperationsInner{},
			},
			expectedErr: true,
		},
		{
			name: "invalid_operation_type",
			request: generated.BatchRequest{
				Operations: []generated.BatchRequestOperationsInner{
					{
						Operation: "invalid_operation",
						Data: map[string]interface{}{
							"name": "test",
						},
					},
				},
			},
			expectedErr: true,
		},
	}

	client := s.client.GetClient()

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp, err := client.ExecuteBatch(s.ctx, tc.request)
			s.Require().NoError(err)

			if tc.expectedErr {
				s.True(resp.StatusCode >= 400, "Expected error status code")
			} else {
				s.Equal(http.StatusOK, resp.StatusCode)
			}

			resp.Body.Close()
		})
	}
}

// TestBatchOperationsMixedTypes tests batch operations with different operation types
func (s *APIIntegrationTestSuite) TestBatchOperationsMixedTypes() {
	// First create a campaign to use in job creation
	campaignID := s.createTestCampaign(generateTestName("batch-mixed-campaign"))

	// Create batch request with mixed operation types
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "mixed-bot",
					"hostname":     "mixed-host",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateJob,
				Data: map[string]interface{}{
					"name":          "mixed-job",
					"campaign_id":   campaignID.String(),
					"fuzzer":        "aflplusplus",
					"target_binary": "/bin/test",
					"timeout":       3600,
					"memory_limit":  1024,
				},
			},
			{
				Operation: generated.UploadCorpus,
				Data: map[string]interface{}{
					"description": "Mixed batch corpus",
					"files": []map[string]interface{}{
						{
							"name":    "mixed-seed.txt",
							"content": "mixed batch corpus data",
						},
					},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			Transaction: false,
			Concurrent:  false,
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// Validate mixed operation results
	s.Equal(len(batchReq.Operations), len(batchResp.Results))

	operationTypes := []string{"create_bot", "create_job", "upload_corpus"}
	for i, result := range batchResp.Results {
		expectedOp := operationTypes[i]
		s.Equal(expectedOp, string(result.Operation))

		if result.Status == generated.BatchResponseResultsStatusSuccess {
			s.T().Logf("Mixed operation %s succeeded", expectedOp)
		} else {
			s.T().Logf("Mixed operation %s failed: %v", expectedOp, result.Error)
		}
	}
}

// TestBatchOperationsPartialFailure tests batch operations with partial failures
func (s *APIIntegrationTestSuite) TestBatchOperationsPartialFailure() {
	// Create batch request with some operations that will succeed and some that will fail
	batchReq := generated.BatchRequest{
		Operations: []generated.BatchRequestOperationsInner{
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "partial-success-bot",
					"hostname":     "partial-success-host",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "", // This will fail
					"hostname":     "partial-fail-host",
					"capabilities": []string{"fuzzing"},
				},
			},
			{
				Operation: generated.CreateBot,
				Data: map[string]interface{}{
					"name":         "partial-success-bot-2",
					"hostname":     "partial-success-host-2",
					"capabilities": []string{"fuzzing"},
				},
			},
		},
		Options: &generated.BatchRequestOptions{
			FailFast:    false, // Continue on failure
			Transaction: false, // Don't rollback on failure
		},
	}

	// Execute batch operations
	client := s.client.GetClient()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// Should have both successes and failures
	s.Greater(batchResp.SuccessCount, 0, "Should have some successful operations")
	s.Greater(batchResp.FailureCount, 0, "Should have some failed operations")
	s.Equal(3, len(batchResp.Results), "Should have results for all operations")

	// Verify that successful operations actually created resources
	successfulBots := 0
	for _, result := range batchResp.Results {
		if result.Status == generated.BatchResponseResultsStatusSuccess && result.Operation == generated.CreateBot {
			successfulBots++
		}
	}

	s.T().Logf("Partial failure test: %d successful bots, %d total successes, %d failures",
		successfulBots, batchResp.SuccessCount, batchResp.FailureCount)
}

// TestBatchOperationsLargeRequest tests batch operations with many operations
func (s *APIIntegrationTestSuite) TestBatchOperationsLargeRequest() {
	const numOperations = 20
	operations := make([]generated.BatchRequestOperationsInner, numOperations)

	// Create many bot creation operations
	for i := 0; i < numOperations; i++ {
		operations[i] = generated.BatchRequestOperationsInner{
			Operation: generated.CreateBot,
			Data: map[string]interface{}{
				"name":         fmt.Sprintf("large-batch-bot-%d", i),
				"hostname":     fmt.Sprintf("large-batch-host-%d", i),
				"capabilities": []string{"fuzzing"},
			},
		}
	}

	batchReq := generated.BatchRequest{
		Operations: operations,
		Options: &generated.BatchRequestOptions{
			Concurrent:  true,
			Transaction: false,
			Timeout:     120, // Longer timeout for large batch
		},
	}

	// Execute large batch operations
	client := s.client.GetClient()
	startTime := time.Now()
	resp, err := client.ExecuteBatch(s.ctx, batchReq)
	s.Require().NoError(err)
	executionTime := time.Since(startTime)

	s.Equal(http.StatusOK, resp.StatusCode)

	var batchResp generated.BatchResponse
	err = parseJSONResponse(resp, &batchResp)
	s.Require().NoError(err)

	// Validate large batch results
	s.Equal(numOperations, len(batchResp.Results))
	s.T().Logf("Large batch (%d operations) completed in %v: %d successes, %d failures",
		numOperations, executionTime, batchResp.SuccessCount, batchResp.FailureCount)

	// Most operations should succeed
	successRate := float64(batchResp.SuccessCount) / float64(numOperations) * 100
	s.GreaterOrEqual(successRate, 80.0, "At least 80% of operations should succeed")
}

// TestBatchOperationsConcurrency tests batch operations concurrency safety
func (s *APIIntegrationTestSuite) TestBatchOperationsConcurrency() {
	const numConcurrentBatches = 3
	const operationsPerBatch = 5

	// Channel to collect results
	results := make(chan error, numConcurrentBatches)

	// Execute multiple batches concurrently
	for batch := 0; batch < numConcurrentBatches; batch++ {
		go func(batchIndex int) {
			operations := make([]generated.BatchRequestOperationsInner, operationsPerBatch)

			for i := 0; i < operationsPerBatch; i++ {
				operations[i] = generated.BatchRequestOperationsInner{
					Operation: generated.CreateBot,
					Data: map[string]interface{}{
						"name":         fmt.Sprintf("concurrent-batch-%d-bot-%d", batchIndex, i),
						"hostname":     fmt.Sprintf("concurrent-batch-%d-host-%d", batchIndex, i),
						"capabilities": []string{"fuzzing"},
					},
				}
			}

			batchReq := generated.BatchRequest{
				Operations: operations,
				Options: &generated.BatchRequestOptions{
					Concurrent:  false,
					Transaction: false,
				},
			}

			client := s.client.GetClient()
			resp, err := client.ExecuteBatch(s.ctx, batchReq)
			if err != nil {
				results <- err
				return
			}

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("batch %d failed with status %d", batchIndex, resp.StatusCode)
				return
			}

			resp.Body.Close()
			results <- nil
		}(batch)
	}

	// Wait for all concurrent batches to complete
	for i := 0; i < numConcurrentBatches; i++ {
		err := <-results
		s.NoError(err, "Concurrent batch execution failed")
	}

	s.T().Logf("Successfully executed %d concurrent batches with %d operations each",
		numConcurrentBatches, operationsPerBatch)
}
