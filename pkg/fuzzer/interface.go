package fuzzer

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// Fuzzer defines the interface for all fuzzing engines
type Fuzzer interface {
	// Basic identification
	Name() string
	Type() FuzzerType
	Version() string
	GetCapabilities() []string
	
	// Configuration and initialization
	Configure(config FuzzConfig) error
	Initialize() error
	Validate() error
	
	// Execution control
	Start(ctx context.Context) error
	Stop() error
	Pause() error
	Resume() error
	
	// Status and monitoring
	GetStatus() FuzzerStatus
	GetStats() FuzzerStats
	GetProgress() FuzzerProgress
	IsRunning() bool
	
	// Results and output
	GetResults() (*FuzzerResults, error)
	GetCrashes() ([]*common.CrashResult, error)
	GetCoverage() (*common.CoverageResult, error)
	GetCorpus() ([]*CorpusEntry, error)
	
	// Event handling
	SetEventHandler(handler EventHandler)
	
	// Cleanup
	Cleanup() error
}

// FuzzerType represents different types of fuzzers
type FuzzerType string

const (
	FuzzerTypeAFL      FuzzerType = "afl++"
	FuzzerTypeLibFuzzer FuzzerType = "libfuzzer"
	FuzzerTypeHonggfuzz FuzzerType = "honggfuzz"
	FuzzerTypeCustom   FuzzerType = "custom"
)

// FuzzerStatus represents the current status of a fuzzer
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

// FuzzConfig holds configuration for fuzzer execution
type FuzzConfig struct {
	// Target configuration
	Target        string            `json:"target" yaml:"target"`
	TargetArgs    []string          `json:"target_args" yaml:"target_args"`
	WorkDirectory string            `json:"work_directory" yaml:"work_directory"`
	
	// Execution parameters
	Duration      time.Duration     `json:"duration" yaml:"duration"`
	Timeout       time.Duration     `json:"timeout" yaml:"timeout"`
	MemoryLimit   int64             `json:"memory_limit" yaml:"memory_limit"`
	
	// Input configuration
	SeedDirectory string            `json:"seed_directory" yaml:"seed_directory"`
	Dictionary    string            `json:"dictionary" yaml:"dictionary"`
	InputFormat   string            `json:"input_format" yaml:"input_format"`
	
	// Fuzzing strategy
	Strategy      FuzzStrategy      `json:"strategy" yaml:"strategy"`
	Mutators      []string          `json:"mutators" yaml:"mutators"`
	Coverage      CoverageType      `json:"coverage" yaml:"coverage"`
	
	// Output configuration
	OutputDirectory string           `json:"output_directory" yaml:"output_directory"`
	CrashDirectory  string           `json:"crash_directory" yaml:"crash_directory"`
	CorpusDirectory string           `json:"corpus_directory" yaml:"corpus_directory"`
	
	// Fuzzer-specific options
	FuzzerOptions map[string]any `json:"fuzzer_options" yaml:"fuzzer_options"`
	
	// Resource limits
	MaxCrashes    int               `json:"max_crashes" yaml:"max_crashes"`
	MaxCorpusSize int64             `json:"max_corpus_size" yaml:"max_corpus_size"`
	MaxExecutions int64             `json:"max_executions" yaml:"max_executions"`
	
	// Monitoring
	StatsInterval time.Duration     `json:"stats_interval" yaml:"stats_interval"`
	LogLevel      string            `json:"log_level" yaml:"log_level"`
	EnableTracing bool              `json:"enable_tracing" yaml:"enable_tracing"`
}

// FuzzStrategy represents different fuzzing strategies
type FuzzStrategy string

const (
	StrategyRandom      FuzzStrategy = "random"
	StrategyDictionary  FuzzStrategy = "dictionary"
	StrategyGrammar     FuzzStrategy = "grammar"
	StrategyEvolutionary FuzzStrategy = "evolutionary"
	StrategyStructural  FuzzStrategy = "structural"
	StrategyCoverage    FuzzStrategy = "coverage"
)

// CoverageType represents different coverage types
type CoverageType string

const (
	CoverageEdge     CoverageType = "edge"
	CoverageBlock    CoverageType = "block"
	CoverageFunction CoverageType = "function"
	CoveragePath     CoverageType = "path"
	CoverageValue    CoverageType = "value"
)

