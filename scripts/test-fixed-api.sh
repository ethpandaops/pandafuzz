#!/bin/bash

# Test script to verify the fixed API can enable coverage

set -e

echo "=== Testing Fixed API with Coverage ==="
echo

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# Create a simple test binary
TEST_DIR="/tmp/api-test-$(date +%s)"
mkdir -p "$TEST_DIR"

echo -e "${BLUE}Creating test binary...${NC}"
cat > "$TEST_DIR/test.c" << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main() {
    char buf[10];
    if (read(0, buf, 10) < 1) return 0;
    
    if (buf[0] == 'T') {
        printf("Found T\n");
        if (buf[1] == 'E') {
            printf("Found TE\n");
            if (buf[2] == 'S') {
                printf("Found TES\n");
                if (buf[3] == 'T') {
                    printf("Found TEST!\n");
                    abort();
                }
            }
        }
    }
    return 0;
}
EOF

gcc -o "$TEST_DIR/test_binary" "$TEST_DIR/test.c"
echo -e "${GREEN}✓${NC} Test binary created"

# Copy to master container
docker cp "$TEST_DIR/test_binary" pandafuzz-master:/app/data/binaries/api-test-binary

# Create corpus
mkdir -p "$TEST_DIR/corpus"
echo "test" > "$TEST_DIR/corpus/seed1"
echo "Test" > "$TEST_DIR/corpus/seed2"
echo "TEst" > "$TEST_DIR/corpus/seed3"
docker cp "$TEST_DIR/corpus" pandafuzz-master:/app/data/corpus/api-test-corpus

echo
echo -e "${BLUE}Testing regular JSON POST with coverage enabled...${NC}"

# Test the fixed API - now coverage fields are at the top level
JOB_RESPONSE=$(curl -s -X POST "http://localhost:8080/api/v3/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "API Coverage Test",
    "fuzzer": "afl++",
    "target": "/app/data/binaries/api-test-binary",
    "duration": 45000000000,
    "enable_coverage": true,
    "coverage_format": "lcov",
    "config": {
      "memory_limit": 256,
      "timeout": 1000
    }
  }')

echo "API Response:"
echo "$JOB_RESPONSE" | jq '.' || echo "$JOB_RESPONSE"

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)

if [ -z "$JOB_ID" ]; then
    echo -e "${RED}✗ Failed to create job${NC}"
    exit 1
fi

echo
echo -e "${GREEN}✓ Job created successfully!${NC}"
echo -e "Job ID: ${YELLOW}$JOB_ID${NC}"

# Check if coverage is enabled
COVERAGE_ENABLED=$(echo "$JOB_RESPONSE" | grep -o '"enable_coverage":[^,]*' | cut -d':' -f2)
COVERAGE_FORMAT=$(echo "$JOB_RESPONSE" | grep -o '"coverage_format":"[^"]*' | cut -d'"' -f4)

echo
echo -e "${BLUE}Job Configuration:${NC}"
echo "  Coverage Enabled: $COVERAGE_ENABLED"
echo "  Coverage Format: $COVERAGE_FORMAT"

if [ "$COVERAGE_ENABLED" = "true" ]; then
    echo -e "${GREEN}✓ Coverage is ENABLED!${NC}"
else
    echo -e "${RED}✗ Coverage is NOT enabled${NC}"
fi

# Wait for job to run
echo
echo -e "${YELLOW}Waiting 30 seconds for job to execute...${NC}"
sleep 30

# Check job status
JOB_STATUS=$(curl -s "http://localhost:8080/api/v3/jobs/$JOB_ID" | jq '.')
echo
echo "Job Status:"
echo "$JOB_STATUS" | jq '{id, status, enable_coverage, coverage_format}' || echo "$JOB_STATUS"

# Check for coverage reports
echo
echo -e "${BLUE}Checking for coverage reports...${NC}"
COVERAGE_RESPONSE=$(curl -s "http://localhost:8080/api/v3/jobs/$JOB_ID/coverage")
echo "$COVERAGE_RESPONSE" | jq '.' || echo "$COVERAGE_RESPONSE"

REPORT_COUNT=$(echo "$COVERAGE_RESPONSE" | grep -o '"id"' | wc -l)
if [ "$REPORT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Found $REPORT_COUNT coverage report(s)!${NC}"
    
    # Check if AFL++ found edges
    echo
    echo -e "${BLUE}Checking AFL++ stats...${NC}"
    docker exec pandafuzz-bot-1 bash -c "
        if [ -d /app/work/jobs/job_$JOB_ID/output/afl_output/default ]; then
            echo 'AFL++ stats:'
            grep -E 'edges_found|paths_total|bitmap_cvg' /app/work/jobs/job_$JOB_ID/output/afl_output/default/fuzzer_stats 2>/dev/null || echo 'Stats file not found'
        else
            echo 'AFL++ output directory not found'
        fi
    " || echo "Could not check AFL++ stats"
else
    echo -e "${YELLOW}⚠ No coverage reports found yet${NC}"
fi

# Check debug logs if enabled
echo
echo -e "${BLUE}Checking for debug logs...${NC}"
docker logs pandafuzz-bot-1 2>&1 | grep "$JOB_ID" | grep -i "DEBUG.*coverage" | tail -5 || echo "No debug logs found"

# Cleanup
rm -rf "$TEST_DIR"

echo
echo -e "${BLUE}=== Test Summary ===${NC}"
if [ "$COVERAGE_ENABLED" = "true" ] && [ "$REPORT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ API FIX SUCCESSFUL!${NC}"
    echo "The regular JSON POST endpoint can now enable coverage collection."
else
    echo -e "${YELLOW}⚠ Check results above for issues${NC}"
fi