#!/bin/bash

# Simple test script for AFL++ and LibFuzzer
# This script creates and tests both fuzzers with minimal complexity

set -e

# Configuration
MASTER_URL=${MASTER_URL:-"http://localhost:8080"}

echo "=== Simple Fuzzer Test ==="
echo "Master URL: $MASTER_URL"

# Check if master is running
if ! curl -s "$MASTER_URL/health" >/dev/null 2>&1; then
    echo "Error: Master service is not running at $MASTER_URL"
    exit 1
fi
echo "✓ Master is running"

# Create test binaries
echo -e "\n--- Creating Test Binaries ---"

# AFL++ test binary
echo "Creating AFL++ test binary..."
cat > /tmp/afl_test.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main() {
    char buf[100];
    ssize_t len = read(0, buf, sizeof(buf));
    
    if (len >= 5) {
        if (memcmp(buf, "CRASH", 5) == 0) {
            abort();  // This will crash
        }
        if (memcmp(buf, "FAULT", 5) == 0) {
            int *p = NULL;
            *p = 42;  // Segfault
        }
    }
    
    printf("Input processed: %.*s\n", (int)len, buf);
    return 0;
}
EOF

gcc -o /tmp/afl_test /tmp/afl_test.c
echo "✓ AFL++ test binary created"

# LibFuzzer test binary (if clang is available)
LIBFUZZER_BINARY=""
if command -v clang >/dev/null 2>&1; then
    echo "Creating LibFuzzer test binary..."
    cat > /tmp/libfuzzer_test.cc << 'EOF'
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size >= 5) {
        if (memcmp(data, "CRASH", 5) == 0) {
            abort();
        }
        if (memcmp(data, "FAULT", 5) == 0) {
            int *p = nullptr;
            *p = 42;
        }
    }
    return 0;
}
EOF
    
    if clang++ -fsanitize=fuzzer -o /tmp/libfuzzer_test /tmp/libfuzzer_test.cc 2>/dev/null; then
        LIBFUZZER_BINARY="/tmp/libfuzzer_test"
        echo "✓ LibFuzzer test binary created"
    else
        echo "⚠ LibFuzzer compilation failed (fuzzer support not available)"
    fi
else
    echo "⚠ Clang not found, skipping LibFuzzer"
fi

# Create comprehensive seed corpus
echo -e "\nCreating seed corpus..."
mkdir -p /tmp/seeds

# Create various seed inputs
echo "test" > /tmp/seeds/seed_normal1
echo "normal input" > /tmp/seeds/seed_normal2
echo "fuzzing test" > /tmp/seeds/seed_normal3
echo "CRASH" > /tmp/seeds/seed_crash1
echo "FAULT" > /tmp/seeds/seed_fault1
echo "CRASH_TEST" > /tmp/seeds/seed_crash2
echo "FAULT_TEST" > /tmp/seeds/seed_fault2
echo "aaaaaaaaaa" > /tmp/seeds/seed_long1
echo -n -e "\x00\x01\x02\x03\x04" > /tmp/seeds/seed_binary1
echo -n -e "CRASH\x00\x00\x00" > /tmp/seeds/seed_crash_binary
echo "12345678901234567890" > /tmp/seeds/seed_numbers

# Create corpus archive
cd /tmp/seeds && tar -czf /tmp/seeds.tar.gz * && cd - >/dev/null
echo "✓ Seed corpus created with $(ls -1 /tmp/seeds | wc -l) seeds"

