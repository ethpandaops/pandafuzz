#!/bin/bash
# Script to create a test job via the API

set -e

# Default master URL
MASTER_URL="${MASTER_URL:-http://localhost:8080}"

echo "Creating test job on $MASTER_URL..."

# Create a test job
JOB_DATA='{
  "name": "Test Fuzzing Job",
  "target": "/usr/bin/test-binary",
  "fuzzer": "afl++",
  "duration": 3600000000000,
  "config": {
    "duration": 3600000000000,
    "memory_limit": 1073741824,
    "timeout": 30000000000,
    "dictionary": "/path/to/dict.txt",
    "seed_corpus": ["/path/to/seeds"],
    "output_dir": "/tmp/fuzzing-output"
  }
}'

RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d "$JOB_DATA" \
    "$MASTER_URL/api/v1/jobs")

if echo "$RESPONSE" | grep -q '"id"'; then
    echo "Job created successfully!"
    echo "$RESPONSE" | jq '.'
else
    echo "Failed to create job:"
    echo "$RESPONSE"
    exit 1
fi

# List all jobs
echo -e "\nListing all jobs:"
curl -s "$MASTER_URL/api/v1/jobs" | jq '.'