package types

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// FuzzerConfig represents configuration for a fuzzer instance
type FuzzerConfig struct {
	// Target binary configuration
	Target     string   `json:"target,omitempty"`      // Path to target binary
	TargetArgs []string `json:"target_args,omitempty"` // Arguments to pass to target

	// Common configuration options
	Timeout         time.Duration     `json:"timeout,omitempty"`
	MaxDuration     time.Duration     `json:"max_duration,omitempty"` // Maximum time to run fuzzer before graceful exit
	MemoryLimit     uint64            `json:"memory_limit,omitempty"`
	Workers         int               `json:"workers,omitempty"`
	Dictionary      string            `json:"dictionary,omitempty"`
	SeedCorpus      string            `json:"seed_corpus,omitempty"`
	OutputDir       string            `json:"output_dir"`
	MaxLen          int               `json:"max_len,omitempty"`
	MinLen          int               `json:"min_len,omitempty"`
	OnlyCrashes     bool              `json:"only_crashes,omitempty"`
	PrintFinalStats bool              `json:"print_final_stats,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`

	// Fuzzer-specific options
	LibFuzzerOptions   *LibFuzzerOptions   `json:"libfuzzer_options,omitempty"`
	AFLPlusPlusOptions *AFLPlusPlusOptions `json:"afl_plus_plus_options,omitempty"`
	HonggfuzzOptions   *HonggfuzzOptions   `json:"honggfuzz_options,omitempty"`

	// Coverage options
	EnableCoverage bool   `json:"enable_coverage,omitempty"` // Enable coverage collection
	CoverageFormat string `json:"coverage_format,omitempty"` // Coverage output format (lcov, html, json, profdata, sancov)
	CoverageDir    string `json:"coverage_dir,omitempty"`    // Directory to store coverage data

	// Advanced options
	ExtraArgs     []string          `json:"extra_args,omitempty"`
	CustomOptions map[string]string `json:"custom_options,omitempty"`

	// Output writer for capturing fuzzer output to log file
	OutputWriter io.Writer `json:"-"`
}

// LibFuzzerOptions contains libFuzzer-specific configuration
type LibFuzzerOptions struct {
	Runs                int    `json:"runs,omitempty"`
	MaxTotalTime        int    `json:"max_total_time,omitempty"`
	LenControl          int    `json:"len_control,omitempty"`
	SeedInputs          string `json:"seed_inputs,omitempty"`
	KeepSeed            int    `json:"keep_seed,omitempty"`
	CrossOver           int    `json:"cross_over,omitempty"`
	MutateDepth         int    `json:"mutate_depth,omitempty"`
	ReduceInputs        int    `json:"reduce_inputs,omitempty"`
	UseCounters         int    `json:"use_counters,omitempty"`
	UseMemmem           int    `json:"use_memmem,omitempty"`
	UseValueProfile     int    `json:"use_value_profile,omitempty"`
	ShrinkCorpus        int    `json:"shrink,omitempty"`
	PrintPCs            int    `json:"print_pcs,omitempty"`
	PrintFuncs          int    `json:"print_funcs,omitempty"`
	PrintCoverage       int    `json:"print_coverage,omitempty"`
	PrintCorpusStats    int    `json:"print_corpus_stats,omitempty"`
	PrintFinalStats     int    `json:"print_final_stats,omitempty"`
	PrintCoveragePCs    int    `json:"print_coverage_pcs,omitempty"` // Print coverage PCs for analysis
	DetectLeaks         int    `json:"detect_leaks,omitempty"`
	PurgeAllocatorCache int    `json:"purge_allocator_cache,omitempty"`
	TraceMalloc         int    `json:"trace_malloc,omitempty"`
	RssLimitMB          int    `json:"rss_limit_mb,omitempty"`
	MallocLimitMB       int    `json:"malloc_limit_mb,omitempty"`
}

