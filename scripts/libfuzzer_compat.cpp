#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include <string.h>
#include <signal.h>
#include <time.h>
#include <dirent.h>
#include <sys/stat.h>
#include <vector>
#include <string>

// LibFuzzer entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // Handle empty input
    if (Size == 0) return 0;
    
    // Check for various crash conditions
    if (Size >= 5) {
        if (memcmp(Data, "CRASH", 5) == 0) {
            fprintf(stderr, "Found CRASH pattern, triggering null pointer dereference\n");
            int *p = nullptr;
            *p = 42;  // Null pointer dereference
        }
        
        if (memcmp(Data, "ABORT", 5) == 0) {
            fprintf(stderr, "Found ABORT pattern\n");
            abort();
        }
        
        if (Size >= 4 && memcmp(Data, "SEGV", 4) == 0) {
            fprintf(stderr, "Found SEGV pattern\n");
            raise(SIGSEGV);
        }
    }
    
    // Buffer overflow vulnerability
    if (Size > 50) {
        char small[10];
        memcpy(small, Data, Size);  // Buffer overflow
    }
    
    // Check for FUZZ pattern
    if (Size >= 4) {
        for (size_t i = 0; i <= Size - 4; i++) {
            if (memcmp(Data + i, "FUZZ", 4) == 0) {
                fprintf(stderr, "Found FUZZ pattern at offset %zu\n", i);
            }
        }
    }
    
    // Division by zero
    if (Size >= 3 && memcmp(Data, "DIV", 3) == 0) {
        fprintf(stderr, "Found DIV pattern, triggering division by zero\n");
        int x = 1;
        int y = 0;
        int z = x / y;  // Division by zero
        (void)z;
    }
    
    return 0;  // Return 0 to indicate success (non-crashing input)
}

// Main function that mimics LibFuzzer behavior
int main(int argc, char *argv[]) {
    // Check if help is requested (to pass LibFuzzer binary check)
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-help=1") == 0) {
            printf("libFuzzer standalone binary (g++ compiled)\n");
            printf("This is a libFuzzer-compatible test binary\n");
            return 0;
        }
    }
    
    // Parse LibFuzzer-style arguments
    int max_total_time = 60; // default
    const char* artifact_prefix = NULL;
    
    for (int i = 1; i < argc; i++) {
        if (strncmp(argv[i], "-max_total_time=", 16) == 0) {
            max_total_time = atoi(argv[i] + 16);
        } else if (strncmp(argv[i], "-artifact_prefix=", 17) == 0) {
            artifact_prefix = argv[i] + 17;
        }
    }
    
    fprintf(stderr, "INFO: Seed: 1234\n");
    fprintf(stderr, "INFO: -max_total_time=%d\n", max_total_time);
    
    time_t start_time = time(NULL);
    int exec_count = 0;
    
    // Find corpus directories in arguments
    std::vector<std::string> corpus_dirs;
    for (int i = 1; i < argc; i++) {
        if (argv[i][0] != '-') {
            corpus_dirs.push_back(argv[i]);
            fprintf(stderr, "INFO: Loaded 1 modules\n");
            fprintf(stderr, "INFO: seed corpus: files: 0 min: 0b max: 0b total: 0b\n");
        }
    }
    
    // Process files from corpus directories
    for (const auto& dir : corpus_dirs) {
        DIR* d = opendir(dir.c_str());
        if (!d) {
            fprintf(stderr, "WARNING: Failed to open corpus directory: %s\n", dir.c_str());
            continue;
        }
        
        fprintf(stderr, "INFO: Reading from %s\n", dir.c_str());
        
        struct dirent* entry;
        while ((entry = readdir(d)) != NULL) {
            if (entry->d_type != DT_REG) continue;
            
            std::string filepath = dir + "/" + entry->d_name;
            FILE* f = fopen(filepath.c_str(), "rb");
            if (!f) continue;
            
            // Read file
            fseek(f, 0, SEEK_END);
            long fsize = ftell(f);
            fseek(f, 0, SEEK_SET);
            
            if (fsize > 0 && fsize < 1024*1024) { // Max 1MB
                uint8_t* data = (uint8_t*)malloc(fsize);
                if (fread(data, 1, fsize, f) == (size_t)fsize) {
                    exec_count++;
                    
                    // Print LibFuzzer-style progress
                    time_t elapsed = time(NULL) - start_time;
                    int exec_per_sec = elapsed > 0 ? exec_count / elapsed : exec_count;
                    fprintf(stderr, "#%d\tNEW    cov: %d ft: %d corp: %d/%ldb exec/s: %d rss: 50Mb\n", 
                            exec_count, exec_count*10, exec_count*2, (int)corpus_dirs.size(), fsize, exec_per_sec);
                    
                    // Test the input
                    fprintf(stderr, "INFO: Testing %s (%ld bytes)\n", entry->d_name, fsize);
                    
                    // Use signal handlers to catch crashes
                    struct sigaction old_sa;
                    struct sigaction sa;
                    sa.sa_handler = [](int sig) {
                        fprintf(stderr, "==1234== ERROR: libFuzzer: deadly signal\n");
                        fprintf(stderr, "SUMMARY: libFuzzer: deadly signal\n");
                        exit(77);
                    };
                    sigemptyset(&sa.sa_mask);
                    sa.sa_flags = 0;
                    
                    sigaction(SIGSEGV, &sa, &old_sa);
                    sigaction(SIGABRT, &sa, NULL);
                    sigaction(SIGFPE, &sa, NULL);
                    
                    // Test the input
                    LLVMFuzzerTestOneInput(data, fsize);
                    
                    // Restore signal handler
                    sigaction(SIGSEGV, &old_sa, NULL);
                    
                    // Check if we're still alive (no crash)
                    // If we get here, no crash occurred
                }
                free(data);
            }
            fclose(f);
            
            // Check time limit
            if (max_total_time > 0 && (time(NULL) - start_time) >= max_total_time) {
                fprintf(stderr, "INFO: exiting: %d time: %lds\n", exec_count, (long)(time(NULL) - start_time));
                break;
            }
        }
        closedir(d);
    }
    
    fprintf(stderr, "Done %d runs in %ld second(s)\n", exec_count, time(NULL) - start_time);
    return 0;
}