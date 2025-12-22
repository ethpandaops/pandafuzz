// Package types defines interfaces and data structures for fuzzer engines.
//
// This package provides the contract that all fuzzer implementations must
// satisfy, along with configuration and result types used for fuzzer operation.
//
// # Fuzzer Interface
//
// The Fuzzer interface is the core abstraction for fuzzing engines:
//
//	type Fuzzer interface {
//	    Configure(config *FuzzerConfig) error
//	    SetCorpus(path string) error
//	    SetOutput(path string) error
//	    Start(ctx context.Context) error
//	    Stop() error
//	    IsRunning() bool
//	    GetStats() <-chan *FuzzerStats
//	    GetCrashes() <-chan *CrashInfo
//	    GetProgress() <-chan *ProgressUpdate
//	}
//
// # Lifecycle
//
// Fuzzer implementations follow this lifecycle:
//
//  1. Create via factory (pkg/domain/fuzzer/factory)
//  2. Configure with FuzzerConfig
//  3. SetCorpus and SetOutput directories
//  4. Start blocks until completion or context cancellation
//  5. Monitor via GetStats, GetCrashes, GetProgress channels
//  6. Stop for early termination
//
// # Channel-Based Results
//
// Unlike synchronous APIs, this interface uses channels for results:
//
//	go func() {
//	    for crash := range fuzzer.GetCrashes() {
//	        log.Printf("Found crash: %s", crash.Hash)
//	    }
//	}()
//
//	go func() {
//	    for stats := range fuzzer.GetStats() {
//	        log.Printf("Executions: %d", stats.TotalExecs)
//	    }
//	}()
//
//	err := fuzzer.Start(ctx)
//
// # Configuration
//
// FuzzerConfig controls execution parameters:
//
//	config := &types.FuzzerConfig{
//	    Duration:     1 * time.Hour,
//	    MemoryLimit:  1 << 30, // 1GB
//	    Timeout:      5 * time.Second,
//	    Jobs:         4,
//	    EnableDict:   true,
//	    DictPath:     "/path/to/dict.txt",
//	}
//
// # Crash Information
//
// CrashInfo captures details about discovered crashes:
//
//	type CrashInfo struct {
//	    Hash       string    // SHA256 of input
//	    InputPath  string    // Path to crash input
//	    Signal     int       // Signal that caused crash
//	    Stderr     string    // Error output
//	    Timestamp  time.Time
//	}
//
// # Supported Engines
//
// Implementations are provided in sub-packages:
//
//   - pkg/domain/fuzzer/engines/libfuzzer: LLVM LibFuzzer
//   - pkg/domain/fuzzer/engines/aflplusplus: AFL++
//   - pkg/domain/fuzzer/engines/honggfuzz: Honggfuzz
//
// # Thread Safety
//
// Fuzzer implementations must be safe for concurrent channel reads while
// the fuzzer is running. The Start method blocks until completion.
package types
