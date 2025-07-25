package analyzer

import (
	"context"
	"testing"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrashClassifier(t *testing.T) {
	tests := []struct {
		name             string
		stackTrace       string
		expectedType     types.CrashType
		expectedSeverity types.Severity
	}{
		{
			name: "heap use after free",
			stackTrace: `==12345==ERROR: AddressSanitizer: heap-use-after-free on address 0x60400000dff0
READ of size 8 at 0x60400000dff0 thread T0
    #0 0x7f8a0a6c3b8f in process_data /src/main.c:42:5
    #1 0x7f8a0a6c4321 in main /src/main.c:100:3`,
			expectedType:     types.CrashTypeHeapOverflow,
			expectedSeverity: types.SeverityHigh,
		},
		{
			name: "null pointer dereference",
			stackTrace: `Program received signal SIGSEGV, Segmentation fault.
0x00000000004004f6 in process () at test.c:10
10	    *ptr = 42;
(gdb) bt
#0  0x00000000004004f6 in process () at test.c:10
#1  0x0000000000400506 in main () at test.c:15`,
			expectedType:     types.CrashTypeSegmentationFault,
			expectedSeverity: types.SeverityMedium,
		},
		{
			name: "stack buffer overflow",
			stackTrace: `==98765==ERROR: AddressSanitizer: stack-buffer-overflow on address 0x7ffd12345678
WRITE of size 4 at 0x7ffd12345678 thread T0
    #0 0x4c3210 in vulnerable_function /app/vuln.c:25:10
    #1 0x4c3456 in main /app/vuln.c:50:5`,
			expectedType:     types.CrashTypeStackOverflow,
			expectedSeverity: types.SeverityHigh,
		},
		{
			name: "assertion failure",
			stackTrace: `test: test.cpp:42: void TestFunction(): Assertion 'value > 0' failed.
Program received signal SIGABRT, Aborted.
0x00007ffff7a62428 in __GI_raise (sig=sig@entry=6) at ../sysdeps/unix/sysv/linux/raise.c:54`,
			expectedType:     types.CrashTypeAssertion,
			expectedSeverity: types.SeverityMedium,
		},
		{
			name: "double free",
			stackTrace: `*** Error in './test': double free or corruption (fasttop): 0x0000000001234560 ***
======= Backtrace: =========
/lib/x86_64-linux-gnu/libc.so.6(+0x777e5)[0x7ffff7a777e5]
./test[0x400645]`,
			expectedType:     types.CrashTypeHeapOverflow,
			expectedSeverity: types.SeverityHigh,
		},
		{
			name: "format string vulnerability",
			stackTrace: `==11111==ERROR: AddressSanitizer: SEGV on unknown address 0x41414141
The signal is caused by a WRITE memory access.
Hint: this fault was caused by a format string vulnerability with user controlled input`,
			expectedType:     types.CrashTypeOther,
			expectedSeverity: types.SeverityCritical,
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	classifier := NewCrashClassifier(logger, nil, Config{
		EnablePatternLearning: true,
		SimilarityThreshold:   0.85,
		MaxPatternCache:       100,
	})

	ctx := context.Background()
	require.NoError(t, classifier.Start(ctx))
	defer classifier.Stop()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crashType, severity, err := classifier.ClassifyByStackTrace(ctx, tt.stackTrace)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedType, crashType, "crash type mismatch")
			assert.Equal(t, tt.expectedSeverity, severity, "severity mismatch")
		})
	}
}

func TestCrashPatternMatching(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	classifier := NewCrashClassifier(logger, nil, Config{})

	ctx := context.Background()
	require.NoError(t, classifier.Start(ctx))
	defer classifier.Stop()

	// Test that default patterns are registered
	patterns := classifier.GetKnownPatterns()
	assert.Greater(t, len(patterns), 5, "should have default patterns registered")

	// Test custom pattern registration
	customPattern := CrashPattern{
		ID:          "custom-test-01",
		Name:        "Custom Test Pattern",
		Type:        types.CrashTypeOther,
		Severity:    types.SeverityHigh,
		Keywords:    []string{"custom", "test", "error"},
		Description: "Test pattern for unit tests",
		Priority:    50,
	}

	err := classifier.RegisterPattern(customPattern)
	require.NoError(t, err)

	// Verify pattern was added
	patterns = classifier.GetKnownPatterns()
	found := false
	for _, p := range patterns {
		if p.ID == "custom-test-01" {
			found = true
			break
		}
	}
	assert.True(t, found, "custom pattern should be registered")

	// Test duplicate pattern registration
	err = classifier.RegisterPattern(customPattern)
	assert.Error(t, err, "should error on duplicate pattern")
}

func TestClassifierHelperMethods(t *testing.T) {
	logger := logrus.New()
	c := &classifier{log: logger}

	tests := []struct {
		name     string
		method   func(string) bool
		input    string
		expected bool
	}{
		{
			name:     "detect use after free",
			method:   c.isUseAfterFree,
			input:    "heap-use-after-free detected at address 0x12345",
			expected: true,
		},
		{
			name:     "detect integer overflow",
			method:   c.isIntegerOverflow,
			input:    "runtime error: signed integer overflow: 2147483647 + 1",
			expected: true,
		},
		{
			name:     "detect exploitable",
			method:   c.isExploitable,
			input:    "Exploitable: arbitrary write detected with user controlled data",
			expected: true,
		},
		{
			name:     "detect null pointer",
			method:   c.isNullPointer,
			input:    "Segmentation fault accessing address 0x0",
			expected: true,
		},
		{
			name:     "detect production code",
			method:   c.isProductionCode,
			input:    "/src/main/app/server.go:123",
			expected: true,
		},
		{
			name:     "detect test code",
			method:   c.isProductionCode,
			input:    "/src/test/app/server_test.go:456",
			expected: false,
		},
		{
			name:     "detect availability impact",
			method:   c.affectsAvailability,
			input:    "Service unavailable due to deadlock condition",
			expected: true,
		},
		{
			name:     "detect severe memory leak",
			method:   c.isSevereMemoryLeak,
			input:    "Process terminated by OOM killer after 10GB leaked",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
