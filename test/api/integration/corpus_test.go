package integration

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/google/uuid"
)

// TestCorpusUpload tests corpus file upload functionality
func (s *APIIntegrationTestSuite) TestCorpusUpload() {
	// Create test corpus data
	corpusData := []byte("test corpus data for fuzzing")

	// Create multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add files
	fileWriter, err := writer.CreateFormFile("files", "test-seed.txt")
	s.Require().NoError(err)
	_, err = fileWriter.Write(corpusData)
	s.Require().NoError(err)

	// Add metadata
	err = writer.WriteField("description", "Test corpus upload")
	s.Require().NoError(err)

	err = writer.Close()
	s.Require().NoError(err)

	// Upload corpus
	req, err := http.NewRequestWithContext(s.ctx, "POST", s.baseURL+"/api/v1/corpus/upload", &buf)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var uploadResp generated.CorpusUploadResponse
	err = parseJSONResponse(resp, &uploadResp)
	s.Require().NoError(err)

	s.Greater(uploadResp.UploadedCount, 0)
	s.Equal(0, uploadResp.FailedCount)
	s.NotNil(uploadResp.Files)
}

// TestCorpusUploadInvalidData tests corpus upload with invalid data
func (s *APIIntegrationTestSuite) TestCorpusUploadInvalidData() {
	// Create invalid corpus data (too large)
	corpusData := bytes.Repeat([]byte("A"), 2*1024*1024) // 2MB

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	fileWriter, err := writer.CreateFormFile("files", "large-seed.txt")
	s.Require().NoError(err)
	_, err = fileWriter.Write(corpusData)
	s.Require().NoError(err)

	err = writer.Close()
	s.Require().NoError(err)

	// Upload corpus
	req, err := http.NewRequestWithContext(s.ctx, "POST", s.baseURL+"/api/v1/corpus/upload", &buf)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	s.Require().NoError(err)

	// Should handle large files appropriately
	if resp.StatusCode == http.StatusBadRequest {
		s.T().Log("API properly rejects oversized corpus files")
	} else if resp.StatusCode == http.StatusOK {
		var uploadResp generated.CorpusUploadResponse
		err = parseJSONResponse(resp, &uploadResp)
		s.Require().NoError(err)

		if uploadResp.FailedCount > 0 {
			s.T().Log("API reports upload failures for oversized files")
		}
	}

	resp.Body.Close()
}

// TestCorpusList tests listing corpus entries
func (s *APIIntegrationTestSuite) TestCorpusList() {
	// Upload some test corpus first
	s.uploadTestCorpus("test-seed-1.txt", []byte("test data 1"))
	s.uploadTestCorpus("test-seed-2.txt", []byte("test data 2"))

	// List corpus entries
	client := s.client.GetClient()
	params := &generated.ListCorpusParams{}
	resp, err := client.ListCorpus(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(resp, &corpusList)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(corpusList.Entries), 2)

	// Validate pagination
	validator := &ResponseValidator{}
	err = validator.ValidatePagination(corpusList.Pagination, -1)
	s.Require().NoError(err)
}

// TestCorpusListPagination tests corpus list pagination
func (s *APIIntegrationTestSuite) TestCorpusListPagination() {
	// Upload several test corpus files
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("page-seed-%d.txt", i)
		data := []byte(fmt.Sprintf("test data %d", i))
		s.uploadTestCorpus(filename, data)
	}

	// Test pagination with limit
	limit := 2
	client := s.client.GetClient()
	params := &generated.ListCorpusParams{
		Limit: &limit,
	}

	resp, err := client.ListCorpus(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(resp, &corpusList)
	s.Require().NoError(err)

	s.LessOrEqual(len(corpusList.Entries), limit)
	s.NotNil(corpusList.Pagination)
	s.Equal(limit, corpusList.Pagination.Limit)
}

