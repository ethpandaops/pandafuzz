package factory_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/factory"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// This example demonstrates how to use the fuzzer factory
func Example() {
	// Create a new factory
	f := factory.NewFactory(factory.Options{
		// Add repositories and services as needed
	})

	// Register fuzzer engines
	// In a real application, these would be actual engine constructors
	libFuzzerConstructor := func(target string, args []string) (types.Fuzzer, error) {
		// This would create an actual LibFuzzer instance
		// For example: return libfuzzer.NewEngine(target, args)
		return nil, fmt.Errorf("libfuzzer not implemented in example")
	}

	aflConstructor := func(target string, args []string) (types.Fuzzer, error) {
		// This would create an actual AFL++ instance
		// For example: return aflplusplus.NewEngine(target, args)
		return nil, fmt.Errorf("afl++ not implemented in example")
	}

	// Register the engines
	if err := factory.RegisterLibFuzzer(f, libFuzzerConstructor); err != nil {
		log.Fatalf("Failed to register LibFuzzer: %v", err)
	}

	if err := factory.RegisterAFLPlusPlus(f, aflConstructor); err != nil {
		log.Fatalf("Failed to register AFL++: %v", err)
	}

	// List supported fuzzer types
	supportedTypes := f.GetSupportedTypes()
	fmt.Println("Supported fuzzer types:")
	for _, t := range supportedTypes {
		fmt.Printf("  - %s\n", t)
	}

	// Get information about a specific engine
	info, err := f.GetEngineInfo("libfuzzer")
	if err != nil {
		log.Fatalf("Failed to get engine info: %v", err)
	}
	fmt.Printf("\nLibFuzzer info:\n")
	fmt.Printf("  Name: %s\n", info.Name)
	fmt.Printf("  Description: %s\n", info.Description)
	fmt.Printf("  Supports parallel: %v\n", info.Capabilities.SupportsParallel)

	// Output:
	// Supported fuzzer types:
	//   - libfuzzer
	//   - afl++
	//
	// LibFuzzer info:
	//   Name: LibFuzzer
	//   Description: LLVM LibFuzzer - coverage-guided, evolutionary fuzzing engine
	//   Supports parallel: false
}

// This example shows how to create a fuzzer using the factory
func ExampleFactory_CreateFuzzer() {
	// Create factory with dependencies
	f := factory.NewFactory(factory.Options{
		// Add any required repositories and services
	})

	// Register a mock fuzzer for demonstration
	mockConstructor := func(target string, args []string) (types.Fuzzer, error) {
		fmt.Printf("Creating fuzzer for target: %s\n", target)
		fmt.Printf("Arguments: %v\n", args)
		// In a real implementation, this would return an actual fuzzer instance
		return nil, fmt.Errorf("mock fuzzer")
	}

	err := f.RegisterEngine("mock", mockConstructor, &factory.EngineInfo{
		Type:        "mock",
		Name:        "Mock Fuzzer",
		Description: "A mock fuzzer for testing",
		Version:     "1.0",
		Capabilities: &types.FuzzerCapabilities{
			SupportsCoverage: true,
			SupportsTimeout:  true,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a fuzzer instance
	_, err = f.CreateFuzzer("mock", "/path/to/target", []string{"--timeout=30", "--workers=4"})
	if err != nil {
		// Expected error from mock
		fmt.Printf("Error: %v\n", err)
	}

	// Output:
	// Creating fuzzer for target: /path/to/target
	// Arguments: [--timeout=30 --workers=4]
	// Error: failed to create mock fuzzer: mock fuzzer
}

// This example demonstrates using the builder pattern
func ExampleFactory_Builder() {
	ctx := context.Background()

	// Create factory
	f := factory.NewFactory(factory.Options{})

	// Register a mock fuzzer
	mockConstructor := func(target string, args []string) (types.Fuzzer, error) {
		// Mock implementation that just prints the configuration
		return &mockFuzzer{
			target: target,
			args:   args,
		}, nil
	}

	f.RegisterEngine("mock", mockConstructor, &factory.EngineInfo{
		Type: "mock",
		Name: "Mock Fuzzer",
		Capabilities: &types.FuzzerCapabilities{
			SupportsCoverage: true,
		},
	})

	// Create configuration
	config := &types.FuzzerConfig{
		OutputDir:   "/tmp/fuzzing/output",
		MemoryLimit: 2 * 1024 * 1024 * 1024, // 2GB
		Workers:     8,
		Timeout:     30 * 1000000000, // 30 seconds as duration
	}

	// Build and configure a fuzzer
	fuzzer, err := f.NewBuilder("mock").
		WithTarget("/path/to/target").
		WithArgs([]string{"--flag1", "--flag2"}).
		WithConfig(config).
		Build()

	if err != nil {
		log.Fatal(err)
	}

	// Use the fuzzer
	if err := fuzzer.Start(ctx); err != nil {
		log.Printf("Failed to start fuzzer: %v", err)
	}

	fmt.Println("Fuzzer created and started successfully")

	// Output:
	// Fuzzer created and started successfully
}

// mockFuzzer is a simple mock implementation for examples
type mockFuzzer struct {
	target string
	args   []string
	config *types.FuzzerConfig
}

func (m *mockFuzzer) Start(ctx context.Context) error           { return nil }
func (m *mockFuzzer) Stop() error                               { return nil }
func (m *mockFuzzer) GetStats() (*types.FuzzerStats, error)     { return nil, nil }
func (m *mockFuzzer) GetCrashes() <-chan *types.CrashInfo       { return nil }
func (m *mockFuzzer) GetProgress() <-chan *types.ProgressUpdate { return nil }
func (m *mockFuzzer) IsRunning() bool                           { return false }
func (m *mockFuzzer) GetType() string                           { return "mock" }
func (m *mockFuzzer) GetVersion() string                        { return "1.0" }
func (m *mockFuzzer) SetCorpus(path string) error               { return nil }
func (m *mockFuzzer) SetOutput(path string) error               { return nil }
func (m *mockFuzzer) Configure(config *types.FuzzerConfig) error {
	m.config = config
	return nil
}
