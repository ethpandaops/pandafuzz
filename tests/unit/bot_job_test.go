package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pandafuzz/pkg/bot"
	"pandafuzz/pkg/common"
)

// TestBotReceiveJob tests the bot receiving and processing jobs from master
func TestBotReceiveJob(t *testing.T) {
	botID := "test-bot-123"
	jobID := "job-456"
	
	// Mock job data
	mockJob := &common.Job{
		ID:          jobID,
		CampaignID:  "campaign-1",
		BotID:       botID,
		Status:      common.JobStatusPending,
		FuzzerType:  "afl++",
		TargetPath:  "/test/target",
		Duration:    time.Hour,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Config: common.JobConfig{
			Arguments:      []string{"-i", "input", "-o", "output"},
			EnvironmentVars: map[string]string{"AFL_NO_UI": "1"},
			Timeout:        3600,
			MemoryLimit:    2048,
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

		case fmt.Sprintf("/api/v1/jobs/%s/results", jobID):
			// Receive job results
			var results common.JobResult
			err := json.NewDecoder(r.Body).Decode(&results)
			require.NoError(t, err)
			
			assert.Equal(t, jobID, results.JobID)
			assert.NotZero(t, results.TotalExecs)
			assert.NotNil(t, results.Coverage)
			
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "results_received"})

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
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
			JobExecution:        time.Minute,
		},
	}

	// Create bot client
	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	// Test 1: Fetch pending job
	t.Run("fetch pending job", func(t *testing.T) {
		job, err := client.GetPendingJob(ctx, botID)
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

// TestBotJobExecutionFlow tests the complete job execution workflow
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
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
			HeartbeatInterval:   100 * time.Millisecond,
		},
	}

	// Simulate job execution flow
	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	// Step 1: Get pending job
	job, err := client.GetPendingJob(ctx, botID)
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
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	// Try to get a job - should return nil without error
	job, err := client.GetPendingJob(ctx, botID)
	assert.NoError(t, err)
	assert.Nil(t, job)
	assert.Equal(t, 1, callCount)
}

// TestBotJobTimeout tests handling of job execution timeout
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
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
			JobExecution:        100 * time.Millisecond, // Very short timeout
		},
	}

	client := bot.NewRetryClient(cfg)
	
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.JobExecution)
	defer cancel()

	// Start job
	err := client.UpdateJobStatus(ctx, jobID, common.JobStatusRunning, "")
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
			var metadata common.CrashInfo
			err = json.Unmarshal([]byte(crashData), &metadata)
			require.NoError(t, err)
			
			assert.Equal(t, crashID, metadata.ID)
			assert.Equal(t, "SIGSEGV", metadata.Signal)
			assert.NotEmpty(t, metadata.Stacktrace)
			
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
		Timeouts: common.BotTimeouts{
			ResultUpload: time.Second,
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	// Create crash info
	crashInfo := &common.CrashInfo{
		ID:         crashID,
		JobID:      jobID,
		BotID:      botID,
		Signal:     "SIGSEGV",
		Stacktrace: []string{
			"#0 0x00007f8b4c4a5520 in __GI_raise",
			"#1 0x00007f8b4c4a6b01 in __GI_abort",
			"#2 0x0000000000401234 in vulnerable_function",
		},
		InputSize: 1024,
		Timestamp: time.Now(),
	}

	// Simulate crash file data
	crashData := []byte("CRASH_INPUT_DATA_THAT_TRIGGERS_BUG")

	// Report crash
	err := client.ReportCrash(ctx, jobID, crashInfo, crashData)
	require.NoError(t, err)
}

// BenchmarkJobProcessing benchmarks job processing operations
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
		Timeouts: common.BotTimeouts{
			MasterCommunication: time.Second,
		},
	}

	client := bot.NewRetryClient(cfg)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Get job
		job, _ := client.GetPendingJob(ctx, cfg.ID)
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