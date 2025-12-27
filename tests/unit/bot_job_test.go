package unit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/api/v1/bots/%s/job", botID) {
			callCount++
			// Return JSON null to indicate no job is available
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("null"))
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

	logger := logrus.New()
	client, err := bot.NewRetryClient(cfg, logger)
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

	logger := logrus.New()
	client, err := bot.NewRetryClient(cfg, logger)
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
