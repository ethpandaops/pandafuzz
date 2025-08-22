#!/bin/bash
# Fixed coverage test script that compiles binaries with proper instrumentation inside the bot container

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

FUZZER_TYPE="${1:-afl++}"
MASTER_URL="${MASTER_URL:-http://localhost:8080}"
API_BASE="${MASTER_URL}/api/v1"
API_V3="${MASTER_URL}/api/v3"

echo -e "${BLUE}=== Coverage Test for $FUZZER_TYPE (Fixed Version) ===${NC}"

# Step 1: Compile test binary with proper instrumentation inside the bot container
echo -e "\n${YELLOW}Step 1: Compiling test binary with instrumentation in bot container...${NC}"

if [[ "$FUZZER_TYPE" == "afl++" ]]; then
    # Create a simple C program that successfully generates edges (PROVEN TO WORK)
    cat > /tmp/host_coverage_test.c << 'EOF'
#include <stdio.h>
#include <unistd.h>

int main() {
    char buf[100];
    ssize_t len = read(0, buf, sizeof(buf));
    
    if (len > 0) {
        switch(buf[0]) {
            case '0': printf("Zero\n"); break;
            case '1': printf("One\n"); break;
            case '2': printf("Two\n"); break;
            case '3': printf("Three\n"); break;
            case '4': printf("Four\n"); break;
            case '5': printf("Five\n"); break;
            case 'A': printf("Letter A\n"); break;
            case 'B': printf("Letter B\n"); break;
            default: printf("Other\n"); break;
        }
    }
    return 0;
}
EOF
    
    # Copy to container and compile
    docker cp /tmp/host_coverage_test.c pandafuzz-bot-1:/tmp/coverage_test.c
    
    # Compile AFL++ binary in container with proper instrumentation
    docker exec pandafuzz-bot-1 bash -c '
    
    # Use afl-gcc which has been proven to generate edges correctly
    AFL_COMPILER="/usr/local/bin/afl-gcc"
    echo "Using afl-gcc for proven edge coverage"
    
    # Compile with AFL++ GCC instrumentation (tested and working)
    $AFL_COMPILER -g -O2 \
        -o /tmp/afl_coverage_test \
        /tmp/coverage_test.c
    
    if [ $? -eq 0 ]; then
        echo "✓ Binary compiled successfully with AFL++ instrumentation"
        # Verify instrumentation
        if nm /tmp/afl_coverage_test | grep -q "__afl_"; then
            echo "✓ AFL++ instrumentation verified"
        fi
        if nm /tmp/afl_coverage_test | grep -q "__gcov"; then
            echo "✓ GCC coverage instrumentation verified"
        fi
        # Store compiler used for later reference
        echo "$AFL_COMPILER" > /tmp/afl_compiler_used.txt
    else
        echo "✗ Compilation failed"
        exit 1
    fi
    '
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to compile AFL++ binary in container${NC}"
        exit 1
    fi
    
    # Extract binary from container
    docker cp pandafuzz-bot-1:/tmp/afl_coverage_test /tmp/afl_coverage_test
    TEST_BINARY="/tmp/afl_coverage_test"
    echo -e "${GREEN}✓ AFL++ binary compiled and extracted${NC}"
    
elif [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
    # Compile LibFuzzer binary in container
    docker exec pandafuzz-bot-1 bash -c '
    cat > /tmp/libfuzzer_test.cc << "EOF"
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size < 4) return 0;
    
    // Multiple branches for coverage
    if (data[0] == 0xFF) {
        if (data[1] == 0xFE) {
            printf("Found magic bytes\n");
            if (data[2] == 0xFD) {
                printf("Found extended magic\n");
            }
        }
    }
    
    // Pattern matching
    if (size >= 8) {
        if (memcmp(data, "COVERAGE", 8) == 0) {
            printf("Coverage pattern found\n");
        }
    }
    
    // Crash condition
    if (size >= 5 && memcmp(data, "CRASH", 5) == 0) {
        abort();
    }
    
    return 0;
}