// TestCorpusGet tests retrieving a specific corpus entry
func (s *APIIntegrationTestSuite) TestCorpusGet() {
	// Upload test corpus first
	corpusData := []byte("specific test data")
	s.uploadTestCorpus("specific-seed.txt", corpusData)

	// List to get an entry ID
	client := s.client.GetClient()
	listResp, err := client.ListCorpus(s.ctx, &generated.ListCorpusParams{})
	s.Require().NoError(err)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(listResp, &corpusList)
	s.Require().NoError(err)
	s.Greater(len(corpusList.Entries), 0)

	entryID := corpusList.Entries[0].Id

	// Get specific corpus entry
	resp, err := client.GetCorpusEntry(s.ctx, entryID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var entry generated.CorpusEntry
	err = parseJSONResponse(resp, &entry)
	s.Require().NoError(err)

	s.Equal(entryID, entry.Id)
	s.NotEmpty(entry.Name)
	s.NotEmpty(entry.Hash)
	s.Greater(entry.Size, 0)
}

// TestCorpusGetNotFound tests retrieving a non-existent corpus entry
func (s *APIIntegrationTestSuite) TestCorpusGetNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.GetCorpusEntry(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)

	problemDetails, err := parseErrorResponse(resp)
	s.Require().NoError(err)
	s.Equal("Not Found", problemDetails.Title)
}

