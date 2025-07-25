package factory

import (
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// RegisterLibFuzzer registers the LibFuzzer engine with the factory
func RegisterLibFuzzer(factory *Factory, constructor EngineConstructor) error {
	info := &EngineInfo{
		Type:        string(types.FuzzerTypeLibFuzzer),
		Name:        "LibFuzzer",
		Description: "LLVM LibFuzzer - coverage-guided, evolutionary fuzzing engine",
		Version:     "LLVM",
		Capabilities: &types.FuzzerCapabilities{
			SupportsParallel:        false,
			SupportsMinimization:    true,
			SupportsDictionary:      true,
			SupportsCoverage:        true,
			SupportsTimeout:         true,
			SupportsMemoryLimit:     true,
			RequiresInstrumentation: true,
		},
	}

	return factory.RegisterEngine(string(types.FuzzerTypeLibFuzzer), constructor, info)
}

// RegisterAFLPlusPlus registers the AFL++ engine with the factory
func RegisterAFLPlusPlus(factory *Factory, constructor EngineConstructor) error {
	info := &EngineInfo{
		Type:        string(types.FuzzerTypeAFLPlusPlus),
		Name:        "AFL++",
		Description: "American Fuzzy Lop Plus Plus - evolutionary coverage-guided fuzzer",
		Version:     "4.0+",
		Capabilities: &types.FuzzerCapabilities{
			SupportsParallel:        true,
			SupportsMinimization:    true,
			SupportsDictionary:      true,
			SupportsCoverage:        true,
			SupportsTimeout:         true,
			SupportsMemoryLimit:     true,
			RequiresInstrumentation: true,
		},
	}

	return factory.RegisterEngine(string(types.FuzzerTypeAFLPlusPlus), constructor, info)
}

// RegisterHonggfuzz registers the Honggfuzz engine with the factory
func RegisterHonggfuzz(factory *Factory, constructor EngineConstructor) error {
	info := &EngineInfo{
		Type:        string(types.FuzzerTypeHonggfuzz),
		Name:        "Honggfuzz",
		Description: "Security-oriented, multi-process, feedback-driven fuzzer",
		Version:     "2.5+",
		Capabilities: &types.FuzzerCapabilities{
			SupportsParallel:        true,
			SupportsMinimization:    true,
			SupportsDictionary:      true,
			SupportsCoverage:        true,
			SupportsTimeout:         true,
			SupportsMemoryLimit:     true,
			RequiresInstrumentation: false, // Can work with and without instrumentation
		},
	}

	return factory.RegisterEngine(string(types.FuzzerTypeHonggfuzz), constructor, info)
}

// RegisterAllEngines is a convenience function to register all built-in engines
func RegisterAllEngines(factory *Factory, libFuzzerConstructor, aflConstructor, honggfuzzConstructor EngineConstructor) error {
	// Register LibFuzzer if constructor provided
	if libFuzzerConstructor != nil {
		if err := RegisterLibFuzzer(factory, libFuzzerConstructor); err != nil {
			return fmt.Errorf("failed to register LibFuzzer: %w", err)
		}
	}

	// Register AFL++ if constructor provided
	if aflConstructor != nil {
		if err := RegisterAFLPlusPlus(factory, aflConstructor); err != nil {
			return fmt.Errorf("failed to register AFL++: %w", err)
		}
	}

	// Register Honggfuzz if constructor provided
	if honggfuzzConstructor != nil {
		if err := RegisterHonggfuzz(factory, honggfuzzConstructor); err != nil {
			return fmt.Errorf("failed to register Honggfuzz: %w", err)
		}
	}

	return nil
}

// CreateDefaultFactory creates a factory with all built-in engines registered
func CreateDefaultFactory(opts Options, libFuzzerConstructor, aflConstructor, honggfuzzConstructor EngineConstructor) (*Factory, error) {
	factory := NewFactory(opts)

	if err := RegisterAllEngines(factory, libFuzzerConstructor, aflConstructor, honggfuzzConstructor); err != nil {
		return nil, err
	}

	return factory, nil
}
