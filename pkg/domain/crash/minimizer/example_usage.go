package minimizer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	fuzzerTypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// ExampleMinimizeCrash demonstrates how to use the minimizer service
func ExampleMinimizeCrash() {
	// Initialize dependencies
	var crashRepo repository.CrashRepository // Use your actual repository implementation
	fuzzerFactory := &exampleFuzzerFactory{} // Use your actual fuzzer factory

	// Create minimizer service
	minimizer, err := NewService(crashRepo, fuzzerFactory)
	if err != nil {
		log.Fatal(err)
	}

	// Create a sample crash
	crash, err := types.NewCrash(
		[]byte("This is a long crash input that could be minimized"),
		"SIGSEGV at 0x12345678",
		types.TargetInfo{
			Name:    "test-target",
			Version: "1.0.0",
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Save the crash
	ctx := context.Background()
	if err := crashRepo.Create(ctx, crash); err != nil {
		log.Fatal(err)
	}

	// Configure minimization options
	options := &MinimizationOptions{
		MaxIterations: 500,
		Timeout:       10 * time.Minute,
		Strategies:    []string{"binary_search", "delta_debugging"},
		ResourceLimits: &ResourceLimits{
			MaxMemory:        512 * 1024 * 1024, // 512MB
			MaxCPUPercent:    75.0,
			MaxExecutionTime: 3 * time.Second,
		},
	}

	// Start minimization
	result, err := minimizer.MinimizeCrash(ctx, crash.ID, options)
	if err != nil {
		log.Fatal(err)
	}

	if result.Success {
		fmt.Printf("Original size: %d bytes\n", result.OriginalSize)
		fmt.Printf("Minimized size: %d bytes\n", result.MinimizedSize)
		fmt.Printf("Reduction: %.2f%%\n", result.ReductionRatio*100)
		fmt.Printf("Duration: %s\n", result.Duration)
	}
}

// ExampleResumeMinimization demonstrates resumable minimization
func ExampleResumeMinimization() {
	// Initialize service
	var crashRepo repository.CrashRepository // Use your actual repository
	fuzzerFactory := &exampleFuzzerFactory{}
	minimizer, _ := NewService(crashRepo, fuzzerFactory)

	ctx := context.Background()
	crashID := "crash-123"

	// Start minimization with a short timeout
	options := &MinimizationOptions{
		Timeout: 1 * time.Minute,
	}

	// First attempt (might timeout)
	_, err := minimizer.MinimizeCrash(ctx, crashID, options)
	if err == context.DeadlineExceeded {
		// Export progress before shutdown
		progress, _ := minimizer.ExportProgress(crashID)

		// Later, resume from saved progress
		result, err := minimizer.ResumeMinimization(ctx, crashID, progress, options)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Resumed minimization completed: %v\n", result.Success)
	}
}

// ExampleCustomStrategy demonstrates registering a custom minimization strategy
func ExampleCustomStrategy() {
	// Initialize service
	var crashRepo repository.CrashRepository // Use your actual repository
	fuzzerFactory := &exampleFuzzerFactory{}
	minimizer, _ := NewService(crashRepo, fuzzerFactory)

	// Create custom strategy
	customStrategy := &customMinimizationStrategy{
		name: "custom_strategy",
	}

	// Register the strategy
	minimizer.RegisterStrategy("custom", customStrategy)

	// Use it in minimization
	options := &MinimizationOptions{
		Strategies: []string{"custom", "binary_search"},
	}

	ctx := context.Background()
	result, err := minimizer.MinimizeCrash(ctx, "crash-id", options)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Minimization completed with custom strategy: %v\n", result.Success)
}

// ExampleProgressMonitoring demonstrates monitoring minimization progress
func ExampleProgressMonitoring() {
	// Initialize service
	var crashRepo repository.CrashRepository // Use your actual repository
	fuzzerFactory := &exampleFuzzerFactory{}
	minimizer, _ := NewService(crashRepo, fuzzerFactory)

	ctx := context.Background()
	crashID := "crash-456"

	// Start minimization in background
	go func() {
		options := &MinimizationOptions{
			Timeout: 30 * time.Minute,
		}
		_, err := minimizer.MinimizeCrash(ctx, crashID, options)
		if err != nil {
			log.Printf("Minimization error: %v", err)
		}
	}()

	// Monitor progress
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			progress, err := minimizer.GetProgress(crashID)
			if err != nil {
				log.Printf("Progress check error: %v", err)
				return
			}

			fmt.Printf("Progress: %d/%d bytes (%.2f%% reduction)\n",
				progress.CurrentSize,
				progress.OriginalSize,
				progress.ReductionRatio*100)
			fmt.Printf("Strategy: %s, Iterations: %d\n",
				progress.CurrentStrategy,
				progress.Iterations)
			fmt.Printf("Estimated time left: %s\n",
				progress.EstimatedTimeLeft)
		case <-ctx.Done():
			return
		}
	}
}

// Example implementations for demonstration

type exampleFuzzerFactory struct{}

func (f *exampleFuzzerFactory) CreateFuzzer(fuzzerType string, target string, args []string) (fuzzerTypes.Fuzzer, error) {
	// Return a mock fuzzer for example
	return &mockFuzzer{}, nil
}

func (f *exampleFuzzerFactory) GetSupportedTypes() []string {
	return []string{"libfuzzer", "afl++", "honggfuzz"}
}

func (f *exampleFuzzerFactory) IsSupported(fuzzerType string) bool {
	for _, t := range f.GetSupportedTypes() {
		if t == fuzzerType {
			return true
		}
	}
	return false
}

type mockFuzzer struct{}

func (m *mockFuzzer) Start(ctx context.Context) error                  { return nil }
func (m *mockFuzzer) Stop() error                                      { return nil }
func (m *mockFuzzer) GetStats() (*fuzzerTypes.FuzzerStats, error)      { return nil, nil }
func (m *mockFuzzer) GetCrashes() <-chan *fuzzerTypes.CrashInfo        { return nil }
func (m *mockFuzzer) GetProgress() <-chan *fuzzerTypes.ProgressUpdate  { return nil }
func (m *mockFuzzer) IsRunning() bool                                  { return false }
func (m *mockFuzzer) GetType() string                                  { return "mock" }
func (m *mockFuzzer) GetVersion() string                               { return "1.0" }
func (m *mockFuzzer) SetCorpus(path string) error                      { return nil }
func (m *mockFuzzer) SetOutput(path string) error                      { return nil }
func (m *mockFuzzer) Configure(config *fuzzerTypes.FuzzerConfig) error { return nil }

type customMinimizationStrategy struct {
	name string
}

func (s *customMinimizationStrategy) Minimize(
	ctx context.Context,
	input []byte,
	verifier ReproductionVerifier,
	progress *MinimizationProgress,
) ([]byte, error) {
	// Custom minimization logic here
	return input, nil
}

func (s *customMinimizationStrategy) Name() string {
	return s.name
}

func (s *customMinimizationStrategy) Description() string {
	return "Custom minimization strategy for specific use cases"
}