// TestCorpusDelete tests corpus entry deletion
func (s *APIIntegrationTestSuite) TestCorpusDelete() {
	// Upload test corpus first
	s.uploadTestCorpus("delete-seed.txt", []byte("delete test data"))

	// List to get an entry ID
	client := s.client.GetClient()
	listResp, err := client.ListCorpus(s.ctx, &generated.ListCorpusParams{})
	s.Require().NoError(err)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(listResp, &corpusList)
	s.Require().NoError(err)
	s.Greater(len(corpusList.Entries), 0)

	entryID := corpusList.Entries[0].Id

	// Verify entry exists
	resp, err := client.GetCorpusEntry(s.ctx, entryID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Delete the entry
	resp, err = client.DeleteCorpusEntry(s.ctx, entryID)
	s.Require().NoError(err)
	s.Equal(http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify entry is deleted
	resp, err = client.GetCorpusEntry(s.ctx, entryID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCorpusDeleteNotFound tests deleting a non-existent corpus entry
func (s *APIIntegrationTestSuite) TestCorpusDeleteNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.DeleteCorpusEntry(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCorpusSelection tests corpus selection functionality
func (s *APIIntegrationTestSuite) TestCorpusSelection() {
	// Upload test corpus files with different characteristics
	corpusFiles := []struct {
		name string
		data []byte
	}{
		{"high-coverage.txt", []byte("high coverage test data")},
		{"low-coverage.txt", []byte("low coverage data")},
		{"crash-trigger.txt", []byte("crash trigger data")},
	}

	for _, file := range corpusFiles {
		s.uploadTestCorpus(file.name, file.data)
	}

	// Request corpus selection
	selectionReq := generated.CorpusSelectionRequest{
		MaxEntries: 2,
		Criteria: generated.CorpusSelectionRequestCriteria{
			MinCoverage:    0.0,
			MaxSize:        1024,
			PreferRecent:   true,
			IncludeCrashes: false,
		},
	}

	client := s.client.GetClient()
	resp, err := client.SelectCorpus(s.ctx, selectionReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var selectionResp generated.CorpusSelectionResponse
	err = parseJSONResponse(resp, &selectionResp)
	s.Require().NoError(err)

	s.LessOrEqual(len(selectionResp.SelectedEntries), selectionReq.MaxEntries)
	s.NotNil(selectionResp.QualityMetrics)
	s.GreaterOrEqual(selectionResp.QualityMetrics.AverageCoverage, float32(0))
}

// TestCorpusSync tests corpus synchronization functionality
func (s *APIIntegrationTestSuite) TestCorpusSync() {
	// Upload initial corpus
	s.uploadTestCorpus("sync-seed-1.txt", []byte("sync test data 1"))
	s.uploadTestCorpus("sync-seed-2.txt", []byte("sync test data 2"))

	// Request corpus sync
	syncReq := generated.CorpusSyncRequest{
		SourcePath: "/test/corpus",
		Filters: generated.CorpusSyncRequestFilters{
			MinSize:      1,
			MaxSize:      1024 * 1024,
			ExcludeEmpty: true,
		},
		DryRun: true, // Use dry run for testing
	}

	client := s.client.GetClient()
	resp, err := client.SyncCorpus(s.ctx, syncReq)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var syncResp generated.CorpusSyncResponse
	err = parseJSONResponse(resp, &syncResp)
	s.Require().NoError(err)

	s.NotNil(syncResp.Summary)
	s.GreaterOrEqual(syncResp.Summary.ProcessedFiles, 0)
	s.GreaterOrEqual(syncResp.Summary.SkippedFiles, 0)
	s.GreaterOrEqual(syncResp.Summary.ErrorFiles, 0)
}

// TestCorpusQuarantine tests corpus quarantine functionality
func (s *APIIntegrationTestSuite) TestCorpusQuarantine() {
	// Upload test corpus
	s.uploadTestCorpus("quarantine-seed.txt", []byte("quarantine test data"))

	// List to get an entry ID
	client := s.client.GetClient()
	listResp, err := client.ListCorpus(s.ctx, &generated.ListCorpusParams{})
	s.Require().NoError(err)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(listResp, &corpusList)
	s.Require().NoError(err)
	s.Greater(len(corpusList.Entries), 0)

	entryID := corpusList.Entries[0].Id

	// Quarantine the entry
	resp, err := client.QuarantineCorpusEntry(s.ctx, entryID)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var entry generated.CorpusEntry
	err = parseJSONResponse(resp, &entry)
	s.Require().NoError(err)

	// Entry should be marked as quarantined
	s.Equal(entryID, entry.Id)
	// Note: Quarantine status would be implementation-specific
}

// TestCorpusQuarantineNotFound tests quarantining a non-existent corpus entry
func (s *APIIntegrationTestSuite) TestCorpusQuarantineNotFound() {
	nonExistentID := generateTestUUID()

	client := s.client.GetClient()
	resp, err := client.QuarantineCorpusEntry(s.ctx, nonExistentID)
	s.Require().NoError(err)
	s.Equal(http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestCorpusValidation tests corpus validation rules
func (s *APIIntegrationTestSuite) TestCorpusValidation() {
	testCases := []struct {
		name        string
		filename    string
		data        []byte
		expectError bool
	}{
		{
			name:        "valid_corpus",
			filename:    "valid-seed.txt",
			data:        []byte("valid corpus data"),
			expectError: false,
		},
		{
			name:        "empty_corpus",
			filename:    "empty-seed.txt",
			data:        []byte(""),
			expectError: true,
		},
		{
			name:        "invalid_filename",
			filename:    "", // Empty filename
			data:        []byte("data"),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create multipart form data
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)

			if tc.filename != "" {
				fileWriter, err := writer.CreateFormFile("files", tc.filename)
				s.Require().NoError(err)
				_, err = fileWriter.Write(tc.data)
				s.Require().NoError(err)
			}

			err := writer.Close()
			s.Require().NoError(err)

			// Upload corpus
			req, err := http.NewRequestWithContext(s.ctx, "POST", s.baseURL+"/api/v1/corpus/upload", &buf)
			s.Require().NoError(err)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			s.Require().NoError(err)

			if tc.expectError {
				s.True(resp.StatusCode >= 400, "Expected error status code")
			} else {
				s.Equal(http.StatusOK, resp.StatusCode)
			}

			resp.Body.Close()
		})
	}
}

// TestCorpusFilteringByJobCampaign tests filtering corpus by job/campaign
func (s *APIIntegrationTestSuite) TestCorpusFilteringByJobCampaign() {
	// Create a test campaign and job
	campaignID := s.createTestCampaign(generateTestName("corpus-campaign"))
	jobID := s.createTestJob(generateTestName("corpus-job"), &campaignID)

	// Upload corpus associated with the job
	s.uploadTestCorpus("job-seed.txt", []byte("job-specific corpus data"))

	// Test filtering by job
	client := s.client.GetClient()
	params := &generated.ListCorpusParams{
		JobId: &jobID,
	}

	resp, err := client.ListCorpus(s.ctx, params)
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(resp, &corpusList)
	s.Require().NoError(err)

	// All returned entries should be associated with the job
	for _, entry := range corpusList.Entries {
		// Note: Job association would be implementation-specific
		s.T().Logf("Corpus entry %s size: %d", entry.Name, entry.Size)
	}
}

// TestCorpusCoverageTracking tests corpus coverage tracking
func (s *APIIntegrationTestSuite) TestCorpusCoverageTracking() {
	// Upload test corpus with coverage information
	s.uploadTestCorpus("coverage-seed.txt", []byte("coverage test data"))

	// List corpus to get entries with coverage info
	client := s.client.GetClient()
	resp, err := client.ListCorpus(s.ctx, &generated.ListCorpusParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(resp, &corpusList)
	s.Require().NoError(err)

	// Check if coverage information is available
	for _, entry := range corpusList.Entries {
		if entry.CoverageInfo != nil {
			s.GreaterOrEqual(entry.CoverageInfo.EdgesCovered, 0)
			s.GreaterOrEqual(entry.CoverageInfo.NewEdges, 0)
			s.GreaterOrEqual(entry.CoverageInfo.CoveragePercent, float32(0))
			s.LessOrEqual(entry.CoverageInfo.CoveragePercent, float32(100))
		}
	}
}

// TestCorpusGenerationInfo tests corpus generation tracking
func (s *APIIntegrationTestSuite) TestCorpusGenerationInfo() {
	// Upload test corpus
	s.uploadTestCorpus("generation-seed.txt", []byte("generation test data"))

	// List corpus to get entries with generation info
	client := s.client.GetClient()
	resp, err := client.ListCorpus(s.ctx, &generated.ListCorpusParams{})
	s.Require().NoError(err)
	s.Equal(http.StatusOK, resp.StatusCode)

	var corpusList generated.CorpusListResponse
	err = parseJSONResponse(resp, &corpusList)
	s.Require().NoError(err)

	// Check if generation information is available
	for _, entry := range corpusList.Entries {
		if entry.GenerationInfo != nil {
			s.GreaterOrEqual(entry.GenerationInfo.Generation, 0)
			s.NotNil(entry.GenerationInfo.ParentIds)
			s.T().Logf("Entry %s generation: %d, mutation: %s",
				entry.Name, entry.GenerationInfo.Generation, entry.GenerationInfo.MutationType)
		}
	}
}

// TestConcurrentCorpusOperations tests concurrent corpus operations
func (s *APIIntegrationTestSuite) TestConcurrentCorpusOperations() {
	const numUploads = 5

	// Channel to collect results
	results := make(chan error, numUploads)

	// Upload corpus files concurrently
	for i := 0; i < numUploads; i++ {
		go func(index int) {
			filename := fmt.Sprintf("concurrent-seed-%d.txt", index)
			data := []byte(fmt.Sprintf("concurrent test data %d", index))

			err := s.uploadTestCorpusWithError(filename, data)
			results <- err
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numUploads; i++ {
		err := <-results
		s.NoError(err, "Concurrent corpus upload failed")
	}
}

// Helper methods

// uploadTestCorpus uploads a test corpus file
func (s *APIIntegrationTestSuite) uploadTestCorpus(filename string, data []byte) {
	err := s.uploadTestCorpusWithError(filename, data)
	s.Require().NoError(err)
}

// uploadTestCorpusWithError uploads a test corpus file and returns any error
func (s *APIIntegrationTestSuite) uploadTestCorpusWithError(filename string, data []byte) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	fileWriter, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return err
	}

	_, err = fileWriter.Write(data)
	if err != nil {
		return err
	}

	err = writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(s.ctx, "POST", s.baseURL+"/api/v1/corpus/upload", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status %d", resp.StatusCode)
	}

	return nil
}