// AFLPlusPlusOptions contains AFL++-specific configuration
type AFLPlusPlusOptions struct {
	Mode            string `json:"mode,omitempty"` // e.g., "fast", "explore", "coe"
	PowerSchedule   string `json:"power_schedule,omitempty"`
	InputDir        string `json:"input_dir"`
	SkipCrashed     bool   `json:"skip_crashed,omitempty"`
	SkipHangs       bool   `json:"skip_hangs,omitempty"`
	NoUI            bool   `json:"no_ui,omitempty"`
	BenchUntilCrash bool   `json:"bench_until_crash,omitempty"`
	Debug           bool   `json:"debug,omitempty"`
	Deterministic   bool   `json:"deterministic,omitempty"`
	DumbMode        bool   `json:"dumb_mode,omitempty"`
	MainNode        bool   `json:"main_node,omitempty"`
	SecondaryNode   bool   `json:"secondary_node,omitempty"`
	FileExtension   string `json:"file_extension,omitempty"`
	TargetEnv       string `json:"target_env,omitempty"`
	Affinity        string `json:"affinity,omitempty"`
	BannerFile      string `json:"banner_file,omitempty"`
	CustomMutator   string `json:"custom_mutator,omitempty"`
	PythonModule    string `json:"python_module,omitempty"`
	QemuMode        bool   `json:"qemu_mode,omitempty"`
	UniMode         bool   `json:"unicorn_mode,omitempty"`
	FraidaMode      bool   `json:"frida_mode,omitempty"`
	CmplogMode      bool   `json:"cmplog_mode,omitempty"`
	WineMode        bool   `json:"wine_mode,omitempty"`
	UseAFLCov       bool   `json:"use_afl_cov,omitempty"`          // Enable AFL coverage analysis
	LLVMMode        bool   `json:"llvm_mode,omitempty"`            // Enable LLVM coverage mode
	SourceDir       string `json:"source_dir,omitempty"`           // Source directory for coverage analysis
	LLVMCovBinary   string `json:"llvm_cov_binary,omitempty"`      // Path to llvm-cov binary
	LLVMProfData    string `json:"llvm_profdata_binary,omitempty"` // Path to llvm-profdata binary
}

// HonggfuzzOptions contains Honggfuzz-specific configuration
type HonggfuzzOptions struct {
	InputDir            string   `json:"input_dir"`
	Threads             int      `json:"threads,omitempty"`
	RunTime             int      `json:"run_time,omitempty"`
	Iterations          uint64   `json:"iterations,omitempty"`
	KeepOutput          bool     `json:"keep_output,omitempty"`
	Debug               bool     `json:"debug,omitempty"`
	Verbose             bool     `json:"verbose,omitempty"`
	QuietMode           bool     `json:"quiet_mode,omitempty"`
	NoFeedback          bool     `json:"no_feedback,omitempty"`
	SaveAll             bool     `json:"save_all,omitempty"`
	SaveSmaller         bool     `json:"save_smaller,omitempty"`
	FilterCrashingFiles bool     `json:"filter_crashing_files,omitempty"`
	ClearWorkspace      bool     `json:"clear_workspace,omitempty"`
	InstrumentMode      string   `json:"instrument_mode,omitempty"`
	EnvVars             []string `json:"env_vars,omitempty"`
	ExtensionFilter     string   `json:"extension_filter,omitempty"`
	CovBitmapSize       int      `json:"cov_bitmap_size,omitempty"`
	DynamicCutoff       bool     `json:"dynamic_cutoff,omitempty"`
	MaxFileSize         int      `json:"max_file_size,omitempty"`
	ReportFile          string   `json:"report_file,omitempty"`
	MonitorSigAbrt      bool     `json:"monitor_sigabrt,omitempty"`
	NoMinify            bool     `json:"no_minify,omitempty"`
	ExitUponCrash       bool     `json:"exit_upon_crash,omitempty"`
	PostProcessorCmd    string   `json:"post_processor_cmd,omitempty"`
	SocketFuzzing       bool     `json:"socket_fuzzing,omitempty"`
	StdinMode           bool     `json:"stdin_mode,omitempty"`      // Use stdin for input instead of file (-s flag)
	Persistent          bool     `json:"persistent,omitempty"`      // Use persistent mode for targets with HF_ITER
	Sancov              bool     `json:"sancov,omitempty"`          // Enable sanitizer coverage
	CoverageReport      bool     `json:"coverage_report,omitempty"` // Generate coverage report
	NetDriver           bool     `json:"net_driver,omitempty"`
	NetBindTo           string   `json:"net_bind_to,omitempty"`
	NetConnectTo        string   `json:"net_connect_to,omitempty"`
	SancovDir           string   `json:"sancov_dir,omitempty"`
	AsLimit             int      `json:"as_limit,omitempty"`
	RssLimit            int      `json:"rss_limit,omitempty"`
	DataLimit           int      `json:"data_limit,omitempty"`
	StackLimit          int      `json:"stack_limit,omitempty"`
}

// NewFuzzerConfig creates a new fuzzer configuration with defaults
func NewFuzzerConfig(outputDir string) (*FuzzerConfig, error) {
	if outputDir == "" {
		return nil, errors.New("output directory cannot be empty")
	}

	return &FuzzerConfig{
		OutputDir:       outputDir,
		Workers:         1,
		PrintFinalStats: true,
		Environment:     make(map[string]string),
		CustomOptions:   make(map[string]string),
	}, nil
}

