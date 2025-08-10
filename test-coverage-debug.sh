#!/bin/bash

echo "=== Coverage Debug Test ==="
echo "This will create a test job and monitor the coverage data flow"
echo

# Set very verbose logging
export LOG_LEVEL=debug

# Create test binary and corpus
TEST_DIR="/tmp/coverage-debug-test-$(date +%s)"
mkdir -p "$TEST_DIR"

cat > "$TEST_DIR/test.c" << 'EOF'
#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>

int main() {
    char buf[10];
    if (read(0, buf, 10) < 1) return 0;
    
    if (buf[0] == 'A') {
        if (buf[1] == 'B') {
            if (buf[2] == 'C') {
                abort();
            }
        }
    }
    return 0;
}
EOF

gcc -o "$TEST_DIR/test_binary" "$TEST_DIR/test.c"
mkdir -p "$TEST_DIR/corpus"
echo "test" > "$TEST_DIR/corpus/seed1"
echo "Atest" > "$TEST_DIR/corpus/seed2"
echo "ABtest" > "$TEST_DIR/corpus/seed3"

# Copy to containers
docker cp "$TEST_DIR/test_binary" pandafuzz-master:/app/data/binaries/debug-test
docker cp "$TEST_DIR/corpus" pandafuzz-master:/app/data/corpus/debug-corpus

# Start tailing logs
echo "Starting log monitoring..."
docker logs -f pandafuzz-bot-1 2>&1 | grep -E "DEBUG:|ERROR:" > bot-debug.log &
BOT_LOG_PID=$!

docker logs -f pandafuzz-master 2>&1 | grep -E "DEBUG:|ERROR:" > master-debug.log &
MASTER_LOG_PID=$!

# Create job
echo "Creating test job..."
JOB_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v3/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coverage Debug Test",
    "fuzzer": "afl++",
    "target": "/app/data/binaries/debug-test",
    "duration": 30000000000,
    "config": {
      "memory_limit": 256,
      "timeout": 1000
    }
  }')

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo "Created job: $JOB_ID"

# Wait for job to complete
echo "Waiting 45 seconds for job to complete..."
sleep 45

# Get job status
echo "Checking job status..."
curl -s http://localhost:8080/api/v3/jobs/$JOB_ID | jq '.status'

# Get coverage
echo "Checking coverage..."
curl -s http://localhost:8080/api/v3/jobs/$JOB_ID/coverage | jq '.'

# Stop log tailing
kill $BOT_LOG_PID $MASTER_LOG_PID 2>/dev/null

# Check fuzzer stats directly
echo "Checking fuzzer stats in container..."
docker exec pandafuzz-bot-1 find /app/work/jobs -name "fuzzer_stats" -exec cat {} \; 2>/dev/null | grep -E "edges_found|bitmap_cvg|paths_total"

echo
echo "=== Debug Output Summary ==="
echo "Bot debug log: bot-debug.log"
echo "Master debug log: master-debug.log"
echo
echo "Key things to look for:"
echo "1. 'DEBUG: Starting coverage collection' - confirms collection started"
echo "2. 'DEBUG: Edges found and coverage calculated' - shows edge parsing"
echo "3. 'DEBUG: Coverage data collected from fuzzer' - shows data received by executor"
echo "4. 'DEBUG: Coverage percentage extraction' - shows type conversion attempts"
echo "5. 'DEBUG: Sending coverage data to master' - shows what's being sent"
echo "6. 'DEBUG: Received coverage report from bot' - shows what master received"

# Cleanup
rm -rf "$TEST_DIR"
