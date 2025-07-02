# ResultCollector Implementation

## Overview

The `ResultCollector` is a component in the PandaFuzz bot that efficiently collects and batches fuzzing results before sending them to the master node. It handles network failures with retry logic and ensures reliable delivery of crash reports, coverage data, corpus updates, and statistics.

## Key Features

- **Batching**: Collects results in batches to reduce network overhead
- **Automatic Flushing**: Periodically flushes batches based on a configurable interval
- **Retry Logic**: Handles network failures with exponential backoff retry
- **Thread-Safe**: Safe for concurrent use by multiple fuzzer instances
- **Graceful Shutdown**: Ensures all pending results are sent before stopping

## Architecture

### Core Components

1. **Event Channel**: Receives fuzzer events asynchronously
2. **Batch Buffers**: Separate buffers for each result type (crashes, coverage, corpus, stats)
3. **Processing Loop**: Handles events and triggers batch flushes
4. **Retry Client**: Manages network communication with retry logic

### Event Types Supported

- `FuzzerEventCrashFound`: New crash discovered
- `FuzzerEventCoverage`: Coverage update
- `FuzzerEventCorpusUpdate`: New corpus entries
- `FuzzerEventStats`: Fuzzer statistics

## Usage

```go
// Create result collector
collector, err := bot.NewResultCollector(config, masterURL, logger)
if err != nil {
    return err
}

// Configure batching
collector.SetBatchSize(100)                    // Batch up to 100 results
collector.SetFlushInterval(10 * time.Second)   // Flush every 10 seconds

// Start collector
ctx := context.Background()
if err := collector.Start(ctx); err != nil {
    return err
}

// Handle fuzzer events
event := common.FuzzerEvent{
    Type:      common.FuzzerEventCrashFound,
    Timestamp: time.Now(),
    JobID:     jobID,
    Data: map[string]interface{}{
        "crash": crashResult,
    },
}
collector.HandleEvent(event)

// Stop collector (flushes remaining results)
if err := collector.Stop(); err != nil {
    return err
}
```

## Configuration

The ResultCollector uses the bot configuration for:

- **Timeouts**: Result reporting timeout, master communication timeout
- **Retry Policy**: Configurable retry with exponential backoff
- **Master URL**: Endpoint for sending results

## Integration with Fuzzers

Fuzzers can integrate with the ResultCollector by implementing the `EventHandler` interface:

```go
type FuzzerEventHandler struct {
    collector *bot.ResultCollector
    jobID     string
    botID     string
}

func (h *FuzzerEventHandler) OnCrash(f fuzzer.Fuzzer, crash *common.CrashResult) {
    event := common.FuzzerEvent{
        Type:      common.FuzzerEventCrashFound,
        Timestamp: time.Now(),
        JobID:     h.jobID,
        Data: map[string]interface{}{
            "crash": crash,
        },
    }
    h.collector.HandleEvent(event)
}
```

## Network Resilience

The ResultCollector ensures reliable delivery through:

1. **Retry Logic**: Failed sends are retried with exponential backoff
2. **Circuit Breaker**: Prevents overwhelming a failing master
3. **Re-queueing**: Failed results are re-added to batches for retry
4. **Graceful Degradation**: Continues collecting results even if master is temporarily unavailable

## Monitoring

Get collector statistics:

```go
stats := collector.GetStats()
// Returns:
// - Batch sizes for each result type
// - Event queue size and capacity  
// - Configuration settings
```

## Testing

The implementation includes comprehensive unit tests covering:

- Basic functionality (start/stop, event handling)
- Batch size limits and flushing
- Configuration updates
- Error scenarios

Run tests with:
```bash
go test -v ./pkg/bot -run TestResultCollector
```

## Performance Considerations

- **Batch Size**: Larger batches reduce network overhead but increase memory usage
- **Flush Interval**: Shorter intervals provide more real-time updates but increase network traffic
- **Event Queue**: Default capacity of 1000 events; may need adjustment for high-throughput fuzzing

## Future Enhancements

1. **Compression**: Compress batches before sending to reduce bandwidth
2. **Persistent Queue**: Store results to disk for recovery after crashes
3. **Metrics**: Prometheus metrics for monitoring
4. **Batch API**: Master endpoint for receiving batched results in a single request