#!/bin/bash

# Create a test fuzzing job for libfuzzer
curl -X POST http://localhost:8088/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-libfuzzer-crash",
    "target": "test_fuzzer",
    "fuzzer": "libfuzzer",
    "duration": 300000000000,
    "config": {
      "timeout": 10000000000,
      "memory_limit": 1024,
      "dictionary": ""
    }
  }'