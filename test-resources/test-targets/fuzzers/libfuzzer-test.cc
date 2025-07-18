// LibFuzzer test harness
#include <stdint.h>
#include <stddef.h>
#include <string.h>

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Check if input contains "HI!"
    if (size >= 3 && memcmp(data, "HI!", 3) == 0) {
        // Trigger a crash by dereferencing null
        int *p = nullptr;
        *p = 42;
    }
    return 0;
}