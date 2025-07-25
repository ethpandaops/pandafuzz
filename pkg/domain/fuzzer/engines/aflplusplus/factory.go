package aflplusplus

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// Factory implements FuzzerFactory for AFL++
type Factory struct {
	log logrus.FieldLogger
}

// NewFactory creates a new AFL++ factory
func NewFactory(log logrus.FieldLogger) types.FuzzerFactory {
	if log == nil {
		log = logrus.New()
	}

	return &Factory{
		log: log.WithField("factory", "afl++"),
	}
}

// CreateFuzzer creates a new AFL++ instance
func (f *Factory) CreateFuzzer(fuzzerType string, target string, args []string) (types.Fuzzer, error) {
	if !f.IsSupported(fuzzerType) {
		return nil, fmt.Errorf("unsupported fuzzer type: %s", fuzzerType)
	}

	if target == "" {
		return nil, errors.New("target binary path cannot be empty")
	}

	engine := NewEngine(target, args, f.log)
	return engine, nil
}

// GetSupportedTypes returns a list of supported fuzzer types
func (f *Factory) GetSupportedTypes() []string {
	return []string{types.FuzzerTypeAFLPlusPlus.String()}
}

// IsSupported checks if a fuzzer type is supported
func (f *Factory) IsSupported(fuzzerType string) bool {
	return fuzzerType == types.FuzzerTypeAFLPlusPlus.String() || fuzzerType == "afl++" || fuzzerType == "aflplusplus"
}