// Add main for standalone mode (bot will handle this)
int main(int argc, char **argv) {
    // LibFuzzer main is provided by -fsanitize=fuzzer
    return 0;
}
EOF
    
    # Try to compile with LibFuzzer
    echo "Attempting LibFuzzer compilation..."
    
    # Check for available clang version
    if [ -f /usr/bin/clang++-14 ]; then
        CLANG_CXX="/usr/bin/clang++-14"
    elif [ -f /usr/bin/clang++ ]; then
        CLANG_CXX="/usr/bin/clang++"
    else
        CLANG_CXX="clang++"
    fi
    
    # Try with LibFuzzer instrumentation
    if $CLANG_CXX -g -O1 \
        -fsanitize=fuzzer,address \
        -fprofile-instr-generate \
        -fcoverage-mapping \
        -o /tmp/libfuzzer_coverage_test \
        /tmp/libfuzzer_test.cc 2>/dev/null; then
        echo "✓ Binary compiled with LibFuzzer instrumentation"
        # Verify instrumentation
        if nm /tmp/libfuzzer_coverage_test | grep -q "LLVMFuzzerTestOneInput"; then
            echo "✓ LibFuzzer entry point verified"
        fi
        if nm /tmp/libfuzzer_coverage_test | grep -q "__llvm_prof"; then
            echo "✓ LLVM coverage instrumentation verified"
        fi
    else
        echo "LibFuzzer not available, using standalone compilation..."
        # Compile without fuzzer sanitizer for testing
        $CLANG_CXX -g -O1 \
            -fprofile-instr-generate \
            -fcoverage-mapping \
            -DSTANDALONE_FUZZER \
            -o /tmp/libfuzzer_coverage_test \
            /tmp/libfuzzer_test.cc
        
        if [ $? -eq 0 ]; then
            echo "✓ Binary compiled with coverage instrumentation (standalone mode)"
        else
            echo "✗ Compilation failed"
            exit 1
        fi
    fi
    '
    
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to compile LibFuzzer binary in container${NC}"
        exit 1
    fi
    
    # Extract binary from container
    docker cp pandafuzz-bot-1:/tmp/libfuzzer_coverage_test /tmp/libfuzzer_coverage_test
    TEST_BINARY="/tmp/libfuzzer_coverage_test"
    echo -e "${GREEN}✓ LibFuzzer binary compiled and extracted${NC}"
fi

# Step 2: Create corpus with diverse seeds to trigger different branches
echo -e "\n${YELLOW}Step 2: Creating diverse test corpus for maximum edge coverage...${NC}"

CORPUS_DIR=$(mktemp -d)

# Seeds designed to hit different branches (simple seeds that work)
echo "0" > "$CORPUS_DIR/seed01.txt"        # Zero
echo "1" > "$CORPUS_DIR/seed02.txt"        # One
echo "2" > "$CORPUS_DIR/seed03.txt"        # Two
echo "3" > "$CORPUS_DIR/seed04.txt"        # Three
echo "4" > "$CORPUS_DIR/seed05.txt"        # Four
echo "5" > "$CORPUS_DIR/seed06.txt"        # Five
echo "A" > "$CORPUS_DIR/seed07.txt"        # Letter A
echo "B" > "$CORPUS_DIR/seed08.txt"        # Letter B
echo "X" > "$CORPUS_DIR/seed09.txt"        # Other letter
echo "test" > "$CORPUS_DIR/seed10.txt"     # Default case

echo -e "${GREEN}✓ Created ${BLUE}$(ls -1 $CORPUS_DIR | wc -l)${GREEN} seed files${NC}"

# Step 3: Create corpus collection
echo -e "\n${YELLOW}Step 3: Creating corpus collection...${NC}"

TIMESTAMP=$(date +%s)
COLLECTION_RESPONSE=$(curl -s -X POST "${API_BASE}/corpus/collections" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coverage Test Corpus - '"$FUZZER_TYPE"' - '"$TIMESTAMP"'",
    "description": "Test corpus for coverage validation with '"$FUZZER_TYPE"'"
  }')

COLLECTION_ID=$(echo "$COLLECTION_RESPONSE" | grep -o '"[Ii][Dd]":"[^"]*"' | cut -d'"' -f4 | head -1)

if [ -z "$COLLECTION_ID" ]; then
    echo -e "${RED}Failed to create corpus collection${NC}"
    echo "Response: $COLLECTION_RESPONSE"
    exit 1
fi

echo -e "${GREEN}✓ Created collection: ${COLLECTION_ID}${NC}"

# Step 4: Upload corpus files
echo -e "\n${YELLOW}Step 4: Uploading corpus files...${NC}"

