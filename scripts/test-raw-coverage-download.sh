#!/bin/bash

# Script to test raw AFL++ coverage file download functionality
# This script creates an AFL++ job, waits for completion, and downloads raw coverage files

set -e

MASTER_URL="${MASTER_URL:-http://localhost:8080}"
API_BASE="${MASTER_URL}/api/v1"

echo "Testing Raw AFL++ Coverage File Downloads"
echo "=========================================="
echo "Master URL: ${MASTER_URL}"
echo ""

# Check if master is running
echo "Checking master health..."
if ! curl -s "${MASTER_URL}/health" > /dev/null; then
    echo "Error: Master is not running at ${MASTER_URL}"
    exit 1
fi
echo "✓ Master is healthy"
echo ""

# Create a test AFL++ job with coverage enabled
echo "Creating AFL++ job with coverage enabled..."
JOB_RESPONSE=$(curl -s -X POST "${API_BASE}/jobs" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "afl-raw-coverage-test-'$(date +%s)'",
        "fuzzer": "aflplusplus",
        "target": "test_binary",
        "duration": 60,
        "enable_coverage": true,
        "coverage_format": "raw",
        "config": {
            "timeout": 1000,
            "memory_limit": 256,
            "duration": 60
        }
    }')

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | sed 's/"id":"//')

if [ -z "$JOB_ID" ]; then
    echo "Error: Failed to create job"
    echo "Response: $JOB_RESPONSE"
    exit 1
fi

echo "✓ Created job: ${JOB_ID}"
echo ""

# Wait for job to complete
echo "Waiting for job to complete (max 2 minutes)..."
MAX_WAIT=120
WAIT_TIME=0
while [ $WAIT_TIME -lt $MAX_WAIT ]; do
    JOB_STATUS=$(curl -s "${API_BASE}/jobs/${JOB_ID}" | grep -o '"status":"[^"]*' | sed 's/"status":"//')
    
    if [ "$JOB_STATUS" = "completed" ] || [ "$JOB_STATUS" = "finished" ]; then
        echo "✓ Job completed with status: ${JOB_STATUS}"
        break
    elif [ "$JOB_STATUS" = "failed" ] || [ "$JOB_STATUS" = "error" ]; then
        echo "✗ Job failed with status: ${JOB_STATUS}"
        exit 1
    fi
    
    echo "  Status: ${JOB_STATUS} (waiting ${WAIT_TIME}s...)"
    sleep 5
    WAIT_TIME=$((WAIT_TIME + 5))
done

if [ $WAIT_TIME -ge $MAX_WAIT ]; then
    echo "✗ Job did not complete within ${MAX_WAIT} seconds"
    exit 1
fi
echo ""

# Check for raw coverage files
echo "Checking for raw coverage files..."
COVERAGE_RESPONSE=$(curl -s "${API_BASE}/jobs/${JOB_ID}/coverage/raw")

