package unit

import (
	// "context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	// "strings"
	// "sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// TestBotReceiveJob tests the bot receiving and processing jobs from master
// TODO: This test needs to be rewritten to use the actual API endpoints
// The test currently uses non-existent types like JobResult and methods like SubmitJobResults
/*
func TestBotReceiveJob(t *testing.T) {
	botID := "test-bot-123"
	jobID := "job-456"
	
	// Mock job data
	mockJob := &common.Job{
		ID:          jobID,
		Name:        "test-job",
		AssignedBot: &botID,
		Status:      common.JobStatusPending,
		Fuzzer:      "afl++",
		Target:      "/test/target",
		CreatedAt:   time.Now(),
		TimeoutAt:   time.Now().Add(time.Hour),
		WorkDir:     "/tmp/pandafuzz/job_" + jobID,
		Config: common.JobConfig{
			Duration:    time.Hour,
			MemoryLimit: 2048,
			Timeout:     3600 * time.Second,
		},
	}

	// Track API calls
	apiCalls := make(map[string]int)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		apiCalls[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case fmt.Sprintf("/api/v1/bots/%s/jobs/pending", botID):
			// Return pending job
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockJob)

		case fmt.Sprintf("/api/v1/jobs/%s/status", jobID):
			// Update job status
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)
			
			status, ok := req["status"].(string)
			assert.True(t, ok, "Status should be a string")
			assert.Contains(t, []string{
				string(common.JobStatusRunning),
				string(common.JobStatusCompleted),
				string(common.JobStatusFailed),
			}, status)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

		case "/api/v1/results/crash":
			// Receive crash results
			var crash common.CrashResult
			err := json.NewDecoder(r.Body).Decode(&crash)
			require.NoError(t, err)
			
			assert.Equal(t, jobID, crash.JobID)
			assert.NotEmpty(t, crash.Hash)
			
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "crash_received"})
			
		case "/api/v1/results/coverage":
			// Receive coverage results
			var coverage common.CoverageResult
			err := json.NewDecoder(r.Body).Decode(&coverage)
			require.NoError(t, err)
			
			assert.Equal(t, jobID, coverage.JobID)
			assert.NotZero(t, coverage.Edges)
			
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "coverage_received"})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create bot configuration
	cfg := &common.BotConfig{
		ID:           botID,
		MasterURL:    server.URL,
		Capabilities: []string{"afl++"},
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
			JobExecution:        time.Minute,
		},
	}

	// Create bot client
	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)
	ctx := context.Background()

	// Test 1: Fetch pending job
	t.Run("fetch pending job", func(t *testing.T) {
		job, err := client.GetJob(botID)
		require.NoError(t, err)
		require.NotNil(t, job)
		
		assert.Equal(t, jobID, job.ID)
		assert.Equal(t, botID, job.BotID)
		assert.Equal(t, "afl++", job.FuzzerType)
		assert.Equal(t, common.JobStatusPending, job.Status)
	})

	// Test 2: Update job status to running
	t.Run("update job status to running", func(t *testing.T) {
		err := client.UpdateJobStatus(ctx, jobID, common.JobStatusRunning, "")
		require.NoError(t, err)
		
		mu.Lock()
		assert.Equal(t, 1, apiCalls[fmt.Sprintf("/api/v1/jobs/%s/status", jobID)])
		mu.Unlock()
	})

	// Test 3: Submit job results
	t.Run("submit job results", func(t *testing.T) {
		results := &common.JobResult{
			JobID:      jobID,
			BotID:      botID,
			StartTime:  time.Now().Add(-time.Hour),
			EndTime:    time.Now(),
			TotalExecs: 1000000,
			ExecsPerSec: 50000,
			Coverage: &common.CoverageStats{
				Lines:     1500,
				Functions: 200,
				Branches:  800,
			},
			CrashesFound: 3,
			TimeoutsFound: 1,
			Metrics: map[string]interface{}{
				"peak_memory_mb": 512,
				"cpu_usage_avg":  85.5,
			},
		}

		err := client.SubmitJobResults(ctx, jobID, results)
		require.NoError(t, err)
		
		mu.Lock()
		assert.Equal(t, 1, apiCalls[fmt.Sprintf("/api/v1/jobs/%s/results", jobID)])
		mu.Unlock()
	})

	// Verify all expected API calls were made
	mu.Lock()
	assert.Equal(t, 1, apiCalls[fmt.Sprintf("/api/v1/bots/%s/jobs/pending", botID)])
	assert.Equal(t, 1, apiCalls[fmt.Sprintf("/api/v1/jobs/%s/status", jobID)])
	assert.Equal(t, 1, apiCalls[fmt.Sprintf("/api/v1/jobs/%s/results", jobID)])
	mu.Unlock()
}
*/

// TestBotJobExecutionFlow tests the complete job execution workflow
// TODO: This test also needs to be rewritten - uses JobResult and JobProgress types
/*
func TestBotJobExecutionFlow(t *testing.T) {
	botID := "executor-bot"
	jobID := "exec-job-1"
	
	jobStates := make(map[string]string)
	var stateMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fmt.Sprintf("/api/v1/bots/%s/jobs/pending", botID):
			stateMu.Lock()
			jobState := jobStates[jobID]
			stateMu.Unlock()

			if jobState == "" {
				// First call - return pending job
				job := &common.Job{
					ID:         jobID,
					BotID:      botID,
					Status:     common.JobStatusPending,
					FuzzerType: "libfuzzer",
					TargetPath: "/test/harness",
					Duration:   10 * time.Second,
					Config: common.JobConfig{
						Arguments:   []string{"-max_total_time=10"},
						MemoryLimit: 1024,
					},
				}
				json.NewEncoder(w).Encode(job)
				
				stateMu.Lock()
				jobStates[jobID] = "assigned"
				stateMu.Unlock()
			} else {
				// No more pending jobs
				w.WriteHeader(http.StatusNoContent)
			}

		case fmt.Sprintf("/api/v1/jobs/%s/status", jobID):
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)
			
			stateMu.Lock()
			jobStates[jobID] = req["status"].(string)
			stateMu.Unlock()
			
			w.WriteHeader(http.StatusOK)

		case fmt.Sprintf("/api/v1/jobs/%s/progress", jobID):
			// Progress update
			var progress common.JobProgress
			json.NewDecoder(r.Body).Decode(&progress)
			
			assert.NotZero(t, progress.CurrentExecs)
			assert.NotZero(t, progress.Coverage)
			
			w.WriteHeader(http.StatusOK)

		case fmt.Sprintf("/api/v1/jobs/%s/results", jobID):
			// Final results
			var results common.JobResult
			json.NewDecoder(r.Body).Decode(&results)
			
			stateMu.Lock()
			jobStates[jobID] = "completed"
			stateMu.Unlock()
			
			w.WriteHeader(http.StatusOK)

		case fmt.Sprintf("/api/v1/bots/%s/heartbeat", botID):
			// Heartbeat during execution
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(common.HeartbeatResponse{
				Status: "acknowledged",
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:           botID,
		MasterURL:    server.URL,
		Capabilities: []string{"libfuzzer"},
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
			HeartbeatInterval:   100 * time.Millisecond,
		},
	}

	// Simulate job execution flow
	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)
	ctx := context.Background()

	// Step 1: Get pending job
	job, err := client.GetJob(botID)
	require.NoError(t, err)
	require.NotNil(t, job)

	// Step 2: Update status to running
	err = client.UpdateJobStatus(ctx, job.ID, common.JobStatusRunning, "Starting fuzzer")
	require.NoError(t, err)

	// Step 3: Send progress updates (simulate fuzzing)
	for i := 0; i < 3; i++ {
		progress := &common.JobProgress{
			JobID:        job.ID,
			CurrentExecs: uint64((i + 1) * 100000),
			ExecsPerSec:  50000,
			Coverage: &common.CoverageStats{
				Lines:     uint64(100 + i*50),
				Functions: uint64(20 + i*10),
				Branches:  uint64(50 + i*25),
			},
			CrashesFound: uint64(i),
			UpdatedAt:    time.Now(),
		}
		
		err = client.UpdateJobProgress(ctx, job.ID, progress)
		require.NoError(t, err)
		
		time.Sleep(50 * time.Millisecond) // Simulate work
	}

	// Step 4: Submit final results
	results := &common.JobResult{
		JobID:         job.ID,
		BotID:         botID,
		StartTime:     time.Now().Add(-time.Minute),
		EndTime:       time.Now(),
		TotalExecs:    300000,
		ExecsPerSec:   5000,
		CrashesFound:  2,
		TimeoutsFound: 0,
		Coverage: &common.CoverageStats{
			Lines:     250,
			Functions: 50,
			Branches:  125,
		},
	}

	err = client.SubmitJobResults(ctx, job.ID, results)
	require.NoError(t, err)

	// Verify final state
	stateMu.Lock()
	assert.Equal(t, "completed", jobStates[jobID])
	stateMu.Unlock()
}
*/

// TestBotHandleNoJobs tests bot behavior when no jobs are available
func TestBotHandleNoJobs(t *testing.T) {
	botID := "idle-bot"
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/api/v1/bots/%s/jobs/pending", botID) {
			callCount++
			// Always return no content (no jobs available)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: server.URL,
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
		},
	}

	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)

	// Try to get a job - should return nil without error
	job, err := client.GetJob(botID)
	assert.NoError(t, err)
	assert.Nil(t, job)
	assert.Equal(t, 1, callCount)
}

