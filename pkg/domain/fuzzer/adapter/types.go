package adapter

import (
	"io"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// FuzzerType represents different types of fuzzers.
type FuzzerType string

const (
	FuzzerTypeAFL       FuzzerType = "afl++"
	FuzzerTypeLibFuzzer FuzzerType = "libfuzzer"
	FuzzerTypeHonggfuzz FuzzerType = "honggfuzz"
	FuzzerTypeCustom    FuzzerType = "custom"
)

// FuzzerStatus represents the current status of a fuzzer.
type FuzzerStatus string

const (
	StatusUninitialized FuzzerStatus = "uninitialized"
	StatusInitialized   FuzzerStatus = "initialized"
	StatusStarting      FuzzerStatus = "starting"
	StatusRunning       FuzzerStatus = "running"
	StatusPaused        FuzzerStatus = "paused"
	StatusStopping      FuzzerStatus = "stopping"
	StatusStopped       FuzzerStatus = "stopped"
	StatusError         FuzzerStatus = "error"
	StatusCompleted     FuzzerStatus = "completed"
)

// CoverageType represents different coverage types.
type CoverageType string

const (
	CoverageEdge     CoverageType = "edge"
	CoverageBlock    CoverageType = "block"
	CoverageFunction CoverageType = "function"
	CoveragePath     CoverageType = "path"
)

// FuzzConfig holds configuration for fuzzer execution.
type FuzzConfig struct {
	// Job identification
	JobID string `json:"job_id"`

	// Target configuration
	Target        string   `json:"target"`
	TargetArgs    []string `json:"target_args"`
	WorkDirectory string   `json:"work_directory"`

	// Execution parameters
	Duration    time.Duration `json:"duration"`
	Timeout     time.Duration `json:"timeout"`
	MemoryLimit int64         `json:"memory_limit"`

	// Input configuration
	SeedDirectory string `json:"seed_directory"`
	Dictionary    string `json:"dictionary"`

	// Coverage
	Coverage CoverageType `json:"coverage"`

	// Output configuration
	OutputDirectory string `json:"output_directory"`
	CrashDirectory  string `json:"crash_directory"`
	CorpusDirectory string `json:"corpus_directory"`

	// Fuzzer-specific options
	FuzzerOptions map[string]any `json:"fuzzer_options"`

	// Resource limits
	MaxCrashes    int   `json:"max_crashes"`
	MaxCorpusSize int64 `json:"max_corpus_size"`
	MaxExecutions int64 `json:"max_executions"`

	// Monitoring
	StatsInterval time.Duration `json:"stats_interval"`
	LogLevel      string        `json:"log_level"`

	// Output writer for capturing fuzzer output
	OutputWriter io.Writer `json:"-"`
}

// FuzzerStats contains runtime statistics.
type FuzzerStats struct {
	StartTime     time.Time     `json:"start_time"`
	ElapsedTime   time.Duration `json:"elapsed_time"`
	Executions    int64         `json:"executions"`
	ExecPerSecond float64       `json:"exec_per_second"`

	// Coverage statistics
	TotalEdges      int     `json:"total_edges"`
	CoveredEdges    int     `json:"covered_edges"`
	CoveragePercent float64 `json:"coverage_percent"`

	// Crash statistics
	UniqueCrashes int     `json:"unique_crashes"`
	TotalCrashes  int     `json:"total_crashes"`
	CrashRate     float64 `json:"crash_rate"`

	// Corpus statistics
	CorpusSize int `json:"corpus_size"`
	NewPaths   int `json:"new_paths"`
	PathsTotal int `json:"paths_total"`

	// Performance metrics
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage int64   `json:"memory_usage"`
	DiskUsage   int64   `json:"disk_usage"`

	// Quality metrics
	Stability    float64   `json:"stability"`
	FuzzingRatio float64   `json:"fuzzing_ratio"`
	LastNewPath  time.Time `json:"last_new_path"`
	LastCrash    time.Time `json:"last_crash"`
}

// FuzzerProgress tracks fuzzing progress.
type FuzzerProgress struct {
	Phase           string        `json:"phase"`
	ProgressPercent float64       `json:"progress_percent"`
	CurrentInput    string        `json:"current_input"`
	QueuePosition   int           `json:"queue_position"`
	QueueSize       int           `json:"queue_size"`
	ETA             time.Duration `json:"eta"`
	LastUpdate      time.Time     `json:"last_update"`
}

// FuzzerResults contains all results from a fuzzing session.
type FuzzerResults struct {
	Summary     ResultSummary         `json:"summary"`
	Crashes     []*common.CrashResult `json:"crashes"`
	Performance PerformanceMetrics    `json:"performance"`
}

// ResultSummary provides a high-level summary of results.
type ResultSummary struct {
	TotalExecutions  int64         `json:"total_executions"`
	ExecutionTime    time.Duration `json:"execution_time"`
	UniqueCrashes    int           `json:"unique_crashes"`
	CoverageAchieved float64       `json:"coverage_achieved"`
	NewInputsFound   int           `json:"new_inputs_found"`
	Success          bool          `json:"success"`
	ExitReason       string        `json:"exit_reason"`
}

// CorpusEntry represents a single corpus entry.
type CorpusEntry struct {
	ID         string         `json:"id"`
	FileName   string         `json:"file_name"`
	Size       int64          `json:"size"`
	Hash       string         `json:"hash"`
	Coverage   []int          `json:"coverage"`
	Timestamp  time.Time      `json:"timestamp"`
	Source     string         `json:"source"`
	Energy     float64        `json:"energy"`
	Executions int64          `json:"executions"`
	Metadata   map[string]any `json:"metadata"`
}

// PerformanceMetrics tracks performance during fuzzing.
type PerformanceMetrics struct {
	AverageExecSpeed float64       `json:"average_exec_speed"`
	PeakExecSpeed    float64       `json:"peak_exec_speed"`
	AverageCPU       float64       `json:"average_cpu"`
	PeakMemory       int64         `json:"peak_memory"`
	TotalDiskIO      int64         `json:"total_disk_io"`
	NetworkTraffic   int64         `json:"network_traffic"`
	StartupTime      time.Duration `json:"startup_time"`
	ShutdownTime     time.Duration `json:"shutdown_time"`
}

// ReproductionConfig configures crash reproduction attempts.
type ReproductionConfig struct {
	Attempts         int               `json:"attempts"`
	Timeout          time.Duration     `json:"timeout"`
	CollectDebugInfo bool              `json:"collect_debug_info"`
	OriginalCrashID  string            `json:"original_crash_id"`
	Environment      map[string]string `json:"environment"`
	Options          map[string]any    `json:"options"`
}
