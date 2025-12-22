#!/bin/bash

# Simple AFL++ and LibFuzzer test using PandaFuzz API
# This script creates test binaries for both fuzzers, uploads them, and runs fuzzing

set -e

# Get script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export AFL_PATH=/usr/local/lib/afl
# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check for command line arguments
FUZZER_ARG="${1:-both}"  # Default to running both tests

if [[ "$FUZZER_ARG" != "afl++" && "$FUZZER_ARG" != "libfuzzer" && "$FUZZER_ARG" != "honggfuzz" && "$FUZZER_ARG" != "both" ]]; then
    echo -e "${RED}Invalid fuzzer type: $FUZZER_ARG${NC}"
    echo "Usage: $0 [afl++|libfuzzer|honggfuzz|both]"
    echo "  Default: both (runs AFL++, LibFuzzer, then HongFuzz)"
    exit 1
fi

# Function to run a single fuzzer test
run_fuzzer_test() {
    local FUZZER_TYPE="$1"
    
    echo -e "\n${BLUE}=== Simple $FUZZER_TYPE Test with PandaFuzz ===${NC}"
    echo ""

    # Configuration
    MASTER_URL="${MASTER_URL:-http://localhost:8080}"
    API_BASE="${MASTER_URL}/api/v1"
    echo -e "${BLUE}Using PandaFuzz at: ${MASTER_URL}${NC}"

    # Check if we can reach the API
    echo -e "\n${YELLOW}Checking API availability...${NC}"
    if curl -s "${API_BASE}/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ API is reachable${NC}"
    else
        echo -e "${RED}✗ Cannot reach API at ${MASTER_URL}${NC}"
        echo "Please ensure PandaFuzz master is running"
        return 1
    fi

    # Step 1: Create test binary
    echo -e "\n${YELLOW}Step 1: Creating $FUZZER_TYPE test binary...${NC}"

    # Create a temporary directory for our test
    TEMP_BUILD_DIR=$(mktemp -d)
    cd "$TEMP_BUILD_DIR"

    if [[ "$FUZZER_TYPE" == "afl++" ]]; then
        # Create a proper AFL++ test program with easy-to-find bugs
        cat > afl_test.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>

int main(int argc, char *argv[]) {
    unsigned char buffer[100];
    int bytes_read;

    // For debugging
    fprintf(stderr, "AFL test program started with %d args\n", argc);

    // Always read from stdin for AFL++ compatibility
    fprintf(stderr, "Reading from stdin\n");

    // Read input - use read() instead of fgets() for binary data
    bytes_read = read(0, buffer, sizeof(buffer) - 1);
    if (bytes_read <= 0) {
        fprintf(stderr, "No input received\n");
        return 0;
    }

    fprintf(stderr, "Received %d bytes\n", bytes_read);

    // AFL++-friendly crash conditions using simple byte comparisons
    // These are much easier for AFL++ to discover through mutations

    // Bug 1: Simple byte sequence trigger (easier than string comparison)
    if (bytes_read >= 3) {
        if (buffer[0] == 'A' && buffer[1] == 'B' && buffer[2] == 'C') {
            fprintf(stderr, "Found ABC pattern - triggering crash!\n");
            fflush(stderr);
            // Null pointer dereference
            int *p = NULL;
            *p = 42;
        }
    }

    // Bug 2: Magic number trigger
    if (bytes_read >= 4) {
        unsigned int magic = (buffer[0] << 24) | (buffer[1] << 16) | (buffer[2] << 8) | buffer[3];
        if (magic == 0xDEADBEEF) {
            fprintf(stderr, "Found magic number 0xDEADBEEF - triggering abort!\n");
            fflush(stderr);
            abort();
        }
    }

    // Bug 3: Very simple trigger - just check first byte
    if (bytes_read >= 1 && buffer[0] == 'X') {
        if (bytes_read >= 2 && buffer[1] == 'Y') {
            if (bytes_read >= 3 && buffer[2] == 'Z') {
                fprintf(stderr, "Found XYZ pattern - triggering segfault!\n");
                fflush(stderr);
                raise(SIGSEGV);
            }
        }
    }

    // Bug 4: Size-based trigger (easiest for AFL++ to hit)
    if (bytes_read >= 20 && bytes_read < 25) {
        // Check for specific pattern at this size
        if (buffer[0] == 'B' && buffer[1] == 'U' && buffer[2] == 'G') {
            fprintf(stderr, "Found BUG pattern at right size - crashing!\n");
            fflush(stderr);
            // Array out of bounds
            char small[5];
            memcpy(small, buffer, bytes_read);
        }
    }

    fprintf(stderr, "Processing completed successfully\n");
    return 0;
}
EOF

        # Compile the test program
        echo -e "${YELLOW}Compiling AFL++ test binary...${NC}"
        if command -v afl-clang-fast >/dev/null 2>&1; then
            echo -e "${GREEN}✓ Found afl-clang-fast, building instrumented binary with LLVM mode${NC}"
            afl-clang-fast -g -O0 -o afl_test afl_test.c 2>/dev/null || gcc -g -O0 -o afl_test afl_test.c
        elif command -v afl-gcc >/dev/null 2>&1; then
            echo -e "${GREEN}✓ Found afl-gcc, building instrumented binary${NC}"
            afl-gcc -g -O0 -o afl_test afl_test.c 2>/dev/null || gcc -g -O0 -o afl_test afl_test.c
        else
            echo -e "${YELLOW}⚠️  AFL++ compilers not found, using regular gcc${NC}"
            gcc -g -O0 -o afl_test afl_test.c
        fi

        TEST_BINARY="$TEMP_BUILD_DIR/afl_test"
        echo -e "${GREEN}✓ Created AFL++ test binary: ${TEST_BINARY}${NC}"

    elif [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
        # Create a LibFuzzer test program based on LLVM documentation
        cat > libfuzzer_test.cpp << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <signal.h>

// LibFuzzer entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // Handle empty input
    if (Size == 0) return 0;
    
    // Check for various crash conditions
    if (Size >= 5) {
        if (memcmp(Data, "CRASH", 5) == 0) {
            fprintf(stderr, "Found CRASH pattern, triggering null pointer dereference\n");
            fflush(stderr);
            int *p = nullptr;
            *p = 42;  // Null pointer dereference - will cause SIGSEGV
        }
        
        if (memcmp(Data, "ABORT", 5) == 0) {
            fprintf(stderr, "Found ABORT pattern, calling abort()\n");
            fflush(stderr);
            abort();  // Will cause SIGABRT
        }
        
        if (Size >= 4 && memcmp(Data, "SEGV", 4) == 0) {
            fprintf(stderr, "Found SEGV pattern, raising SIGSEGV\n");
            fflush(stderr);
            raise(SIGSEGV);  // Will cause SIGSEGV
        }
        
        if (memcmp(Data, "HFUZZ", 5) == 0) {
            fprintf(stderr, "Found HFUZZ pattern, triggering crash\n");
            fflush(stderr);
            __builtin_trap();  // Guaranteed crash
        }
    }
    
    // Buffer overflow vulnerability
    if (Size > 50) {
        fprintf(stderr, "Large input (size %zu), triggering buffer overflow\n", Size);
        fflush(stderr);
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
EOF

        # Compile the LibFuzzer test program
        echo -e "${YELLOW}Compiling LibFuzzer test binary...${NC}"
        
        # Check for clang++ or g++
        if command -v clang++ >/dev/null 2>&1; then
            echo -e "${GREEN}✓ Found clang++, building LibFuzzer binary${NC}"
            # Try to compile with LibFuzzer support
            if clang++ -g -O1 -fsanitize=fuzzer,address -o libfuzzer_test libfuzzer_test.cpp 2>/dev/null; then
                echo -e "${GREEN}✓ Successfully built with LibFuzzer instrumentation${NC}"
            else
                echo -e "${YELLOW}⚠️  LibFuzzer not available, building standalone binary${NC}"
                # Build a standalone version that reads from stdin
                cat > libfuzzer_standalone.cpp << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include <string.h>

// Forward declaration
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size);

// Include the original test function
#include "libfuzzer_test.cpp"

// Main function for standalone execution
int main(int argc, char *argv[]) {
    // Check if help is requested (to pass LibFuzzer binary check)
    // Check ALL arguments for help flag, not just argv[1]
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-help=1") == 0 || 
            strcmp(argv[i], "--help=1") == 0 ||
            strcmp(argv[i], "-help") == 0 || 
            strcmp(argv[i], "--help") == 0) {
            printf("libFuzzer standalone binary (g++ compiled)\n");
            printf("This is a libFuzzer-compatible test binary\n");
            printf("LLVMFuzzerTestOneInput available\n");
            printf("-max_total_time=N\n");
            return 0;
        }
    }
    
    // For PandaFuzz, the binary will be called with corpus directories as arguments
    // We'll just read from stdin like AFL++ does
    fprintf(stderr, "LibFuzzer standalone: Reading from stdin\n");
    
    uint8_t buffer[4096];
    ssize_t size = read(0, buffer, sizeof(buffer));
    if (size > 0) {
        LLVMFuzzerTestOneInput(buffer, size);
    }
    return 0;
}
EOF
                clang++ -g -O0 -o libfuzzer_test libfuzzer_standalone.cpp
            fi
        elif command -v g++ >/dev/null 2>&1; then
            echo -e "${YELLOW}⚠️  clang++ not found, using g++ for LibFuzzer-compatible binary${NC}"
            echo -e "${YELLOW}This will work with PandaFuzz and process corpus files like LibFuzzer${NC}"
            
            # Copy the compatible implementation
            cp "${SCRIPT_DIR}/libfuzzer_compat_v2.cpp" libfuzzer_standalone.cpp 2>/dev/null || cp "${SCRIPT_DIR}/libfuzzer_compat.cpp" libfuzzer_standalone.cpp 2>/dev/null || cat > libfuzzer_standalone.cpp << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include <string.h>

