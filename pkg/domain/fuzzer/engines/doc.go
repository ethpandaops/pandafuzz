// Package engines provides implementations of various fuzzing engines.
//
// This package contains concrete implementations of the types.Fuzzer interface
// for different fuzzing engines:
//
//   - libfuzzer: LLVM's LibFuzzer for in-process coverage-guided fuzzing
//   - aflplusplus: AFL++ for high-performance binary fuzzing
//   - honggfuzz: Honggfuzz for security-oriented feedback-driven fuzzing
//
// Each engine is in its own sub-package and provides:
//   - An Engine struct implementing types.Fuzzer
//   - A Factory struct implementing types.FuzzerFactory
//   - Engine-specific configuration and capabilities
//
// # Usage Example
//
//	import (
//	    "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
//	    "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/aflplusplus"
//	    "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/honggfuzz"
//	)
//
//	// Create a factory for the desired engine
//	factory := libfuzzer.NewFactory(logger)
//
//	// Create a fuzzer instance
//	fuzzer, err := factory.CreateFuzzer("libfuzzer", "/path/to/target", []string{"-max_len=4096"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Configure and start fuzzing
//	config := &types.FuzzerConfig{
//	    OutputDir:   "/tmp/fuzzing",
//	    MemoryLimit: 2048 * 1024 * 1024,
//	    Workers:     4,
//	}
//	fuzzer.Configure(config)
//	fuzzer.Start(ctx)
//
// # Engine Selection
//
// Choose an engine based on your requirements:
//
//   - LibFuzzer: Best for targets with source code access, excellent coverage tracking
//   - AFL++: Best for binary-only targets, great performance and stability
//   - Honggfuzz: Best for security testing, powerful crash analysis
//
// All engines implement the same interface, making it easy to switch between them.
package engines
