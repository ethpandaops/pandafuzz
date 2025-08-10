#!/bin/bash

# Comprehensive test to verify AFL++ coverage fix is working end-to-end
# This tests both edge detection and coverage percentage reporting

set -e

# Set AFL++ path for compilation
export AFL_PATH=/usr/local/lib/afl

echo "=== AFL++ Coverage Fix Verification ==="
echo "This script verifies that AFL++ properly reports edges and coverage through PandaFuzz"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# API endpoint
API_URL="http://localhost:8080/api/v3"

# Test configuration
TEST_NAME="afl-coverage-test-$(date +%s)"
TEST_DIR="/tmp/$TEST_NAME"

echo -e "${BLUE}Test ID: $TEST_NAME${NC}"
echo

# Create test directory
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Step 1: Create and compile test program
echo -e "${YELLOW}Step 1: Creating test program...${NC}"
cat > test.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char** argv) {
    char buffer[100];
    
    if (read(0, buffer, sizeof(buffer)) < 1) {
        return 0;
    }
    
    // Multiple branches to create edges
    if (buffer[0] == 'F') {
        printf("Found F\n");
        if (buffer[1] == 'U') {
            printf("Found FU\n");
            if (buffer[2] == 'Z') {
                printf("Found FUZ\n");
                if (buffer[3] == 'Z') {
                    printf("Found FUZZ\n");
                    if (buffer[4] == '!') {
                        abort(); // Crash for testing
                    }
                }
            }
        }
    } else if (buffer[0] == 'T') {
        printf("Test branch\n");
        if (buffer[1] == 'E') {
            printf("Test E\n");
        }
    }
    
    return 0;
}
EOF

# Compile - try AFL++ first, fallback to regular gcc
COMPILED=false
if [ -f /usr/local/lib/afl/afl-compiler-rt.o ]; then
    if command -v afl-clang-fast >/dev/null 2>&1; then
        afl-clang-fast -o test_binary test.c 2>/dev/null && COMPILED=true
        [ "$COMPILED" = true ] && echo -e "${GREEN}✓${NC} Compiled with afl-clang-fast"
    fi
    
    if [ "$COMPILED" = false ] && command -v afl-gcc >/dev/null 2>&1; then
        afl-gcc -o test_binary test.c 2>/dev/null && COMPILED=true
        [ "$COMPILED" = true ] && echo -e "${GREEN}✓${NC} Compiled with afl-gcc"
    fi
fi

# Fallback to regular gcc (will use AFL++ in dumb mode)
if [ "$COMPILED" = false ]; then
    gcc -o test_binary test.c
    echo -e "${YELLOW}⚠${NC} Compiled with gcc (AFL++ will run in dumb mode)"
    export AFL_DUMB_FORKSRV=1
fi

# Create corpus
mkdir -p corpus
echo "test" > corpus/seed1
echo "Ftest" > corpus/seed2
echo "FUtest" > corpus/seed3
echo "FUZtest" > corpus/seed4
echo "TEST" > corpus/seed5
echo -e "${GREEN}✓${NC} Created corpus with 5 seeds"

# Step 2: Create job via API
echo
echo -e "${YELLOW}Step 2: Creating fuzzing job with coverage enabled...${NC}"

JOB_RESPONSE=$(curl -s -X POST "$API_URL/jobs" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"$TEST_NAME\",
    \"fuzzer\": \"afl++\",
    \"target\": \"$TEST_DIR/test_binary\",
    \"duration\": \"30s\",
    \"config\": {
      \"memory_limit\": 100,
      \"timeout\": 1000,
      \"corpus_dir\": \"$TEST_DIR/corpus\",
      \"enable_coverage\": true
    }
  }")

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)

if [ -z "$JOB_ID" ]; then
    echo -e "${RED}✗${NC} Failed to create job"
    echo "Response: $JOB_RESPONSE"
    exit 1
fi

echo -e "${GREEN}✓${NC} Created job: $JOB_ID"

# Step 3: Wait for job to run
echo
echo -e "${YELLOW}Step 3: Waiting for job to complete (30 seconds)...${NC}"