// Validate ensures the configuration is valid
func (c *FuzzerConfig) Validate() error {
	if c.OutputDir == "" {
		return errors.New("output directory cannot be empty")
	}

	if c.MinLen < 0 {
		return errors.New("minimum length cannot be negative")
	}

	if c.MaxLen > 0 && c.MinLen > c.MaxLen {
		return errors.New("minimum length cannot be greater than maximum length")
	}

	if c.Workers < 1 {
		return errors.New("workers must be at least 1")
	}

	if c.MemoryLimit > 0 && c.MemoryLimit < 1024*1024 { // Less than 1MB
		return errors.New("memory limit too small (minimum 1MB)")
	}

	// Validate coverage options
	if c.EnableCoverage {
		if c.CoverageDir == "" {
			return errors.New("coverage directory must be specified when coverage is enabled")
		}
		if c.CoverageFormat == "" {
			return errors.New("coverage format must be specified when coverage is enabled")
		}
		validFormats := map[string]bool{
			"lcov":     true,
			"html":     true,
			"json":     true,
			"profdata": true,
			"sancov":   true,
		}
		if !validFormats[c.CoverageFormat] {
			return fmt.Errorf("invalid coverage format: %s", c.CoverageFormat)
		}
	}

	// Validate fuzzer-specific options
	if c.LibFuzzerOptions != nil {
		if err := c.LibFuzzerOptions.Validate(); err != nil {
			return fmt.Errorf("invalid libfuzzer options: %w", err)
		}
		if err := c.LibFuzzerOptions.ValidateCoverage(c.EnableCoverage); err != nil {
			return fmt.Errorf("invalid libfuzzer coverage options: %w", err)
		}
	}

	if c.AFLPlusPlusOptions != nil {
		if err := c.AFLPlusPlusOptions.Validate(); err != nil {
			return fmt.Errorf("invalid AFL++ options: %w", err)
		}
		if err := c.AFLPlusPlusOptions.ValidateCoverage(c.EnableCoverage); err != nil {
			return fmt.Errorf("invalid AFL++ coverage options: %w", err)
		}
	}

	if c.HonggfuzzOptions != nil {
		if err := c.HonggfuzzOptions.Validate(); err != nil {
			return fmt.Errorf("invalid honggfuzz options: %w", err)
		}
		if err := c.HonggfuzzOptions.ValidateCoverage(c.EnableCoverage); err != nil {
			return fmt.Errorf("invalid honggfuzz coverage options: %w", err)
		}
	}

	return nil
}

// SetEnvironmentVariable sets an environment variable for the fuzzer
func (c *FuzzerConfig) SetEnvironmentVariable(key, value string) {
	if c.Environment == nil {
		c.Environment = make(map[string]string)
	}
	c.Environment[key] = value
}

// SetCustomOption sets a custom fuzzer option
func (c *FuzzerConfig) SetCustomOption(key, value string) {
	if c.CustomOptions == nil {
		c.CustomOptions = make(map[string]string)
	}
	c.CustomOptions[key] = value
}

// GetCustomOption retrieves a custom option value
func (c *FuzzerConfig) GetCustomOption(key string) (string, bool) {
	value, exists := c.CustomOptions[key]
	return value, exists
}

// AddExtraArg adds an extra command-line argument
func (c *FuzzerConfig) AddExtraArg(arg string) {
	c.ExtraArgs = append(c.ExtraArgs, arg)
}

// Validate validates LibFuzzer-specific options
func (o *LibFuzzerOptions) Validate() error {
	if o.Runs < 0 {
		return errors.New("runs cannot be negative")
	}
	if o.MaxTotalTime < 0 {
		return errors.New("max total time cannot be negative")
	}
	if o.RssLimitMB < 0 {
		return errors.New("RSS limit cannot be negative")
	}
	if o.MallocLimitMB < 0 {
		return errors.New("malloc limit cannot be negative")
	}
	return nil
}

// ValidateCoverage validates LibFuzzer-specific coverage options
func (o *LibFuzzerOptions) ValidateCoverage(enableCoverage bool) error {
	if !enableCoverage {
		return nil
	}

	// When coverage is enabled and no specific options are set, enable sensible defaults
	// that provide maximum information to the end user
	if o.UseCounters == 0 && o.PrintCoveragePCs == 0 && o.PrintCoverage == 0 {
		// Enable counter-based coverage (more precise coverage tracking)
		o.UseCounters = 1
		// Enable coverage summary printing
		o.PrintCoverage = 1
		// Enable final stats printing for comprehensive information
		o.PrintFinalStats = 1
	}

	return nil
}

// Validate validates AFL++-specific options
func (o *AFLPlusPlusOptions) Validate() error {
	if o.InputDir == "" {
		return errors.New("input directory cannot be empty for AFL++")
	}

	// Validate mode if specified
	if o.Mode != "" {
		validModes := map[string]bool{
			"fast":    true,
			"explore": true,
			"coe":     true,
			"lin":     true,
			"quad":    true,
			"exploit": true,
			"rare":    true,
		}
		if !validModes[o.Mode] {
			return fmt.Errorf("invalid AFL++ mode: %s", o.Mode)
		}
	}

	// Cannot be both main and secondary node
	if o.MainNode && o.SecondaryNode {
		return errors.New("cannot be both main and secondary node")
	}

	return nil
}

