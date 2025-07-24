// honggfuzz_file_test_binary.c - HongFuzz test binary that reads from files
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>
#include <fcntl.h>

// HongFuzz persistent mode support
#ifdef __HONGGFUZZ__
extern int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size);
#endif

// Simple fuzzer target that crashes on "HFUZZ" input
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 5 && memcmp(data, "HFUZZ", 5) == 0) {
        fprintf(stderr, "Found HFUZZ pattern, triggering crash\n");
        abort();
    }
    // Also crash on other patterns
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        fprintf(stderr, "Found CRASH pattern, triggering crash\n");
        abort();
    }
    if (size >= 5 && memcmp(data, "ABORT", 5) == 0) {
        fprintf(stderr, "Found ABORT pattern, triggering crash\n");
        abort();
    }
    return 0;
}

// Main function that reads from file or stdin
int main(int argc, char *argv[]) {
    uint8_t buffer[4096];
    ssize_t size = 0;
    
    if (argc > 1 && strcmp(argv[1], "-help=1") != 0) {
        // Read from file (HongFuzz mode)
        int fd = open(argv[1], O_RDONLY);
        if (fd < 0) {
            perror("open");
            return 1;
        }
        size = read(fd, buffer, sizeof(buffer));
        close(fd);
        
        if (size > 0) {
            LLVMFuzzerTestOneInput(buffer, size);
        }
    } else if (argc > 1 && strcmp(argv[1], "-help=1") == 0) {
        // Help mode for compatibility check
        printf("HongFuzz test binary\n");
        printf("This is a HongFuzz-compatible test binary\n");
        return 0;
    } else {
        // Read from stdin (fallback)
        size = read(0, buffer, sizeof(buffer));
        if (size > 0) {
            LLVMFuzzerTestOneInput(buffer, size);
        }
    }
    
    return 0;
}