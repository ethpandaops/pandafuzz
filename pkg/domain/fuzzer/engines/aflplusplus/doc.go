// Package aflplusplus provides an implementation of the Fuzzer interface for AFL++.
//
// AFL++ (American Fuzzy Lop Plus Plus) is a superior fork of AFL, with
// many improvements and new features for fuzzing.
//
// This package handles:
//   - Spawning and managing AFL++ processes
//   - Parsing AFL++ output and status files
//   - Managing input/output queues and crash artifacts
//   - Converting between domain types and AFL++ arguments
//   - Supporting various AFL++ modes (QEMU, Unicorn, persistent, etc.)
//
// # Example usage
//
//	factory := aflplusplus.NewFactory(logger)
//	fuzzer, err := factory.CreateFuzzer("afl++", "/path/to/target", []string{"@@"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Configure the fuzzer
//	config := &types.FuzzerConfig{
//	    OutputDir:   "/tmp/fuzzing/output",
//	    MemoryLimit: 2048 * 1024 * 1024, // 2GB
//	    Workers:     4,
//	    AFLPlusPlusOptions: &types.AFLPlusPlusOptions{
//	        InputDir:      "/path/to/seeds",
//	        Mode:          "fast",
//	        PowerSchedule: "explore",
//	        Deterministic: true,
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
//	        log.Printf("Paths: %d, Crashes: %d, Exec/s: %d",
//	            progress.CorpusSize, progress.CrashCount, progress.ExecsPerSecond)
//	    }
//	}()
//
// # Thread Safety
//
// All public methods of the Engine are thread-safe and can be called concurrently.
// Internal state is protected by appropriate synchronization primitives.
package aflplusplus