if echo "$COVERAGE_RESPONSE" | grep -q "files"; then
    echo "✓ Raw coverage files found"
    echo ""
    
    # Download individual files
    echo "Downloading individual raw coverage files..."
    
    # Create download directory
    DOWNLOAD_DIR="/tmp/pandafuzz-coverage-${JOB_ID}"
    mkdir -p "$DOWNLOAD_DIR"
    
    # Download fuzzer_stats
    echo -n "  Downloading fuzzer_stats... "
    if curl -s -o "${DOWNLOAD_DIR}/fuzzer_stats.txt" \
        "${API_BASE}/jobs/${JOB_ID}/coverage/raw/fuzzer_stats"; then
        SIZE=$(stat -f%z "${DOWNLOAD_DIR}/fuzzer_stats.txt" 2>/dev/null || stat -c%s "${DOWNLOAD_DIR}/fuzzer_stats.txt" 2>/dev/null || echo "0")
        echo "✓ (${SIZE} bytes)"
        
        # Display first few lines
        echo "    Sample content:"
        head -n 5 "${DOWNLOAD_DIR}/fuzzer_stats.txt" | sed 's/^/      /'
    else
        echo "✗ Failed"
    fi
    
    # Download plot_data
    echo -n "  Downloading plot_data... "
    if curl -s -o "${DOWNLOAD_DIR}/plot_data.csv" \
        "${API_BASE}/jobs/${JOB_ID}/coverage/raw/plot_data"; then
        SIZE=$(stat -f%z "${DOWNLOAD_DIR}/plot_data.csv" 2>/dev/null || stat -c%s "${DOWNLOAD_DIR}/plot_data.csv" 2>/dev/null || echo "0")
        echo "✓ (${SIZE} bytes)"
        
        # Display first few lines
        echo "    Sample content:"
        head -n 3 "${DOWNLOAD_DIR}/plot_data.csv" | sed 's/^/      /'
    else
        echo "✗ Failed"
    fi
    
    # Download fuzz_bitmap
    echo -n "  Downloading fuzz_bitmap... "
    if curl -s -o "${DOWNLOAD_DIR}/fuzz_bitmap.bin" \
        "${API_BASE}/jobs/${JOB_ID}/coverage/raw/fuzz_bitmap"; then
        SIZE=$(stat -f%z "${DOWNLOAD_DIR}/fuzz_bitmap.bin" 2>/dev/null || stat -c%s "${DOWNLOAD_DIR}/fuzz_bitmap.bin" 2>/dev/null || echo "0")
        echo "✓ (${SIZE} bytes)"
        
        # Display hex dump of first few bytes
        echo "    Binary content (first 32 bytes):"
        hexdump -C "${DOWNLOAD_DIR}/fuzz_bitmap.bin" 2>/dev/null | head -n 2 | sed 's/^/      /'
    else
        echo "✗ Failed"
    fi
    echo ""
    
    # Download all as ZIP
    echo -n "Downloading all files as ZIP... "
    if curl -s -o "${DOWNLOAD_DIR}/coverage_all.zip" \
        "${API_BASE}/jobs/${JOB_ID}/coverage/raw/all/zip"; then
        SIZE=$(stat -f%z "${DOWNLOAD_DIR}/coverage_all.zip" 2>/dev/null || stat -c%s "${DOWNLOAD_DIR}/coverage_all.zip" 2>/dev/null || echo "0")
        echo "✓ (${SIZE} bytes)"
        
        # List ZIP contents
        echo "  ZIP contents:"
        unzip -l "${DOWNLOAD_DIR}/coverage_all.zip" 2>/dev/null | grep -E "fuzzer_stats|plot_data|fuzz_bitmap" | sed 's/^/    /'
    else
        echo "✗ Failed"
    fi
    echo ""
    
    # Verify files if running with Docker
    if [ -n "$DOCKER_COMPOSE" ] && [ "$DOCKER_COMPOSE" = "true" ]; then
        echo "Verifying against bot container files..."
        
        BOT_CONTAINER="pandafuzz-bot-1"
        AFL_OUTPUT_DIR="/tmp/fuzzing/${JOB_ID}/output/afl_output"
        
        # Compare fuzzer_stats
        echo -n "  Comparing fuzzer_stats... "
        if docker exec "$BOT_CONTAINER" test -f "${AFL_OUTPUT_DIR}/fuzzer_stats" 2>/dev/null; then
            BOT_HASH=$(docker exec "$BOT_CONTAINER" sha256sum "${AFL_OUTPUT_DIR}/fuzzer_stats" 2>/dev/null | cut -d' ' -f1)
            LOCAL_HASH=$(sha256sum "${DOWNLOAD_DIR}/fuzzer_stats.txt" 2>/dev/null | cut -d' ' -f1)
            
            if [ "$BOT_HASH" = "$LOCAL_HASH" ]; then
                echo "✓ Hashes match"
            else
                echo "✗ Hash mismatch"
                echo "    Bot:   ${BOT_HASH}"
                echo "    Local: ${LOCAL_HASH}"
            fi
        else
            echo "- File not found in bot container"
        fi
        
        echo ""
    fi
    
    echo "Summary:"
    echo "========"
    echo "✓ Raw AFL++ coverage files successfully downloaded"
    echo "  Download directory: ${DOWNLOAD_DIR}"
    echo "  Files:"
    ls -la "$DOWNLOAD_DIR" | grep -E "fuzzer_stats|plot_data|fuzz_bitmap|coverage_all" | sed 's/^/    /'
    
else
    echo "✗ No raw coverage files found"
    echo "Response: $COVERAGE_RESPONSE"
fi

echo ""
echo "Test completed!"