#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>

// LibFuzzer entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Vulnerable to buffer overflow when data contains specific pattern
    if (size > 3) {
        if (data[0] == 'C' && data[1] == 'R' && data[2] == 'A' && data[3] == 'S' && data[4] == 'H') {
            // Intentional crash for testing
            int *p = NULL;
            *p = 42;  // NULL pointer dereference
        }
        
        // Another crash pattern
        if (size > 6 && memcmp(data, "BOOM", 4) == 0) {
            // Buffer overflow
            char buffer[10];
            memcpy(buffer, data, size);  // Potential overflow if size > 10
        }
    }
    
    return 0;
}