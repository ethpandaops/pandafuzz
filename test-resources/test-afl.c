#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>

int main() {
    char buffer[100];
    
    // Read from stdin
    if (read(0, buffer, sizeof(buffer)) < 1) {
        return 0;
    }
    
    // Create multiple branches for edge detection
    if (buffer[0] == 'A') {
        printf("Found A\n");
        if (buffer[1] == 'F') {
            printf("Found AF\n");
            if (buffer[2] == 'L') {
                printf("Found AFL\n");
                if (buffer[3] == '+') {
                    printf("Found AFL+\n");
                    if (buffer[4] == '+') {
                        printf("Found AFL++!\n");
                        // Crash for testing
                        if (buffer[5] == '!') {
                            abort();
                        }
                    }
                }
            }
        }
    }
    
    // Additional branches
    if (strcmp(buffer, "test") == 0) {
        printf("Test mode\n");
    }
    
    return 0;
}