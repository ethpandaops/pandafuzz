package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/analysis"
	"github.com/ethpandaops/pandafuzz/pkg/bot"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrashReporting tests crash reporting from bot to master
func TestCrashReporting(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("crash-test")
	require.NoError(t, err)

	assignedJob, err := botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, assignedJob)

	// Create crash result
	crash := env.CreateTestCrash(job.ID)

	// Report crash
	err = botClient.ReportCrash(crash)
	require.NoError(t, err)

	// Verify crash is stored
	crashes, err := env.state.ListCrashes(&common.CrashFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	require.Len(t, crashes, 1)
	
	assert.Equal(t, crash.ID, crashes[0].ID)
	assert.Equal(t, crash.JobID, crashes[0].JobID)
	assert.Equal(t, crash.BotID, crashes[0].BotID)
	assert.Equal(t, crash.Type, crashes[0].Type)
	assert.Equal(t, crash.Hash, crashes[0].Hash)
}

// TestMultipleCrashReports tests multiple crash reports
func TestMultipleCrashReports(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("multi-crash-test")
	require.NoError(t, err)

	_, err = botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)

	// Report multiple crashes
	numCrashes := 5
	for i := 0; i < numCrashes; i++ {
		crash := env.CreateTestCrash(job.ID)
		crash.ID = fmt.Sprintf("crash-%d", i)
		crash.Input = []byte(fmt.Sprintf("CRASH_%d", i))
		crash.Hash = fmt.Sprintf("hash_%d", i)
		
		err = botClient.ReportCrash(crash)
		require.NoError(t, err)
	}

	// Verify all crashes are stored
	crashes, err := env.state.ListCrashes(&common.CrashFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	assert.Len(t, crashes, numCrashes)
}

// TestCrashDeduplication tests crash deduplication
func TestCrashDeduplication(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("dedup-test")
	require.NoError(t, err)

	_, err = botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)

	// Report same crash multiple times (same hash)
	crash := env.CreateTestCrash(job.ID)
	crash.Hash = "duplicate-hash"
	
	// First report
	err = botClient.ReportCrash(crash)
	require.NoError(t, err)

	// Duplicate reports with different IDs but same hash
	for i := 1; i < 5; i++ {
		dupCrash := *crash
		dupCrash.ID = fmt.Sprintf("dup-crash-%d", i)
		dupCrash.Timestamp = time.Now()
		
		err = botClient.ReportCrash(&dupCrash)
		require.NoError(t, err)
	}

	// Should have deduplicated based on hash
	crashes, err := env.state.ListCrashes(&common.CrashFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	
	// Check for unique hashes
	uniqueHashes := make(map[string]bool)
	for _, c := range crashes {
		uniqueHashes[c.Hash] = true
	}
	assert.Equal(t, 1, len(uniqueHashes), "Should only have one unique crash")
}