for i in {1..35}; do
    sleep 1
    
    # Check job status
    JOB_STATUS=$(curl -s "$API_URL/jobs/$JOB_ID" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
    
    if [ "$JOB_STATUS" == "completed" ] || [ "$JOB_STATUS" == "failed" ]; then
        echo
        echo -e "${GREEN}✓${NC} Job $JOB_STATUS after $i seconds"
        break
    fi
    
    # Show progress
    if [ $((i % 5)) -eq 0 ]; then
        echo -n "."
    fi
done

# Step 4: Check fuzzer output directory
echo
echo -e "${YELLOW}Step 4: Checking fuzzer output...${NC}"

# Find the output directory
OUTPUT_DIR="/mnt/fuzzing/jobs/$JOB_ID/output"
STATS_FILE=""

# Check possible locations for fuzzer_stats
for path in \
    "$OUTPUT_DIR/afl_output/default/fuzzer_stats" \
    "$OUTPUT_DIR/afl_output/fuzzer_stats" \
    "$OUTPUT_DIR/fuzzer_stats" \
    "$OUTPUT_DIR/default/fuzzer_stats"
do
    if [ -f "$path" ]; then
        STATS_FILE="$path"
        echo -e "${GREEN}✓${NC} Found fuzzer_stats at: $path"
        break
    fi
done

if [ -z "$STATS_FILE" ]; then
    echo -e "${YELLOW}⚠${NC} fuzzer_stats not found, checking for any output..."
    find "$OUTPUT_DIR" -name "fuzzer_stats" 2>/dev/null || true
else
    # Check edges
    if grep -q "edges_found" "$STATS_FILE"; then
        EDGES=$(grep "edges_found" "$STATS_FILE" | cut -d: -f2 | tr -d ' ')
        if [ "$EDGES" -gt 0 ]; then
            echo -e "${GREEN}✓${NC} AFL++ found $EDGES edges"
        else
            echo -e "${RED}✗${NC} AFL++ found 0 edges"
        fi
    fi
    
    # Check paths
    if grep -q "paths_total" "$STATS_FILE"; then
        PATHS=$(grep "paths_total" "$STATS_FILE" | cut -d: -f2 | tr -d ' ')
        echo -e "${GREEN}✓${NC} AFL++ found $PATHS paths"
    fi
    
    # Check executions
    if grep -q "execs_done" "$STATS_FILE"; then
        EXECS=$(grep "execs_done" "$STATS_FILE" | cut -d: -f2 | tr -d ' ')
        echo -e "${GREEN}✓${NC} AFL++ performed $EXECS executions"
    fi
fi

# Step 5: Check coverage report via API
echo
echo -e "${YELLOW}Step 5: Checking coverage report...${NC}"

COVERAGE_RESPONSE=$(curl -s "$API_URL/jobs/$JOB_ID/coverage")

if echo "$COVERAGE_RESPONSE" | grep -q "coverage"; then
    echo -e "${GREEN}✓${NC} Coverage endpoint returned data"
    
    # Try to parse coverage percentage
    if echo "$COVERAGE_RESPONSE" | grep -q "line_coverage"; then
        LINE_COV=$(echo "$COVERAGE_RESPONSE" | grep -o '"line_coverage":[0-9.]*' | cut -d: -f2)
        if [ ! -z "$LINE_COV" ] && [ "$LINE_COV" != "0" ]; then
            echo -e "${GREEN}✓${NC} Line coverage: ${LINE_COV}%"
        else
            echo -e "${YELLOW}⚠${NC} Line coverage is 0 or not found"
        fi
    fi
    
    # Check for edges in coverage data
    if echo "$COVERAGE_RESPONSE" | grep -q "edges_found"; then
        COV_EDGES=$(echo "$COVERAGE_RESPONSE" | grep -o '"edges_found":[0-9]*' | cut -d: -f2)
        if [ ! -z "$COV_EDGES" ] && [ "$COV_EDGES" -gt 0 ]; then
            echo -e "${GREEN}✓${NC} Coverage report shows $COV_EDGES edges"
        else
            echo -e "${YELLOW}⚠${NC} Coverage report shows 0 edges"
        fi
    fi
    
    # Check for bitmap coverage
    if echo "$COVERAGE_RESPONSE" | grep -q "bitmap_coverage"; then
        BITMAP=$(echo "$COVERAGE_RESPONSE" | grep -o '"bitmap_coverage":"[^"]*' | cut -d'"' -f4)
        if [ ! -z "$BITMAP" ] && [ "$BITMAP" != "0.00%" ]; then
            echo -e "${GREEN}✓${NC} Bitmap coverage: $BITMAP"
        else
            echo -e "${YELLOW}⚠${NC} Bitmap coverage is 0%"
        fi
    fi
else
    echo -e "${YELLOW}⚠${NC} No coverage data returned"
    echo "Response: $COVERAGE_RESPONSE"
fi

# Step 6: Check for zombie processes
echo
echo -e "${YELLOW}Step 6: Checking for zombie processes...${NC}"

ZOMBIES=$(ps aux | grep -c '<defunct>' || true)
if [ "$ZOMBIES" -gt 0 ]; then
    echo -e "${RED}✗${NC} Found $ZOMBIES zombie process(es)"
    ps aux | grep '<defunct>' || true
else
    echo -e "${GREEN}✓${NC} No zombie processes detected"
fi

# Step 7: Summary
echo
echo "=== Test Summary ==="

SUCCESS=true

# Check if edges were found
if [ ! -z "$EDGES" ] && [ "$EDGES" -gt 0 ]; then
    echo -e "${GREEN}✓ AFL++ edge detection: WORKING${NC} ($EDGES edges found)"
else
    echo -e "${RED}✗ AFL++ edge detection: NOT WORKING${NC}"
    SUCCESS=false
fi

# Check if coverage was reported
if [ ! -z "$COV_EDGES" ] && [ "$COV_EDGES" -gt 0 ]; then
    echo -e "${GREEN}✓ Coverage reporting: WORKING${NC} ($COV_EDGES edges in report)"
elif [ ! -z "$BITMAP" ] && [ "$BITMAP" != "0.00%" ]; then
    echo -e "${GREEN}✓ Coverage reporting: WORKING${NC} ($BITMAP bitmap coverage)"
else
    echo -e "${RED}✗ Coverage reporting: NOT WORKING${NC}"
    SUCCESS=false
fi

# Check for zombies
if [ "$ZOMBIES" -eq 0 ]; then
    echo -e "${GREEN}✓ Process management: WORKING${NC} (no zombies)"
else
    echo -e "${RED}✗ Process management: ISSUES DETECTED${NC} ($ZOMBIES zombies)"
    SUCCESS=false
fi

echo
if [ "$SUCCESS" = true ]; then
    echo -e "${GREEN}=== ALL TESTS PASSED ===${NC}"
    echo "The AFL++ coverage fix is working correctly!"
else
    echo -e "${RED}=== SOME TESTS FAILED ===${NC}"
    echo "Please check the output above for details."
fi

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo
echo "Test complete!"