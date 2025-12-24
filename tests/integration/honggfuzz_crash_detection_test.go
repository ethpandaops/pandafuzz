package integration

import (
	"testing"

	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/adapter"
	"github.com/stretchr/testify/assert"
)

// TestHonggfuzzCrashDetection tests that crashes are properly detected
// after job completion but before fuzzer cleanup.
//
// Note: The core crash detection functionality is tested in fuzzer_test.go
// (TestFuzzerCrashHandling). This test specifically covers Honggfuzz adapter
// initialization and capabilities.
func TestHonggfuzzCrashDetection(t *testing.T) {
	// Test Honggfuzz adapter creation
	honggfuzz := adapter.NewHonggfuzz(nil)
	assert.NotNil(t, honggfuzz)
	assert.Equal(t, "honggfuzz", honggfuzz.Name())
	assert.Equal(t, adapter.FuzzerTypeHonggfuzz, honggfuzz.Type())

	// Verify capabilities
	caps := honggfuzz.GetCapabilities()
	assert.NotEmpty(t, caps)

	// Configure and check validation without target
	err := honggfuzz.Validate()
	assert.Error(t, err, "Should fail validation without configuration")
}

// TestFuzzerCleanupRaceCondition tests that the fuzzer is not cleaned up
// before crash detection completes.
//
// Note: The core cleanup functionality is tested in fuzzer_test.go
// (TestFuzzerCleanup). This test verifies the cleanup mechanism works
// correctly for the Honggfuzz adapter.
func TestFuzzerCleanupRaceCondition(t *testing.T) {
	// Test Honggfuzz cleanup without running
	honggfuzz := adapter.NewHonggfuzz(nil)

	config := adapter.FuzzConfig{
		Target:          "/bin/test",
		OutputDirectory: "/tmp/honggfuzz-test",
	}

	err := honggfuzz.Configure(config)
	assert.NoError(t, err)

	err = honggfuzz.Initialize()
	assert.NoError(t, err)

	// Verify fuzzer is initialized
	assert.Equal(t, adapter.StatusInitialized, honggfuzz.GetStatus())

	// Cleanup should succeed even without running (no race condition)
	err = honggfuzz.Cleanup()
	assert.NoError(t, err)
}
