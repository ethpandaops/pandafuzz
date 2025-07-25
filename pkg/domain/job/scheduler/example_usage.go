package scheduler_test

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
)

// Example demonstrates basic queue usage
func Example_basicUsage() {
	ctx := context.Background()
	logger := logrus.New()

	// Create repository and processor (mocked for example)
	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	// Configure the queue
	config := scheduler.DefaultConfig()
	config.Workers = 4
	config.MaxRetries = 3

	// Create and start the queue
	queue := scheduler.NewQueue(config, repo, processor, logger)

	if err := queue.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	defer queue.Stop()

	// Create a job
	job, _ := types.NewJob(
		"fuzz-target-binary",
		"libfuzzer",
		"/bin/target",
		"/corpus",
		"/output",
	)
	job.Priority = types.PriorityHigh

	// Enqueue the job
	if err := queue.Enqueue(ctx, job); err != nil {
		logger.Error(err)
	}

	// Output:
}

// Example_priorityScheduling demonstrates priority-based job scheduling
func Example_priorityScheduling() {
	ctx := context.Background()
	logger := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	// Create priority scheduler
	config := scheduler.DefaultConfig()
	queue := scheduler.NewPriorityScheduler(config, repo, processor, logger)

	// Create jobs with different priorities
	criticalJob, _ := types.NewJob("critical-scan", "libfuzzer", "/bin/critical", "/corpus", "/output")
	criticalJob.Priority = types.PriorityCritical

	normalJob, _ := types.NewJob("normal-scan", "libfuzzer", "/bin/normal", "/corpus", "/output")
	normalJob.Priority = types.PriorityNormal

	lowJob, _ := types.NewJob("background-scan", "libfuzzer", "/bin/background", "/corpus", "/output")
	lowJob.Priority = types.PriorityLow

	// Enqueue in any order - they'll be processed by priority
	queue.Enqueue(ctx, normalJob)
	queue.Enqueue(ctx, lowJob)
	queue.Enqueue(ctx, criticalJob) // This will be processed first

	// Output:
}

// Example_jobDependencies demonstrates setting up job dependencies
func Example_jobDependencies() {
	ctx := context.Background()
	logger := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	queue := scheduler.NewQueue(config, repo, processor, logger)

	// Create a job pipeline
	downloadJob, _ := types.NewJob("download-corpus", "downloader", "/bin/download", "/tmp", "/corpus")
	prepareJob, _ := types.NewJob("prepare-corpus", "preprocessor", "/bin/prepare", "/corpus", "/prepared")
	fuzzJob, _ := types.NewJob("fuzz-target", "libfuzzer", "/bin/target", "/prepared", "/output")
	analyzeJob, _ := types.NewJob("analyze-results", "analyzer", "/bin/analyze", "/output", "/reports")

	// Set up dependencies
	prepareJob.AddDependency(downloadJob.ID) // Prepare depends on download
	fuzzJob.AddDependency(prepareJob.ID)     // Fuzz depends on prepare
	analyzeJob.AddDependency(fuzzJob.ID)     // Analyze depends on fuzz

	// Enqueue all jobs - they'll execute in dependency order
	queue.Enqueue(ctx, downloadJob)
	queue.Enqueue(ctx, prepareJob)
	queue.Enqueue(ctx, fuzzJob)
	queue.Enqueue(ctx, analyzeJob)

	// Output:
}

// Example_scheduledJobs demonstrates scheduling jobs for future execution
func Example_scheduledJobs() {
	ctx := context.Background()
	logger := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	queue := scheduler.NewQueue(config, repo, processor, logger)

	// Create a nightly scan job
	nightlyJob, _ := types.NewJob("nightly-scan", "libfuzzer", "/bin/target", "/corpus", "/output")
	nightlyJob.Priority = types.PriorityLow

	// Schedule for midnight
	midnight := time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour)
	nightlyJob.ScheduledAt = &midnight

	// Or use the convenience method
	queue.EnqueueWithDelay(ctx, nightlyJob, time.Until(midnight))

	// Create a job to run in 30 minutes
	delayedJob, _ := types.NewJob("delayed-scan", "libfuzzer", "/bin/target", "/corpus", "/output")
	queue.EnqueueWithDelay(ctx, delayedJob, 30*time.Minute)

	// Output:
}

// Example_retryConfiguration demonstrates configuring retry logic
func Example_retryConfiguration() {
	ctx := context.Background()
	logger := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	queue := scheduler.NewQueue(config, repo, processor, logger)

	// Create a job with custom retry settings
	job, _ := types.NewJob("retry-job", "libfuzzer", "/bin/target", "/corpus", "/output")

	// Configure retries
	job.MaxRetries = 5
	job.RetryDelay = 1 * time.Minute // Base delay, will use exponential backoff

	queue.Enqueue(ctx, job)

	// If the job fails, it will be retried with delays:
	// 1st retry: 1 minute
	// 2nd retry: 2 minutes
	// 3rd retry: 4 minutes
	// 4th retry: 8 minutes
	// 5th retry: 16 minutes

	// Output:
}

// Example_queueMonitoring demonstrates monitoring queue statistics
func Example_queueMonitoring() {
	ctx := context.Background()
	logger := logrus.New()

	repo := NewMockJobRepository()
	processor := &MockJobProcessor{}

	config := scheduler.DefaultConfig()
	queue := scheduler.NewQueue(config, repo, processor, logger)
	queue.Start(ctx)

	// Monitor queue statistics
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			stats := queue.GetStats()

			fmt.Printf("Queue Status:\n")
			fmt.Printf("  Total Jobs: %d\n", stats.TotalJobs)
			fmt.Printf("  Queue Depth: %d\n", stats.QueueDepth)
			fmt.Printf("  Processing Rate: %.2f jobs/min\n",
				float64(stats.ProcessedCount)/(time.Since(time.Now()).Minutes()))
			fmt.Printf("  Active Workers: %d/%d\n", stats.WorkersActive, stats.WorkersTotal)
			fmt.Printf("  Failed Jobs: %d (%.2f%%)\n",
				stats.FailedCount,
				float64(stats.FailedCount)/float64(stats.ProcessedCount)*100)

			// Alert if queue is backing up
			if stats.QueueDepth > 100 {
				logger.Warn("Queue depth exceeds threshold")
			}
		}
	}()

	// Output:
}

// ExampleJobProcessor_implementation shows how to implement a job processor
func ExampleJobProcessor_implementation() {
	// Define a custom processor for fuzzing jobs
	type FuzzingProcessor struct {
		logger logrus.FieldLogger
	}

	// Implement the Process method
	processor := &FuzzingProcessor{
		logger: logrus.New(),
	}

	// Process method implementation
	_ = func(ctx context.Context, job *types.Job) error {
		log := processor.logger.WithFields(logrus.Fields{
			"job_id": job.ID,
			"fuzzer": job.FuzzerType,
		})

		log.Info("Starting fuzzing job")

		// Simulate fuzzing work
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			// Update progress periodically
			job.UpdateProgress(&types.JobProgress{
				TotalExecs:     1000000,
				ExecsPerSecond: 50000,
				CorpusSize:     250,
				Coverage:       0.75,
				LastUpdated:    time.Now(),
			})
		}

		// Check for crashes
		if job.CrashCount > 0 {
			log.WithField("crashes", job.CrashCount).Info("Crashes found during fuzzing")
		}

		log.Info("Fuzzing job completed")
		return nil
	}

	// Output:
}
