// honggfuzz_test_binary.c - Simple testable implementation
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdlib.h>

// Simple fuzzer target that crashes on "HFUZZ" input
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 5 && memcmp(data, "HFUZZ", 5) == 0) {
        abort();
    }
    return 0;
}

// Minimal main for standalone testing
int main(void) {
    return 0;
}