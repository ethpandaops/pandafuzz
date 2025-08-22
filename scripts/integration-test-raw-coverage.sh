#!/bin/bash

# Integration test for raw AFL++ coverage file functionality
# This script tests the complete flow from job creation to raw file download

set -e

echo "Raw AFL++ Coverage Integration Test"
echo "===================================="
echo ""

# Configuration
MASTER_URL="${MASTER_URL:-http://localhost:8080}"
API_BASE="${MASTER_URL}/api/v1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Configuration:"
echo "  Master URL: ${MASTER_URL}"
echo ""

# Function to check if services are running
check_services() {
    echo "Checking services..."
    
    # Check master
    if curl -s "${MASTER_URL}/health" > /dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} Master is running"
    else
        echo -e "  ${RED}✗${NC} Master is not running"
        echo "    Please start the master with: make run-master"
        exit 1
    fi
    
    # Check if any bots are registered
    BOT_COUNT=$(curl -s "${API_BASE}/bots" | grep -o '"id"' | wc -l)
    if [ "$BOT_COUNT" -gt 0 ]; then
        echo -e "  ${GREEN}✓${NC} Found ${BOT_COUNT} registered bot(s)"
    else
        echo -e "  ${YELLOW}⚠${NC} No bots registered"
        echo "    Please start a bot with: make run-bot"
    fi
    
    echo ""
}

# Function to create an AFL++ job with coverage
create_afl_job() {
    echo "Creating AFL++ job with coverage enabled..."
    
    JOB_DATA=$(cat <<EOF
{
    "name": "raw-coverage-test-$(date +%s)",
    "fuzzer": "aflplusplus",
    "target": "test_binary",
    "duration": 30,
    "enable_coverage": true,
    "coverage_format": "raw",
    "config": {
        "timeout": 1000,
        "memory_limit": 256,
        "duration": 30
    }
}
EOF
)
    
    RESPONSE=$(curl -s -X POST "${API_BASE}/jobs" \
        -H "Content-Type: application/json" \
        -d "$JOB_DATA")
    
    JOB_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | sed 's/"id":"//' | head -1)
    
    if [ -z "$JOB_ID" ]; then
        echo -e "  ${RED}✗${NC} Failed to create job"
        echo "    Response: $RESPONSE"
        exit 1
    fi
    
    echo -e "  ${GREEN}✓${NC} Created job: ${JOB_ID}"
    echo "$JOB_ID"
}

# Function to wait for job completion
wait_for_job() {
    local job_id=$1
    local max_wait=60
    local elapsed=0
    
    echo "Waiting for job to complete (max ${max_wait}s)..."
    
    while [ $elapsed -lt $max_wait ]; do
        STATUS=$(curl -s "${API_BASE}/jobs/${job_id}" | grep -o '"status":"[^"]*' | sed 's/"status":"//' | head -1)
        
        if [ "$STATUS" = "completed" ] || [ "$STATUS" = "finished" ]; then
            echo -e "  ${GREEN}✓${NC} Job completed"
            return 0
        elif [ "$STATUS" = "failed" ] || [ "$STATUS" = "error" ]; then
            echo -e "  ${RED}✗${NC} Job failed with status: ${STATUS}"
            return 1
        fi
        
        printf "  Status: %-12s (${elapsed}s elapsed)\r" "$STATUS"
        sleep 2
        elapsed=$((elapsed + 2))
    done
    
    echo -e "\n  ${YELLOW}⚠${NC} Job did not complete within ${max_wait} seconds"
    return 1
}

