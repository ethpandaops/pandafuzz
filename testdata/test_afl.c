#include <stdint.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main() {
    uint8_t buffer[1024];
    ssize_t len = read(0, buffer, sizeof(buffer));
    
    if (len >= 4) {
        // Create some coverage paths
        if (buffer[0] == 'F') {
            if (buffer[1] == 'U') {
                if (buffer[2] == 'Z') {
                    if (buffer[3] == 'Z') {
                        // Intentional crash for testing
                        abort();
                    }
                }
            }
        }
        
        // Additional paths for coverage
        if (buffer[0] > 127) {
            printf("High byte\n");
        } else {
            printf("Low byte\n");
        }
    }
    
    return 0;
}
