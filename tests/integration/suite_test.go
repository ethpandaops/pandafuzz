package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// TestMain sets up and tears down the integration test suite
func TestMain(m *testing.M) {
	// Setup
	setup()
	
	// Run tests
	code := m.Run()
	
	// Teardown
	teardown()
	
	// Exit
	os.Exit(code)
}

func setup() {
	// Set test environment
	os.Setenv("PANDAFUZZ_TEST_MODE", "true")
	
	// Configure logging for tests
	logrus.SetLevel(logrus.WarnLevel) // Reduce noise during tests
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "15:04:05.000",
		DisableColors:   true,
	})
	
	fmt.Println("=== PandaFuzz Integration Test Suite ===")
}

func teardown() {
	fmt.Println("=== Test Suite Complete ===")
}

// TestSuiteSmoke runs a basic smoke test
func TestSuiteSmoke(t *testing.T) {
	// This ensures the test package compiles and runs
	t.Log("Integration test suite is operational")
}

// Integration test categories:
// 1. Master-Bot Communication (master_bot_test.go)
//    - Bot registration and deregistration
//    - Heartbeat mechanism
//    - Multiple bot connections
//    - Reconnection handling
//
// 2. Job Flow (job_flow_test.go)
//    - Job creation and assignment
//    - Job completion and failure
//    - Job timeout handling
//    - Job priority and filtering
//    - Multi-bot job distribution
//
// 3. Crash Reporting (crash_reporting_test.go)
//    - Crash report submission
//    - Crash deduplication
//    - Crash analysis integration
//    - Coverage reporting
//    - Corpus updates
//
// 4. Recovery Procedures (recovery_test.go)
//    - Startup recovery
//    - Orphaned job recovery
//    - Bot failure handling
//    - Timeout recovery
//    - System state validation
//
// 5. API Endpoints (api_test.go)
//    - Health and status endpoints
//    - CRUD operations
//    - Error handling
//    - Concurrent requests
//    - Authentication
//
// 6. Fuzzer Integration (fuzzer_test.go)
//    - AFL++ integration
//    - LibFuzzer integration
//    - Crash detection
//    - Coverage collection
//    - Event handling

// Benchmark tests can be added here
func BenchmarkBotRegistration(b *testing.B) {
	env := SetupTestEnvironment(&testing.T{})
	defer env.Cleanup()
	
	err := env.StartMaster()
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		botID := fmt.Sprintf("bench-bot-%d", i)
		bot, _ := env.CreateTestBot(botID)
		if bot != nil {
			env.state.DeleteBot(botID)
		}
	}
}

func BenchmarkJobAssignment(b *testing.B) {
	env := SetupTestEnvironment(&testing.T{})
	defer env.Cleanup()
	
	err := env.StartMaster()
	if err != nil {
		b.Fatal(err)
	}
	
	// Create bot
	bot, err := env.CreateTestBot("bench-bot")
	if err != nil {
		b.Fatal(err)
	}
	
	// Create jobs
	jobs := make([]*common.Job, b.N)
	for i := 0; i < b.N; i++ {
		job, err := env.CreateTestJob(fmt.Sprintf("bench-job-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		jobs[i] = job
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Simulate job assignment
		job := jobs[i]
		job.Status = common.JobStatusAssigned
		job.AssignedBot = &bot.ID
		env.state.SaveJob(job)
		
		// Reset for next iteration
		job.Status = common.JobStatusCompleted
		env.state.SaveJob(job)
	}
}