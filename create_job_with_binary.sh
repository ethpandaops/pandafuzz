#!/bin/bash

# Create job metadata JSON
JOB_METADATA='{
    "name": "test-libfuzzer-crash-detection",
    "target": "test_fuzzer",
    "fuzzer": "libfuzzer",
    "duration": 30000000000,
    "config": {
        "timeout": 10000000000,
        "memory_limit": 1024,
        "dictionary": ""
    }
}'

# Upload job with binary
curl -X POST http://localhost:8088/api/v1/jobs/upload \
  -F "job_metadata=$JOB_METADATA" \
  -F "target_binary=@test_fuzzer" \
  -v