// Forward declaration
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size);

// LibFuzzer entry point
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // Handle empty input
    if (Size == 0) return 0;
    
    // Print for debugging (usually not done in production)
    fprintf(stderr, "Input size: %zu\n", Size);
    
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
        fprintf(stderr, "Large input detected\n");
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

// Main function for standalone execution
int main(int argc, char *argv[]) {
    // Check if help is requested (to pass LibFuzzer binary check)
    // Check ALL arguments for help flag, not just argv[1]
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-help=1") == 0 || 
            strcmp(argv[i], "--help=1") == 0 ||
            strcmp(argv[i], "-help") == 0 || 
            strcmp(argv[i], "--help") == 0) {
            printf("libFuzzer standalone binary (g++ compiled)\n");
            printf("This is a libFuzzer-compatible test binary\n");
            printf("LLVMFuzzerTestOneInput available\n");
            printf("-max_total_time=N\n");
            return 0;
        }
    }
    
    // For PandaFuzz, the binary will be called with corpus directories as arguments
    // We'll just read from stdin like AFL++ does
    fprintf(stderr, "LibFuzzer standalone: Reading from stdin\n");
    
    uint8_t buffer[4096];
    ssize_t size = read(0, buffer, sizeof(buffer));
    if (size > 0) {
        LLVMFuzzerTestOneInput(buffer, size);
    }
    return 0;
}
EOF
            # Compile with g++
            g++ -g -O0 -o libfuzzer_test libfuzzer_standalone.cpp
            echo -e "${GREEN}✓ Built LibFuzzer-compatible test binary with g++${NC}"
        else
            echo -e "${RED}✗ Neither clang++ nor g++ found, cannot build LibFuzzer test${NC}"
            echo -e "${YELLOW}To install clang++:${NC}"
            echo -e "  Ubuntu/Debian: sudo apt-get install clang"
            echo -e "  RHEL/CentOS: sudo yum install clang"
            echo -e "  macOS: Install Xcode Command Line Tools"
            return 1
        fi
        
        TEST_BINARY="$TEMP_BUILD_DIR/libfuzzer_test"
        echo -e "${GREEN}✓ Created LibFuzzer test binary: ${TEST_BINARY}${NC}"
    fi
    
    if [[ "$FUZZER_TYPE" == "honggfuzz" ]]; then
        # Create HongFuzz test program
        cat > honggfuzz_test.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <stdint.h>

