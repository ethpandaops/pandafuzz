/*
 * LibFuzzer Coverage Test Harness Template
 * 
 * This template demonstrates how to create a LibFuzzer-compatible fuzzer
 * with proper entry point and coverage-friendly code patterns.
 * 
 * Compile with:
 *   clang++ -g -O1 -fsanitize=fuzzer,address -fprofile-instr-generate -fcoverage-mapping \
 *           -o harness libfuzzer_coverage_harness.cc
 * 
 * Run with:
 *   ./harness corpus_dir/
 * 
 * Generate coverage report:
 *   LLVM_PROFILE_FILE="fuzzer.profraw" ./harness corpus_dir/ -runs=100000
 *   llvm-profdata merge -sparse fuzzer.profraw -o fuzzer.profdata
 *   llvm-cov report ./harness -instr-profile=fuzzer.profdata
 */

#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <vector>
#include <string>

// Global state for demonstrating stateful fuzzing
static int g_path_counter = 0;
static std::vector<std::string> g_processed_patterns;

/* 
 * Example processing functions with multiple branches for coverage testing
 * Replace these with your actual fuzzing logic
 */

// Function with multiple branches
int analyze_header(const uint8_t *data, size_t size) {
    if (size < 4) return 0;
    
    // Check various file format magic bytes
    if (data[0] == 0xFF && data[1] == 0xFE) {
        printf("UTF-16 LE BOM detected\n");
        return 1;
    } else if (data[0] == 0xFE && data[1] == 0xFF) {
        printf("UTF-16 BE BOM detected\n");
        return 2;
    } else if (data[0] == 0xEF && size >= 3 && data[1] == 0xBB && data[2] == 0xBF) {
        printf("UTF-8 BOM detected\n");
        return 3;
    } else if (data[0] == 0x50 && data[1] == 0x4B && data[2] == 0x03 && data[3] == 0x04) {
        printf("ZIP file header detected\n");
        return 4;
    } else if (data[0] == 0x7F && data[1] == 'E' && data[2] == 'L' && data[3] == 'F') {
        printf("ELF header detected\n");
        return 5;
    }
    
    return 0;
}

// Function with loops for coverage
void process_data_blocks(const uint8_t *data, size_t size) {
    // Process data in 4-byte blocks
    for (size_t i = 0; i + 4 <= size && i < 100; i += 4) {
        uint32_t block = *(uint32_t*)(data + i);
        
        // Look for specific patterns
        if (block == 0x12345678) {
            g_path_counter++;
            printf("Found magic block at offset %zu\n", i);
        } else if (block == 0xDEADBEEF) {
            printf("Found marker block at offset %zu\n", i);
        }
        
        // Nested conditions
        if ((block & 0xFF) == 0xAA) {
            if (((block >> 8) & 0xFF) == 0xBB) {
                printf("Found AABB pattern\n");
            }
        }
    }
}

// Complex parsing function with error paths
int parse_command(const uint8_t *data, size_t size) {
    if (size < 2) return -1;
    
    uint8_t cmd = data[0];
    uint8_t len = data[1];
    
    // Command dispatch with length validation
    switch (cmd) {
        case 0x01:  // INIT command
            if (len > 0 && size >= 2 + len) {
                printf("INIT command with %d bytes\n", len);
                // Process INIT data
                for (size_t i = 0; i < len; i++) {
                    if (data[2 + i] == 0xFF) {
                        printf("Special byte in INIT data\n");
                    }
                }
                return 1;
            }
            break;
            
        case 0x02:  // CONFIG command
            if (len == 4 && size >= 6) {
                uint32_t config = *(uint32_t*)(data + 2);
                printf("CONFIG command: 0x%08X\n", config);
                
                // Parse config flags
                if (config & 0x01) printf("Flag 1 set\n");
                if (config & 0x02) printf("Flag 2 set\n");
                if (config & 0x04) printf("Flag 3 set\n");
                if (config & 0x08) printf("Flag 4 set\n");
                
                return 2;
            }
            break;
            
        case 0x03:  // RESET command
            printf("RESET command\n");
            g_path_counter = 0;
            g_processed_patterns.clear();
            return 3;
            
        case 0xFF:  // DEBUG command
            if (len == 0xFF) {
                printf("DEBUG command - special mode\n");
                return 0xFF;
            }
            break;
            
        default:
            if (cmd >= 0x80) {
                printf("Extended command: 0x%02X\n", cmd);
                return cmd;
            }
    }
    
    return 0;
}

