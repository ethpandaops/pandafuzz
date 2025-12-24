#!/bin/bash

# Simple test to verify AFL++ is working with PandaFuzz

set -e

echo "=== Simple AFL++ Test ==="

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Create test directory in shared volume
TEST_DIR="/mnt/fuzzing/test-afl-$(date +%s)"
mkdir -p "$TEST_DIR"

echo "Test directory: $TEST_DIR"

# Create and compile a simple test program
cat > "$TEST_DIR/test.c" << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main() {
    char buf[10];
    if (read(0, buf, 10) < 1) return 0;
    
    if (buf[0] == 'F') {
        if (buf[1] == 'U') {
            if (buf[2] == 'Z') {
                if (buf[3] == 'Z') {
                    abort();
                }
            }
        }
    }
    return 0;
}
EOF

# Compile with regular gcc (AFL++ will run in dumb mode)
gcc -o "$TEST_DIR/test_binary" "$TEST_DIR/test.c"
echo -e "${GREEN}✓${NC} Compiled test binary"

# Create corpus
mkdir -p "$TEST_DIR/corpus"
echo "test" > "$TEST_DIR/corpus/seed1"
echo "Ftest" > "$TEST_DIR/corpus/seed2"
echo "FUtest" > "$TEST_DIR/corpus/seed3"
echo -e "${GREEN}✓${NC} Created corpus"

# Create output directory
mkdir -p "$TEST_DIR/output"

# Run AFL++ directly
echo -e "${YELLOW}Running AFL++ directly...${NC}"
export AFL_DUMB_FORKSRV=1
export AFL_SKIP_CPUFREQ=1
export AFL_I_DONT_CARE_ABOUT_MISSING_CRASHES=1

timeout 5s afl-fuzz -i "$TEST_DIR/corpus" -o "$TEST_DIR/output" -- "$TEST_DIR/test_binary" 2>&1 | head -20 || true

# Check for edges
if [ -f "$TEST_DIR/output/default/fuzzer_stats" ]; then
    STATS_FILE="$TEST_DIR/output/default/fuzzer_stats"
elif [ -f "$TEST_DIR/output/fuzzer_stats" ]; then
    STATS_FILE="$TEST_DIR/output/fuzzer_stats"
else
    STATS_FILE=""
fi

if [ ! -z "$STATS_FILE" ]; then
    echo
    echo "Fuzzer stats found at: $STATS_FILE"
    echo "Key metrics:"
    grep -E "edges_found|paths_total|execs_done" "$STATS_FILE" || true
    
    if grep -q "edges_found" "$STATS_FILE"; then
        EDGES=$(grep "edges_found" "$STATS_FILE" | cut -d: -f2 | tr -d ' ')
        if [ "$EDGES" -gt 0 ]; then
            echo -e "${GREEN}✓ AFL++ found $EDGES edges${NC}"
        else
            echo -e "${RED}✗ AFL++ found 0 edges${NC}"
        fi
    fi
else
    echo -e "${YELLOW}⚠ No fuzzer_stats file found${NC}"
fi

# Now test through PandaFuzz API
echo
echo -e "${YELLOW}Creating job through API...${NC}"

# Create job using curl
JOB_RESPONSE=$(curl -s -X POST "http://localhost:8080/api/v1/jobs" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Simple AFL Test\",
    \"fuzzer\": \"afl++\",
    \"target\": \"$TEST_DIR/test_binary\",
    \"duration\": \"10s\",
    \"config\": {
      \"corpus_dir\": \"$TEST_DIR/corpus\",
      \"output_dir\": \"$TEST_DIR/job_output\",
      \"enable_coverage\": true
    }
  }")

echo "API Response: $JOB_RESPONSE"

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)

if [ ! -z "$JOB_ID" ]; then
    echo -e "${GREEN}✓ Created job: $JOB_ID${NC}"
    
    # Wait for job
    echo "Waiting 15 seconds for job to run..."
    sleep 15
    
    # Check job status
    JOB_STATUS=$(curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID")
    echo "Job status response:"
    echo "$JOB_STATUS" | python3 -m json.tool 2>/dev/null || echo "$JOB_STATUS"
    
    # Check coverage
    echo
    echo "Checking coverage report..."
    COVERAGE=$(curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/coverage")
    echo "$COVERAGE" | python3 -m json.tool 2>/dev/null || echo "$COVERAGE"
else
    echo -e "${RED}✗ Failed to create job${NC}"
fi

echo
echo "Test complete!"