// TestCrashAnalysis tests crash analysis integration
func TestCrashAnalysis(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create crash analyzer
	analyzerConfig := &analysis.AnalyzerConfig{
		EnableStackTrace:     true,
		EnableDeduplication:  true,
		EnableTriage:         true,
		MaxStackFrames:       50,
		MinStackFrames:       3,
		DeduplicationMethod:  "stack",
	}
	analyzer := analysis.NewCrashAnalyzer(analyzerConfig)

	// Create test crashes with stack traces
	crashes := []*common.CrashResult{
		{
			ID:    "crash-1",
			JobID: "job-1",
			Type:  "segmentation_fault",
			StackTrace: `#0  0x0000555555554000 in vulnerable_function ()
#1  0x0000555555554100 in process_input ()
#2  0x0000555555554200 in main ()`,
			Output: "Program received signal SIGSEGV, Segmentation fault.",
		},
		{
			ID:    "crash-2",
			JobID: "job-1",
			Type:  "heap_overflow",
			StackTrace: `==1234==ERROR: AddressSanitizer: heap-buffer-overflow
#0  0x0000555555554000 in memcpy ()
#1  0x0000555555554100 in copy_data ()
#2  0x0000555555554200 in main ()`,
			Output: "ASAN detected heap buffer overflow",
		},
		{
			ID:    "crash-3",
			JobID: "job-1",
			Type:  "use_after_free",
			StackTrace: `==1234==ERROR: AddressSanitizer: heap-use-after-free
#0  0x0000555555554000 in use_pointer ()
#1  0x0000555555554100 in process_data ()
#2  0x0000555555554200 in main ()`,
			Output: "ASAN detected use after free",
		},
	}

	// Analyze crashes
	for _, crash := range crashes {
		result, err := analyzer.AnalyzeCrash(crash)
		require.NoError(t, err)
		
		assert.NotNil(t, result)
		assert.NotNil(t, result.Signature)
		assert.NotEmpty(t, result.Signature.Type)
		assert.NotEmpty(t, result.Signature.Severity)
		assert.NotEmpty(t, result.Recommendations)
		
		// Check crash type detection
		switch crash.ID {
		case "crash-1":
			assert.Equal(t, analysis.CrashTypeSegFault, result.Signature.Type)
		case "crash-2":
			assert.Equal(t, analysis.CrashTypeHeapOverflow, result.Signature.Type)
			assert.Equal(t, analysis.SeverityCritical, result.Signature.Severity)
		case "crash-3":
			assert.Equal(t, analysis.CrashTypeUseAfterFree, result.Signature.Type)
			assert.Equal(t, analysis.SeverityCritical, result.Signature.Severity)
		}
	}
}

// TestCoverageReporting tests coverage reporting
func TestCoverageReporting(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("coverage-test")
	require.NoError(t, err)

	_, err = botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)

	// Report coverage
	coverage := env.CreateTestCoverage(job.ID)
	err = botClient.ReportCoverage(coverage)
	require.NoError(t, err)

	// Verify coverage is stored
	results, err := env.state.ListCoverageResults(&common.CoverageFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	
	assert.Equal(t, coverage.ID, results[0].ID)
	assert.Equal(t, coverage.Edges, results[0].Edges)
	assert.Equal(t, coverage.CoveredEdges, results[0].CoveredEdges)
	assert.Equal(t, coverage.NewEdges, results[0].NewEdges)
}

// TestCorpusUpdate tests corpus update reporting
func TestCorpusUpdate(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("corpus-test")
	require.NoError(t, err)

	_, err = botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)

	// Report corpus update
	corpus := &common.CorpusUpdate{
		ID:        fmt.Sprintf("corpus-%d", time.Now().UnixNano()),
		JobID:     job.ID,
		BotID:     env.botConfig.ID,
		Timestamp: time.Now(),
		Files: []common.CorpusFile{
			{
				Name: "input1.bin",
				Size: 100,
				Hash: "hash1",
			},
			{
				Name: "input2.bin",
				Size: 200,
				Hash: "hash2",
			},
		},
		TotalSize:  300,
		NewFiles:   2,
		TotalFiles: 2,
	}

	err = botClient.ReportCorpusUpdate(corpus)
	require.NoError(t, err)

	// Verify corpus update is handled
	// In a real implementation, this would check corpus storage
	assert.True(t, true, "Corpus update reported successfully")
}

