# LibFuzzer Engine

This package provides a LibFuzzer engine implementation for the PandaFuzz fuzzing framework.

## Overview

The LibFuzzer engine implements the `types.Fuzzer` interface and provides integration with LLVM's LibFuzzer for coverage-guided fuzzing. It supports all standard LibFuzzer features including:

- Edge coverage tracking
- Corpus management
- Crash detection and reproduction
- Dictionary-based fuzzing
- Memory and timeout limits
- Custom fuzzing strategies

## Usage

```go
import (
    "context"
    "time"
    
    "github.com/sirupsen/logrus"
    "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
    "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Create a new factory and fuzzer instance
log := logrus.New()
factory := libfuzzer.NewFactory(log)

// Create fuzzer with target binary and arguments
fuzzer, err := factory.CreateFuzzer("libfuzzer", "/path/to/fuzz_target", []string{"-max_len=4096"})
if err != nil {
    log.Fatal(err)
}

// Configure the fuzzer
config := &types.FuzzerConfig{
    OutputDir:       "/tmp/fuzzing/output",
    MemoryLimit:     2048 * 1024 * 1024, // 2GB
    Workers:         4,
    Timeout:         30 * time.Second,
    Dictionary:      "/path/to/dictionary.txt",
    SeedCorpus:      "/path/to/seeds",
    LibFuzzerOptions: &types.LibFuzzerOptions{
        UseValueProfile: 1,
        PrintFinalStats: 1,
        Runs:            1000000,
        MaxTotalTime:    3600, // 1 hour
    },
}

if err := fuzzer.Configure(config); err != nil {
    log.Fatal(err)
}

// Set corpus and output directories
fuzzer.SetCorpus("/path/to/corpus")
fuzzer.SetOutput("/path/to/crashes")

// Start fuzzing
ctx := context.Background()
if err := fuzzer.Start(ctx); err != nil {
    log.Fatal(err)
}

// Monitor crashes
go func() {
    for crash := range fuzzer.GetCrashes() {
        log.Printf("Found crash: %s at %v", crash.ID, crash.DiscoveredAt)
        log.Printf("Stack trace: %s", crash.StackTrace)
    }
}()

// Monitor progress
go func() {
    for progress := range fuzzer.GetProgress() {
        log.Printf("Executions: %d, Exec/s: %d, Coverage: %.2f%%, Crashes: %d",
            progress.Executions, 
            progress.ExecsPerSecond,
            progress.Coverage, 
            progress.CrashCount)
    }
}()

// Get statistics periodically
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        if !fuzzer.IsRunning() {
            break
        }
        
        stats, err := fuzzer.GetStats()
        if err != nil {
            log.Error(err)
            continue
        }
        
        log.Printf("Runtime: %v, Total execs: %d, Corpus: %d, Memory: %d MB",
            stats.RunTime,
            stats.TotalExecutions,
            stats.CorpusSize,
            stats.MemoryPeak/(1024*1024))
    }
}()

// Stop after some time
time.Sleep(30 * time.Minute)
if err := fuzzer.Stop(); err != nil {
    log.Error(err)
}
```

## Configuration Options

### Required Configuration

- `Target`: Path to the LibFuzzer-instrumented binary
- `OutputDir`: Directory for fuzzing artifacts

### Common Configuration Options

- `MemoryLimit`: Memory limit in bytes (default: 2GB)
- `Timeout`: Timeout per test case (default: 25s)
- `Workers`: Number of parallel workers (default: 1)
- `Dictionary`: Path to dictionary file
- `SeedCorpus`: Directory containing seed inputs
- `MaxLen`: Maximum input length
- `MinLen`: Minimum input length

### LibFuzzer-Specific Options

Configure LibFuzzer-specific options via the `LibFuzzerOptions` struct:

```go
config.LibFuzzerOptions = &types.LibFuzzerOptions{
    Runs:                1000000,  // Number of individual test runs
    MaxTotalTime:        3600,     // Maximum total time in seconds
    LenControl:          100,      // Length control parameter
    SeedInputs:          "",       // Seed inputs path
    KeepSeed:            1,        // Keep seed inputs
    CrossOver:           1,        // Enable crossover mutations
    MutateDepth:         5,        // Mutation depth
    ReduceInputs:        1,        // Reduce corpus
    UseCounters:         1,        // Use edge counters
    UseMemmem:           1,        // Use memmem
    UseValueProfile:     1,        // Use value profiling
    ShrinkCorpus:        1,        // Shrink corpus
    PrintPCs:            0,        // Print coverage PCs
    PrintFuncs:          0,        // Print covered functions
    PrintCoverage:       0,        // Print coverage information
    PrintCorpusStats:    0,        // Print corpus statistics
    PrintFinalStats:     1,        // Print final statistics
    DetectLeaks:         1,        // Detect memory leaks
    PurgeAllocatorCache: 0,        // Purge allocator cache
    TraceMalloc:         0,        // Trace malloc calls
    RssLimitMB:          2048,     // RSS limit in MB
    MallocLimitMB:       0,        // Malloc limit in MB
}
```

## Building LibFuzzer Targets

To use this engine, you need to build your target with LibFuzzer instrumentation:

```bash
# With clang
clang++ -g -fsanitize=fuzzer,address target.cpp -o fuzz_target

# With coverage instrumentation
clang++ -g -fsanitize=fuzzer,address -fprofile-instr-generate -fcoverage-mapping target.cpp -o fuzz_target

# For Go targets
go-fuzz-build -libfuzzer ./...
clang -fsanitize=fuzzer main.a -o fuzz_target
```

The target must implement the fuzzing entry point:

```cpp
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // Fuzzing logic here
    return 0;
}
```

## Output Parsing

The engine parses LibFuzzer output to extract:

- Execution count: `#123456`
- Coverage information: `cov: 1234`
- Corpus size: `corp: 42`
- Execution speed: `exec/s: 1000`
- Memory usage: `rss: 512Mb`
- Crash detection: `==12345==ERROR: AddressSanitizer`

## Performance Tuning

1. **Memory Management**: Set appropriate memory limits to prevent OOM
2. **Parallel Execution**: Use the `Workers` option for multi-core fuzzing
3. **Corpus Management**: The engine automatically manages corpus files
4. **Dictionary**: Provide domain-specific dictionaries for better coverage

## Limitations

- LibFuzzer runs as a subprocess, not in-process
- Configuration changes require restarting the fuzzer
- Output parsing depends on LibFuzzer's output format

## Thread Safety

The engine is thread-safe for all public methods. Internal state is protected by appropriate synchronization primitives.