// ValidateCoverage validates AFL++-specific coverage options
func (o *AFLPlusPlusOptions) ValidateCoverage(enableCoverage bool) error {
	if !enableCoverage {
		return nil
	}

	// When coverage is enabled, validate related fields
	if o.UseAFLCov {
		if o.SourceDir == "" {
			return errors.New("source_dir must be specified when use_afl_cov is enabled")
		}
	}

	if o.LLVMMode {
		if o.LLVMCovBinary != "" && o.LLVMProfData == "" {
			return errors.New("llvm_profdata_binary must be specified when llvm_cov_binary is used")
		}
	}

	return nil
}

// Validate validates Honggfuzz-specific options
func (o *HonggfuzzOptions) Validate() error {
	if o.InputDir == "" {
		return errors.New("input directory cannot be empty for Honggfuzz")
	}

	if o.Threads < 0 {
		return errors.New("threads cannot be negative")
	}

	if o.RunTime < 0 {
		return errors.New("run time cannot be negative")
	}

	if o.MaxFileSize < 0 {
		return errors.New("max file size cannot be negative")
	}

	if o.AsLimit < 0 || o.RssLimit < 0 || o.DataLimit < 0 || o.StackLimit < 0 {
		return errors.New("resource limits cannot be negative")
	}

	return nil
}

// ValidateCoverage validates Honggfuzz-specific coverage options
func (o *HonggfuzzOptions) ValidateCoverage(enableCoverage bool) error {
	if !enableCoverage {
		return nil
	}

	// When coverage is enabled and sancov is used, validate sancov directory
	if o.Sancov && o.SancovDir == "" {
		return errors.New("sancov_dir must be specified when sancov is enabled")
	}

	return nil
}

// ConfigBuilder provides a fluent interface for building fuzzer configurations
type ConfigBuilder struct {
	config *FuzzerConfig
	err    error
}

// NewConfigBuilder creates a new configuration builder
func NewConfigBuilder(outputDir string) *ConfigBuilder {
	config, err := NewFuzzerConfig(outputDir)
	return &ConfigBuilder{
		config: config,
		err:    err,
	}
}

// WithTimeout sets the timeout
func (b *ConfigBuilder) WithTimeout(timeout time.Duration) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.Timeout = timeout
	return b
}

// WithMaxDuration sets the maximum duration for fuzzing
func (b *ConfigBuilder) WithMaxDuration(duration time.Duration) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.MaxDuration = duration
	return b
}

// WithMemoryLimit sets the memory limit
func (b *ConfigBuilder) WithMemoryLimit(limit uint64) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.MemoryLimit = limit
	return b
}

// WithWorkers sets the number of workers
func (b *ConfigBuilder) WithWorkers(workers int) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	if workers < 1 {
		b.err = errors.New("workers must be at least 1")
		return b
	}
	b.config.Workers = workers
	return b
}

// WithDictionary sets the dictionary path
func (b *ConfigBuilder) WithDictionary(path string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.Dictionary = path
	return b
}

// WithSeedCorpus sets the seed corpus path
func (b *ConfigBuilder) WithSeedCorpus(path string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.SeedCorpus = path
	return b
}

// WithCoverage enables coverage collection with specified format and directory
func (b *ConfigBuilder) WithCoverage(enabled bool, format, dir string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.EnableCoverage = enabled
	if enabled {
		b.config.CoverageFormat = format
		b.config.CoverageDir = dir
	}
	return b
}

// WithCoverageFormat sets the coverage format
func (b *ConfigBuilder) WithCoverageFormat(format string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.CoverageFormat = format
	return b
}

// WithCoverageDir sets the coverage directory
func (b *ConfigBuilder) WithCoverageDir(dir string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.CoverageDir = dir
	return b
}

// WithLibFuzzerOptions sets libFuzzer-specific options
func (b *ConfigBuilder) WithLibFuzzerOptions(options *LibFuzzerOptions) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.LibFuzzerOptions = options
	return b
}

// WithAFLPlusPlusOptions sets AFL++-specific options
func (b *ConfigBuilder) WithAFLPlusPlusOptions(options *AFLPlusPlusOptions) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.AFLPlusPlusOptions = options
	return b
}

// WithHonggfuzzOptions sets Honggfuzz-specific options
func (b *ConfigBuilder) WithHonggfuzzOptions(options *HonggfuzzOptions) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	b.config.HonggfuzzOptions = options
	return b
}

// Build validates and returns the configuration
func (b *ConfigBuilder) Build() (*FuzzerConfig, error) {
	if b.err != nil {
		return nil, b.err
	}

	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	return b.config, nil
}