// TestBotJobTimeout tests handling of job execution timeout
// TODO: This test uses UpdateJobStatus which doesn't exist on RetryClient
/*
func TestBotJobTimeout(t *testing.T) {
	botID := "timeout-bot"
	jobID := "timeout-job"
	statusUpdates := []string{}
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fmt.Sprintf("/api/v1/jobs/%s/status", jobID):
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)
			
			mu.Lock()
			statusUpdates = append(statusUpdates, req["status"].(string))
			mu.Unlock()
			
			// Check if error message is provided for timeout
			if req["status"] == string(common.JobStatusFailed) {
				assert.Contains(t, req["error"], "timeout")
			}
			
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: server.URL,
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
			JobExecution:        100 * time.Millisecond, // Very short timeout
		},
	}

	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)
	
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.JobExecution)
	defer cancel()

	// Start job
	err = client.UpdateJobStatus(ctx, jobID, common.JobStatusRunning, "")
	require.NoError(t, err)

	// Wait for timeout
	<-ctx.Done()

	// Report timeout
	err = client.UpdateJobStatus(context.Background(), jobID, common.JobStatusFailed, "Job execution timeout")
	require.NoError(t, err)

	// Verify status updates
	mu.Lock()
	assert.Equal(t, 2, len(statusUpdates))
	assert.Equal(t, string(common.JobStatusRunning), statusUpdates[0])
	assert.Equal(t, string(common.JobStatusFailed), statusUpdates[1])
	mu.Unlock()
}
*/

