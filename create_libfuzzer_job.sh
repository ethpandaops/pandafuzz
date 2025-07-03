#!/bin/bash

# Create job metadata JSON
JOB_METADATA='{
    "name": "test-libfuzzer-proper",
    "target": "libfuzzer_test",
    "fuzzer": "libfuzzer",
    "duration": 30000000000,
    "config": {
        "duration": 30000000000,
        "timeout": 1000000000,
        "memory_limit": 1024,
        "dictionary": ""
    }
}'

# Upload job with proper LibFuzzer binary
curl -X POST http://localhost:8088/api/v1/jobs/upload \
  -F "job_metadata=$JOB_METADATA" \
  -F "target_binary=@libfuzzer_test" \
  -v