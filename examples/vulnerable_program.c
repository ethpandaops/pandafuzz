#include <stdio.h>
#include <string.h>
#include <stdlib.h>

// Simple vulnerable program with a buffer overflow
void vulnerable_function(char *input) {
    char buffer[64];
    
    // Vulnerable strcpy - can cause buffer overflow
    strcpy(buffer, input);
    
    // Trigger crash on specific input
    if (input[0] == 'C' && input[1] == 'R' && input[2] == 'A' && input[3] == 'S' && input[4] == 'H') {
        // Null pointer dereference
        int *p = NULL;
        *p = 42;
    }
    
    // Another bug - array out of bounds
    if (strlen(input) > 10 && input[10] == 'B' && input[11] == 'U' && input[12] == 'G') {
        int arr[10];
        arr[100] = 1337; // Out of bounds write
    }
    
    printf("Processed: %s\n", buffer);
}

int main(int argc, char *argv[]) {
    if (argc != 2) {
        printf("Usage: %s <input_file>\n", argv[0]);
        return 1;
    }
    
    // Read input from file
    FILE *fp = fopen(argv[1], "r");
    if (!fp) {
        perror("Failed to open input file");
        return 1;
    }
    
    char input[256];
    if (fgets(input, sizeof(input), fp) != NULL) {
        // Remove newline
        input[strcspn(input, "\n")] = 0;
        vulnerable_function(input);
    }
    
    fclose(fp);
    return 0;
}