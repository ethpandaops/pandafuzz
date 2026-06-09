#include <stdio.h>
#include <unistd.h>

int main() { 
    char buf[10];
    if (read(0, buf, 10) > 0 && buf[0] == 'A') 
        return 1;
    return 0; 
}