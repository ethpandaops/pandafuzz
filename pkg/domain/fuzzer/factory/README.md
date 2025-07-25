# Fuzzer Factory

The fuzzer factory package provides a flexible and extensible way to create and manage different fuzzer engine instances.

## Features

- **Engine Registration**: Register multiple fuzzer engines with the factory
- **Dynamic Creation**: Create fuzzer instances based on type at runtime
- **Dependency Injection**: Support for repositories and services injection
- **Builder Pattern**: Fluent interface for creating configured fuzzers
- **Type Safety**: Strong typing with interfaces from the types package
- **Thread Safe**: Concurrent access is properly synchronized

## Usage

### Basic Factory Creation

```go
import "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/factory"

// Create a factory with dependencies
f := factory.NewFactory(factory.Options{
    Repositories: factory.Repositories{
        // Add repositories as needed
    },
    Services: factory.Services{
        // Add services as needed
    },
})
```

### Registering Engines

```go
// Register a LibFuzzer engine
libFuzzerConstructor := func(target string, args []string) (types.Fuzzer, error) {
    return libfuzzer.NewEngine(target, args)
}

err := factory.RegisterLibFuzzer(f, libFuzzerConstructor)

// Or register a custom engine
customConstructor := func(target string, args []string) (types.Fuzzer, error) {
    return custom.NewEngine(target, args)
}

info := &factory.EngineInfo{
    Type:        "custom",
    Name:        "Custom Fuzzer",
    Description: "My custom fuzzing engine",
    Version:     "1.0",
    Capabilities: &types.FuzzerCapabilities{
        SupportsCoverage: true,
        SupportsTimeout:  true,
    },
}

err := f.RegisterEngine("custom", customConstructor, info)
```

### Creating Fuzzers

```go
// Simple creation
fuzzer, err := f.CreateFuzzer("libfuzzer", "/path/to/target", []string{"--arg1", "--arg2"})

// With configuration
config := &types.FuzzerConfig{
    OutputDir:   "/tmp/output",
    MemoryLimit: 2 * 1024 * 1024 * 1024, // 2GB
    Workers:     4,
}

fuzzer, err := f.CreateFuzzerWithConfig("afl++", "/path/to/target", nil, config)

// Using the builder pattern
fuzzer, err := f.NewBuilder("honggfuzz").
    WithTarget("/path/to/target").
    WithArgs([]string{"--flag"}).
    WithConfig(config).
    Build()
```

### Querying Available Engines

```go
// Get all supported types
types := f.GetSupportedTypes()

// Check if a type is supported
if f.IsSupported("libfuzzer") {
    // LibFuzzer is available
}

// Get engine information
info, err := f.GetEngineInfo("afl++")
if err == nil {
    fmt.Printf("Engine: %s\n", info.Name)
    fmt.Printf("Description: %s\n", info.Description)
    fmt.Printf("Supports parallel: %v\n", info.Capabilities.SupportsParallel)
}

// Get all engine information
allInfo := f.GetAllEngineInfo()
```

## Built-in Engine Support

The factory includes registration helpers for common fuzzing engines:

- **LibFuzzer**: LLVM's coverage-guided fuzzer
- **AFL++**: American Fuzzy Lop Plus Plus
- **Honggfuzz**: Security-oriented fuzzer with multi-process support

Use the convenience functions to register these engines:

```go
// Register all built-in engines at once
factory, err := factory.CreateDefaultFactory(
    factory.Options{},
    libFuzzerConstructor,
    aflConstructor,
    honggfuzzConstructor,
)
```

## Engine Requirements

Engines registered with the factory must implement the `types.Fuzzer` interface:

```go
type Fuzzer interface {
    Start(ctx context.Context) error
    Stop() error
    GetStats() (*FuzzerStats, error)
    GetCrashes() <-chan *CrashInfo
    GetProgress() <-chan *ProgressUpdate
    IsRunning() bool
    GetType() string
    GetVersion() string
    SetCorpus(path string) error
    SetOutput(path string) error
    Configure(config *FuzzerConfig) error
}
```

## Thread Safety

The factory is thread-safe and can be safely used from multiple goroutines:

- Engine registration/unregistration is synchronized
- Fuzzer creation can be called concurrently
- Engine information queries are safe for concurrent access

## Error Handling

The factory provides detailed error messages for common issues:

- Unsupported fuzzer types
- Empty targets
- Registration conflicts
- Constructor failures
- Configuration errors

## Best Practices

1. **Register engines early**: Register all engines during application initialization
2. **Validate configurations**: Use the config validation methods before creating fuzzers
3. **Handle cleanup**: Ensure proper fuzzer cleanup in defer blocks
4. **Monitor resources**: Use the stats and metrics interfaces to monitor fuzzer performance
5. **Use dependency injection**: Pass repositories and services through the factory options