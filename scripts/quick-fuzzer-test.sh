#!/bin/bash

# Quick test script to start AFL++ and LibFuzzer jobs
# This is a simplified version for quick testing

set -e

# Configuration
MASTER_URL=${MASTER_URL:-"http://localhost:8080"}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Quick Fuzzer Test ==="
echo "Master URL: $MASTER_URL"

# Function to check if service is up
check_service() {
    if curl -s "$MASTER_URL/health" >/dev/null 2>&1; then
        echo "✓ Master service is running"
        return 0
    else
        echo "✗ Master service is not running at $MASTER_URL"
        echo "  Please start the master first"
        return 1
    fi
}

# Function to create and upload a simple test binary
create_simple_job() {
    local fuzzer_type=$1
    local job_name="Quick $fuzzer_type Test"
    
    echo -e "\n--- Testing $fuzzer_type ---"
    
    # Use pre-built test binary if available
    local test_binary=""
    if [ "$fuzzer_type" = "afl++" ]; then
        test_binary="$PROJECT_ROOT/test-resources/test-targets/crashers/test-crasher"
    else
        test_binary="$PROJECT_ROOT/test-resources/test-targets/fuzzers/libfuzzer-test"
    fi
    
    # If no pre-built binary, create a simple one
    if [ ! -f "$test_binary" ]; then
        echo "Creating simple test binary..."
        cat > /tmp/simple_fuzzer.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        abort();
    }
    return 0;
}

#ifdef __AFL_COMPILER
int main() {
    uint8_t buf[1024];
    ssize_t len = read(0, buf, sizeof(buf));
    if (len > 0) LLVMFuzzerTestOneInput(buf, len);
    return 0;
}
#endif
EOF
        
        if [ "$fuzzer_type" = "afl++" ]; then
            gcc -o /tmp/simple_fuzzer /tmp/simple_fuzzer.c 2>/dev/null || {
                echo "Failed to compile test binary"
                return 1
            }
        else
            clang -fsanitize=fuzzer -o /tmp/simple_fuzzer /tmp/simple_fuzzer.c 2>/dev/null || {
                echo "Failed to compile libfuzzer binary (clang with fuzzer support required)"
                return 1
            }
        fi
        test_binary="/tmp/simple_fuzzer"
    fi
    
    # Create simple seed corpus
    mkdir -p /tmp/seeds
    echo "test" > /tmp/seeds/seed1
    echo "CRASH" > /tmp/seeds/seed2
    cd /tmp/seeds && tar -czf /tmp/seeds.tar.gz * && cd - >/dev/null
    
    # Create job with upload
    echo "Creating $fuzzer_type job..."
    local response=$(curl -s -X POST "$MASTER_URL/api/v1/jobs/upload" \
        -F "job_metadata={\"name\":\"$job_name\",\"type\":\"fuzzing\",\"fuzzer\":\"$fuzzer_type\",\"config\":{\"duration\":30,\"timeout\":1000,\"memory_limit\":536870912}}" \
        -F "target_binary=@$test_binary" \
        -F "seed_corpus=@/tmp/seeds.tar.gz")
    
    local job_id=$(echo "$response" | jq -r '.id' 2>/dev/null)
    
    if [ -z "$job_id" ] || [ "$job_id" = "null" ]; then
        echo "Failed to create job"
        echo "Response: $response"
        return 1
    fi
    
    echo "✓ Created job: $job_id"
    echo "  View at: $MASTER_URL/jobs/$job_id"
    
    # Quick status check
    sleep 5
    local status=$(curl -s "$MASTER_URL/api/v1/jobs/$job_id" | jq -r '.status')
    echo "  Status: $status"
    
    # Check for crashes
    local crashes=$(curl -s "$MASTER_URL/api/v1/results/crashes" | jq -r ".crashes | map(select(.job_id == \"$job_id\")) | length")
    echo "  Crashes found: $crashes"
    
    return 0
}

# Main execution
main() {
    # Check if master is running
    if ! check_service; then
        exit 1
    fi
    
    # Test AFL++
    if ! create_simple_job "afl++"; then
        echo "AFL++ test failed"
    fi
    
    # Test LibFuzzer
    if ! create_simple_job "libfuzzer"; then
        echo "LibFuzzer test failed"
    fi
    
    echo -e "\n=== Quick Test Complete ==="
    echo "Check the web UI at: $MASTER_URL"
    echo "API endpoints:"
    echo "  - Jobs: $MASTER_URL/api/v1/jobs"
    echo "  - Crashes: $MASTER_URL/api/v1/results/crashes"
    
    # Cleanup
    rm -f /tmp/simple_fuzzer /tmp/simple_fuzzer.c
    rm -rf /tmp/seeds /tmp/seeds.tar.gz
}

main "$@"