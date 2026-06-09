package unit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// TestBotHandleNoJobs tests bot behavior when no jobs are available
func TestBotHandleNoJobs(t *testing.T) {
	botID := "idle-bot"
	callCount := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/api/v1/bots/%s/jobs/next", botID)
		if r.URL.Path != expectedPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		assert.Equal(t, http.MethodPost, r.Method)
		callCount++
		// Return empty job wrapper to indicate no job is available
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"job":     nil,
			"message": "no jobs available",
		})
	})

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: "http://master.test",
		Timeouts: common.BotTimeoutConfig{
			MasterCommunication: time.Second,
		},
	}

	logger := logrus.New()
	httpClient := newHandlerClient(handler, time.Second)
	client, err := bot.NewRetryClientWithHTTPClient(cfg, logger, httpClient)
	require.NoError(t, err)

	// Try to get a job - should return a job with an empty ID without error
	job, err := client.GetJob(botID)
	require.NoError(t, err)
	if job != nil {
		assert.Empty(t, job.ID)
	}
	assert.Equal(t, 1, callCount)
}

// TestBotCrashReporting tests crash artifact reporting
func TestBotCrashReporting(t *testing.T) {
	botID := "crash-bot"
	jobID := "crash-job"
	crashID := "crash-001"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/results/crash" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		assert.Equal(t, http.MethodPost, r.Method)

		var payload common.CrashResult
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		assert.Equal(t, crashID, payload.ID)
		assert.Equal(t, 11, payload.Signal)
		assert.NotEmpty(t, payload.StackTrace)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"crash_id": crashID,
			"status":   "stored",
		})
	})

	cfg := &common.BotConfig{
		ID:        botID,
		MasterURL: "http://master.test",
		Timeouts: common.BotTimeoutConfig{
			ResultReporting: time.Second,
		},
	}

	logger := logrus.New()
	httpClient := newHandlerClient(handler, time.Second)
	client, err := bot.NewRetryClientWithHTTPClient(cfg, logger, httpClient)
	require.NoError(t, err)

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
