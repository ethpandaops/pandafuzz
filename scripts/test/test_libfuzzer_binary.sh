#!/bin/bash
# Test script to verify libfuzzer binary execution

set -e

echo "=== Testing LibFuzzer Binary Execution ==="

# Create test directory
TEST_DIR="/tmp/pandafuzz_test"
mkdir -p "$TEST_DIR"

# Create test fuzzer from the user's example
cat << 'EOF' > "$TEST_DIR/test_fuzzer.cc"
#include <stdint.h>
#include <stddef.h>
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
  if (size > 0 && data[0] == 'H')
    if (size > 1 && data[1] == 'I')
       if (size > 2 && data[2] == '!')
       __builtin_trap();
  return 0;
}
EOF

# Build test fuzzer
echo "Building test fuzzer..."
clang++ -fsanitize=address,fuzzer "$TEST_DIR/test_fuzzer.cc" -o "$TEST_DIR/test_fuzzer"

# Check if binary was created
if [ ! -f "$TEST_DIR/test_fuzzer" ]; then
    echo "ERROR: Failed to create test binary"
    exit 1
fi

echo "Binary created successfully"

# Check binary properties
echo ""
echo "Binary information:"
file "$TEST_DIR/test_fuzzer"
ldd "$TEST_DIR/test_fuzzer" 2>&1 || echo "ldd failed (static binary?)"

# Test if it's a libfuzzer binary
echo ""
echo "Testing libfuzzer detection (running with -help=1):"
"$TEST_DIR/test_fuzzer" -help=1 2>&1 | head -10 || echo "Help command failed"

# Test basic execution
echo ""
echo "Testing basic execution (max 10 runs):"
cd "$TEST_DIR"
./test_fuzzer -runs=10 2>&1 || echo "Basic execution failed with exit code: $?"

# Test with corpus directory
echo ""
echo "Testing with corpus directory:"
mkdir -p corpus
echo "test" > corpus/seed1
./test_fuzzer corpus -runs=10 2>&1 || echo "Corpus execution failed with exit code: $?"

# Test crash detection
echo ""
echo "Testing crash detection:"
echo "HI!" > crash_input
./test_fuzzer crash_input 2>&1 || echo "Expected crash occurred with exit code: $?"

echo ""
echo "=== Test completed ==="
echo "Test directory: $TEST_DIR"