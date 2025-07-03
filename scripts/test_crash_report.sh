#!/bin/bash

# Test script to verify crash reporting

echo "Testing crash reporting to master..."

# Create a test crash result
curl -X POST http://localhost:8080/api/v1/results/crash \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-crash-001",
    "job_id": "test-job-001",
    "bot_id": "test-bot-001",
    "hash": "7a8dc3985d2a90fb6e62e94910fc11d31949c348",
    "file_path": "./crash-7a8dc3985d2a90fb6e62e94910fc11d31949c348",
    "type": "libfuzzer",
    "signal": 11,
    "exit_code": 77,
    "timestamp": "'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'",
    "size": 1024,
    "is_unique": true,
    "output": "==12345==ERROR: AddressSanitizer: heap-buffer-overflow",
    "stack_trace": "SUMMARY: AddressSanitizer: heap-buffer-overflow"
  }' \
  -w "\n\nHTTP Status: %{http_code}\n"

echo -e "\nChecking if crash was stored..."
sleep 1

# Check crashes endpoint
echo -e "\nFetching crashes from API..."
curl -s http://localhost:8080/api/v1/results/crashes | jq '.crashes | length'