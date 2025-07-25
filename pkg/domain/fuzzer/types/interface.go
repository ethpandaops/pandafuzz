package types

import (
	"context"
	"io"
	"time"
)

// Fuzzer defines the interface that all fuzzer implementations must satisfy
type Fuzzer interface {
	// Start begins the fuzzing process
	Start(ctx context.Context) error

	// Stop gracefully stops the fuzzing process
	Stop() error

	// GetStats returns current fuzzing statistics
	GetStats() (*FuzzerStats, error)

	// GetCrashes returns a channel that emits discovered crashes
	GetCrashes() <-chan *CrashInfo

	// GetProgress returns a channel that emits progress updates
	GetProgress() <-chan *ProgressUpdate

	// IsRunning checks if the fuzzer is currently running
	IsRunning() bool

	// GetType returns the fuzzer engine type (e.g., "libfuzzer", "afl++", "honggfuzz")
	GetType() string

	// GetVersion returns the fuzzer engine version
	GetVersion() string

	// SetCorpus sets the input corpus directory
	SetCorpus(path string) error

	// SetOutput sets the output directory for crashes and artifacts
	SetOutput(path string) error

	// Configure applies fuzzer-specific configuration
	Configure(config *FuzzerConfig) error
}

// FuzzerFactory creates fuzzer instances based on type
type FuzzerFactory interface {
	// CreateFuzzer creates a new fuzzer instance of the specified type
	CreateFuzzer(fuzzerType string, target string, args []string) (Fuzzer, error)

	// GetSupportedTypes returns a list of supported fuzzer types
	GetSupportedTypes() []string

	// IsSupported checks if a fuzzer type is supported
	IsSupported(fuzzerType string) bool
}

// FuzzerStats represents runtime statistics from a fuzzer
type FuzzerStats struct {
	StartTime       time.Time     `json:"start_time"`
	RunTime         time.Duration `json:"run_time"`
	TotalExecutions uint64        `json:"total_executions"`
	ExecsPerSecond  uint64        `json:"execs_per_second"`
	CorpusSize      uint64        `json:"corpus_size"`
	Coverage        float64       `json:"coverage"`
	CrashesFound    uint64        `json:"crashes_found"`
	TimeoutsFound   uint64        `json:"timeouts_found"`
	MemoryPeak      uint64        `json:"memory_peak"`
	LastCrashTime   *time.Time    `json:"last_crash_time,omitempty"`
	LastNewPathTime *time.Time    `json:"last_new_path_time,omitempty"`
}

// CrashInfo contains information about a discovered crash
type CrashInfo struct {
	ID           string            `json:"id"`
	Input        []byte            `json:"input"`
	StackTrace   string            `json:"stack_trace"`
	Signal       int               `json:"signal"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	FuzzerType   string            `json:"fuzzer_type"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ProgressUpdate represents a fuzzing progress update
type ProgressUpdate struct {
	Timestamp      time.Time `json:"timestamp"`
	Executions     uint64    `json:"executions"`
	ExecsPerSecond uint64    `json:"execs_per_second"`
	CorpusSize     uint64    `json:"corpus_size"`
	Coverage       float64   `json:"coverage"`
	CrashCount     uint64    `json:"crash_count"`
}

// FuzzerLogger defines logging interface for fuzzers
type FuzzerLogger interface {
	// LogOutput logs fuzzer stdout/stderr output
	LogOutput(output string)

	// LogError logs error messages
	LogError(err error)

	// LogCrash logs crash discovery
	LogCrash(crash *CrashInfo)

	// LogProgress logs progress updates
	LogProgress(progress *ProgressUpdate)

	// GetReader returns a reader for the log output
	GetReader() io.Reader
}

// FuzzerHooks defines lifecycle hooks for fuzzer events
type FuzzerHooks interface {
	// OnStart is called when fuzzing starts
	OnStart(fuzzer Fuzzer) error

	// OnStop is called when fuzzing stops
	OnStop(fuzzer Fuzzer, stats *FuzzerStats) error

	// OnCrash is called when a crash is discovered
	OnCrash(fuzzer Fuzzer, crash *CrashInfo) error

	// OnProgress is called on progress updates
	OnProgress(fuzzer Fuzzer, progress *ProgressUpdate) error

	// OnError is called when an error occurs
	OnError(fuzzer Fuzzer, err error)
}

// SupportedFuzzerType represents a supported fuzzer engine
type SupportedFuzzerType string

const (
	// FuzzerTypeLibFuzzer represents libFuzzer engine
	FuzzerTypeLibFuzzer SupportedFuzzerType = "libfuzzer"

	// FuzzerTypeAFLPlusPlus represents AFL++ engine
	FuzzerTypeAFLPlusPlus SupportedFuzzerType = "afl++"

	// FuzzerTypeHonggfuzz represents Honggfuzz engine
	FuzzerTypeHonggfuzz SupportedFuzzerType = "honggfuzz"
)

// String returns the string representation of the fuzzer type
func (t SupportedFuzzerType) String() string {
	return string(t)
}

// IsValid checks if the fuzzer type is valid
func (t SupportedFuzzerType) IsValid() bool {
	switch t {
	case FuzzerTypeLibFuzzer, FuzzerTypeAFLPlusPlus, FuzzerTypeHonggfuzz:
		return true
	default:
		return false
	}
}

// FuzzerCapabilities describes what a fuzzer implementation supports
type FuzzerCapabilities struct {
	SupportsParallel        bool `json:"supports_parallel"`
	SupportsMinimization    bool `json:"supports_minimization"`
	SupportsDictionary      bool `json:"supports_dictionary"`
	SupportsCoverage        bool `json:"supports_coverage"`
	SupportsTimeout         bool `json:"supports_timeout"`
	SupportsMemoryLimit     bool `json:"supports_memory_limit"`
	RequiresInstrumentation bool `json:"requires_instrumentation"`
}

// FuzzerMetrics defines metrics that can be collected from fuzzers
type FuzzerMetrics interface {
	// GetTotalExecutions returns total number of executions
	GetTotalExecutions() uint64

	// GetExecsPerSecond returns current execution rate
	GetExecsPerSecond() uint64

	// GetCoverage returns code coverage percentage
	GetCoverage() float64

	// GetMemoryUsage returns current memory usage in bytes
	GetMemoryUsage() uint64

	// GetUptime returns how long the fuzzer has been running
	GetUptime() time.Duration

	// Reset resets all metrics
	Reset()
}