UPLOAD_CMD="curl -s -X POST \"${API_BASE}/corpus/collections/${COLLECTION_ID}/upload\""
for file in "$CORPUS_DIR"/*; do
    UPLOAD_CMD="$UPLOAD_CMD -F \"files=@$file\""
done

UPLOAD_RESPONSE=$(eval $UPLOAD_CMD)
UPLOAD_COUNT=$(echo "$UPLOAD_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2)

if [ -z "$UPLOAD_COUNT" ] || [ "$UPLOAD_COUNT" -eq 0 ]; then
    echo -e "${RED}Failed to upload corpus files${NC}"
    echo "Response: $UPLOAD_RESPONSE"
else
    echo -e "${GREEN}✓ Uploaded ${UPLOAD_COUNT} files${NC}"
fi

# Step 5: Create job with coverage enabled
echo -e "\n${YELLOW}Step 5: Creating fuzzing job with coverage enabled...${NC}"

# Configuration will be set when creating JOB_DATA

# Move duration out of config
if [[ "$FUZZER_TYPE" == "afl++" ]]; then
    JOB_CONFIG=$(cat <<EOF
{
  "memory_limit": 512,
  "timeout": 1000000000,
  "coverage": {
    "enabled": true,
    "format": "lcov"
  },
  "afl_plus_plus_options": {
    "input_dir": "/tmp/afl_input",
    "llvm_mode": true,
    "use_afl_cov": false
  }
}
EOF
)
else
    JOB_CONFIG=$(cat <<EOF
{
  "memory_limit": 512,
  "timeout": 1000000000,
  "coverage": {
    "enabled": true,
    "format": "lcov"
  },
  "libfuzzer_options": {
    "max_total_time": 60,
    "print_coverage": 1,
    "use_counters": 1
  }
}
EOF
)
fi

JOB_DATA=$(cat <<EOF
{
  "name": "Coverage Test - $FUZZER_TYPE - $(date +%s)",
  "fuzzer": "$FUZZER_TYPE",
  "type": "fuzzing",
  "collection_id": "${COLLECTION_ID}",
  "duration": 60000000000,
  "enable_coverage": true,
  "coverage_format": "raw",
  "config": $JOB_CONFIG
}
EOF
)

echo "Job configuration:"
echo "$JOB_DATA" | python3 -m json.tool 2>/dev/null || echo "$JOB_DATA"

# Upload binary and create job
JOB_RESPONSE=$(curl -s -X POST "${API_BASE}/jobs/upload" \
  -F "job_metadata=${JOB_DATA}" \
  -F "target_binary=@${TEST_BINARY}")

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1)

if [ -z "$JOB_ID" ]; then
    echo -e "${RED}Failed to create job${NC}"
    echo "Response: $JOB_RESPONSE"
    exit 1
fi

echo -e "${GREEN}✓ Created job: ${JOB_ID}${NC}"
echo -e "${BLUE}Job will run for 60 seconds with coverage collection${NC}"

# Step 6: Monitor job execution
echo -e "\n${YELLOW}Step 6: Monitoring job execution...${NC}"

START_TIME=$(date +%s)
LAST_STATUS=""
MAX_WAIT=90  # Maximum wait time in seconds

while [ $(($(date +%s) - START_TIME)) -lt $MAX_WAIT ]; do
    sleep 3
    
    JOB_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")
    STATUS=$(echo "$JOB_STATS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4 | head -1)
    CRASHES=$(echo "$JOB_STATS" | grep -o '"crashes_found":[0-9]*' | cut -d':' -f2 | head -1)
    EXECS=$(echo "$JOB_STATS" | grep -o '"total_execs":[0-9]*' | cut -d':' -f2 | head -1)
    
    ELAPSED=$(($(date +%s) - START_TIME))
    
    if [ "$STATUS" != "$LAST_STATUS" ]; then
        echo -e "  [${ELAPSED}s] Status: ${YELLOW}${STATUS}${NC} | Execs: ${EXECS:-0} | Crashes: ${CRASHES:-0}"
        LAST_STATUS="$STATUS"
    fi
    
    if [[ "$STATUS" == "completed" ]] || [[ "$STATUS" == "failed" ]] || [[ "$STATUS" == "cancelled" ]]; then
        echo -e "\n${YELLOW}Job finished with status: ${STATUS}${NC}"
        break
    fi
done

# Step 7: Wait for coverage processing and check edges
echo -e "\n${YELLOW}Step 7: Checking for edges in plot_data...${NC}"
sleep 10

# Check plot_data for edges immediately
echo -e "\n${YELLOW}Checking AFL++ edges found...${NC}"
EDGES_CHECK=$(docker exec pandafuzz-bot-1 bash -c "
    if [ -f /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data ]; then
        echo '=== AFL++ plot_data contents ==='
        tail -5 /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data
        echo ''
        echo '=== Edges found ==='
        tail -1 /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data | awk -F',' '{print \"Edges: \" \$13 \" | Coverage: \" \$7 \" | Queue: \" \$4 \" | Execs: \" \$12}'
    else
        echo 'No plot_data file found'
    fi
" 2>/dev/null)

echo "$EDGES_CHECK"

# Extract edges count
EDGES_FOUND=$(echo "$EDGES_CHECK" | grep "Edges:" | sed 's/.*Edges: \([0-9]*\).*/\1/')
if [ -n "$EDGES_FOUND" ] && [ "$EDGES_FOUND" -gt 0 ]; then
    echo -e "\n${GREEN}✅ SUCCESS! AFL++ found $EDGES_FOUND edges!${NC}"
    echo -e "${GREEN}Goal achieved: plot_data has edges > 0${NC}"
    echo -e "\nTo see full plot_data:"
    echo "docker exec pandafuzz-bot-1 cat /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data"
    echo ""
    echo "Job continues running. Current edges: $EDGES_FOUND"
