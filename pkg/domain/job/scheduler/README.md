# Job Scheduler

The job scheduler provides a robust, priority-based job scheduling system for PandaFuzz with support for job dependencies, retries, and scheduled execution.

## Features

- **Priority-Based Scheduling**: Jobs are processed based on priority levels (Critical > High > Normal > Low)
- **FIFO Queue**: Jobs with the same priority are processed in First-In-First-Out order
- **Job Dependencies**: Jobs can depend on other jobs and will only run when dependencies are completed
- **Retry Logic**: Failed jobs can be automatically retried with exponential backoff
- **Scheduled Jobs**: Jobs can be scheduled for future execution
- **Worker Pool**: Configurable number of concurrent workers
- **Lock Management**: Distributed locking support for job processing
- **Stale Lock Recovery**: Automatic recovery of jobs with expired locks
- **Queue Statistics**: Real-time metrics and performance tracking
- **Thread Safety**: All operations are thread-safe for concurrent access

## Usage

### Basic Queue Setup

```go
import (
    "github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
    "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
    "github.com/sirupsen/logrus"
)

// Create configuration
config := scheduler.DefaultConfig()
config.Workers = 8
config.MaxRetries = 3
config.RetryDelay = 30 * time.Second

// Create queue
queue := scheduler.NewQueue(config, jobRepo, processor, logger)

// Start processing
err := queue.Start(ctx)
if err != nil {
    log.Fatal(err)
}
defer queue.Stop()
```

### Priority Queue Setup

```go
// Create priority-based scheduler
queue := scheduler.NewPriorityScheduler(config, jobRepo, processor, logger)
```

### Enqueuing Jobs

```go
// Create a job
job, err := types.NewJob(
    "analyze-crash",
    "libfuzzer",
    "/bin/target",
    "/corpus",
    "/output",
)

// Set priority
job.Priority = types.PriorityHigh

// Set retry configuration
job.MaxRetries = 5
job.RetryDelay = 1 * time.Minute

// Enqueue immediately
err = queue.Enqueue(ctx, job)

// Or schedule for later
err = queue.EnqueueWithDelay(ctx, job, 30*time.Minute)
```

### Job Dependencies

```go
// Create dependent jobs
preprocessJob, _ := types.NewJob("preprocess", "libfuzzer", "/bin/pre", "/corpus", "/output")
analyzeJob, _ := types.NewJob("analyze", "libfuzzer", "/bin/analyze", "/corpus", "/output")
reportJob, _ := types.NewJob("report", "libfuzzer", "/bin/report", "/corpus", "/output")

// Set up dependency chain
analyzeJob.AddDependency(preprocessJob.ID)
reportJob.AddDependency(analyzeJob.ID)

// Enqueue all jobs (they'll run in dependency order)
queue.Enqueue(ctx, preprocessJob)
queue.Enqueue(ctx, analyzeJob)
queue.Enqueue(ctx, reportJob)
```

### Implementing a Job Processor

```go
type FuzzerProcessor struct {
    fuzzerFactory FuzzerFactory
    logger        logrus.FieldLogger
}

func (p *FuzzerProcessor) Process(ctx context.Context, job *types.Job) error {
    // Create fuzzer instance
    fuzzer, err := p.fuzzerFactory.Create(job.FuzzerType, job.FuzzerConfig)
    if err != nil {
        return fmt.Errorf("failed to create fuzzer: %w", err)
    }

    // Run fuzzing job
    result, err := fuzzer.Run(ctx, job.TargetBinary, job.CorpusPath, job.OutputPath)
    if err != nil {
        return fmt.Errorf("fuzzing failed: %w", err)
    }

    // Update job progress
    job.UpdateProgress(&types.JobProgress{
        TotalExecs:     result.TotalExecs,
        ExecsPerSecond: result.ExecsPerSec,
        CorpusSize:     result.CorpusSize,
        Coverage:       result.Coverage,
        LastUpdated:    time.Now(),
    })

    return nil
}
```

### Monitoring Queue Statistics

```go
// Get queue statistics
stats := queue.GetStats()

fmt.Printf("Queue Statistics:\n")
fmt.Printf("  Total Jobs: %d\n", stats.TotalJobs)
fmt.Printf("  Enqueued: %d\n", stats.EnqueuedCount)
fmt.Printf("  Processed: %d\n", stats.ProcessedCount)
fmt.Printf("  Failed: %d\n", stats.FailedCount)
fmt.Printf("  Retried: %d\n", stats.RetryCount)
fmt.Printf("  Queue Depth: %d\n", stats.QueueDepth)
fmt.Printf("  Active Workers: %d/%d\n", stats.WorkersActive, stats.WorkersTotal)
fmt.Printf("  Avg Wait Time: %s\n", stats.AverageWaitTime)
fmt.Printf("  Avg Exec Time: %s\n", stats.AverageExecTime)
```

### Canceling Jobs

```go
// Cancel a specific job
err := queue.Cancel(ctx, jobID)
```

### Listing Jobs

```go
// List jobs with filtering
filter := repository.JobFilter{
    Status:      &types.StatusQueued,
    MinPriority: &types.PriorityHigh,
    Limit:       100,
    OrderBy:     repository.OrderByPriority,
    OrderDirection: repository.OrderDesc,
}

jobs, err := queue.ListJobs(ctx, filter)
```

## Configuration

The scheduler can be configured with the following options:

```go
type Config struct {
    // Number of concurrent workers
    Workers          int           
    
    // Maximum retry attempts for failed jobs
    MaxRetries       int           
    
    // Base delay between retries (exponential backoff applied)
    RetryDelay       time.Duration 
    
    // Duration to lock a job for processing
    LockDuration     time.Duration 
    
    // Time after which locked jobs are considered stale
    StaleLockTimeout time.Duration 
    
    // Interval for checking pending jobs
    PollInterval     time.Duration 
    
    // Enable priority-based scheduling
    EnablePriority   bool          
}
```

Default configuration:
- Workers: 4
- MaxRetries: 3
- RetryDelay: 30 seconds
- LockDuration: 5 minutes
- StaleLockTimeout: 10 minutes
- PollInterval: 1 second
- EnablePriority: true

## Priority Queue Implementation

The priority queue uses a max-heap implementation where jobs with higher priority are processed first. For jobs with the same priority, FIFO ordering is maintained based on the queued timestamp.

### Priority Levels

1. **Critical** - Urgent jobs that must be processed immediately
2. **High** - Important jobs that should be processed soon
3. **Normal** - Standard priority jobs
4. **Low** - Background jobs that can wait

## Thread Safety

All queue operations are thread-safe and can be called concurrently:
- Enqueue/Dequeue operations use mutex protection
- Statistics use atomic operations
- Worker pool management is synchronized
- Priority queue operations are protected by read/write locks

## Error Handling

The scheduler implements comprehensive error handling:
- Failed jobs are automatically retried based on configuration
- Jobs that exceed max retries are marked as permanently failed
- Lock timeouts are handled gracefully
- Worker crashes don't affect other workers
- Graceful shutdown ensures no jobs are lost

## Integration with Repository

The scheduler requires a `JobRepository` implementation that provides:
- CRUD operations for jobs
- Atomic status updates
- Lock management
- Dependency tracking
- Query capabilities

See `pkg/domain/job/repository/interface.go` for the complete interface definition.