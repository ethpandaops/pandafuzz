#!/bin/bash

# Comprehensive test script for AFL++ and LibFuzzer crash detection
# This script tests both fuzzers to ensure crash detection and logging work correctly

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
MASTER_URL=${MASTER_URL:-"http://localhost:8080"}
TEST_DURATION=${TEST_DURATION:-30}  # seconds
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo -e "${GREEN}=== PandaFuzz Fuzzer Test Suite ===${NC}"
echo "Master URL: $MASTER_URL"
echo "Test Duration: $TEST_DURATION seconds"
echo

# Function to check if service is up
check_service() {
    local url=$1
    local service=$2
    local max_attempts=10
    local attempt=1
    
    echo -n "Checking $service..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url/health" >/dev/null 2>&1; then
            echo -e " ${GREEN}✓${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        ((attempt++))
    done
    echo -e " ${RED}✗${NC}"
    return 1
}

# Function to create a test binary
create_test_binary() {
    local fuzzer_type=$1
    local output_file=$2
    
    echo "Creating $fuzzer_type test binary..."
    
    # Create source file based on fuzzer type
    if [ "$fuzzer_type" = "afl++" ]; then
        # AFL++ version with main function
        cat > /tmp/test_fuzzer.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>

int main() {
    uint8_t buf[1024];
    ssize_t len = read(0, buf, sizeof(buf));
    
    if (len >= 5) {
        // Crash on "CRASH"
        if (memcmp(buf, "CRASH", 5) == 0) {
            abort();
        }
        // Segfault on "FAULT"
        if (memcmp(buf, "FAULT", 5) == 0) {
            int *p = NULL;
            *p = 42;
        }
        // Divide by zero on "DIVZERO"
        if (len >= 7 && memcmp(buf, "DIVZERO", 7) == 0) {
            int x = 1;
            int y = 0;
            int z = x / y;
            printf("%d\n", z);
        }
    }
    
    printf("OK\n");
    return 0;
}
EOF
    else
        # LibFuzzer version
        cat > /tmp/test_fuzzer.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

// LibFuzzer interface
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 5) {
        // Crash on "CRASH"
        if (memcmp(data, "CRASH", 5) == 0) {
            abort();
        }
        // Segfault on "FAULT"
        if (memcmp(data, "FAULT", 5) == 0) {
            int *p = NULL;
            *p = 42;
        }
        // Divide by zero on "DIVZERO"
        if (size >= 7 && memcmp(data, "DIVZERO", 7) == 0) {
            int x = 1;
            int y = 0;
            int z = x / y;
            printf("%d\n", z);
        }
    }
    return 0;
}
EOF
    fi

    # Compile based on fuzzer type
    if [ "$fuzzer_type" = "afl++" ]; then
        # Try AFL++ compiler first
        if command -v afl-clang-fast >/dev/null 2>&1; then
            afl-clang-fast -o "$output_file" /tmp/test_fuzzer.c 2>/dev/null || \
                gcc -o "$output_file" /tmp/test_fuzzer.c
        else
            # Fall back to regular compiler
            gcc -o "$output_file" /tmp/test_fuzzer.c
        fi
    else
        # LibFuzzer requires clang with fuzzer support
        if command -v clang++ >/dev/null 2>&1; then
            clang++ -fsanitize=fuzzer,address -g -o "$output_file" /tmp/test_fuzzer.c 2>/dev/null || {
                echo -e "${YELLOW}Warning: clang fuzzer support not available${NC}"
                return 1
            }
        elif command -v clang >/dev/null 2>&1; then
            clang -fsanitize=fuzzer,address -g -o "$output_file" /tmp/test_fuzzer.c 2>/dev/null || {
                echo -e "${YELLOW}Warning: clang fuzzer support not available${NC}"
                return 1
            }
        else
            echo -e "${YELLOW}Warning: clang not found, using pre-built binary if available${NC}"
            return 1
        fi
    fi
    
    chmod +x "$output_file"
    echo -e "Created: $output_file ${GREEN}✓${NC}"
    return 0
}

