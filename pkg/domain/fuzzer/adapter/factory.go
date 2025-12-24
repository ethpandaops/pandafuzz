package adapter

import (
	"fmt"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/aflplusplus"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/honggfuzz"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
	"github.com/sirupsen/logrus"
)

// NewAFLPlusPlus creates a new AFL++ fuzzer adapter.
func NewAFLPlusPlus(log logrus.FieldLogger) *FuzzerAdapter {
	// Create empty engine, target will be set via Configure
	engine := aflplusplus.NewEngine("", nil, log)
	adapter := NewAdapter(engine, log)
	return adapter
}

// NewLibFuzzer creates a new LibFuzzer adapter.
func NewLibFuzzer(log logrus.FieldLogger) *FuzzerAdapter {
	engine := libfuzzer.NewEngine("", nil, log)
	adapter := NewAdapter(engine, log)
	return adapter
}

// NewHonggfuzz creates a new Honggfuzz adapter.
func NewHonggfuzz(log logrus.FieldLogger) *FuzzerAdapter {
	engine := honggfuzz.NewEngine("", nil, log)
	adapter := NewAdapter(engine, log)
	return adapter
}

// CreateFuzzer creates a fuzzer adapter based on type.
func CreateFuzzer(fuzzerType string, log logrus.FieldLogger) (*FuzzerAdapter, error) {
	switch fuzzerType {
	case "aflplusplus", "afl++", "afl":
		return NewAFLPlusPlus(log), nil
	case "libfuzzer":
		return NewLibFuzzer(log), nil
	case "honggfuzz":
		return NewHonggfuzz(log), nil
	default:
		return nil, fmt.Errorf("unsupported fuzzer type: %s", fuzzerType)
	}
}

// CreateAFLPlusPlus creates a new AFL++ fuzzer adapter (alias for compatibility).
func CreateAFLPlusPlus(log logrus.FieldLogger) (*FuzzerAdapter, error) {
	return NewAFLPlusPlus(log), nil
}

// CreateLibFuzzer creates a new LibFuzzer adapter (alias for compatibility).
func CreateLibFuzzer(log logrus.FieldLogger) (*FuzzerAdapter, error) {
	return NewLibFuzzer(log), nil
}

// CreateHonggfuzz creates a new Honggfuzz adapter (alias for compatibility).
func CreateHonggfuzz(log logrus.FieldLogger) (*FuzzerAdapter, error) {
	return NewHonggfuzz(log), nil
}
