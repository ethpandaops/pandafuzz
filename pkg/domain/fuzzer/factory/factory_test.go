package factory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// mockFuzzer implements types.Fuzzer for testing
type mockFuzzer struct {
	fuzzerType string
	version    string
	target     string
	args       []string
	isRunning  bool
	config     *types.FuzzerConfig
}

func (m *mockFuzzer) Start(ctx context.Context) error {
	if m.isRunning {
		return errors.New("already running")
	}
	m.isRunning = true
	return nil
}

func (m *mockFuzzer) Stop() error {
	m.isRunning = false
	return nil
}

func (m *mockFuzzer) GetStats() (*types.FuzzerStats, error) {
	return &types.FuzzerStats{
		StartTime:       time.Now(),
		RunTime:         time.Hour,
		TotalExecutions: 1000000,
		ExecsPerSecond:  1000,
		CorpusSize:      100,
		Coverage:        75.5,
		CrashesFound:    2,
		TimeoutsFound:   1,
		MemoryPeak:      1024 * 1024 * 512, // 512MB
	}, nil
}

func (m *mockFuzzer) GetCrashes() <-chan *types.CrashInfo {
	ch := make(chan *types.CrashInfo)
	close(ch)
	return ch
}

func (m *mockFuzzer) GetProgress() <-chan *types.ProgressUpdate {
	ch := make(chan *types.ProgressUpdate)
	close(ch)
	return ch
}

func (m *mockFuzzer) IsRunning() bool {
	return m.isRunning
}

func (m *mockFuzzer) GetType() string {
	return m.fuzzerType
}

func (m *mockFuzzer) GetVersion() string {
	return m.version
}

func (m *mockFuzzer) SetCorpus(path string) error {
	return nil
}

func (m *mockFuzzer) SetOutput(path string) error {
	return nil
}

func (m *mockFuzzer) Configure(config *types.FuzzerConfig) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}
	m.config = config
	return nil
}