// String processing with multiple paths
void process_string_data(const uint8_t *data, size_t size) {
    // Look for text patterns
    const char *patterns[] = {"FUZZ", "TEST", "COVERAGE", "CRASH"};
    
    for (const char *pattern : patterns) {
        size_t pattern_len = strlen(pattern);
        if (size >= pattern_len) {
            if (memcmp(data, pattern, pattern_len) == 0) {
                g_processed_patterns.push_back(pattern);
                printf("Found pattern: %s\n", pattern);
                
                // Special handling for certain patterns
                if (strcmp(pattern, "COVERAGE") == 0) {
                    // Execute rarely taken branch
                    for (int i = 0; i < 5; i++) {
                        g_path_counter += i * i;
                    }
                }
            }
        }
    }
}

// Error injection for robustness testing
void check_error_conditions(const uint8_t *data, size_t size) {
    // Buffer overflow check
    if (size > 10000) {
        printf("Large input detected\n");
    }
    
    // Null pointer check pattern
    if (size >= 8 && memcmp(data, "NULLPTR", 7) == 0) {
        int *p = nullptr;
        if (data[7] == '!') {
            // Intentional null pointer dereference
            *p = 42;
        }
    }
    
    // Division by zero check
    if (size >= 4) {
        int divisor = *(int*)data;
        if (divisor == 0x12340000) {
            int result = 100 / (divisor >> 16);  // Divide by zero
            printf("Result: %d\n", result);
        }
    }
}

// Main LibFuzzer entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Handle empty input
    if (size == 0) return 0;
    
    // Test various code paths for coverage
    int header_type = analyze_header(data, size);
    
    // Process based on header type
    if (header_type > 0) {
        process_data_blocks(data, size);
    }
    
    // Parse commands if size permits
    if (size >= 6) {
        int cmd_result = parse_command(data, size);
        if (cmd_result > 0) {
            // Process command-specific data
            if (size > 6) {
                process_string_data(data + 6, size - 6);
            }
        }
    }
    
    // String processing
    process_string_data(data, size);
    
    // Error condition checks
    check_error_conditions(data, size);
    
    // Crash conditions for testing
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        abort();
    }
    
    // Memory leak simulation for AddressSanitizer
    if (size >= 4 && memcmp(data, "LEAK", 4) == 0) {
        void *leaked = malloc(1024);
        (void)leaked;  // Intentional leak
    }
    
    // Use-after-free for AddressSanitizer
    if (size >= 3 && memcmp(data, "UAF", 3) == 0) {
        int *p = new int(42);
        delete p;
        *p = 13;  // Use after free
    }
    
    // Multiple return paths for coverage
    if (g_path_counter > 100) {
        return 1;  // Rare path
    } else if (g_path_counter > 50) {
        return 2;  // Uncommon path
    } else if (g_processed_patterns.size() > 3) {
        return 3;  // Pattern-based path
    }
    
    return 0;  // Common path
}

// Optional: Custom mutator for LibFuzzer (advanced feature)
extern "C" size_t LLVMFuzzerCustomMutator(uint8_t *data, size_t size,
                                          size_t max_size, unsigned int seed) {
    // Example: Insert command headers randomly
    if (size + 2 <= max_size && (seed % 4) == 0) {
        memmove(data + 2, data, size);
        data[0] = seed % 4;  // Random command
        data[1] = size < 255 ? size : 255;  // Length
        return size + 2;
    }
    return size;
}

// Optional: Initialize function for one-time setup
extern "C" int LLVMFuzzerInitialize(int *argc, char ***argv) {
    // Parse custom arguments if needed
    for (int i = 0; i < *argc; i++) {
        if (strcmp((*argv)[i], "-help=1") == 0) {
            printf("LibFuzzer coverage harness\n");
            printf("This harness demonstrates coverage-friendly patterns\n");
            return 0;
        }
    }
    
    // Initialization
    printf("LibFuzzer harness initialized\n");
    return 0;
}