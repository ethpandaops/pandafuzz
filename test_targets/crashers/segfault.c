#include <stdio.h>
#include <stdlib.h>
#include <string.h>

int main(int argc, char **argv) {
    // Read from stdin for libfuzzer
    size_t len;
    char *line = NULL;
    if (getline(&line, &len, stdin) != -1) {
        // Simple crash condition - if input starts with "CRASH"
        if (strncmp(line, "CRASH", 5) == 0) {
            // Null pointer dereference to cause a crash
            int *p = NULL;
            *p = 42;
        }
        printf("Input processed: %s", line);
        free(line);
    }
    return 0;
}