func TestFactory_RegisterEngine(t *testing.T) {
	factory := NewFactory(Options{})

	constructor := func(target string, args []string) (types.Fuzzer, error) {
		return &mockFuzzer{
			fuzzerType: "test",
			version:    "1.0",
			target:     target,
			args:       args,
		}, nil
	}

	info := &EngineInfo{
		Type:        "test",
		Name:        "Test Fuzzer",
		Description: "A test fuzzer",
		Version:     "1.0",
		Capabilities: &types.FuzzerCapabilities{
			SupportsCoverage: true,
		},
	}

	// Test successful registration
	err := factory.RegisterEngine("test", constructor, info)
	if err != nil {
		t.Fatalf("Failed to register engine: %v", err)
	}

	// Test duplicate registration
	err = factory.RegisterEngine("test", constructor, info)
	if err == nil {
		t.Fatal("Expected error for duplicate registration")
	}

	// Test invalid registrations
	tests := []struct {
		name        string
		engineType  string
		constructor EngineConstructor
		info        *EngineInfo
		wantErr     bool
	}{
		{
			name:        "empty engine type",
			engineType:  "",
			constructor: constructor,
			info:        info,
			wantErr:     true,
		},
		{
			name:        "nil constructor",
			engineType:  "test2",
			constructor: nil,
			info:        info,
			wantErr:     true,
		},
		{
			name:        "nil info",
			engineType:  "test3",
			constructor: constructor,
			info:        nil,
			wantErr:     true,
		},
		{
			name:        "type mismatch",
			engineType:  "test4",
			constructor: constructor,
			info: &EngineInfo{
				Type: "different",
				Name: "Different",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := factory.RegisterEngine(tt.engineType, tt.constructor, tt.info)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterEngine() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFactory_CreateFuzzer(t *testing.T) {
	factory := NewFactory(Options{})

	// Register a test engine
	constructor := func(target string, args []string) (types.Fuzzer, error) {
		if target == "error" {
			return nil, errors.New("constructor error")
		}
		return &mockFuzzer{
			fuzzerType: "test",
			version:    "1.0",
			target:     target,
			args:       args,
		}, nil
	}

	info := &EngineInfo{
		Type:        "test",
		Name:        "Test Fuzzer",
		Description: "A test fuzzer",
		Version:     "1.0",
	}

	err := factory.RegisterEngine("test", constructor, info)
	if err != nil {
		t.Fatalf("Failed to register engine: %v", err)
	}

	// Test successful creation
	fuzzer, err := factory.CreateFuzzer("test", "/path/to/target", []string{"--arg1", "--arg2"})
	if err != nil {
		t.Fatalf("Failed to create fuzzer: %v", err)
	}

	if fuzzer.GetType() != "test" {
		t.Errorf("Expected type 'test', got '%s'", fuzzer.GetType())
	}

	// Test unsupported type
	_, err = factory.CreateFuzzer("unsupported", "/path/to/target", nil)
	if err == nil {
		t.Fatal("Expected error for unsupported fuzzer type")
	}

	// Test empty target
	_, err = factory.CreateFuzzer("test", "", nil)
	if err == nil {
		t.Fatal("Expected error for empty target")
	}

	// Test constructor error
	_, err = factory.CreateFuzzer("test", "error", nil)
	if err == nil {
		t.Fatal("Expected error from constructor")
	}
}

func TestFactory_GetSupportedTypes(t *testing.T) {
	factory := NewFactory(Options{})

	// Initially empty
	supportedTypes := factory.GetSupportedTypes()
	if len(supportedTypes) != 0 {
		t.Errorf("Expected 0 types, got %d", len(supportedTypes))
	}

	// Register some engines
	for i := 1; i <= 3; i++ {
		engineType := string(rune('a' + i - 1))
		err := factory.RegisterEngine(engineType, func(string, []string) (types.Fuzzer, error) {
			return &mockFuzzer{fuzzerType: engineType}, nil
		}, &EngineInfo{
			Type: engineType,
			Name: engineType,
		})
		if err != nil {
			t.Fatalf("Failed to register engine %s: %v", engineType, err)
		}
	}

	supportedTypes = factory.GetSupportedTypes()
	if len(supportedTypes) != 3 {
		t.Errorf("Expected 3 types, got %d", len(supportedTypes))
	}
}

func TestFactory_IsSupported(t *testing.T) {
	factory := NewFactory(Options{})

	// Register a test engine
	err := factory.RegisterEngine("test", func(string, []string) (types.Fuzzer, error) {
		return &mockFuzzer{fuzzerType: "test"}, nil
	}, &EngineInfo{
		Type: "test",
		Name: "Test",
	})
	if err != nil {
		t.Fatalf("Failed to register engine: %v", err)
	}

	if !factory.IsSupported("test") {
		t.Error("Expected 'test' to be supported")
	}

	if factory.IsSupported("unsupported") {
		t.Error("Expected 'unsupported' to not be supported")
	}
}

func TestFactory_CreateFuzzerWithConfig(t *testing.T) {
	factory := NewFactory(Options{})

	// Register a test engine
	constructor := func(target string, args []string) (types.Fuzzer, error) {
		return &mockFuzzer{
			fuzzerType: "test",
			version:    "1.0",
			target:     target,
			args:       args,
		}, nil
	}

	err := factory.RegisterEngine("test", constructor, &EngineInfo{
		Type: "test",
		Name: "Test",
	})
	if err != nil {
		t.Fatalf("Failed to register engine: %v", err)
	}

	config := &types.FuzzerConfig{
		OutputDir:   "/tmp/output",
		MemoryLimit: 1024 * 1024 * 1024, // 1GB
		Workers:     4,
	}

	fuzzer, err := factory.CreateFuzzerWithConfig("test", "/path/to/target", nil, config)
	if err != nil {
		t.Fatalf("Failed to create fuzzer with config: %v", err)
	}

	// Verify the fuzzer was created and configured
	mock := fuzzer.(*mockFuzzer)
	if mock.config == nil {
		t.Fatal("Expected config to be set")
	}
	if mock.config.OutputDir != config.OutputDir {
		t.Errorf("Expected output dir '%s', got '%s'", config.OutputDir, mock.config.OutputDir)
	}
}

func TestFactory_Builder(t *testing.T) {
	factory := NewFactory(Options{})

	// Register a test engine
	err := factory.RegisterEngine("test", func(target string, args []string) (types.Fuzzer, error) {
		return &mockFuzzer{
			fuzzerType: "test",
			version:    "1.0",
			target:     target,
			args:       args,
		}, nil
	}, &EngineInfo{
		Type: "test",
		Name: "Test",
	})
	if err != nil {
		t.Fatalf("Failed to register engine: %v", err)
	}

	config := &types.FuzzerConfig{
		OutputDir: "/tmp/output",
		Workers:   2,
	}

	fuzzer, err := factory.NewBuilder("test").
		WithTarget("/path/to/target").
		WithArgs([]string{"--flag"}).
		WithConfig(config).
		Build()

	if err != nil {
		t.Fatalf("Failed to build fuzzer: %v", err)
	}

	mock := fuzzer.(*mockFuzzer)
	if mock.target != "/path/to/target" {
		t.Errorf("Expected target '/path/to/target', got '%s'", mock.target)
	}
	if len(mock.args) != 1 || mock.args[0] != "--flag" {
		t.Errorf("Expected args ['--flag'], got %v", mock.args)
	}
}