# Function to create seed corpus
create_seed_corpus() {
    local corpus_dir="/tmp/test_corpus"
    mkdir -p "$corpus_dir"
    
    # Create various test inputs including crash triggers
    echo "CRASH" > "$corpus_dir/crash_input"
    echo "FAULT" > "$corpus_dir/fault_input"
    echo "DIVZERO" > "$corpus_dir/div_input"
    echo "normal" > "$corpus_dir/normal1"
    echo "test123" > "$corpus_dir/normal2"
    echo "fuzzing" > "$corpus_dir/normal3"
    
    # Create corpus archive
    cd "$corpus_dir"
    tar -czf /tmp/test_corpus.tar.gz *
    cd - >/dev/null
    
    echo -e "Created seed corpus ${GREEN}✓${NC}"
}

# Function to create and monitor a fuzzing job
test_fuzzer() {
    local fuzzer_type=$1
    local binary_path=$2
    
    echo -e "\n${YELLOW}=== Testing $fuzzer_type ===${NC}"
    
    # Create job with binary upload
    echo "Creating $fuzzer_type job..."
    local response=$(curl -s -X POST "$MASTER_URL/api/v1/jobs/upload" \
        -F "job_metadata={\"name\":\"$fuzzer_type Test\",\"type\":\"fuzzing\",\"fuzzer\":\"$fuzzer_type\",\"config\":{\"duration\":$TEST_DURATION,\"timeout\":1000,\"memory_limit\":536870912}}" \
        -F "target_binary=@$binary_path" \
        -F "seed_corpus=@/tmp/test_corpus.tar.gz")
    
    local job_id=$(echo "$response" | jq -r '.id' 2>/dev/null)
    local job_status=$(echo "$response" | jq -r '.status' 2>/dev/null)
    
    if [ -z "$job_id" ] || [ "$job_id" = "null" ]; then
        echo -e "${RED}Failed to create job${NC}"
        echo "Response: $response"
        return 1
    fi
    
    echo "Created job: $job_id (status: $job_status)"
    
    # Monitor job execution
    echo "Monitoring job execution..."
    local start_time=$(date +%s)
    local crashes_found=0
    local last_crash_count=0
    
    while true; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        
        # Get job status
        local job_info=$(curl -s "$MASTER_URL/api/v1/jobs/$job_id")
        local status=$(echo "$job_info" | jq -r '.status')
        
        # Check crashes
        local crashes=$(curl -s "$MASTER_URL/api/v1/results/crashes" | jq -r '.crashes')
        local crash_count=$(echo "$crashes" | jq -r "map(select(.job_id == \"$job_id\")) | length")
        
        # Report new crashes
        if [ "$crash_count" -gt "$last_crash_count" ]; then
            echo -e "${GREEN}Found new crashes! Total: $crash_count${NC}"
            last_crash_count=$crash_count
        fi
        
        # Display progress
        printf "\r[%3d s] Status: %-10s Crashes: %d" "$elapsed" "$status" "$crash_count"
        
        # Check if job is done or timeout
        if [ "$status" = "completed" ] || [ "$status" = "failed" ] || [ $elapsed -gt $((TEST_DURATION + 30)) ]; then
            echo
            break
        fi
        
        sleep 2
    done
    
    # Get final results
    echo -e "\n${YELLOW}Results for $fuzzer_type:${NC}"
    
    # Get crash details
    local final_crashes=$(curl -s "$MASTER_URL/api/v1/results/crashes" | \
        jq -r ".crashes | map(select(.job_id == \"$job_id\"))")
    local final_crash_count=$(echo "$final_crashes" | jq -r 'length')
    
    echo "Total crashes found: $final_crash_count"
    
    if [ "$final_crash_count" -gt 0 ]; then
        echo "Crash details:"
        echo "$final_crashes" | jq -r '.[] | "  - Type: \(.type), Hash: \(.hash[0:12])..., Size: \(.size) bytes"'
        crashes_found=$final_crash_count
    fi
    
    # Check job logs
    echo -e "\nChecking job logs..."
    local log_response=$(curl -s "$MASTER_URL/api/v1/jobs/$job_id/logs?limit=10")
    local log_exists=$(echo "$log_response" | jq -r '.exists' 2>/dev/null)
    
    if [ "$log_exists" = "true" ]; then
        echo -e "${GREEN}Job logs available ✓${NC}"
        echo "Last few log lines:"
        echo "$log_response" | jq -r '.logs' | tail -5
    else
        echo -e "${RED}Job logs not available ✗${NC}"
    fi
    
    # Summary
    if [ "$crashes_found" -gt 0 ] && [ "$log_exists" = "true" ]; then
        echo -e "\n${GREEN}✓ $fuzzer_type test PASSED${NC}"
        return 0
    else
        echo -e "\n${RED}✗ $fuzzer_type test FAILED${NC}"
        [ "$crashes_found" -eq 0 ] && echo "  - No crashes detected"
        [ "$log_exists" != "true" ] && echo "  - Logs not captured"
        return 1
    fi
}

