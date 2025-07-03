#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

// LibFuzzer target - reads from file provided as argument
int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <input_file>\n", argv[0]);
        return 1;
    }
    
    FILE *fp = fopen(argv[1], "r");
    if (!fp) {
        fprintf(stderr, "Failed to open %s\n", argv[1]);
        return 1;
    }
    
    // Read the file content
    char buffer[1024];
    size_t bytes_read = fread(buffer, 1, sizeof(buffer) - 1, fp);
    fclose(fp);
    
    if (bytes_read == 0) {
        return 0;
    }
    
    buffer[bytes_read] = '\0';
    
    // Simple crash condition - if input starts with "CRASH"
    if (strncmp(buffer, "CRASH", 5) == 0) {
        fprintf(stderr, "Crash triggered!\n");
        // Null pointer dereference to cause a crash
        int *p = NULL;
        *p = 42;
    }
    
    printf("Input processed: %s", buffer);
    return 0;
}