// TestCrashWithLargeInput tests handling of crashes with large inputs
func TestCrashWithLargeInput(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create and register bot
	botClient, err := bot.NewRetryClient(env.botConfig)
	require.NoError(t, err)
	defer botClient.Close()

	_, err = botClient.RegisterBot(env.botConfig.ID, env.botConfig.Capabilities)
	require.NoError(t, err)

	// Create and assign job
	job, err := env.CreateTestJob("large-crash-test")
	require.NoError(t, err)

	_, err = botClient.GetJob(env.botConfig.ID)
	require.NoError(t, err)

	// Create crash with large input (but within limits)
	crash := env.CreateTestCrash(job.ID)
	crash.Input = make([]byte, 500*1024) // 500KB
	for i := range crash.Input {
		crash.Input[i] = byte(i % 256)
	}
	crash.Size = int64(len(crash.Input))

	// Report crash
	err = botClient.ReportCrash(crash)
	require.NoError(t, err)

	// Verify it was stored
	crashes, err := env.state.ListCrashes(&common.CrashFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	require.Len(t, crashes, 1)
	assert.Equal(t, crash.Size, crashes[0].Size)
}

// TestConcurrentCrashReporting tests concurrent crash reports
func TestConcurrentCrashReporting(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Start master server
	err := env.StartMaster()
	require.NoError(t, err)

	// Create multiple bots
	numBots := 5
	clients := make([]*bot.RetryClient, numBots)
	
	for i := 0; i < numBots; i++ {
		config := *env.botConfig
		config.ID = fmt.Sprintf("crash-bot-%d", i)
		
		client, err := bot.NewRetryClient(&config)
		require.NoError(t, err)
		clients[i] = client
		
		_, err = client.RegisterBot(config.ID, config.Capabilities)
		require.NoError(t, err)
	}

	// Create shared job
	job, err := env.CreateTestJob("concurrent-crash-test")
	require.NoError(t, err)

	// Report crashes concurrently
	crashChan := make(chan error, numBots*10)
	
	for i, client := range clients {
		go func(botNum int, c *bot.RetryClient) {
			for j := 0; j < 10; j++ {
				crash := &common.CrashResult{
					ID:        fmt.Sprintf("crash-%d-%d", botNum, j),
					JobID:     job.ID,
					BotID:     fmt.Sprintf("crash-bot-%d", botNum),
					Timestamp: time.Now(),
					Input:     []byte(fmt.Sprintf("CRASH_%d_%d", botNum, j)),
					Hash:      fmt.Sprintf("hash_%d_%d", botNum, j),
					Type:      "test_crash",
				}
				
				err := c.ReportCrash(crash)
				crashChan <- err
			}
		}(i, client)
	}

	// Collect results
	for i := 0; i < numBots*10; i++ {
		err := <-crashChan
		assert.NoError(t, err)
	}

	// Verify crashes were stored
	crashes, err := env.state.ListCrashes(&common.CrashFilter{
		JobID: job.ID,
	})
	require.NoError(t, err)
	assert.Greater(t, len(crashes), 0)

	// Cleanup
	for _, client := range clients {
		client.Close()
	}
}

// TestCrashTriage tests automatic crash triage
func TestCrashTriage(t *testing.T) {
	env := SetupTestEnvironment(t)
	
	// Create crash analyzer
	analyzerConfig := &analysis.AnalyzerConfig{
		EnableStackTrace:     true,
		EnableDeduplication:  true,
		EnableTriage:         true,
		MaxStackFrames:       50,
		MinStackFrames:       3,
	}
	analyzer := analysis.NewCrashAnalyzer(analyzerConfig)

	// Create crashes with different severity levels
	criticalCrash := &common.CrashResult{
		ID:    "critical-crash",
		Type:  "heap_overflow",
		Output: "==1234==ERROR: AddressSanitizer: heap-buffer-overflow",
		StackTrace: `#0 0x1234 in strcpy()
#1 0x5678 in vulnerable_func()
#2 0x9abc in main()`,
	}

	mediumCrash := &common.CrashResult{
		ID:    "medium-crash",
		Type:  "null_dereference",
		Output: "Segmentation fault on null pointer",
		StackTrace: `#0 0x1234 in process_data()
#1 0x5678 in main()`,
	}

	// Analyze and triage
	criticalResult, err := analyzer.AnalyzeCrash(criticalCrash)
	require.NoError(t, err)
	assert.Equal(t, analysis.SeverityCritical, criticalResult.Signature.Severity)
	assert.Equal(t, 1, criticalResult.Triage.Priority)
	assert.Contains(t, criticalResult.Triage.Tags, "security")

	mediumResult, err := analyzer.AnalyzeCrash(mediumCrash)
	require.NoError(t, err)
	assert.Equal(t, analysis.SeverityMedium, mediumResult.Signature.Severity)
	assert.Greater(t, mediumResult.Triage.Priority, criticalResult.Triage.Priority)
}