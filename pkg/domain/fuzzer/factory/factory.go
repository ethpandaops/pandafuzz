package factory

import (
	"fmt"
	"sync"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// EngineConstructor is a function that creates a new fuzzer engine instance
type EngineConstructor func(target string, args []string) (types.Fuzzer, error)

// Factory implements the FuzzerFactory interface
type Factory struct {
	// engines stores registered fuzzer engine constructors
	engines map[string]EngineConstructor

	// engineInfo stores metadata about each engine
	engineInfo map[string]*EngineInfo

	// mu protects concurrent access to maps
	mu sync.RWMutex

	// dependencies for creating engines
	repositories Repositories
	services     Services
}

// EngineInfo contains metadata about a fuzzer engine
type EngineInfo struct {
	Type         string
	Name         string
	Description  string
	Capabilities *types.FuzzerCapabilities
	Version      string
}

// Repositories contains repository dependencies
type Repositories struct {
	// Add repository interfaces as needed
	// For example:
	// CrashRepository     CrashRepository
	// CorpusRepository    CorpusRepository
	// CoverageRepository  CoverageRepository
}

// Services contains service dependencies
type Services struct {
	// Add service interfaces as needed
	// For example:
	// MetricsService      MetricsService
	// NotificationService NotificationService
	// StorageService      StorageService
}

// Options for creating the factory
type Options struct {
	Repositories Repositories
	Services     Services
}

// NewFactory creates a new fuzzer factory instance
func NewFactory(opts Options) *Factory {
	return &Factory{
		engines:      make(map[string]EngineConstructor),
		engineInfo:   make(map[string]*EngineInfo),
		repositories: opts.Repositories,
		services:     opts.Services,
	}
}

// RegisterEngine registers a new fuzzer engine with the factory
func (f *Factory) RegisterEngine(engineType string, constructor EngineConstructor, info *EngineInfo) error {
	if engineType == "" {
		return fmt.Errorf("engine type cannot be empty")
	}

	if constructor == nil {
		return fmt.Errorf("engine constructor cannot be nil")
	}

	if info == nil {
		return fmt.Errorf("engine info cannot be nil")
	}

	// Validate that the engine type matches the info
	if info.Type != engineType {
		return fmt.Errorf("engine type mismatch: %s != %s", engineType, info.Type)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if already registered
	if _, exists := f.engines[engineType]; exists {
		return fmt.Errorf("engine type %s is already registered", engineType)
	}

	f.engines[engineType] = constructor
	f.engineInfo[engineType] = info

	return nil
}

// UnregisterEngine removes a fuzzer engine from the factory
func (f *Factory) UnregisterEngine(engineType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.engines[engineType]; !exists {
		return fmt.Errorf("engine type %s is not registered", engineType)
	}

	delete(f.engines, engineType)
	delete(f.engineInfo, engineType)

	return nil
}

// CreateFuzzer creates a new fuzzer instance of the specified type
func (f *Factory) CreateFuzzer(fuzzerType string, target string, args []string) (types.Fuzzer, error) {
	f.mu.RLock()
	constructor, exists := f.engines[fuzzerType]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unsupported fuzzer type: %s", fuzzerType)
	}

	// Validate inputs
	if target == "" {
		return nil, fmt.Errorf("target cannot be empty")
	}

	// Create the fuzzer instance
	fuzzer, err := constructor(target, args)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s fuzzer: %w", fuzzerType, err)
	}

	return fuzzer, nil
}

// GetSupportedTypes returns a list of supported fuzzer types
func (f *Factory) GetSupportedTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.engines))
	for engineType := range f.engines {
		types = append(types, engineType)
	}

	return types
}

// IsSupported checks if a fuzzer type is supported
func (f *Factory) IsSupported(fuzzerType string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	_, exists := f.engines[fuzzerType]
	return exists
}

// GetEngineInfo returns metadata about a specific engine
func (f *Factory) GetEngineInfo(engineType string) (*EngineInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	info, exists := f.engineInfo[engineType]
	if !exists {
		return nil, fmt.Errorf("engine type %s is not registered", engineType)
	}

	// Return a copy to prevent modification
	infoCopy := *info
	if info.Capabilities != nil {
		capCopy := *info.Capabilities
		infoCopy.Capabilities = &capCopy
	}

	return &infoCopy, nil
}