// FuzzerStats contains runtime statistics
type FuzzerStats struct {
	StartTime       time.Time     `json:"start_time"`
	ElapsedTime     time.Duration `json:"elapsed_time"`
	Executions      int64         `json:"executions"`
	ExecPerSecond   float64       `json:"exec_per_second"`
	
	// Coverage statistics
	TotalEdges      int           `json:"total_edges"`
	CoveredEdges    int           `json:"covered_edges"`
	CoveragePercent float64       `json:"coverage_percent"`
	
	// Crash statistics
	UniqueCrashes   int           `json:"unique_crashes"`
	TotalCrashes    int           `json:"total_crashes"`
	CrashRate       float64       `json:"crash_rate"`
	
	// Corpus statistics
	CorpusSize      int           `json:"corpus_size"`
	NewPaths        int           `json:"new_paths"`
	PathsTotal      int           `json:"paths_total"`
	
	// Performance metrics
	CPUUsage        float64       `json:"cpu_usage"`
	MemoryUsage     int64         `json:"memory_usage"`
	DiskUsage       int64         `json:"disk_usage"`
	
	// Quality metrics
	Stability       float64       `json:"stability"`
	FuzzingRatio    float64       `json:"fuzzing_ratio"`
	LastNewPath     time.Time     `json:"last_new_path"`
	LastCrash       time.Time     `json:"last_crash"`
}

// FuzzerProgress tracks fuzzing progress
type FuzzerProgress struct {
	Phase           string        `json:"phase"`
	ProgressPercent float64       `json:"progress_percent"`
	CurrentInput    string        `json:"current_input"`
	QueuePosition   int           `json:"queue_position"`
	QueueSize       int           `json:"queue_size"`
	ETA             time.Duration `json:"eta"`
	LastUpdate      time.Time     `json:"last_update"`
}

// FuzzerResults contains all results from a fuzzing session
type FuzzerResults struct {
	Summary     ResultSummary          `json:"summary"`
	Crashes     []*common.CrashResult  `json:"crashes"`
	Coverage    *common.CoverageResult `json:"coverage"`
	Corpus      []*CorpusEntry         `json:"corpus"`
	Performance PerformanceMetrics     `json:"performance"`
	Artifacts   []Artifact             `json:"artifacts"`
}

// ResultSummary provides a high-level summary of results
type ResultSummary struct {
	TotalExecutions   int64         `json:"total_executions"`
	ExecutionTime     time.Duration `json:"execution_time"`
	UniqueCrashes     int           `json:"unique_crashes"`
	CoverageAchieved  float64       `json:"coverage_achieved"`
	NewInputsFound    int           `json:"new_inputs_found"`
	Success           bool          `json:"success"`
	ExitReason        string        `json:"exit_reason"`
}

// CorpusEntry represents a single corpus entry
type CorpusEntry struct {
	ID          string            `json:"id"`
	FileName    string            `json:"file_name"`
	Size        int64             `json:"size"`
	Hash        string            `json:"hash"`
	Coverage    []int             `json:"coverage"`
	Timestamp   time.Time         `json:"timestamp"`
	Source      string            `json:"source"`
	Energy      float64           `json:"energy"`
	Executions  int64             `json:"executions"`
	Metadata    map[string]any `json:"metadata"`
}

// PerformanceMetrics tracks performance during fuzzing
type PerformanceMetrics struct {
	AverageExecSpeed  float64       `json:"average_exec_speed"`
	PeakExecSpeed     float64       `json:"peak_exec_speed"`
	AverageCPU        float64       `json:"average_cpu"`
	PeakMemory        int64         `json:"peak_memory"`
	TotalDiskIO       int64         `json:"total_disk_io"`
	NetworkTraffic    int64         `json:"network_traffic"`
	StartupTime       time.Duration `json:"startup_time"`
	ShutdownTime      time.Duration `json:"shutdown_time"`
}

// Artifact represents an output artifact from fuzzing
type Artifact struct {
	Type        ArtifactType `json:"type"`
	Name        string       `json:"name"`
	Path        string       `json:"path"`
	Size        int64        `json:"size"`
	Hash        string       `json:"hash"`
	Description string       `json:"description"`
	Timestamp   time.Time    `json:"timestamp"`
}

// ArtifactType represents different types of artifacts
type ArtifactType string

const (
	ArtifactCrash    ArtifactType = "crash"
	ArtifactCorpus   ArtifactType = "corpus"
	ArtifactLog      ArtifactType = "log"
	ArtifactStats    ArtifactType = "stats"
	ArtifactPlot     ArtifactType = "plot"
	ArtifactReport   ArtifactType = "report"
)

// EventHandler handles fuzzer events
type EventHandler interface {
	OnStart(fuzzer Fuzzer)
	OnStop(fuzzer Fuzzer, reason string)
	OnCrash(fuzzer Fuzzer, crash *common.CrashResult)
	OnNewPath(fuzzer Fuzzer, path *CorpusEntry)
	OnStats(fuzzer Fuzzer, stats FuzzerStats)
	OnError(fuzzer Fuzzer, err error)
	OnProgress(fuzzer Fuzzer, progress FuzzerProgress)
}

// DefaultEventHandler provides a default implementation
type DefaultEventHandler struct{}

func (h *DefaultEventHandler) OnStart(fuzzer Fuzzer)                              {}
func (h *DefaultEventHandler) OnStop(fuzzer Fuzzer, reason string)               {}
func (h *DefaultEventHandler) OnCrash(fuzzer Fuzzer, crash *common.CrashResult)  {}
func (h *DefaultEventHandler) OnNewPath(fuzzer Fuzzer, path *CorpusEntry)        {}
func (h *DefaultEventHandler) OnStats(fuzzer Fuzzer, stats FuzzerStats)          {}
func (h *DefaultEventHandler) OnError(fuzzer Fuzzer, err error)                  {}
func (h *DefaultEventHandler) OnProgress(fuzzer Fuzzer, progress FuzzerProgress) {}

