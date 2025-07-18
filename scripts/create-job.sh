#!/bin/bash
# Script to create a fuzzing job via the API

set -e

# Configuration
MASTER_URL="${MASTER_URL:-http://localhost:8080}"

# Default values
JOB_NAME="${1:-Test Fuzzing Job}"
FUZZER="${2:-afl++}"
DURATION="${3:-60}"  # seconds
BINARY_PATH="${4}"

# Usage
if [ -z "$BINARY_PATH" ]; then
    echo "Usage: $0 <job_name> <fuzzer> <duration_seconds> <binary_path> [seed_corpus_path]"
    echo ""
    echo "Examples:"
    echo "  $0 'My Test' afl++ 60 /path/to/binary"
    echo "  $0 'LibFuzzer Test' libfuzzer 120 /path/to/binary /path/to/seeds.tar.gz"
    echo ""
    echo "Environment:"
    echo "  MASTER_URL=$MASTER_URL"
    exit 1
fi

SEED_CORPUS_PATH="${5}"

echo "Creating fuzzing job..."
echo "  Name: $JOB_NAME"
echo "  Fuzzer: $FUZZER"
echo "  Duration: ${DURATION}s"
echo "  Binary: $BINARY_PATH"
[ -n "$SEED_CORPUS_PATH" ] && echo "  Seeds: $SEED_CORPUS_PATH"

# Build the curl command
CURL_CMD="curl -s -X POST $MASTER_URL/api/v1/jobs/upload"
CURL_CMD="$CURL_CMD -F \"job_metadata={\\\"name\\\":\\\"$JOB_NAME\\\",\\\"type\\\":\\\"fuzzing\\\",\\\"fuzzer\\\":\\\"$FUZZER\\\",\\\"config\\\":{\\\"duration\\\":$DURATION,\\\"timeout\\\":1000,\\\"memory_limit\\\":536870912}}\""
CURL_CMD="$CURL_CMD -F \"target_binary=@$BINARY_PATH\""

if [ -n "$SEED_CORPUS_PATH" ] && [ -f "$SEED_CORPUS_PATH" ]; then
    CURL_CMD="$CURL_CMD -F \"seed_corpus=@$SEED_CORPUS_PATH\""
fi

# Execute the request
echo -e "\nSending request to $MASTER_URL..."
RESPONSE=$(eval $CURL_CMD)

# Parse response
JOB_ID=$(echo "$RESPONSE" | jq -r '.id' 2>/dev/null)
STATUS=$(echo "$RESPONSE" | jq -r '.status' 2>/dev/null)

if [ -z "$JOB_ID" ] || [ "$JOB_ID" = "null" ]; then
    echo "Failed to create job:"
    echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE"
    exit 1
fi

echo -e "\n✓ Job created successfully!"
echo "  ID: $JOB_ID"
echo "  Status: $STATUS"
echo "  View at: $MASTER_URL/jobs/$JOB_ID"

# Quick status check after a few seconds
echo -e "\nChecking job status..."
sleep 5

JOB_INFO=$(curl -s "$MASTER_URL/api/v1/jobs/$JOB_ID")
CURRENT_STATUS=$(echo "$JOB_INFO" | jq -r '.status')
ASSIGNED_BOT=$(echo "$JOB_INFO" | jq -r '.assigned_bot // "none"')

echo "  Status: $CURRENT_STATUS"
echo "  Bot: $ASSIGNED_BOT"

# Check for early crashes
CRASHES=$(curl -s "$MASTER_URL/api/v1/results/crashes" | jq -r ".crashes | map(select(.job_id == \"$JOB_ID\")) | length")
echo "  Crashes: $CRASHES"

echo -e "\nMonitor progress:"
echo "  - Job details: $MASTER_URL/api/v1/jobs/$JOB_ID"
echo "  - Job logs: $MASTER_URL/api/v1/jobs/$JOB_ID/logs"
echo "  - Crashes: $MASTER_URL/api/v1/results/crashes"