// GetAllEngineInfo returns metadata about all registered engines
func (f *Factory) GetAllEngineInfo() map[string]*EngineInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]*EngineInfo, len(f.engineInfo))
	for k, v := range f.engineInfo {
		// Create a copy
		infoCopy := *v
		if v.Capabilities != nil {
			capCopy := *v.Capabilities
			infoCopy.Capabilities = &capCopy
		}
		result[k] = &infoCopy
	}

	return result
}

// GetSupportedEngines returns detailed information about all supported engines
func (f *Factory) GetSupportedEngines() []EngineInfo {
	f.mu.RLock()
	defer f.mu.RUnlock()

	engines := make([]EngineInfo, 0, len(f.engineInfo))
	for _, info := range f.engineInfo {
		// Create a copy
		infoCopy := *info
		if info.Capabilities != nil {
			capCopy := *info.Capabilities
			infoCopy.Capabilities = &capCopy
		}
		engines = append(engines, infoCopy)
	}

	return engines
}

// CreateFuzzerWithConfig creates a fuzzer with a specific configuration
func (f *Factory) CreateFuzzerWithConfig(fuzzerType string, target string, args []string, config *types.FuzzerConfig) (types.Fuzzer, error) {
	// First create the fuzzer
	fuzzer, err := f.CreateFuzzer(fuzzerType, target, args)
	if err != nil {
		return nil, err
	}

	// Apply configuration if provided
	if config != nil {
		if err := fuzzer.Configure(config); err != nil {
			return nil, fmt.Errorf("failed to configure fuzzer: %w", err)
		}
	}

	return fuzzer, nil
}

// Builder provides a fluent interface for creating fuzzers
type Builder struct {
	factory    *Factory
	fuzzerType string
	target     string
	args       []string
	config     *types.FuzzerConfig
	hooks      types.FuzzerHooks
	logger     types.FuzzerLogger
}

// NewBuilder creates a new fuzzer builder
func (f *Factory) NewBuilder(fuzzerType string) *Builder {
	return &Builder{
		factory:    f,
		fuzzerType: fuzzerType,
		args:       []string{},
	}
}

// WithTarget sets the target for the fuzzer
func (b *Builder) WithTarget(target string) *Builder {
	b.target = target
	return b
}

// WithArgs sets the arguments for the fuzzer
func (b *Builder) WithArgs(args []string) *Builder {
	b.args = args
	return b
}

// WithConfig sets the configuration for the fuzzer
func (b *Builder) WithConfig(config *types.FuzzerConfig) *Builder {
	b.config = config
	return b
}

// WithHooks sets the lifecycle hooks for the fuzzer
func (b *Builder) WithHooks(hooks types.FuzzerHooks) *Builder {
	b.hooks = hooks
	return b
}

// WithLogger sets the logger for the fuzzer
func (b *Builder) WithLogger(logger types.FuzzerLogger) *Builder {
	b.logger = logger
	return b
}

// Build creates the fuzzer instance
func (b *Builder) Build() (types.Fuzzer, error) {
	// Create the fuzzer
	fuzzer, err := b.factory.CreateFuzzerWithConfig(b.fuzzerType, b.target, b.args, b.config)
	if err != nil {
		return nil, err
	}

	// Apply hooks if provided
	// Note: This would require the Fuzzer interface to have methods for setting hooks and logger
	// which are not currently defined in the interface. These would need to be added
	// or handled through type assertions to specific implementations.

	return fuzzer, nil
}

// DefaultEngineCapabilities returns default capabilities for engines
func DefaultEngineCapabilities() *types.FuzzerCapabilities {
	return &types.FuzzerCapabilities{
		SupportsParallel:        false,
		SupportsMinimization:    false,
		SupportsDictionary:      false,
		SupportsCoverage:        true,
		SupportsTimeout:         true,
		SupportsMemoryLimit:     true,
		RequiresInstrumentation: true,
	}
}

// ValidateEngine validates that an engine meets basic requirements
func ValidateEngine(fuzzer types.Fuzzer) error {
	if fuzzer == nil {
		return fmt.Errorf("fuzzer instance is nil")
	}

	// Check required methods work
	fuzzerType := fuzzer.GetType()
	if fuzzerType == "" {
		return fmt.Errorf("fuzzer type is empty")
	}

	version := fuzzer.GetVersion()
	if version == "" {
		return fmt.Errorf("fuzzer version is empty")
	}

	return nil
}