// Factory creates fuzzer instances
type Factory interface {
	CreateFuzzer(fuzzerType FuzzerType) (Fuzzer, error)
	ListAvailableFuzzers() []FuzzerInfo
	GetFuzzerInfo(fuzzerType FuzzerType) (*FuzzerInfo, error)
}

// FuzzerInfo provides information about a fuzzer
type FuzzerInfo struct {
	Type         FuzzerType `json:"type"`
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	Description  string     `json:"description"`
	Capabilities []string   `json:"capabilities"`
	SupportedOS  []string   `json:"supported_os"`
	Required     []string   `json:"required"`
	Optional     []string   `json:"optional"`
	Homepage     string     `json:"homepage"`
	License      string     `json:"license"`
}

// Manager coordinates multiple fuzzers
type Manager interface {
	RegisterFuzzer(fuzzerType FuzzerType, factory func() (Fuzzer, error)) error
	CreateFuzzer(fuzzerType FuzzerType, config FuzzConfig) (Fuzzer, error)
	StartFuzzing(fuzzer Fuzzer) error
	StopFuzzing(fuzzer Fuzzer) error
	GetActiveFuzzers() []Fuzzer
	GetFuzzerStats(fuzzer Fuzzer) (*FuzzerStats, error)
	SetGlobalEventHandler(handler EventHandler)
}

// Validator validates fuzzer configurations and targets
type Validator interface {
	ValidateConfig(config FuzzConfig) error
	ValidateTarget(target string) error
	ValidateSeedCorpus(directory string) error
	ValidateDictionary(dictionary string) error
	CheckDependencies(fuzzerType FuzzerType) error
}

// Monitor provides fuzzing monitoring capabilities
type Monitor interface {
	StartMonitoring(fuzzer Fuzzer) error
	StopMonitoring(fuzzer Fuzzer) error
	GetMetrics(fuzzer Fuzzer) (*FuzzerStats, error)
	SetAlerts(alerts []Alert) error
	GetAlerts() []Alert
}

// Alert represents a monitoring alert
type Alert struct {
	ID          string      `json:"id"`
	Type        AlertType   `json:"type"`
	Threshold   float64     `json:"threshold"`
	Condition   string      `json:"condition"`
	Action      string      `json:"action"`
	Enabled     bool        `json:"enabled"`
	Description string      `json:"description"`
}

// AlertType represents different types of alerts
type AlertType string

const (
	AlertCrashRate      AlertType = "crash_rate"
	AlertCoverageStall  AlertType = "coverage_stall"
	AlertPerformance    AlertType = "performance"
	AlertMemoryUsage    AlertType = "memory_usage"
	AlertExecutionSpeed AlertType = "execution_speed"
	AlertError          AlertType = "error"
)

// Utils provides utility functions for fuzzing
type Utils interface {
	CalculateHash(data []byte) string
	MinimizeInput(input []byte, target string) ([]byte, error)
	ValidateInput(input []byte, format string) error
	ConvertFormat(input []byte, fromFormat, toFormat string) ([]byte, error)
	AnalyzeCrash(crash *common.CrashResult) (*CrashAnalysis, error)
}

// CrashAnalysis provides detailed crash analysis
type CrashAnalysis struct {
	Type            string            `json:"type"`
	Severity        string            `json:"severity"`
	Exploitability  string            `json:"exploitability"`
	CallStack       []string          `json:"call_stack"`
	Registers       map[string]string `json:"registers"`
	MemoryRegions   []MemoryRegion    `json:"memory_regions"`
	Disassembly     []string          `json:"disassembly"`
	SourceLocation  *SourceLocation   `json:"source_location"`
	Recommendations []string          `json:"recommendations"`
}

// MemoryRegion represents a memory region in crash analysis
type MemoryRegion struct {
	Address     uint64 `json:"address"`
	Size        uint64 `json:"size"`
	Permissions string `json:"permissions"`
	Content     string `json:"content"`
	Type        string `json:"type"`
}

// SourceLocation represents source code location
type SourceLocation struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Function string `json:"function"`
}

// Common error types
type FuzzerError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Fuzzer  string `json:"fuzzer"`
	Code    int    `json:"code"`
}

func (e *FuzzerError) Error() string {
	return e.Message
}

// Error constants
const (
	ErrInvalidConfig     = "invalid_config"
	ErrTargetNotFound    = "target_not_found"
	ErrPermissionDenied  = "permission_denied"
	ErrInsufficientSpace = "insufficient_space"
	ErrTimeout           = "timeout"
	ErrCrash             = "crash"
	ErrInternal          = "internal_error"
)