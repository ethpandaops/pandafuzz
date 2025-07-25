// Package factory provides a flexible factory pattern implementation for creating
// and managing different fuzzer engine instances. It supports dynamic registration
// of fuzzer engines, dependency injection, and provides a unified interface for
// fuzzer creation regardless of the underlying engine type.
//
// The factory pattern allows for:
//   - Runtime selection of fuzzer engines
//   - Easy addition of new fuzzer types without modifying existing code
//   - Centralized configuration and dependency management
//   - Type-safe fuzzer creation with proper validation
//
// Example usage:
//
//	// Create a factory
//	f := factory.NewFactory(factory.Options{
//		Repositories: repos,
//		Services:     services,
//	})
//
//	// Register engines
//	factory.RegisterLibFuzzer(f, libFuzzerConstructor)
//	factory.RegisterAFLPlusPlus(f, aflConstructor)
//
//	// Create a fuzzer
//	fuzzer, err := f.CreateFuzzer("libfuzzer", "/path/to/target", []string{"--timeout=30"})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Start fuzzing
//	if err := fuzzer.Start(context.Background()); err != nil {
//		log.Fatal(err)
//	}
//
// The package also provides a builder pattern for more complex fuzzer creation:
//
//	fuzzer, err := f.NewBuilder("afl++").
//		WithTarget("/path/to/target").
//		WithArgs([]string{"--flag"}).
//		WithConfig(config).
//		Build()
//
// Thread Safety:
//
// All factory methods are thread-safe and can be called concurrently.
// The factory uses read-write mutexes to ensure safe concurrent access
// to the engine registry while minimizing lock contention.
package factory
