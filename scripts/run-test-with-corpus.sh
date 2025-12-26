#!/bin/bash

# Simple AFL++ and LibFuzzer test using PandaFuzz API
# This script creates test binaries for both fuzzers, uploads them, and runs fuzzing
#
# NOTE: This script has been updated to work with the current API v1 structure.
# The API endpoints have been refactored and some features are still being implemented.

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
        BINARY_NAME="afl_test"
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

    return 0;  // Return 0 to indicate success (non-crashing input)
}
EOF

        # Compile the LibFuzzer test program inside the container for compatibility
        echo -e "${YELLOW}Compiling LibFuzzer test binary...${NC}"

        # Check if we can use docker to compile inside the container
        if docker ps --format '{{.Names}}' | grep -q 'pandafuzz-bot'; then
            echo -e "${GREEN}✓ Found pandafuzz-bot container, compiling inside container for compatibility${NC}"
            # Copy source to container
            docker cp libfuzzer_test.cpp pandafuzz-bot-1:/tmp/libfuzzer_test.cpp
            # Compile inside container
            if docker exec pandafuzz-bot-1 clang++ -g -O1 -fsanitize=fuzzer,address -o /tmp/libfuzzer_test /tmp/libfuzzer_test.cpp 2>/dev/null; then
                echo -e "${GREEN}✓ Successfully built with LibFuzzer instrumentation inside container${NC}"
                # Copy back to host
                docker cp pandafuzz-bot-1:/tmp/libfuzzer_test libfuzzer_test
            else
                echo -e "${RED}✗ Failed to compile inside container${NC}"
                return 1
            fi
        # Fallback to local compilation if container not available
        elif command -v clang++ >/dev/null 2>&1; then
            echo -e "${YELLOW}⚠️  Container not available, building locally (may have compatibility issues)${NC}"
            # Try to compile with LibFuzzer support
            if clang++ -g -O1 -fsanitize=fuzzer,address -o libfuzzer_test libfuzzer_test.cpp 2>/dev/null; then
                echo -e "${GREEN}✓ Successfully built with LibFuzzer instrumentation${NC}"
            else
                echo -e "${YELLOW}⚠️  LibFuzzer not available, building standalone binary${NC}"
                clang++ -g -O0 -o libfuzzer_test libfuzzer_test.cpp 2>/dev/null || g++ -g -O0 -o libfuzzer_test libfuzzer_test.cpp
            fi
        elif command -v g++ >/dev/null 2>&1; then
            echo -e "${YELLOW}⚠️  clang++ not found, using g++ for LibFuzzer-compatible binary${NC}"
            g++ -g -O0 -o libfuzzer_test libfuzzer_test.cpp
        else
            echo -e "${RED}✗ Neither clang++ nor g++ found, cannot build LibFuzzer test${NC}"
            return 1
        fi

        TEST_BINARY="$TEMP_BUILD_DIR/libfuzzer_test"
        BINARY_NAME="libfuzzer_test"
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
#include <signal.h>

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
            raise(SIGSEGV);
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
        BINARY_NAME="honggfuzz_test"
    fi

    # Step 2: Create campaign
    echo -e "\n${YELLOW}Step 2: Creating campaign...${NC}"
    CAMPAIGN_NAME="$FUZZER_TYPE Test Campaign $(date +%s)"

    # Map fuzzer type to API-compatible value
    API_FUZZER_TYPE="$FUZZER_TYPE"
    if [[ "$FUZZER_TYPE" == "afl++" ]]; then
        API_FUZZER_TYPE="aflplusplus"
    fi

    CAMPAIGN_DATA=$(cat <<EOF
{
  "name": "${CAMPAIGN_NAME}",
  "description": "Test campaign for $FUZZER_TYPE fuzzing",
  "target_binary": "/app/work/binaries/${BINARY_NAME}",
  "job_template": {
    "fuzzer": "$API_FUZZER_TYPE"
  }
}
EOF
)

    CAMPAIGN_RESPONSE=$(curl -s -X POST "${API_BASE}/campaigns" \
      -H "Content-Type: application/json" \
      -d "${CAMPAIGN_DATA}")

    CAMPAIGN_ID=$(echo "$CAMPAIGN_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1)

    if [ -z "$CAMPAIGN_ID" ]; then
        echo -e "${YELLOW}⚠️  Campaign creation returned empty ID${NC}"
        echo "Response: $CAMPAIGN_RESPONSE"
        echo -e "${YELLOW}Continuing without campaign (using standalone job)...${NC}"
        CAMPAIGN_ID=""
    else
        echo -e "${GREEN}✓ Created campaign: ${CAMPAIGN_ID}${NC}"
    fi

    # Step 3: Create seed corpus files
    echo -e "\n${YELLOW}Step 3: Creating seed corpus files...${NC}"

    # Create temporary directory for corpus files
    TEMP_DIR=$(mktemp -d)
    trap "rm -rf $TEMP_DIR $TEMP_BUILD_DIR" EXIT

    # Create seed files for fuzzing (including ones that trigger crashes)
    echo "test" > "$TEMP_DIR/seed_01_normal.txt"
    echo "hello" > "$TEMP_DIR/seed_02_hello.txt"
    echo "world" > "$TEMP_DIR/seed_03_world.txt"
    echo "fuzz" > "$TEMP_DIR/seed_04_fuzz.txt"
    echo "AAAA" > "$TEMP_DIR/seed_05_aaaa.txt"
    echo "1234" > "$TEMP_DIR/seed_06_numbers.txt"
    echo "AFL++" > "$TEMP_DIR/seed_07_afl.txt"
    echo "x" > "$TEMP_DIR/seed_08_single.txt"
    # Add seeds that are close to crash triggers
    echo "ABD" > "$TEMP_DIR/seed_09_abc_close.txt"
    echo "XYW" > "$TEMP_DIR/seed_10_xyz_close.txt"
    printf "\xDE\xAD\xBE\xEE" > "$TEMP_DIR/seed_11_magic_close.bin"
    printf "BVG%-18s" "padding_data_here" > "$TEMP_DIR/seed_12_bug_close.txt"

    echo -e "${GREEN}✓ Created 12 seed files${NC}"

    # Step 4: Upload corpus files (if campaign was created)
    if [ -n "$CAMPAIGN_ID" ]; then
        echo -e "\n${YELLOW}Step 4: Uploading corpus files...${NC}"

        UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus" \
          -F "campaign_id=${CAMPAIGN_ID}" \
          -F "files=@$TEMP_DIR/seed_01_normal.txt" \
          -F "files=@$TEMP_DIR/seed_02_hello.txt" \
          -F "files=@$TEMP_DIR/seed_03_world.txt" \
          -F "files=@$TEMP_DIR/seed_04_fuzz.txt" \
          -F "files=@$TEMP_DIR/seed_05_aaaa.txt" \
          -F "files=@$TEMP_DIR/seed_06_numbers.txt")

        UPLOAD_COUNT=$(echo "$UPLOAD_RESPONSE" | grep -o '"uploaded_count":[0-9]*' | cut -d':' -f2)

        if [ -z "$UPLOAD_COUNT" ] || [ "$UPLOAD_COUNT" -eq 0 ]; then
            echo -e "${YELLOW}⚠️  Corpus upload may have failed${NC}"
            echo "Response: $UPLOAD_RESPONSE"
        else
            echo -e "${GREEN}✓ Uploaded ${UPLOAD_COUNT} corpus files${NC}"
        fi
    else
        echo -e "\n${YELLOW}Step 4: Skipping corpus upload (no campaign)${NC}"
    fi

    # Step 5: Upload binary to master storage (MUST happen before job creation)
    echo -e "\n${YELLOW}Step 5: Uploading binary to master storage...${NC}"

    # Upload the binary to master using the API
    UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE}/binaries?name=${BINARY_NAME}" \
      --data-binary "@${TEST_BINARY}" \
      -H "Content-Type: application/octet-stream")

    if echo "$UPLOAD_RESPONSE" | grep -q '"status":"success"'; then
        UPLOADED_SIZE=$(echo "$UPLOAD_RESPONSE" | grep -o '"size":[0-9]*' | cut -d':' -f2)
        echo -e "${GREEN}✓ Binary uploaded to master storage (${UPLOADED_SIZE} bytes)${NC}"
    else
        echo -e "${RED}✗ Failed to upload binary to master${NC}"
        echo "Response: $UPLOAD_RESPONSE"
        return 1
    fi

    # Step 6: Create fuzzing job (after binary is uploaded)
    echo -e "\n${YELLOW}Step 6: Creating $FUZZER_TYPE job...${NC}"

    # Build job configuration based on fuzzer type
    # Use API_FUZZER_TYPE which was set earlier (maps afl++ to aflplusplus)
    if [[ "$FUZZER_TYPE" == "afl++" ]]; then
        JOB_CONFIG=$(cat <<EOF
{
  "name": "$FUZZER_TYPE Test $(date +%s)",
  "fuzzer": "aflplusplus",
  "target_binary": "/app/work/binaries/${BINARY_NAME}",
  "timeout_seconds": 60,
  "enable_coverage": true,
  "priority": 1,
  "config": {
    "memory_limit": 512,
    "dumb_mode": true
  }
}
EOF
)
    elif [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
        JOB_CONFIG=$(cat <<EOF
{
  "name": "$FUZZER_TYPE Test $(date +%s)",
  "fuzzer": "libfuzzer",
  "target_binary": "/app/work/binaries/${BINARY_NAME}",
  "timeout_seconds": 60,
  "enable_coverage": true,
  "priority": 1,
  "config": {
    "memory_limit": 512,
    "max_len": 1024
  }
}
EOF
)
    else
        JOB_CONFIG=$(cat <<EOF
{
  "name": "$FUZZER_TYPE Test $(date +%s)",
  "fuzzer": "honggfuzz",
  "target_binary": "/app/work/binaries/${BINARY_NAME}",
  "timeout_seconds": 60,
  "enable_coverage": true,
  "priority": 1,
  "config": {
    "memory_limit": 512
  }
}
EOF
)
    fi

    # Add campaign_id if we have one
    if [ -n "$CAMPAIGN_ID" ]; then
        JOB_CONFIG=$(echo "$JOB_CONFIG" | sed 's/}$/,"campaign_id":"'$CAMPAIGN_ID'"}/')
    fi

    JOB_RESPONSE=$(curl -s -X POST "${API_BASE}/jobs" \
      -H "Content-Type: application/json" \
      -d "${JOB_CONFIG}")

    JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1)

    if [ -z "$JOB_ID" ]; then
        echo -e "${RED}Failed to create job${NC}"
        echo "Response: $JOB_RESPONSE"
        return 1
    fi

    echo -e "${GREEN}✓ Created job: ${JOB_ID}${NC}"

    # Step 7: Monitor job execution
    echo -e "\n${YELLOW}Step 7: Monitoring job execution...${NC}"
    MONITOR_TIME=70  # Monitor for job duration + 10s buffer
    START_TIME=$(date +%s)
    LAST_STATUS=""

    while [ $(($(date +%s) - START_TIME)) -lt $MONITOR_TIME ]; do
        # Check job stats
        JOB_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")

        # Extract stats
        STATUS=$(echo "$JOB_STATS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 | head -1)

        ELAPSED=$(($(date +%s) - START_TIME))

        # Display update if status changed
        if [ "$STATUS" != "$LAST_STATUS" ]; then
            echo -e "  [${ELAPSED}s] Status: ${STATUS}"
            LAST_STATUS="$STATUS"
        fi

        # Check if job completed or failed
        if [[ "$STATUS" == "completed" ]] || [[ "$STATUS" == "failed" ]] || [[ "$STATUS" == "cancelled" ]]; then
            echo -e "\n${YELLOW}Job finished with status: ${STATUS}${NC}"

            # If failed, show error details
            if [[ "$STATUS" == "failed" ]]; then
                ERROR_MSG=$(echo "$JOB_STATS" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
                [ -n "$ERROR_MSG" ] && echo -e "${RED}Error: ${ERROR_MSG}${NC}"
            fi
            break
        fi

        sleep 3
    done

    # Step 8: Final statistics
    echo -e "\n${YELLOW}Step 8: Final job statistics...${NC}"
    FINAL_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")

    echo "$FINAL_STATS" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    job = data.get('job', data)
    print(f'Job ID: {job.get(\"id\", \"unknown\")}')
    print(f'Status: {job.get(\"status\", \"unknown\")}')
    print(f'Fuzzer: {job.get(\"fuzzer\", \"unknown\")}')
except Exception as e:
    print(f'Could not parse stats: {e}')
" 2>/dev/null

    # Step 9: Check for crashes
    echo -e "\n${YELLOW}Step 9: Checking for crashes...${NC}"
    CRASHES_RESPONSE=$(curl -s "${API_BASE}/jobs/${JOB_ID}/crashes")

    echo "$CRASHES_RESPONSE" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    crashes = data.get('crashes', [])
    if crashes:
        print(f'Found {len(crashes)} crash(es)')
        for i, crash in enumerate(crashes[:3]):
            print(f'  Crash {i+1}: {crash.get(\"type\", \"unknown\")}')
    else:
        print('No crashes found during this run')
except:
    print('Could not parse crash data')
" 2>/dev/null

    # Cleanup info
    echo -e "\n${YELLOW}Cleanup info:${NC}"
    echo -e "${BLUE}Job ID: ${JOB_ID}${NC}"
    if [ -n "$CAMPAIGN_ID" ]; then
        echo -e "${BLUE}Campaign ID: ${CAMPAIGN_ID}${NC}"
    fi
    echo -e "${BLUE}Binary: ${TEST_BINARY}${NC}"

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
