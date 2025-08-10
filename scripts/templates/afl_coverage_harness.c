/*
 * AFL++ Coverage Test Harness Template
 * 
 * This template demonstrates how to create an AFL++-compatible fuzzer
 * with proper persistent mode support for maximum performance and
 * effective coverage collection.
 * 
 * Compile with:
 *   afl-clang-fast -g -O0 -o harness afl_coverage_harness.c
 * 
 * Run with:
 *   afl-fuzz -i input_dir -o output_dir -- ./harness
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>

#ifdef __AFL_FUZZ_TESTCASE_LEN
  // AFL++ persistent mode support
  #include <unistd.h>
#else
  // Fallback for testing without AFL++
  #define __AFL_FUZZ_TESTCASE_BUF buf
  #define __AFL_FUZZ_TESTCASE_LEN len
  #define __AFL_FUZZ_INIT() void
  #define __AFL_INIT() void
  #define __AFL_LOOP(x) ((len = read(0, buf, sizeof(buf))) > 0)
  static unsigned char buf[1024000];
  static int len;
#endif

/* 
 * Example processing functions with multiple branches for coverage testing
 * Replace these with your actual fuzzing logic
 */

// Function to test branch coverage
int process_command(const uint8_t *data, size_t size) {
    if (size < 3) return -1;
    
    // Multiple branches for coverage
    if (memcmp(data, "ADD", 3) == 0) {
        printf("ADD command\n");
        return 1;
    } else if (memcmp(data, "DEL", 3) == 0) {
        printf("DEL command\n");
        return 2;
    } else if (memcmp(data, "MOD", 3) == 0) {
        printf("MOD command\n");
        return 3;
    } else if (memcmp(data, "GET", 3) == 0) {
        printf("GET command\n");
        return 4;
    }
    return 0;
}

// Function to test nested conditions and loops
void process_data(const uint8_t *data, size_t size) {
    if (size < 4) return;
    
    // Test different data patterns
    if (data[0] == 'A') {
        printf("Type A data\n");
        if (size > 1 && data[1] == 'B') {
            printf("Subtype AB\n");
            // Nested loop for coverage
            for (size_t i = 2; i < size && i < 10; i++) {
                if (data[i] == 'C') {
                    printf("Found C at position %zu\n", i);
                }
            }
        }
    } else if (data[0] == 'B') {
        printf("Type B data\n");
        if (size > 1 && data[1] == 'C') {
            printf("Subtype BC\n");
        }
    }
    
    // Complex pattern matching
    if (size >= 10) {
        if (data[0] == 'X' && data[1] == 'Y' && data[2] == 'Z') {
            printf("XYZ pattern found\n");
            if (data[3] == '1') {
                printf("XYZ1 variant\n");
            } else if (data[3] == '2') {
                printf("XYZ2 variant\n");
            }
        }
    }
}

// Error handling paths for coverage
int validate_input(const uint8_t *data, size_t size) {
    if (size == 0) {
        fprintf(stderr, "Empty input\n");
        return -1;
    }
    
    if (size > 100000) {
        fprintf(stderr, "Input too large\n");
        return -2;
    }
    
    // Check for null bytes in specific positions
    for (size_t i = 0; i < size && i < 10; i++) {
        if (data[i] == 0) {
            fprintf(stderr, "Null byte at position %zu\n", i);
            return -3;
        }
    }
    
    return 0;
}

// Main fuzzing function that processes input
void fuzz_one_input(const uint8_t *data, size_t size) {
    // Validate input first
    if (validate_input(data, size) < 0) {
        return;
    }
    
    // Process commands
    int cmd_result = process_command(data, size);
    if (cmd_result > 0) {
        // Command was recognized, process associated data
        if (size > 3) {
            process_data(data + 3, size - 3);
        }
    } else {
        // No command, process as raw data
        process_data(data, size);
    }
    
    // Crash condition for testing (should be found quickly)
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        // Intentional crash for fuzzer testing
        abort();
    }
    
    // Another crash pattern with more specific conditions
    if (size == 17 && data[0] == 0xFF && data[16] == 0xFF) {
        if (memcmp(data + 1, "VULNERABLE", 10) == 0) {
            // Another crash path
            int *p = NULL;
            *p = 42;
        }
    }
}

// AFL++ persistent mode main function
__AFL_FUZZ_INIT();

int main(int argc, char *argv[]) {
    // Enable AFL++ deferred forkserver mode
    #ifdef __AFL_HAVE_MANUAL_CONTROL
        __AFL_INIT();
    #endif
    
    // AFL++ persistent mode loop
    // The loop runs up to 10000 iterations in the same process
    // This significantly improves fuzzing performance
    unsigned char *buf = __AFL_FUZZ_TESTCASE_BUF;
    while (__AFL_LOOP(10000)) {
        int len = __AFL_FUZZ_TESTCASE_LEN;
        
        // Process the test case
        fuzz_one_input(buf, len);
    }
    
    return 0;
}