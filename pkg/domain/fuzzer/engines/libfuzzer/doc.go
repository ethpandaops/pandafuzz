// Package libfuzzer provides an implementation of the Fuzzer interface for LibFuzzer.
//
// LibFuzzer is a library for in-process, coverage-guided, evolutionary fuzzing
// of other libraries. It is part of LLVM and is integrated with Clang.
//
// This package handles:
//   - Spawning and managing LibFuzzer processes
//   - Parsing LibFuzzer output for crash detection and statistics
//   - Managing input corpus and crash artifacts
//   - Converting between domain types and LibFuzzer arguments
//
// # Example usage
//
//	factory := libfuzzer.NewFactory(logger)
//	fuzzer, err := factory.CreateFuzzer("libfuzzer", "/path/to/target", []string{"-max_len=4096"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Configure the fuzzer
//	config := &types.FuzzerConfig{
//	    OutputDir:   "/tmp/fuzzing/output",
//	    MemoryLimit: 2048 * 1024 * 1024, // 2GB
//	    Workers:     4,
//	    LibFuzzerOptions: &types.LibFuzzerOptions{
//	        UseValueProfile: 1,
//	        PrintFinalStats: 1,
//	    },
//	}
//	if err := fuzzer.Configure(config); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Set corpus and output directories
//	fuzzer.SetCorpus("/path/to/corpus")
//	fuzzer.SetOutput("/path/to/crashes")
//
//	ctx := context.Background()
//	if err := fuzzer.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// Monitor crashes
//	go func() {
//	    for crash := range fuzzer.GetCrashes() {
//	        log.Printf("Found crash: %s", crash.ID)
//	    }
//	}()
//
//	// Monitor progress
//	go func() {
//	    for progress := range fuzzer.GetProgress() {
//	        log.Printf("Executions: %d, Coverage: %.2f%%", progress.Executions, progress.Coverage)
//	    }
//	}()
//
//	// Let it run for a while
//	time.Sleep(10 * time.Minute)
//	fuzzer.Stop()
//
// # Building LibFuzzer Targets
//
// To build a target for LibFuzzer, compile with the -fsanitize=fuzzer flag:
//
//	clang++ -g -fsanitize=fuzzer,address target.cpp -o fuzz_target
//
// The target must implement the fuzzing entry point:
//
//	extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
//	    // Fuzzing logic here
//	    return 0;
//	}
//
// # Thread Safety
//
// All public methods of the Engine are thread-safe and can be called concurrently.
// Internal state is protected by appropriate synchronization primitives.
package libfuzzer
