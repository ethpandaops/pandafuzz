#include <stdint.h>
#include <stddef.h>

// A simple test program that crashes on specific input
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 4) {
        if (data[0] == 'F' && data[1] == 'U' && data[2] == 'Z' && data[3] == 'Z') {
            // Null pointer dereference - will crash
            int *p = NULL;
            *p = 42;
        }
    }
    return 0;
}