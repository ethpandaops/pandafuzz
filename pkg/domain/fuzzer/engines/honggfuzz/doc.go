// Package honggfuzz provides an implementation of the Fuzzer interface for Honggfuzz.
//
// Honggfuzz is a security-oriented, feedback-driven, evolutionary fuzzing tool
// with powerful analysis options and great performance.
//
// This package handles:
//   - Spawning and managing Honggfuzz processes
//   - Parsing Honggfuzz output and reports
//   - Managing input corpus and crash artifacts
//   - Converting between domain types and Honggfuzz arguments
//   - Supporting various Honggfuzz modes (persistent, socket fuzzing, etc.)
//
// # Example usage
//
//	factory := honggfuzz.NewFactory(logger)
//	fuzzer, err := factory.CreateFuzzer("honggfuzz", "/path/to/target", []string{"___FILE___"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Configure the fuzzer
//	config := &types.FuzzerConfig{
//	    OutputDir:   "/tmp/fuzzing/output",
//	    MemoryLimit: 2048 * 1024 * 1024, // 2GB
//	    Workers:     4,
//	    HonggfuzzOptions: &types.HonggfuzzOptions{
//	        InputDir:    "/path/to/seeds",
//	        Threads:     4,
//	        Iterations:  1000000,
//	        Verbose:     true,
//	        SaveAll:     true,
//	    },
//	}
//	if err := fuzzer.Configure(config); err != nil {
//	    log.Fatal(err)
//	}
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
//	        log.Printf("Iterations: %d, Crashes: %d, Coverage: %.2f%%",
//	            progress.Executions, progress.CrashCount, progress.Coverage)
//	    }
//	}()
//
// # Thread Safety
//
// All public methods of the Engine are thread-safe and can be called concurrently.
// Internal state is protected by appropriate synchronization primitives.
package honggfuzz
