#!/bin/bash
# Debug script to test libfuzzer binaries in Alpine container

set -e

echo "=== Alpine Fuzzer Debug Script ==="
echo

# Check system info
echo "1. System Information:"
uname -a
echo

# Check Alpine version
echo "2. Alpine Version:"
cat /etc/alpine-release 2>/dev/null || echo "Not Alpine"
echo

# Check installed packages
echo "3. Checking for required packages:"
apk list --installed | grep -E "(clang|compiler-rt|gcompat|musl|libc)" || echo "Failed to list packages"
echo

# Build a simple libfuzzer test
echo "4. Building test libfuzzer binary:"
cat > /tmp/test_fuzzer.c << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size > 0) {
        printf("Fuzzing with %zu bytes\n", size);
    }
    return 0;
}
EOF

# Try different compilation approaches
echo
echo "5. Testing different compilation methods:"

echo "Method 1: Basic libfuzzer compilation"
clang -g -O1 -fsanitize=fuzzer /tmp/test_fuzzer.c -o /tmp/test_fuzzer1 2>&1 || echo "Method 1 failed"

echo
echo "Method 2: With address sanitizer"
clang -g -O1 -fsanitize=fuzzer,address /tmp/test_fuzzer.c -o /tmp/test_fuzzer2 2>&1 || echo "Method 2 failed"

echo
echo "Method 3: Static linking"
clang -g -O1 -fsanitize=fuzzer -static /tmp/test_fuzzer.c -o /tmp/test_fuzzer3 2>&1 || echo "Method 3 failed"

echo
echo "Method 4: With compiler-rt explicitly"
clang -g -O1 -fsanitize=fuzzer -lcompiler_rt /tmp/test_fuzzer.c -o /tmp/test_fuzzer4 2>&1 || echo "Method 4 failed"

echo
echo "6. Checking binaries:"
for i in 1 2 3 4; do
    if [ -f /tmp/test_fuzzer$i ]; then
        echo
        echo "test_fuzzer$i:"
        file /tmp/test_fuzzer$i
        ldd /tmp/test_fuzzer$i 2>&1 || echo "ldd failed"
        echo "Running: /tmp/test_fuzzer$i -help=1"
        /tmp/test_fuzzer$i -help=1 2>&1 | head -5 || echo "Execution failed"
    fi
done

echo
echo "7. Checking compiler-rt installation:"
find /usr -name "*fuzzer*" -type f 2>/dev/null | grep -v "/proc" | head -10
echo
find /usr -name "*compiler-rt*" -type f 2>/dev/null | grep -v "/proc" | head -10

echo
echo "8. Checking clang version and sanitizer support:"
clang --version
echo
clang -fsanitize=fuzzer -### /tmp/test_fuzzer.c 2>&1 | grep -E "(fuzzer|sanitizer)" | head -5

echo
echo "9. Testing with manual fuzzer main:"
cat > /tmp/manual_fuzzer.c << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size > 0) {
        printf("Manual fuzzer: %zu bytes\n", size);
    }
    return 0;
}

// Manual main function for testing
int main(int argc, char **argv) {
    printf("Manual fuzzer starting...\n");
    
    // Test with some dummy data
    uint8_t data[] = {1, 2, 3, 4, 5};
    LLVMFuzzerTestOneInput(data, sizeof(data));
    
    printf("Manual fuzzer completed\n");
    return 0;
}
EOF

echo "Compiling manual fuzzer..."
clang++ -g -O1 /tmp/manual_fuzzer.c -o /tmp/manual_fuzzer 2>&1 || echo "Manual compilation failed"

if [ -f /tmp/manual_fuzzer ]; then
    echo "Running manual fuzzer:"
    /tmp/manual_fuzzer || echo "Manual fuzzer failed"
fi

echo
echo "=== Debug script completed ==="