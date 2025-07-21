// Simple test program for AFL++ fuzzing
// This program will crash on specific inputs
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char *argv[]) {
    char buffer[100];
    
    // Read from stdin or file (AFL++ will use @@)
    FILE *input;
    if (argc > 1) {
        input = fopen(argv[1], "r");
        if (!input) {
            perror("Failed to open input file");
            return 1;
        }
    } else {
        input = stdin;
    }
    
    // Read input
    if (fgets(buffer, sizeof(buffer), input) == NULL) {
        if (argc > 1) fclose(input);
        return 0;
    }
    
    if (argc > 1) {
        fclose(input);
    }
    
    // Remove newline
    buffer[strcspn(buffer, "\n")] = 0;
    
    // Simple crash conditions
    if (strlen(buffer) > 10 && buffer[0] == 'C' && buffer[1] == 'R' && buffer[2] == 'A' && buffer[3] == 'S' && buffer[4] == 'H') {
        // Null pointer dereference
        int *p = NULL;
        *p = 42;
    }
    
    if (strcmp(buffer, "SEGFAULT") == 0) {
        // Segmentation fault
        raise(11);
    }
    
    if (strcmp(buffer, "ABORT") == 0) {
        // Abort
        abort();
    }
    
    // Buffer overflow trigger
    if (strlen(buffer) > 50) {
        char small[10];
        strcpy(small, buffer); // Buffer overflow
    }
    
    printf("Processed: %s\n", buffer);
    return 0;
}