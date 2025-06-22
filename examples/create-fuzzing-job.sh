#!/bin/bash
# Create a real fuzzing job for the vulnerable program

set -e

# Default master URL
MASTER_URL="${MASTER_URL:-http://localhost:8080}"

echo "Creating fuzzing job on $MASTER_URL..."

# Create AFL++ fuzzing job
AFL_JOB_DATA='{
  "name": "Fuzz Vulnerable Program - AFL++",
  "target": "/app/targets/vulnerable_afl",
  "fuzzer": "afl++",
  "duration": 300000000000,
  "config": {
    "duration": 300000000000,
    "memory_limit": 536870912,
    "timeout": 5000000000,
    "dictionary": "/app/targets/vuln.dict",
    "seed_corpus": ["/app/corpus/seed1.txt", "/app/corpus/seed2.txt", "/app/corpus/seed3.txt", "/app/corpus/seed4.txt"],
    "output_dir": "/app/work/afl_output"
  }
}'

echo "Creating AFL++ job..."
AFL_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "$AFL_JOB_DATA" \
    "$MASTER_URL/api/v1/jobs")

if echo "$AFL_RESPONSE" | grep -q '"id"'; then
    echo "AFL++ job created successfully!"
    echo "$AFL_RESPONSE" | jq '.'
    AFL_JOB_ID=$(echo "$AFL_RESPONSE" | jq -r '.id')
else
    echo "Failed to create AFL++ job:"
    echo "$AFL_RESPONSE"
fi

echo ""

# Create LibFuzzer job
LIBFUZZER_JOB_DATA='{
  "name": "Fuzz Vulnerable Program - LibFuzzer",
  "target": "/app/targets/vulnerable_libfuzzer",
  "fuzzer": "libfuzzer",
  "duration": 300000000000,
  "config": {
    "duration": 300000000000,
    "memory_limit": 536870912,
    "timeout": 5000000000,
    "dictionary": "/app/targets/vuln.dict",
    "seed_corpus": ["/app/corpus"],
    "output_dir": "/app/work/libfuzzer_output"
  }
}'

echo "Creating LibFuzzer job..."
LIBFUZZER_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "$LIBFUZZER_JOB_DATA" \
    "$MASTER_URL/api/v1/jobs")

if echo "$LIBFUZZER_RESPONSE" | grep -q '"id"'; then
    echo "LibFuzzer job created successfully!"
    echo "$LIBFUZZER_RESPONSE" | jq '.'
    LIBFUZZER_JOB_ID=$(echo "$LIBFUZZER_RESPONSE" | jq -r '.id')
else
    echo "Failed to create LibFuzzer job:"
    echo "$LIBFUZZER_RESPONSE"
fi

echo ""
echo "Jobs created! You can view them at:"
echo "  $MASTER_URL/jobs"
echo ""
echo "To check job status:"
echo "  AFL++:     curl $MASTER_URL/api/v1/jobs/$AFL_JOB_ID | jq '.'"
echo "  LibFuzzer: curl $MASTER_URL/api/v1/jobs/$LIBFUZZER_JOB_ID | jq '.'"
echo ""
echo "The fuzzer will try to find:"
echo "  1. Buffer overflow (> 64 chars)"
echo "  2. Crash on 'CRASH' input"
echo "  3. Out-of-bounds on 'BUG' at position 10-12"