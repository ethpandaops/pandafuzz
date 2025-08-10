#include <stdint.h>
#include <stddef.h>
#include <stdio.h>

// Simple fuzzer target that finds crashes on specific inputs
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 4) {
        // Create some coverage paths
        if (data[0] == 'F') {
            if (data[1] == 'U') {
                if (data[2] == 'Z') {
                    if (data[3] == 'Z') {
                        // Intentional crash for testing
                        *(int*)0 = 0;
                    }
                }
            }
        }
        
        // Additional paths for coverage
        if (data[0] > 127) {
            printf("High byte\n");
        } else {
            printf("Low byte\n");
        }
    }
    return 0;
}