# Also create individual corpus directory for direct use
mkdir -p /tmp/corpus_dir
cp /tmp/seeds/* /tmp/corpus_dir/
echo "✓ Corpus directory created at /tmp/corpus_dir"

# Test AFL++
echo -e "\n--- Testing AFL++ ---"
echo "Uploading AFL++ job with:"
echo "  - Binary: /tmp/afl_test"
echo "  - Corpus: /tmp/seeds.tar.gz ($(ls -1 /tmp/seeds | wc -l) seeds)"
echo "  - Duration: 30 seconds"

RESPONSE=$(curl -s -X POST "$MASTER_URL/api/v1/jobs/upload" \
    -F "job_metadata={\"name\":\"AFL++ Test with Corpus\",\"type\":\"fuzzing\",\"fuzzer\":\"afl++\",\"config\":{\"duration\":30,\"timeout\":1000,\"memory_limit\":536870912}}" \
    -F "target_binary=@/tmp/afl_test" \
    -F "seed_corpus=@/tmp/seeds.tar.gz")

AFL_JOB_ID=$(echo "$RESPONSE" | jq -r '.id' 2>/dev/null)
if [ -n "$AFL_JOB_ID" ] && [ "$AFL_JOB_ID" != "null" ]; then
    echo "✓ AFL++ job created: $AFL_JOB_ID"
    
    # Wait and check status
    sleep 10
    STATUS=$(curl -s "$MASTER_URL/api/v1/jobs/$AFL_JOB_ID" | jq -r '.status')
    echo "  Status: $STATUS"
    
    # Check for crashes
    CRASHES=$(curl -s "$MASTER_URL/api/v1/results/crashes" | jq -r ".crashes | map(select(.job_id == \"$AFL_JOB_ID\")) | length")
    echo "  Crashes found: $CRASHES"
    
    # Check logs
    LOG_CHECK=$(curl -s "$MASTER_URL/api/v1/jobs/$AFL_JOB_ID/logs" | jq -r '.exists')
    if [ "$LOG_CHECK" = "true" ]; then
        echo "  ✓ Logs captured"
    else
        echo "  ⚠ No logs found"
    fi
else
    echo "✗ Failed to create AFL++ job"
    echo "Response: $RESPONSE"
fi

# Test LibFuzzer
if [ -n "$LIBFUZZER_BINARY" ]; then
    echo -e "\n--- Testing LibFuzzer ---"
    echo "Uploading LibFuzzer job with:"
    echo "  - Binary: $LIBFUZZER_BINARY"
    echo "  - Corpus: /tmp/seeds.tar.gz ($(ls -1 /tmp/seeds | wc -l) seeds)"
    echo "  - Duration: 30 seconds"
    
    RESPONSE=$(curl -s -X POST "$MASTER_URL/api/v1/jobs/upload" \
        -F "job_metadata={\"name\":\"LibFuzzer Test with Corpus\",\"type\":\"fuzzing\",\"fuzzer\":\"libfuzzer\",\"config\":{\"duration\":30,\"timeout\":1000,\"memory_limit\":536870912}}" \
        -F "target_binary=@$LIBFUZZER_BINARY" \
        -F "seed_corpus=@/tmp/seeds.tar.gz")
    
    LF_JOB_ID=$(echo "$RESPONSE" | jq -r '.id' 2>/dev/null)
    if [ -n "$LF_JOB_ID" ] && [ "$LF_JOB_ID" != "null" ]; then
        echo "✓ LibFuzzer job created: $LF_JOB_ID"
        
        # Wait and check status
        sleep 10
        STATUS=$(curl -s "$MASTER_URL/api/v1/jobs/$LF_JOB_ID" | jq -r '.status')
        echo "  Status: $STATUS"
        
        # Check for crashes
        CRASHES=$(curl -s "$MASTER_URL/api/v1/results/crashes" | jq -r ".crashes | map(select(.job_id == \"$LF_JOB_ID\")) | length")
        echo "  Crashes found: $CRASHES"
        
        # Check logs
        LOG_CHECK=$(curl -s "$MASTER_URL/api/v1/jobs/$LF_JOB_ID/logs" | jq -r '.exists')
        if [ "$LOG_CHECK" = "true" ]; then
            echo "  ✓ Logs captured"
        else
            echo "  ⚠ No logs found"
        fi
    else
        echo "✗ Failed to create LibFuzzer job"
        echo "Response: $RESPONSE"
    fi
else
    echo -e "\n--- LibFuzzer Test Skipped (no binary) ---"
fi

# Summary
echo -e "\n=== Test Complete ==="
echo "View results at: $MASTER_URL"
echo "API: $MASTER_URL/api/v1/results/crashes"

# Cleanup
rm -f /tmp/afl_test /tmp/afl_test.c /tmp/libfuzzer_test /tmp/libfuzzer_test.cc
rm -rf /tmp/seeds /tmp/seeds.tar.gz