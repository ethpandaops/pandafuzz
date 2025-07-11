#!/bin/bash
# Comprehensive test script for pandafuzz binary execution flow

set -e

echo "=== Testing PandaFuzz Binary Execution Flow ==="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ $2${NC}"
    else
        echo -e "${RED}✗ $2${NC}"
        exit 1
    fi
}

# 1. Apply the storage path fix first
echo "Step 1: Applying storage path fixes..."
if [ -f ./scripts/fix-storage-paths.sh ]; then
    ./scripts/fix-storage-paths.sh
    print_status $? "Storage paths fixed"
else
    echo -e "${YELLOW}Warning: fix-storage-paths.sh not found, continuing...${NC}"
fi

echo ""
echo "Step 2: Creating test libfuzzer binary in master container..."
docker-compose exec master bash -c '
# Create test directory
mkdir -p /tmp/test_binary

# Create simple libfuzzer harness
cat > /tmp/test_binary/test_fuzzer.cc << "EOF"
#include <stdint.h>
#include <stddef.h>
#include <stdio.h>

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size > 0) {
        printf("Fuzzing with %zu bytes\n", size);
        if (size > 2 && data[0] == '\''H'\'' && data[1] == '\''I'\'' && data[2] == '\''!'\'') {
            __builtin_trap();
        }
    }
    return 0;
}
EOF

# Try to compile in container
if command -v clang++ &> /dev/null; then
    clang++ -fsanitize=fuzzer /tmp/test_binary/test_fuzzer.cc -o /tmp/test_binary/test_fuzzer
    echo "Binary compiled successfully"
else
    # Create a simple executable script as fallback
    cat > /tmp/test_binary/test_fuzzer << "EEOF"
#!/bin/bash
if [[ "$1" == "-help=1" ]]; then
    echo "libFuzzer fake help output"
    echo "This is a test binary"
    exit 0
fi
echo "Fake fuzzer running..."
exit 0
EEOF
    chmod +x /tmp/test_binary/test_fuzzer
    echo "Created fake test binary (clang not available)"
fi

# Copy to binaries directory with timestamp
TIMESTAMP=$(date +%s)
BINARY_NAME="test_fuzzer_${TIMESTAMP}"
cp /tmp/test_binary/test_fuzzer /app/data/binaries/${BINARY_NAME}
chmod 755 /app/data/binaries/${BINARY_NAME}

echo "Binary stored at: /app/data/binaries/${BINARY_NAME}"
ls -la /app/data/binaries/${BINARY_NAME}

# Store the path for later use
echo "binaries/${BINARY_NAME}" > /tmp/binary_path.txt
'
print_status $? "Test binary created in master"

# Get the binary path
BINARY_PATH=$(docker-compose exec master cat /tmp/binary_path.txt | tr -d '\r\n')
echo "Binary path: $BINARY_PATH"

echo ""
echo "Step 3: Creating test job via API..."
JOB_RESPONSE=$(curl -s -X POST http://localhost:8088/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test LibFuzzer Job",
    "fuzzer": "libfuzzer",
    "target": "'"${BINARY_PATH}"'",
    "config": {
      "duration": 30,
      "timeout": 5,
      "memory_limit": 512,
      "max_executions": 100
    }
  }')

JOB_ID=$(echo $JOB_RESPONSE | grep -o '"id":"[^"]*' | grep -o '[^"]*$')
if [ -z "$JOB_ID" ]; then
    echo -e "${RED}Failed to create job. Response: $JOB_RESPONSE${NC}"
    exit 1
fi
print_status 0 "Job created with ID: $JOB_ID"

echo ""
echo "Step 4: Waiting for job assignment..."
sleep 5

echo ""
echo "Step 5: Checking job status..."
JOB_STATUS=$(curl -s http://localhost:8088/api/v1/jobs/${JOB_ID} | grep -o '"status":"[^"]*' | grep -o '[^"]*$')
echo "Job status: $JOB_STATUS"

echo ""
echo "Step 6: Checking bot work directory..."
docker-compose exec bot bash -c "
echo 'Bot work directory contents:'
ls -la /app/work/

echo ''
echo 'Looking for job directories:'
find /app/work -type d -name 'job*' 2>/dev/null | head -10

echo ''
echo 'Looking for downloaded binaries:'
find /app/work -name 'target_binary' -type f 2>/dev/null | head -10
"

echo ""
echo "Step 7: Checking bot logs for download activity..."
echo "Recent bot logs:"
docker-compose logs --tail=50 bot | grep -E "(download|binary|job_id|work_dir)" | tail -20

echo ""
echo "Step 8: Testing direct binary download..."
# Get bot ID
BOT_ID=$(docker-compose exec bot bash -c "grep 'Bot ID:' /app/logs/bot.log 2>/dev/null | tail -1 | grep -o 'bot-[a-zA-Z0-9]*'" | tr -d '\r\n' || echo "bot-test")
echo "Using Bot ID: $BOT_ID"

# Test direct download endpoint
echo "Testing download endpoint..."
DOWNLOAD_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" \
  -H "X-Bot-ID: ${BOT_ID}" \
  http://localhost:8088/api/v1/jobs/${JOB_ID}/binary/download)

HTTP_CODE=$(echo "$DOWNLOAD_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
echo "Download response code: $HTTP_CODE"

if [ "$HTTP_CODE" = "200" ]; then
    print_status 0 "Binary download endpoint working"
else
    echo "Download failed. Response:"
    echo "$DOWNLOAD_RESPONSE" | head -20
fi

echo ""
echo "Step 9: Manual job execution test..."
docker-compose exec bot bash -c "
# Create job directory manually
mkdir -p /app/work/jobs/manual_test

# Try to download binary manually using curl
echo 'Attempting manual download...'
curl -s -H 'X-Bot-ID: ${BOT_ID}' \
  http://master:8080/api/v1/jobs/${JOB_ID}/binary/download \
  -o /app/work/jobs/manual_test/target_binary

# Check if download succeeded
if [ -f /app/work/jobs/manual_test/target_binary ]; then
    echo 'Manual download succeeded!'
    chmod +x /app/work/jobs/manual_test/target_binary
    ls -la /app/work/jobs/manual_test/target_binary
    
    # Test if it's a libfuzzer binary
    echo ''
    echo 'Testing binary:'
    /app/work/jobs/manual_test/target_binary -help=1 2>&1 | head -5 || echo 'Binary execution failed'
else
    echo 'Manual download failed'
fi
"

echo ""
echo "Step 10: Summary and debugging info..."
echo "----------------------------------------"
echo "Master storage contents:"
docker-compose exec master ls -la /app/data/binaries/ | head -10

echo ""
echo "Bot work directory:"
docker-compose exec bot ls -la /app/work/jobs/ 2>/dev/null || echo "No job directories found"

echo ""
echo "Recent job assignments:"
docker-compose logs master | grep -E "(assigned|job_id)" | tail -10

echo ""
echo "=== Test Complete ==="
echo ""
echo "Debugging commands:"
echo "- Check master logs: docker-compose logs -f master"
echo "- Check bot logs: docker-compose logs -f bot"
echo "- Enter master: docker exec -it pandafuzz-master bash"
echo "- Enter bot: docker exec -it pandafuzz-bot bash"
echo "- Check job status: curl http://localhost:8088/api/v1/jobs/${JOB_ID}"