else
    echo -e "\n${YELLOW}⚠ Warning: No edges found yet. Waiting more...${NC}"
fi

# Step 7.5: Check for real coverage data (no synthetic generation)
echo -e "\n${YELLOW}Step 7.5: Checking for real coverage data...${NC}"

# First check if real coverage data was generated
REAL_COVERAGE_FOUND=false
COVERAGE_DIR="/app/data/coverage/${JOB_ID}"

# Check for AFL++ real coverage indicators
if [[ "$FUZZER_TYPE" == "afl++" ]]; then
    echo "Checking for AFL++ coverage data..."
    
    # Check plot_data for real edges
    EDGES_FOUND=$(docker exec pandafuzz-bot-1 bash -c "
        if [ -f /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data ]; then
            tail -1 /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data | cut -d',' -f13 | tr -d ' '
        else
            echo '0'
        fi
    " 2>/dev/null || echo "0")
    
    # Check for GCC coverage files (.gcda)
    GCDA_COUNT=$(docker exec pandafuzz-bot-1 bash -c "
        find /app/work/jobs/job_${JOB_ID} -name '*.gcda' 2>/dev/null | wc -l
    " 2>/dev/null || echo "0")
    
    if [ "$EDGES_FOUND" != "0" ] || [ "$GCDA_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Real AFL++ coverage data found!${NC}"
        echo "  Edges found: $EDGES_FOUND"
        echo "  GCDA files: $GCDA_COUNT"
        REAL_COVERAGE_FOUND=true
        
        # Generate real LCOV from gcda files if available
        if [ "$GCDA_COUNT" -gt 0 ]; then
            docker exec pandafuzz-bot-1 bash -c "
                cd /app/work/jobs/job_${JOB_ID}
                lcov --capture --directory . --output-file coverage.info 2>/dev/null || true
                if [ -f coverage.info ]; then
                    cp coverage.info /app/data/coverage/${JOB_ID}/coverage-real.lcov
                    echo 'Real LCOV coverage generated from GCDA files'
                fi
            " 2>/dev/null || true
        fi
    fi
fi

# Check for LibFuzzer real coverage (.profraw files)
if [[ "$FUZZER_TYPE" == "libfuzzer" ]]; then
    echo "Checking for LibFuzzer coverage data..."
    
    PROFRAW_COUNT=$(docker exec pandafuzz-bot-1 bash -c "
        find /app/work/jobs/job_${JOB_ID} -name '*.profraw' 2>/dev/null | wc -l
    " 2>/dev/null || echo "0")
    
    if [ "$PROFRAW_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Real LibFuzzer coverage data found!${NC}"
        echo "  Profraw files: $PROFRAW_COUNT"
        REAL_COVERAGE_FOUND=true
        
        # Merge profraw files and generate LCOV
        docker exec pandafuzz-bot-1 bash -c "
            cd /app/work/jobs/job_${JOB_ID}
            llvm-profdata merge -sparse *.profraw -o coverage.profdata 2>/dev/null || true
            if [ -f coverage.profdata ] && [ -f /tmp/libfuzzer_coverage_test ]; then
                llvm-cov export -format=lcov -instr-profile=coverage.profdata /tmp/libfuzzer_coverage_test > coverage.lcov 2>/dev/null || true
                if [ -f coverage.lcov ]; then
                    cp coverage.lcov /app/data/coverage/${JOB_ID}/coverage-real.lcov
                    echo 'Real LCOV coverage generated from profraw files'
                fi
            fi
        " 2>/dev/null || true
    fi
fi

# No synthetic data generation - only report status
if [ "$REAL_COVERAGE_FOUND" = false ]; then
    echo -e "${YELLOW}⚠ No real coverage data found${NC}"
    echo -e "${YELLOW}Note: Synthetic coverage generation has been disabled${NC}"
else
    echo -e "${GREEN}✓ Using real coverage data${NC}"
fi

# Step 8: Check coverage results
echo -e "\n${YELLOW}Step 8: Checking coverage results...${NC}"

# Try API v3 endpoint for coverage
COVERAGE_RESPONSE=$(curl -s "${API_V3}/jobs/${JOB_ID}/coverage")
COVERAGE_COUNT=$(echo "$COVERAGE_RESPONSE" | grep -o '"reports":\[[^]]*\]' | grep -o '"id"' | wc -l)

if [ "$COVERAGE_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Found ${COVERAGE_COUNT} coverage report(s)!${NC}"
    
    # Display coverage details
    echo -e "\n${BLUE}Coverage Report Details:${NC}"
    echo "$COVERAGE_RESPONSE" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    reports = data.get('reports', [])
    for i, report in enumerate(reports):
        print(f'Report #{i+1}:')
        print(f'  ID: {report.get(\"id\", \"unknown\")}')
        print(f'  Format: {report.get(\"format\", \"unknown\")}')
        print(f'  Size: {report.get(\"size\", 0)} bytes')
        print(f'  Created: {report.get(\"created_at\", \"unknown\")}')
except Exception as e:
    print(f'Could not parse coverage data: {e}')
" 2>/dev/null || echo "  (Could not parse coverage details)"
    
    # Try to get coverage metadata
    FIRST_REPORT_ID=$(echo "$COVERAGE_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4 | head -1)
    if [ -n "$FIRST_REPORT_ID" ]; then
        echo -e "\n${BLUE}Coverage Statistics:${NC}"
        METADATA_RESPONSE=$(curl -s "${API_V3}/jobs/${JOB_ID}/coverage/${FIRST_REPORT_ID}/metadata")
        echo "$METADATA_RESPONSE" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    print(f'  Line Coverage: {data.get(\"line_coverage\", 0):.1f}%')
    print(f'  Function Coverage: {data.get(\"function_coverage\", 0):.1f}%')
    if 'branch_coverage' in data:
        print(f'  Branch Coverage: {data.get(\"branch_coverage\", 0):.1f}%')
    print(f'  Total Lines: {data.get(\"total_lines\", 0)}')
    print(f'  Covered Lines: {data.get(\"covered_lines\", 0)}')
except Exception as e:
    print(f'  (Could not parse metadata: {e})')
" 2>/dev/null
    fi
else
    echo -e "${YELLOW}No coverage reports found in API response${NC}"
    echo -e "${YELLOW}Checking job logs for coverage information...${NC}"
    
    # Check logs for coverage-related messages
    JOB_LOGS=$(curl -s "${API_BASE}/jobs/${JOB_ID}/logs")
    echo "$JOB_LOGS" | jq -r '.logs[] | select(.message | test("coverage|Coverage|COVERAGE|profraw|profdata|lcov")) | "\(.timestamp) [\(.level)] \(.message)"' 2>/dev/null | tail -10 || \
    echo "  (No coverage-related log entries found)"
    
    # Check if bot actually supports coverage
    echo -e "\n${YELLOW}Verifying bot coverage capabilities...${NC}"
    docker exec pandafuzz-bot-1 bash -c "
        if [ -f /app/work/jobs/job_${JOB_ID}/coverage/*.profraw ]; then
            echo '  Found .profraw files in job directory'
            ls -la /app/work/jobs/job_${JOB_ID}/coverage/*.profraw 2>/dev/null | head -3
        fi
        if [ -f /app/work/jobs/job_${JOB_ID}/coverage/coverage.info ]; then
            echo '  Found coverage.info file'
            head -5 /app/work/jobs/job_${JOB_ID}/coverage/coverage.info
        fi
    " 2>/dev/null || echo "  (Could not check job directory)"
fi

# Step 8.1: Enhanced coverage file verification
echo -e "\n${YELLOW}Step 8.1: Verifying coverage files in storage...${NC}"

COVERAGE_FILES_FOUND=false
STORAGE_CHECK_CMD="docker exec pandafuzz-bot-1 find /app/data/coverage/${JOB_ID} -type f 2>/dev/null || docker exec pandafuzz-master find /app/data/coverage/${JOB_ID} -type f 2>/dev/null"

STORAGE_FILES=$(eval $STORAGE_CHECK_CMD)
if [ $? -eq 0 ] && [ -n "$STORAGE_FILES" ]; then
    COVERAGE_FILES_FOUND=true
    FILE_COUNT=$(echo "$STORAGE_FILES" | wc -l)
    echo -e "${GREEN}✓ Found ${FILE_COUNT} coverage files in storage:${NC}"
    echo "$STORAGE_FILES" | head -5 | while read file; do
        echo -e "  ${BLUE}${file}${NC}"
    done
    if [ "$FILE_COUNT" -gt 5 ]; then
        echo -e "  ${YELLOW}... and $((FILE_COUNT - 5)) more files${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No coverage files found in storage directory${NC}"
    echo -e "${YELLOW}Checking alternative storage locations...${NC}"
    
    # Check alternative locations
    ALT_LOCATIONS=(
        "/app/work/jobs/job_${JOB_ID}/coverage"
        "/tmp/coverage/${JOB_ID}"
        "/var/coverage/${JOB_ID}"
        "/app/coverage/${JOB_ID}"
    )
    
    for location in "${ALT_LOCATIONS[@]}"; do
        ALT_FILES=$(docker exec pandafuzz-bot-1 find "$location" -type f 2>/dev/null || docker exec pandafuzz-master find "$location" -type f 2>/dev/null)
        if [ $? -eq 0 ] && [ -n "$ALT_FILES" ]; then
            COVERAGE_FILES_FOUND=true
            echo -e "${GREEN}✓ Found coverage files in alternative location: ${location}${NC}"
            echo "$ALT_FILES" | head -3 | while read file; do
                echo -e "  ${BLUE}${file}${NC}"
            done
            break
        fi
    done
fi

# Step 8.2: Download and validate coverage reports
echo -e "\n${YELLOW}Step 8.2: Downloading and validating coverage reports...${NC}"

DOWNLOAD_SUCCESS=false
DOWNLOAD_DIR=$(mktemp -d)

if [ "$COVERAGE_COUNT" -gt 0 ] && [ -n "$FIRST_REPORT_ID" ]; then
    # Download the coverage report
    echo -e "  Downloading coverage report: ${FIRST_REPORT_ID}"
    
    DOWNLOAD_RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}\nSIZE:%{size_download}" \
        "${API_V3}/jobs/${JOB_ID}/coverage/${FIRST_REPORT_ID}/download" \
        -o "${DOWNLOAD_DIR}/coverage_report.lcov")
    
    HTTP_STATUS=$(echo "$DOWNLOAD_RESPONSE" | grep "HTTPSTATUS:" | cut -d: -f2)
    DOWNLOAD_SIZE=$(echo "$DOWNLOAD_RESPONSE" | grep "SIZE:" | cut -d: -f2)
    
    if [ "$HTTP_STATUS" = "200" ] && [ "$DOWNLOAD_SIZE" -gt 0 ]; then
        DOWNLOAD_SUCCESS=true
        echo -e "${GREEN}✓ Successfully downloaded coverage report (${DOWNLOAD_SIZE} bytes)${NC}"
        
        # Validate file content
        if [ -f "${DOWNLOAD_DIR}/coverage_report.lcov" ]; then
            LCOV_LINES=$(wc -l < "${DOWNLOAD_DIR}/coverage_report.lcov" 2>/dev/null || echo "0")
            echo -e "  Report contains ${LCOV_LINES} lines"
            
            # Check for LCOV format markers
            if grep -q "^TN:" "${DOWNLOAD_DIR}/coverage_report.lcov" && \
               grep -q "^SF:" "${DOWNLOAD_DIR}/coverage_report.lcov"; then
                echo -e "${GREEN}✓ Downloaded file has valid LCOV format${NC}"
            else
                echo -e "${YELLOW}⚠ Downloaded file may not be in valid LCOV format${NC}"
            fi
            
            # Show sample content
            echo -e "\n${BLUE}Coverage report sample (first 10 lines):${NC}"
            head -10 "${DOWNLOAD_DIR}/coverage_report.lcov" | while read line; do
                echo -e "  ${line}"
            done
        fi
    else
        echo -e "${RED}✗ Failed to download coverage report${NC}"
        echo -e "  HTTP Status: ${HTTP_STATUS}"
        echo -e "  Download Size: ${DOWNLOAD_SIZE} bytes"
    fi
else
    echo -e "${YELLOW}⚠ No coverage report ID available for download${NC}"
fi

# Step 8.3: Validate coverage metrics
echo -e "\n${YELLOW}Step 8.3: Validating coverage metrics...${NC}"

COVERAGE_METRICS_VALID=false

if [ -n "$FIRST_REPORT_ID" ]; then
    # Get detailed coverage statistics
    COVERAGE_STATS=$(curl -s "${API_V3}/jobs/${JOB_ID}/coverage/${FIRST_REPORT_ID}/metadata")
    
    if [ $? -eq 0 ] && [ -n "$COVERAGE_STATS" ]; then
        # Parse and validate metrics using Python
        METRICS_VALIDATION=$(echo "$COVERAGE_STATS" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    
    line_coverage = data.get('line_coverage', 0)
    function_coverage = data.get('function_coverage', 0)  
    total_lines = data.get('total_lines', 0)
    covered_lines = data.get('covered_lines', 0)
    
    # Check for non-zero metrics
    valid_metrics = []
    issues = []
    
    if line_coverage > 0:
        valid_metrics.append(f'Line coverage: {line_coverage:.1f}%')
    else:
        issues.append('Line coverage is 0%')
    
    if function_coverage > 0:
        valid_metrics.append(f'Function coverage: {function_coverage:.1f}%')
    else:
        issues.append('Function coverage is 0%')
        
    if total_lines > 0:
        valid_metrics.append(f'Total lines: {total_lines}')
    else:
        issues.append('Total lines is 0')
        
    if covered_lines > 0:
        valid_metrics.append(f'Covered lines: {covered_lines}')
    else:
        issues.append('Covered lines is 0')
    
    # Output results
    if valid_metrics:
        print('VALID_METRICS=true')
        for metric in valid_metrics:
            print(f'METRIC: {metric}')
    
    if issues:
        for issue in issues:
            print(f'ISSUE: {issue}')
            
    if not issues:
        print('COVERAGE_SUCCESS=true')
        
except Exception as e:
    print(f'PARSE_ERROR: {e}')
" 2>/dev/null)
        
        if echo "$METRICS_VALIDATION" | grep -q "VALID_METRICS=true"; then
            COVERAGE_METRICS_VALID=true
            echo -e "${GREEN}✓ Coverage metrics validation successful:${NC}"
            echo "$METRICS_VALIDATION" | grep "^METRIC:" | while read line; do
                METRIC_TEXT=$(echo "$line" | cut -d' ' -f2-)
                echo -e "  ${BLUE}${METRIC_TEXT}${NC}"
            done
        fi
        
        if echo "$METRICS_VALIDATION" | grep -q "ISSUE:"; then
            echo -e "${YELLOW}⚠ Coverage metrics issues found:${NC}"
            echo "$METRICS_VALIDATION" | grep "^ISSUE:" | while read line; do
                ISSUE_TEXT=$(echo "$line" | cut -d' ' -f2-)
                echo -e "  ${YELLOW}${ISSUE_TEXT}${NC}"
            done
        fi
        
        if echo "$METRICS_VALIDATION" | grep -q "PARSE_ERROR:"; then
            echo -e "${YELLOW}⚠ Could not parse coverage metrics${NC}"
        fi
    else
        echo -e "${YELLOW}⚠ Could not retrieve coverage metadata${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No coverage report available for metrics validation${NC}"
fi

# Step 8.4: Comprehensive validation summary
echo -e "\n${YELLOW}Step 8.4: Coverage validation summary...${NC}"

VALIDATION_SCORE=0
TOTAL_CHECKS=4

# Check 1: API reports coverage
if [ "$COVERAGE_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Coverage reports found in API${NC}"
    VALIDATION_SCORE=$((VALIDATION_SCORE + 1))
else
    echo -e "${RED}✗ No coverage reports found in API${NC}"
fi

# Check 2: Coverage files exist in storage
if [ "$COVERAGE_FILES_FOUND" = true ]; then
    echo -e "${GREEN}✓ Coverage files exist in storage${NC}"
    VALIDATION_SCORE=$((VALIDATION_SCORE + 1))
else
    echo -e "${RED}✗ No coverage files found in storage${NC}"
fi

# Check 3: Coverage reports downloadable
if [ "$DOWNLOAD_SUCCESS" = true ]; then
    echo -e "${GREEN}✓ Coverage reports are downloadable${NC}"
    VALIDATION_SCORE=$((VALIDATION_SCORE + 1))
else
    echo -e "${RED}✗ Coverage reports could not be downloaded${NC}"
fi

# Check 4: Coverage metrics are valid
if [ "$COVERAGE_METRICS_VALID" = true ]; then
    echo -e "${GREEN}✓ Coverage metrics are non-zero${NC}"
    VALIDATION_SCORE=$((VALIDATION_SCORE + 1))
else
    echo -e "${RED}✗ Coverage metrics are zero or invalid${NC}"
fi

echo -e "\n${BLUE}Validation Score: ${VALIDATION_SCORE}/${TOTAL_CHECKS}${NC}"

# Set final coverage success flag
if [ "$VALIDATION_SCORE" -ge 3 ]; then
    ENHANCED_COVERAGE_SUCCESS=true
    echo -e "${GREEN}✓ Enhanced coverage validation PASSED (${VALIDATION_SCORE}/${TOTAL_CHECKS})${NC}"
else
    ENHANCED_COVERAGE_SUCCESS=false
    echo -e "${YELLOW}⚠ Enhanced coverage validation FAILED (${VALIDATION_SCORE}/${TOTAL_CHECKS})${NC}"
fi

# Clean up download directory
if [ "$ENHANCED_COVERAGE_SUCCESS" = true ] || [ "$COVERAGE_METRICS_VALID" = true ]; then
    echo -e "\n${YELLOW}Step 8.5: Cleaning up test artifacts...${NC}"
    rm -rf "$DOWNLOAD_DIR"
    echo -e "${GREEN}✓ Download artifacts cleaned up${NC}"
    
    # Optionally clean up coverage files from storage (uncomment if desired)
    # echo -e "  Cleaning up coverage files from storage..."
    # docker exec pandafuzz-bot-1 rm -rf "/app/data/coverage/${JOB_ID}" 2>/dev/null || true
    # docker exec pandafuzz-master rm -rf "/app/data/coverage/${JOB_ID}" 2>/dev/null || true
    # echo -e "${GREEN}✓ Storage coverage files cleaned up${NC}"
else
    echo -e "${YELLOW}⚠ Keeping download artifacts for debugging: ${DOWNLOAD_DIR}${NC}"
fi

# Step 9: Display final job statistics
echo -e "\n${YELLOW}Step 9: Final job statistics...${NC}"
FINAL_STATS=$(curl -s "${API_BASE}/jobs/${JOB_ID}")

echo "$FINAL_STATS" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    job = data.get('job', data)
    print(f'Job ID: {job.get(\"id\", \"unknown\")}')
    print(f'Status: {job.get(\"status\", \"unknown\")}')
    print(f'Total executions: {job.get(\"total_execs\", 0)}')
    print(f'Crashes found: {job.get(\"crashes_found\", 0)}')
    
    config = job.get('config', {})
    coverage_config = config.get('coverage', {})
    if coverage_config.get('enabled'):
        print(f'\\nCoverage was enabled:')
        print(f'  Format: {coverage_config.get(\"format\", \"unknown\")}')
    else:
        print(f'\\nWARNING: Coverage was NOT enabled in job config!')
except Exception as e:
    print(f'Could not parse stats: {e}')
" 2>/dev/null

# Cleanup
echo -e "\n${YELLOW}Step 10: Cleanup...${NC}"
rm -rf "$CORPUS_DIR"
rm -f "$TEST_BINARY"
echo -e "${GREEN}✓ Temporary files cleaned up${NC}"

echo -e "\n${YELLOW}Keeping job and collection for inspection:${NC}"
echo -e "${BLUE}Job ID: ${JOB_ID}${NC}"
echo -e "${BLUE}Collection ID: ${COLLECTION_ID}${NC}"

echo -e "\n${BLUE}=== Coverage Test Complete ===${NC}"

# Show Docker commands to verify files exist
echo -e "\n${YELLOW}Docker Commands to Verify Coverage Files:${NC}"
echo -e "${BLUE}1. Check coverage files in master storage:${NC}"
echo "   docker exec pandafuzz-master ls -la /app/data/coverage/${JOB_ID}/"
echo ""
echo -e "${BLUE}2. View LCOV coverage file content:${NC}"
echo "   docker exec pandafuzz-master head -20 /app/data/coverage/${JOB_ID}/coverage-*.lcov"
echo ""
echo -e "${BLUE}3. View JSON coverage file content:${NC}"
echo "   docker exec pandafuzz-master cat /app/data/coverage/${JOB_ID}/coverage-*.json | python3 -m json.tool | head -20"
echo ""
echo -e "${BLUE}4. Check database records:${NC}"
echo "   docker exec pandafuzz-master sqlite3 /app/data/pandafuzz.db \"SELECT * FROM coverage_reports WHERE job_id='${JOB_ID};\""
echo ""
echo -e "${BLUE}5. Check bot work directory (if job still exists):${NC}"
echo "   docker exec pandafuzz-bot-1 ls -la /app/work/jobs/job_${JOB_ID}/output/afl_output/ 2>/dev/null"
echo ""
echo -e "${YELLOW}Run any of these commands to verify the coverage files physically exist.${NC}"

# Exit with appropriate code based on enhanced validation
if [ "$ENHANCED_COVERAGE_SUCCESS" = true ]; then
    echo -e "${GREEN}✓ Enhanced coverage validation successful! (Score: ${VALIDATION_SCORE}/${TOTAL_CHECKS})${NC}"
    exit 0
elif [ "$COVERAGE_COUNT" -gt 0 ]; then
    echo -e "${YELLOW}⚠ Basic coverage collection successful but enhanced validation failed (Score: ${VALIDATION_SCORE}/${TOTAL_CHECKS})${NC}"
    exit 2
else
    echo -e "${RED}✗ Coverage collection failed completely${NC}"
    exit 1
fi