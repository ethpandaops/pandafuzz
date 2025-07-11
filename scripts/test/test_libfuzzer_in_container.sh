#!/bin/bash
# Test libfuzzer binary execution inside the bot container

set -e

echo "=== Testing LibFuzzer in Container ==="

# Create test directory
TEST_DIR="test_libfuzzer_$(date +%s)"
mkdir -p "$TEST_DIR"

# Create the same test fuzzer from the user's example
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

# Run test in bot container
echo "Running test in bot container..."
docker-compose run --rm bot bash -c "
set -e
cd /app

# Check environment
echo '=== Container Environment ==='
echo 'OS Info:'
cat /etc/os-release | grep -E '(NAME|VERSION)' || true
echo ''
echo 'Available compilers:'
which clang clang++ gcc g++ 2>/dev/null || echo 'No compilers found'
echo ''
echo 'Clang version:'
clang --version 2>/dev/null || echo 'Clang not available'
echo ''

# Build test fuzzer
echo '=== Building Test Fuzzer ==='
cd /tmp
cat > test_fuzzer.cc << 'EOFF'
#include <stdint.h>
#include <stddef.h>
extern \"C\" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
  if (size > 0 && data[0] == 'H')
    if (size > 1 && data[1] == 'I')
       if (size > 2 && data[2] == '!')
       __builtin_trap();
  return 0;
}
EOFF

# Try to build
if clang++ -fsanitize=address,fuzzer test_fuzzer.cc -o test_fuzzer 2>&1; then
    echo 'Build successful!'
    
    # Check binary
    echo ''
    echo '=== Binary Info ==='
    file test_fuzzer
    ldd test_fuzzer 2>&1 || echo 'ldd failed'
    
    # Test libfuzzer detection
    echo ''
    echo '=== LibFuzzer Detection Test ==='
    ./test_fuzzer -help=1 2>&1 | grep -i libfuzzer || echo 'LibFuzzer not detected'
    
    # Run with limited iterations
    echo ''
    echo '=== Running Fuzzer (10 iterations) ==='
    ./test_fuzzer -runs=10 2>&1 || echo \"Fuzzer exited with code: \$?\"
    
    # Test crash
    echo ''
    echo '=== Testing Crash Detection ==='
    echo 'HI!' > crash_input
    ./test_fuzzer crash_input 2>&1 || echo \"Expected crash with code: \$?\"
    
else
    echo 'Build FAILED!'
    echo ''
    echo '=== Checking libfuzzer libraries ==='
    find /usr -name '*fuzzer*.a' -o -name '*libFuzzer*' 2>/dev/null | head -10
    echo ''
    echo '=== Checking compiler runtime ==='
    find /usr -name '*compiler-rt*' -o -name '*sanitizer*' 2>/dev/null | grep -v proc | head -10
fi

echo ''
echo '=== Test Complete ==='
"

# Clean up
rm -rf "$TEST_DIR"

echo ""
echo "Container test completed. Check output above for issues."