#include <stdint.h>
#include <stddef.h>
#include <string.h>

// This is a libfuzzer harness that crashes on specific input
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Check if input starts with "CRASH"
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        // Trigger a crash by dereferencing null
        int *p = nullptr;
        *p = 42;
    }
    return 0;
}