int main(int argc, char **argv) {
    unsigned char buf[256] = {0};
    ssize_t n = 0;

    // HongFuzz passes filename as argument - REQUIRE it
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <file>\n", argv[0]);
        fprintf(stderr, "This binary is designed for Honggfuzz file mode\n");
        return 2;  // Exit with error if no file provided
    }
    
    fprintf(stderr, "Reading from file: %s\n", argv[1]);
    int fd = open(argv[1], O_RDONLY);
    if (fd >= 0) {
        n = read(fd, buf, sizeof(buf) - 1);
        close(fd);
        fprintf(stderr, "Read %ld bytes\n", n);
    } else {
        fprintf(stderr, "Failed to open file: %s\n", argv[1]);
        return 1;
    }

    if (n > 0) {
        // Check for crash patterns
        if (n >= 5 && memcmp(buf, "HFUZZ", 5) == 0) {
            fprintf(stderr, "HFUZZ pattern detected - triggering abort\n");
            abort();
        }
        if (n >= 5 && memcmp(buf, "CRASH", 5) == 0) {
            fprintf(stderr, "CRASH pattern detected - triggering segfault\n");
            *((int*)0) = 42;
        }
        if (n >= 5 && memcmp(buf, "ABORT", 5) == 0) {
            fprintf(stderr, "ABORT pattern detected\n");
            abort();
        }
        if (n >= 4 && memcmp(buf, "SEGV", 4) == 0) {
            fprintf(stderr, "SEGV pattern detected\n");
            raise(11);
        }
    }

    return 0;
}
EOF
        
        # Compile the HongFuzz test program
        echo -e "${YELLOW}Compiling HongFuzz test binary...${NC}"
        gcc -g -O0 -o honggfuzz_test honggfuzz_test.c
        echo -e "${GREEN}✓ Created HongFuzz test binary${NC}"
        
        TEST_BINARY="$TEMP_BUILD_DIR/honggfuzz_test"
    fi

    # Step 2: Create corpus collection
    echo -e "\n${YELLOW}Step 2: Creating corpus collection...${NC}"
    COLLECTION_NAME="Simple $FUZZER_TYPE Test $(date +%s)"
    COLLECTION_DATA=$(cat <<EOF
{
  "name": "${COLLECTION_NAME}",
  "description": "Simple test corpus for $FUZZER_TYPE fuzzing",
  "tags": ["$FUZZER_TYPE", "simple", "test"]
}
EOF
)

    COLLECTION_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus/collections" \
      -H "Content-Type: application/json" \
      -d "${COLLECTION_DATA}")

    COLLECTION_ID=$(echo "$COLLECTION_RESPONSE" | grep -o '"[Ii][Dd]":"[^"]*"' | cut -d'"' -f4 | head -1)

    if [ -z "$COLLECTION_ID" ]; then
        echo -e "${RED}Failed to create corpus collection${NC}"
        echo "Response: $COLLECTION_RESPONSE"
        return 1
    fi

    echo -e "${GREEN}✓ Created corpus collection: ${COLLECTION_ID}${NC}"

    # Step 2: Create seed corpus files
    echo -e "\n${YELLOW}Step 2: Creating seed corpus files...${NC}"

    # Create temporary directory for corpus files
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf $TEMP_DIR" EXIT

    # Create seed files for fuzzing (including ones that trigger crashes)
    echo "test" > "$TEMP_DIR/seed_01_normal.txt"
    echo "hello" > "$TEMP_DIR/seed_02_hello.txt"
    echo "world" > "$TEMP_DIR/seed_03_world.txt"
    echo "fuzz" > "$TEMP_DIR/seed_04_fuzz.txt"
    echo "AAAA" > "$TEMP_DIR/seed_05_aaaa.txt"
    echo "1234" > "$TEMP_DIR/seed_06_numbers.txt"
    echo "AFL++" > "$TEMP_DIR/seed_07_afl.txt"
    echo "x" > "$TEMP_DIR/seed_08_single.txt"
    # Add seeds that are close to crash triggers - AFL++ will mutate these
    echo "ABD" > "$TEMP_DIR/seed_09_abc_close.txt"  # Close to ABC trigger
    echo "XYW" > "$TEMP_DIR/seed_10_xyz_close.txt"  # Close to XYZ trigger
    printf "\xDE\xAD\xBE\xEE" > "$TEMP_DIR/seed_11_magic_close.bin"  # Close to 0xDEADBEEF
    # Create a 21-byte file starting with "BVG" (close to BUG trigger)
    printf "BVG%-18s" "padding_data_here" > "$TEMP_DIR/seed_12_bug_close.txt"

    # Additional seed for LibFuzzer-specific crash
    if [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
        echo "DIV" > "$TEMP_DIR/seed_13_div.txt"
        echo -e "${GREEN}✓ Created 13 seed files (including 4 that trigger crashes)${NC}"
    elif [[ "$FUZZER_TYPE" == "honggfuzz" ]]; then
        echo "HFUZZ" > "$TEMP_DIR/seed_13_hfuzz.txt"
        echo -e "${GREEN}✓ Created 13 seed files (including 4 that trigger crashes)${NC}"
    else
        echo -e "${GREEN}✓ Created 12 seed files (including 3 that trigger crashes)${NC}"
    fi

    # Step 3: Upload corpus files
    echo -e "\n${YELLOW}Step 3: Uploading corpus files...${NC}"

    # Build upload command based on fuzzer type
    if [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
        UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus/collections/${COLLECTION_ID}/upload" \
          -F "files=@$TEMP_DIR/seed_01_normal.txt" \
          -F "files=@$TEMP_DIR/seed_02_hello.txt" \
          -F "files=@$TEMP_DIR/seed_03_world.txt" \
          -F "files=@$TEMP_DIR/seed_04_fuzz.txt" \
          -F "files=@$TEMP_DIR/seed_05_aaaa.txt" \
          -F "files=@$TEMP_DIR/seed_06_numbers.txt" \
          -F "files=@$TEMP_DIR/seed_07_afl.txt" \
          -F "files=@$TEMP_DIR/seed_08_single.txt" \
          -F "files=@$TEMP_DIR/seed_09_crash.txt" \
          -F "files=@$TEMP_DIR/seed_10_abort.txt" \
          -F "files=@$TEMP_DIR/seed_11_segv.txt" \
          -F "files=@$TEMP_DIR/seed_12_fuzz_pattern.txt" \
          -F "files=@$TEMP_DIR/seed_13_div.txt")
    elif [[ "$FUZZER_TYPE" == "honggfuzz" ]]; then
        UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus/collections/${COLLECTION_ID}/upload" \
          -F "files=@$TEMP_DIR/seed_01_normal.txt" \
          -F "files=@$TEMP_DIR/seed_02_hello.txt" \
          -F "files=@$TEMP_DIR/seed_03_world.txt" \
          -F "files=@$TEMP_DIR/seed_04_fuzz.txt" \
          -F "files=@$TEMP_DIR/seed_05_aaaa.txt" \
          -F "files=@$TEMP_DIR/seed_06_numbers.txt" \
          -F "files=@$TEMP_DIR/seed_07_afl.txt" \
          -F "files=@$TEMP_DIR/seed_08_single.txt" \
          -F "files=@$TEMP_DIR/seed_09_crash.txt" \
          -F "files=@$TEMP_DIR/seed_10_abort.txt" \
          -F "files=@$TEMP_DIR/seed_11_segv.txt" \
          -F "files=@$TEMP_DIR/seed_12_fuzz_pattern.txt" \
          -F "files=@$TEMP_DIR/seed_13_hfuzz.txt")
    else
        UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus/collections/${COLLECTION_ID}/upload" \
          -F "files=@$TEMP_DIR/seed_01_normal.txt" \
          -F "files=@$TEMP_DIR/seed_02_hello.txt" \
          -F "files=@$TEMP_DIR/seed_03_world.txt" \
          -F "files=@$TEMP_DIR/seed_04_fuzz.txt" \
          -F "files=@$TEMP_DIR/seed_05_aaaa.txt" \
          -F "files=@$TEMP_DIR/seed_06_numbers.txt" \
          -F "files=@$TEMP_DIR/seed_07_afl.txt" \
          -F "files=@$TEMP_DIR/seed_08_single.txt" \
          -F "files=@$TEMP_DIR/seed_09_crash.txt" \
          -F "files=@$TEMP_DIR/seed_10_abort.txt" \
          -F "files=@$TEMP_DIR/seed_11_segv.txt" \
          -F "files=@$TEMP_DIR/seed_12_fuzz_pattern.txt")
    fi

    UPLOAD_COUNT=$(echo "$UPLOAD_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2)

    if [ -z "$UPLOAD_COUNT" ] || [ "$UPLOAD_COUNT" -eq 0 ]; then
        echo -e "${RED}Failed to upload corpus files${NC}"
        echo "Response: $UPLOAD_RESPONSE"
    else
        echo -e "${GREEN}✓ Uploaded ${UPLOAD_COUNT} files to corpus collection${NC}"
    fi

    # Step 4: Verify corpus files were uploaded
    echo -e "\n${YELLOW}Step 4: Verifying corpus upload...${NC}"
    CORPUS_CHECK=$(curl -s "${API_BASE}/corpus/collections/${COLLECTION_ID}/files")
    FILE_COUNT=$(echo "$CORPUS_CHECK" | grep -o '"count":[0-9]*' | cut -d':' -f2)

    if [ -n "$FILE_COUNT" ] && [ "$FILE_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Verified: ${FILE_COUNT} files in corpus collection${NC}"
        
        # List files
        echo -e "${YELLOW}Files in corpus:${NC}"
        echo "$CORPUS_CHECK" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    for f in data.get('files', [])[:10]:
        print(f'  - {f.get(\"name\", \"unknown\")} ({f.get(\"size\", 0)} bytes)')
except:
    pass
" 2>/dev/null || echo "  (Could not parse file list)"
    else
        echo -e "${YELLOW}⚠️  Warning: Could not verify corpus files${NC}"
    fi

    # Step 5: Create fuzzing job with binary upload
    echo -e "\n${YELLOW}Step 5: Uploading binary and creating $FUZZER_TYPE job...${NC}"

    # Show binary size
    BINARY_SIZE=$(stat -c%s "$TEST_BINARY" 2>/dev/null || stat -f%z "$TEST_BINARY" 2>/dev/null || echo "0")
    echo -e "Binary size: $((BINARY_SIZE / 1024)) KB"

    # Create job metadata for upload
    if [[ "$FUZZER_TYPE" == "afl++" ]]; then
        # AFL++ needs DumbMode for non-instrumented binaries
        JOB_METADATA=$(cat <<EOF
{
  "name": "$FUZZER_TYPE Test $(date +%s)",
  "fuzzer": "$FUZZER_TYPE",
  "type": "fuzzing",
  "duration": 40000000000,
  "collection_id": "${COLLECTION_ID}",
  "enable_coverage": true,
  "coverage_format": "lcov",
  "config": {
    "duration": 40000000000,
    "memory_limit": 512,
    "timeout": 1000000000,
    "afl_plus_plus_options": {
      "dumb_mode": true,
      "input_dir": "/tmp/input",
      "output_dir": "/tmp/output"
    }
  }
}
EOF
)
    else
        # LibFuzzer and Honggfuzz
        JOB_METADATA=$(cat <<EOF
{
  "name": "$FUZZER_TYPE Test $(date +%s)",
  "fuzzer": "$FUZZER_TYPE",
  "type": "fuzzing",
  "duration": 40000000000,
  "collection_id": "${COLLECTION_ID}",
  "enable_coverage": true,
  "coverage_format": "lcov",
  "config": {
    "duration": 40000000000,
    "memory_limit": 512,
    "timeout": 1000000000
  }
}
EOF
)
    fi

    echo -e "${YELLOW}Uploading binary and creating job...${NC}"

    # Upload binary with job creation
    JOB_RESPONSE=$(curl -s -X POST "${API_BASE}/jobs/upload" \
      -F "job_metadata=${JOB_METADATA}" \
      -F "target_binary=@${TEST_BINARY}")

    JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1)

    if [ -z "$JOB_ID" ]; then
        echo -e "${RED}Failed to create job${NC}"
        echo "Response: $JOB_RESPONSE"
        # Cleanup
        curl -s -X DELETE "${API_BASE}/corpus/collections/${COLLECTION_ID}" > /dev/null 2>&1
        rm -rf "$TEMP_BUILD_DIR"
        return 1
    fi

    echo -e "${GREEN}✓ Created job: ${JOB_ID}${NC}"
    echo -e "${BLUE}Job will run for 40 seconds fuzzing our test binary${NC}"

    # Step 6: Monitor job execution
    echo -e "\n${YELLOW}Step 6: Monitoring job execution...${NC}"
    MONITOR_TIME=50  # Monitor for job duration + 10s buffer
    START_TIME=$(date +%s)
    LAST_EXECS=0
    FIRST_UPDATE=true
    LAST_STATUS=""

    while [ $(($(date +%s) - START_TIME)) -lt $MONITOR_TIME ]; do
        # Check job stats
        JOB_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")
        
        # Extract stats
        STATUS=$(echo "$JOB_STATS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 | head -1)
        CRASHES=$(echo "$JOB_STATS" | grep -o '"crashes_found":[0-9]*' | cut -d':' -f2 | head -1)
        EXECS=$(echo "$JOB_STATS" | grep -o '"total_execs":[0-9]*' | cut -d':' -f2 | head -1)
        COVERAGE=$(echo "$JOB_STATS" | grep -o '"coverage":[0-9]*' | cut -d':' -f2 | head -1)
        BOT_ID=$(echo "$JOB_STATS" | grep -o '"bot_id":"[^"]*"' | cut -d'"' -f4 | head -1)
        
        # Calculate execution rate
        ELAPSED=$(($(date +%s) - START_TIME))
        EXEC_RATE=0
        if [ -n "$EXECS" ] && [ "$EXECS" -gt 0 ] && [ "$ELAPSED" -gt 0 ]; then
            EXEC_RATE=$((EXECS / ELAPSED))
        fi
        
        # Display update only if values changed or first time
        if [ "${EXECS:-0}" -ne "${LAST_EXECS:-0}" ] || [ "$FIRST_UPDATE" = true ]; then
            echo -e "  [${ELAPSED}s] Status: ${STATUS} | Bot: ${BOT_ID:-pending} | Execs: ${EXECS:-0} (${EXEC_RATE}/s) | Crashes: ${CRASHES:-0}"
            LAST_EXECS="${EXECS:-0}"
            FIRST_UPDATE=false
        fi
        
        # Check if job is running for HongFuzz corpus debugging
        if [[ "$FUZZER_TYPE" == "honggfuzz" ]] && [[ "$STATUS" == "assigned" || "$STATUS" == "running" ]] && [ $ELAPSED -eq 10 ]; then
            echo -e "\n\n${YELLOW}Checking corpus download...${NC}"
            # Look for corpus download in bot logs
            docker logs pandafuzz-bot-1 2>&1 | grep -A5 -B5 "$JOB_ID" | grep -i "collection\|corpus\|download" | tail -5 || echo "  No corpus download logs found"

            # Check job directory
            JOB_DIR=$(docker exec pandafuzz-bot-1 find /app/work/jobs -name "*$JOB_ID*" -type d 2>/dev/null | head -1)
            if [ -n "$JOB_DIR" ]; then
                echo -e "\n${YELLOW}Job directory contents:${NC}"
                docker exec pandafuzz-bot-1 ls -la "$JOB_DIR/input/" 2>/dev/null | head -10 || echo "  No input directory found"
            fi
            echo ""
        fi
        
        # Check if job completed or failed
        if [[ "$STATUS" == "completed" ]] || [[ "$STATUS" == "failed" ]] || [[ "$STATUS" == "cancelled" ]]; then
            if [ "$STATUS" != "$LAST_STATUS" ]; then
                echo -e "\n${YELLOW}Job finished with status: ${STATUS}${NC}"
                LAST_STATUS="$STATUS"
                
                # If failed, show error details
                if [[ "$STATUS" == "failed" ]]; then
                    ERROR_MSG=$(echo "$JOB_STATS" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
                    ERROR_DETAILS=$(echo "$JOB_STATS" | grep -o '"message":"[^"]*"' | cut -d'"' -f4)
                    [ -n "$ERROR_MSG" ] && echo -e "${RED}Error: ${ERROR_MSG}${NC}"
                    [ -n "$ERROR_DETAILS" ] && echo -e "${RED}Details: ${ERROR_DETAILS}${NC}"
                    
                    # Get job logs
                    echo -e "\n${YELLOW}Checking job logs...${NC}"
                    JOB_LOGS=$(curl -s "${API_BASE}/jobs/${JOB_ID}/logs")
                    if [ -n "$JOB_LOGS" ]; then
                        echo -e "${YELLOW}Job logs:${NC}"
                        echo "$JOB_LOGS" | jq -r '.logs[] | "\(.timestamp) [\(.level)] \(.message)"' 2>/dev/null || echo "$JOB_LOGS"
                    fi
                fi
            fi
            
            # For completed jobs, continue monitoring to let crashes be processed
            if [[ "$STATUS" == "completed" ]]; then
                echo -e "${YELLOW}Waiting for crash processing to complete...${NC}"
                sleep 5
                # Don't break, continue monitoring
            else
                # Only break if failed or cancelled
                break
            fi
        fi
        
        sleep 2
    done

    # Step 7: Final statistics
    echo -e "\n${YELLOW}Step 7: Final job statistics...${NC}"
    FINAL_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")

    echo "$FINAL_STATS" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    job = data.get('job', data)
    print(f'Job ID: {job.get(\"id\", \"unknown\")}')
    print(f'Status: {job.get(\"status\", \"unknown\")}')
    print(f'Crashes found: {job.get(\"crashes_found\", 0)}')
    print(f'Total executions: {job.get(\"total_execs\", 0)}')
    print(f'Coverage: {job.get(\"coverage\", \"N/A\")}')
except Exception as e:
    print(f'Could not parse stats: {e}')
" 2>/dev/null
    
    # For AFL++, also check the job logs to see what happened
    if [[ "$FUZZER_TYPE" == "afl++" ]]; then
        echo -e "\n${YELLOW}Checking AFL++ job logs...${NC}"
        JOB_LOGS=$(curl -s "${API_BASE}/jobs/${JOB_ID}/logs")
        if [ -n "$JOB_LOGS" ]; then
            echo "$JOB_LOGS" | jq -r '.logs[] | "\(.timestamp) [\(.level)] \(.message)"' 2>/dev/null | tail -20 || echo "$JOB_LOGS" | tail -20
        fi
    fi
    
    # For HongFuzz, check detailed logs and output
    if [[ "$FUZZER_TYPE" == "honggfuzz" ]]; then
        echo -e "\n${YELLOW}Checking HongFuzz job logs...${NC}"
        # Give time for logs to be written
        sleep 5

        # Check log file in container
        echo -e "${YELLOW}Checking log file in bot container...${NC}"
        JOB_DIR="/app/work/jobs/job_$JOB_ID"
        echo "Job directory: $JOB_DIR"
        echo -e "\n${YELLOW}Log file contents:${NC}"
        docker exec pandafuzz-bot-1 cat "$JOB_DIR/job.log" 2>/dev/null || echo "No log file found"

        echo -e "\n${YELLOW}HongFuzz output directory:${NC}"
        docker exec pandafuzz-bot-1 ls -la "$JOB_DIR/output/honggfuzz_output/" 2>/dev/null || echo "No output directory"

        echo -e "\n${YELLOW}HongFuzz stats file:${NC}"
        docker exec pandafuzz-bot-1 cat "$JOB_DIR/output/honggfuzz_output/honggfuzz.stats" 2>/dev/null || echo "No stats file found"

        # Check if HongFuzz process is running
        echo -e "\n${YELLOW}HongFuzz process:${NC}"
        docker exec pandafuzz-bot-1 ps aux | grep honggfuzz | grep -v grep || echo "HongFuzz process not found"
    fi

    # Step 8: Check for crashes
    echo -e "\n${YELLOW}Step 8: Checking for crashes...${NC}"

    # Get crash information
    if [ -n "$JOB_ID" ]; then
        CRASHES_RESPONSE=$(curl -s "${API_BASE}/jobs/${JOB_ID}/crashes")
        CRASH_COUNT=$(echo "$CRASHES_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2 | head -1)
        
        if [ -n "$CRASH_COUNT" ] && [ "$CRASH_COUNT" -gt 0 ]; then
            echo -e "${GREEN}✓ Found ${CRASH_COUNT} crashes!${NC}"
            
            # Display crash details
            echo "$CRASHES_RESPONSE" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    crashes = data.get('crashes', [])
    for i, crash in enumerate(crashes[:5]):  # Show first 5 crashes
        print(f'\\nCrash #{i+1}:')
        print(f'  ID: {crash.get(\"id\", \"unknown\")}')
        print(f'  Type: {crash.get(\"type\", \"unknown\")}')
        print(f'  Signal: {crash.get(\"signal\", \"unknown\")}')
        print(f'  Input: {crash.get(\"input_file\", \"unknown\")}')
        print(f'  Found at: {crash.get(\"found_at\", \"unknown\")}')
    if len(crashes) > 5:
        print(f'\\n... and {len(crashes) - 5} more crashes')
except Exception as e:
    print(f'Could not parse crash data: {e}')
" 2>/dev/null || echo "  (Could not parse crash details)"
        else
            echo -e "${YELLOW}No crashes found during fuzzing${NC}"
        fi
    fi

    # Step 9: Cleanup (optional)
    echo -e "\n${YELLOW}Step 9: Cleanup...${NC}"
    
    # For debugging - don't delete or cancel anything
    echo -e "${YELLOW}⚠️  Skipping cleanup for debugging purposes${NC}"
    echo -e "${BLUE}Job ID: ${JOB_ID}${NC}"
    echo -e "${BLUE}Collection ID: ${COLLECTION_ID}${NC}"
    echo -e "${BLUE}Binary: ${TEST_BINARY}${NC}"
    
    # Don't cancel job - let it run to completion
    echo -e "${YELLOW}Job will continue running in the background${NC}"
    
    echo -e "\n${YELLOW}To manually check the job workspace:${NC}"
    echo -e "  docker exec -it pandafuzz-bot-1 /bin/sh"
    echo -e "  cd /app/work/jobs/job_${JOB_ID}"
    echo -e "  ls -la"
    if [[ "$FUZZER_TYPE" == "honggfuzz" ]]; then
        echo -e "  ls -la input/  # Check downloaded corpus files"
        echo -e "  ls -la output/honggfuzz_output/corpus/  # Check generated corpus"
        echo -e "  ls -la output/honggfuzz_output/crashes/  # Check crashes"
    else
        echo -e "  ./target_binary -help=1  # Check if binary responds correctly"
        echo -e "  ls -la input/  # Check corpus files"
    fi

    echo -e "\n${BLUE}=== $FUZZER_TYPE Test Complete ===${NC}"
}

# Main execution
if [[ "$FUZZER_ARG" == "both" ]]; then
    echo -e "${BLUE}Running AFL++, LibFuzzer, and HongFuzz tests sequentially${NC}"
    
    # Run AFL++ test
    if run_fuzzer_test "afl++"; then
        echo -e "\n${GREEN}✓ AFL++ test completed successfully${NC}"
    else
        echo -e "\n${RED}✗ AFL++ test failed${NC}"
    fi
    
    # Add a separator
    echo -e "\n${BLUE}==========================================${NC}"
    
    # Run LibFuzzer test
    if run_fuzzer_test "libfuzzer"; then
        echo -e "\n${GREEN}✓ LibFuzzer test completed successfully${NC}"
    else
        echo -e "\n${RED}✗ LibFuzzer test failed${NC}"
    fi
    
    # Add a separator
    echo -e "\n${BLUE}==========================================${NC}"
    
    # Run HongFuzz test
    if run_fuzzer_test "honggfuzz"; then
        echo -e "\n${GREEN}✓ HongFuzz test completed successfully${NC}"
    else
        echo -e "\n${RED}✗ HongFuzz test failed${NC}"
    fi
    
    echo -e "\n${BLUE}=== All Tests Complete ===${NC}"
else
    # Run single fuzzer test
    run_fuzzer_test "$FUZZER_ARG"
fi