# Function to check raw coverage files
check_raw_coverage() {
    local job_id=$1
    
    echo "Checking for raw coverage files..."
    
    RESPONSE=$(curl -s "${API_BASE}/jobs/${job_id}/coverage/raw")
    
    if echo "$RESPONSE" | grep -q '"files"'; then
        FILE_COUNT=$(echo "$RESPONSE" | grep -o '"fuzzer_stats_path"\|"plot_data_path"\|"fuzz_bitmap_path"' | wc -l)
        
        if [ "$FILE_COUNT" -gt 0 ]; then
            echo -e "  ${GREEN}✓${NC} Found raw coverage files"
            
            # Check each file type
            if echo "$RESPONSE" | grep -q '"fuzzer_stats_path"'; then
                echo -e "    ${GREEN}✓${NC} fuzzer_stats available"
            fi
            if echo "$RESPONSE" | grep -q '"plot_data_path"'; then
                echo -e "    ${GREEN}✓${NC} plot_data available"
            fi
            if echo "$RESPONSE" | grep -q '"fuzz_bitmap_path"'; then
                echo -e "    ${GREEN}✓${NC} fuzz_bitmap available"
            fi
            
            return 0
        fi
    fi
    
    echo -e "  ${YELLOW}⚠${NC} No raw coverage files found yet"
    return 1
}

# Function to download and verify files
download_files() {
    local job_id=$1
    local download_dir="/tmp/pandafuzz-test-${job_id}"
    
    echo "Downloading raw coverage files..."
    mkdir -p "$download_dir"
    
    # Download individual files
    for file_type in fuzzer_stats plot_data fuzz_bitmap; do
        echo -n "  Downloading ${file_type}... "
        
        if curl -s -o "${download_dir}/${file_type}" \
            "${API_BASE}/jobs/${job_id}/coverage/raw/${file_type}"; then
            
            SIZE=$(stat -f%z "${download_dir}/${file_type}" 2>/dev/null || \
                   stat -c%s "${download_dir}/${file_type}" 2>/dev/null || echo "0")
            
            if [ "$SIZE" -gt 0 ]; then
                echo -e "${GREEN}✓${NC} (${SIZE} bytes)"
            else
                echo -e "${YELLOW}⚠${NC} (empty file)"
            fi
        else
            echo -e "${RED}✗${NC}"
        fi
    done
    
    # Download ZIP archive
    echo -n "  Downloading ZIP archive... "
    if curl -s -o "${download_dir}/coverage.zip" \
        "${API_BASE}/jobs/${job_id}/coverage/raw/all/zip"; then
        
        SIZE=$(stat -f%z "${download_dir}/coverage.zip" 2>/dev/null || \
               stat -c%s "${download_dir}/coverage.zip" 2>/dev/null || echo "0")
        
        if [ "$SIZE" -gt 0 ]; then
            echo -e "${GREEN}✓${NC} (${SIZE} bytes)"
            
            # Verify ZIP contents
            echo "  Verifying ZIP contents:"
            unzip -l "${download_dir}/coverage.zip" 2>/dev/null | \
                grep -E "fuzzer_stats|plot_data|fuzz_bitmap" | \
                sed 's/^/    /'
        else
            echo -e "${YELLOW}⚠${NC} (empty file)"
        fi
    else
        echo -e "${RED}✗${NC}"
    fi
    
    echo ""
    echo "  Files downloaded to: ${download_dir}"
}

# Main test flow
main() {
    echo "Starting integration test..."
    echo ""
    
    # Step 1: Check services
    check_services
    
    # Step 2: Create AFL++ job
    JOB_ID=$(create_afl_job)
    echo ""
    
    # Step 3: Wait for job to complete
    if wait_for_job "$JOB_ID"; then
        echo ""
        
        # Step 4: Check for raw coverage files
        if check_raw_coverage "$JOB_ID"; then
            echo ""
            
            # Step 5: Download and verify files
            download_files "$JOB_ID"
            
            echo ""
            echo -e "${GREEN}Integration test completed successfully!${NC}"
            echo ""
            echo "Summary:"
            echo "  - Job ID: ${JOB_ID}"
            echo "  - Raw coverage files were successfully collected"
            echo "  - Files can be downloaded via API"
            echo ""
            echo "To view in UI:"
            echo "  Open: ${MASTER_URL}/jobs/${JOB_ID}"
            echo "  Navigate to: Coverage Reports tab"
        else
            echo ""
            echo -e "${YELLOW}Warning: Job completed but no raw coverage files found${NC}"
            echo "This may happen if the fuzzer didn't generate coverage data yet."
        fi
    else
        echo ""
        echo -e "${RED}Integration test failed${NC}"
        exit 1
    fi
}

# Run the test
main