// TestBotCrashReporting tests crash artifact reporting
func TestBotCrashReporting(t *testing.T) {
	botID := "crash-bot"
	jobID := "crash-job"
	crashID := "crash-001"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/api/v1/jobs/%s/crashes", jobID) {
			// Verify multipart form data
			err := r.ParseMultipartForm(10 << 20) // 10MB
			require.NoError(t, err)
			
			// Check crash metadata
			crashData := r.FormValue("metadata")
			var metadata common.CrashResult
			err = json.Unmarshal([]byte(crashData), &metadata)
			require.NoError(t, err)
			
			assert.Equal(t, crashID, metadata.ID)
			assert.Equal(t, 11, metadata.Signal) // SIGSEGV = 11
			assert.NotEmpty(t, metadata.StackTrace)
			
			// Check crash file
			file, header, err := r.FormFile("crash_file")
			require.NoError(t, err)
			defer file.Close()
			
			assert.Equal(t, "crash_001.bin", header.Filename)
			
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"crash_id": crashID,
				"status":   "stored",
			})
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: server.URL,
		Timeouts: common.BotTimeoutConfig{
			ResultReporting: time.Second,
		},
	}

	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)

	// Create crash info
	crashInfo := &common.CrashResult{
		ID:         crashID,
		JobID:      jobID,
		BotID:      botID,
		Signal:     11, // SIGSEGV
		StackTrace: "#0 0x00007f8b4c4a5520 in __GI_raise\n#1 0x00007f8b4c4a6b01 in __GI_abort\n#2 0x0000000000401234 in vulnerable_function",
		Size:       1024,
		Timestamp:  time.Now(),
		Type:       "segfault",
		Hash:       "deadbeef",
		FilePath:   "/tmp/crash-" + crashID + ".input",
	}

	// Report crash
	err = client.ReportCrash(crashInfo)
	require.NoError(t, err)
}

// BenchmarkJobProcessing benchmarks job processing operations
// TODO: This benchmark also needs to be rewritten - uses JobResult
/*
func BenchmarkJobProcessing(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/jobs/pending"):
			job := &common.Job{
				ID:         "bench-job",
				BotID:      "bench-bot",
				Status:     common.JobStatusPending,
				FuzzerType: "afl++",
			}
			json.NewEncoder(w).Encode(job)
			
		case strings.Contains(r.URL.Path, "/status"):
			w.WriteHeader(http.StatusOK)
			
		case strings.Contains(r.URL.Path, "/results"):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := &common.BotConfig{
		ID:        "bench-bot",
		MasterURL: server.URL,
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
		},
	}

	client, err := bot.NewRetryClient(cfg); require.NoError(t, err)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Get job
		job, _ := client.GetJob(cfg.ID)
		if job != nil {
			// Update status
			client.UpdateJobStatus(ctx, job.ID, common.JobStatusRunning, "")
			
			// Submit results
			results := &common.JobResult{
				JobID:      job.ID,
				TotalExecs: 100000,
			}
			client.SubmitJobResults(ctx, job.ID, results)
		}
	}
}
*/