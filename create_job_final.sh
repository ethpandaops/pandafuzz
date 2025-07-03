#!/bin/bash

# Create job metadata JSON with duration in config
JOB_METADATA='{
    "name": "test-libfuzzer-crash-with-seed",
    "target": "test_fuzzer",
    "fuzzer": "libfuzzer",
    "duration": 30000000000,
    "config": {
        "duration": 30000000000,
        "timeout": 10000000000,
        "memory_limit": 1024,
        "dictionary": ""
    }
}'

# Create a zip file with seed corpus
zip seed_corpus.zip seed_input.txt

# Upload job with binary and seed corpus
curl -X POST http://localhost:8088/api/v1/jobs/upload \
  -F "job_metadata=$JOB_METADATA" \
  -F "target_binary=@test_fuzzer" \
  -F "seed_corpus=@seed_corpus.zip" \
  -v