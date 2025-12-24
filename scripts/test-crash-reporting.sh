#!/bin/bash

# Test script to verify AFL++ crash reporting is fixed
# This script creates a test job and monitors for crash reporting

set -e

echo "Starting containers..."
docker compose up -d

echo "Waiting for services to be ready..."
sleep 10

echo "Creating a test job with AFL++..."
# Create a test job using the API
JOB_ID=$(curl -s -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test AFL++ Crash Reporting",
    "fuzzer": "afl++",
    "target": "test_crash",
    "duration": 60,
    "timeout": 1000,
    "memory_limit": 536870912,
    "campaign_id": "test-campaign"
  }' | jq -r '.id')

echo "Created job: $JOB_ID"

# Wait for the job to start running
echo "Waiting for job to start..."
sleep 5

# Monitor the job status
echo "Monitoring job for crashes..."
for i in {1..20}; do
    STATUS=$(curl -s http://localhost:8080/api/v1/jobs/$JOB_ID | jq -r '.status')
    echo "[$i/20] Job status: $STATUS"

    # Check for crashes
    CRASHES=$(curl -s http://localhost:8080/api/v1/crashes | jq -r '.crashes | length')
    echo "  Crashes found: $CRASHES"

    if [ "$CRASHES" -gt 0 ]; then
        echo "SUCCESS: Crashes are being reported!"
        curl -s http://localhost:8080/api/v1/crashes | jq '.crashes[] | {id, job_id, bot_id, type, timestamp}'
        break
    fi

    if [ "$STATUS" == "completed" ] || [ "$STATUS" == "failed" ]; then
        echo "Job finished with status: $STATUS"
        break
    fi

    sleep 5
done

# Check bot logs for crash detection
echo ""
echo "Checking bot logs for crash detection..."
BOT_CONTAINER=$(docker compose ps -q bot)
docker logs $BOT_CONTAINER 2>&1 | grep -i "crash" | tail -20 || echo "No crash logs found"

echo ""
echo "Test complete."