# Main test execution
main() {
    local exit_code=0
    
    # Check if master is running
    if ! check_service "$MASTER_URL" "Master"; then
        echo -e "${RED}Error: Master service is not running${NC}"
        exit 1
    fi
    
    # Create test binaries
    echo -e "\n${YELLOW}=== Preparing Test Binaries ===${NC}"
    
    local afl_binary="/tmp/afl_test_binary"
    local libfuzzer_binary="/tmp/libfuzzer_test_binary"
    
    if ! create_test_binary "afl++" "$afl_binary"; then
        # Try using existing test binary
        if [ -f "$PROJECT_ROOT/test-resources/test-targets/crashers/test-crasher" ]; then
            cp "$PROJECT_ROOT/test-resources/test-targets/crashers/test-crasher" "$afl_binary"
            echo "Using existing test binary for AFL++"
        else
            echo -e "${RED}Failed to create AFL++ test binary${NC}"
            afl_binary=""
        fi
    fi
    
    if ! create_test_binary "libfuzzer" "$libfuzzer_binary"; then
        # Try using existing test binary
        if [ -f "$PROJECT_ROOT/test-resources/test-targets/fuzzers/libfuzzer-test" ]; then
            cp "$PROJECT_ROOT/test-resources/test-targets/fuzzers/libfuzzer-test" "$libfuzzer_binary"
            echo "Using existing test binary for LibFuzzer"
        else
            echo -e "${RED}Failed to create LibFuzzer test binary${NC}"
            libfuzzer_binary=""
        fi
    fi
    
    # Create seed corpus
    create_seed_corpus
    
    # Test AFL++
    if [ -n "$afl_binary" ] && [ -f "$afl_binary" ]; then
        if ! test_fuzzer "afl++" "$afl_binary"; then
            exit_code=1
        fi
    else
        echo -e "${YELLOW}Skipping AFL++ test (no binary)${NC}"
    fi
    
    # Test LibFuzzer
    if [ -n "$libfuzzer_binary" ] && [ -f "$libfuzzer_binary" ]; then
        if ! test_fuzzer "libfuzzer" "$libfuzzer_binary"; then
            exit_code=1
        fi
    else
        echo -e "${YELLOW}Skipping LibFuzzer test (no binary)${NC}"
    fi
    
    # Final summary
    echo -e "\n${YELLOW}=== Test Summary ===${NC}"
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}All tests passed! ✓${NC}"
        echo "- AFL++ crash detection: Working"
        echo "- LibFuzzer crash detection: Working"
        echo "- Log capture: Working"
    else
        echo -e "${RED}Some tests failed ✗${NC}"
    fi
    
    # Cleanup
    rm -f /tmp/test_fuzzer.c "$afl_binary" "$libfuzzer_binary"
    rm -rf /tmp/test_corpus /tmp/test_corpus.tar.gz
    
    exit $exit_code
}

# Run main function
main "$@"