#!/bin/bash

echo "=== Testing Raw Coverage Persistence Fix ==="

# Create a simple test job
echo "Creating test job..."
JOB_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Manual Raw Coverage Test",
    "fuzzer": "aflplusplus",
    "target": "test_binary",
    "duration": 30,
    "enable_coverage": true,
    "coverage_format": "raw",
    "config": {
      "timeout": 1000,
      "memory_limit": 256,
      "duration": 30
    }
  }')

JOB_ID=$(echo "$JOB_RESPONSE" | jq -r '.id')
echo "Created job: $JOB_ID"

# Manually update the job to fix timeout issue
echo "Fixing job timeout..."
docker exec pandafuzz-master sqlite3 /app/data/pandafuzz.db \
  "UPDATE jobs SET timeout_at=datetime('now', '+5 minutes') WHERE id='$JOB_ID';"

# Force assign to bot
BOT_ID=$(docker exec pandafuzz-master sqlite3 /app/data/pandafuzz.db \
  "SELECT id FROM bots WHERE status='active' ORDER BY last_heartbeat DESC LIMIT 1;")
echo "Assigning to bot: $BOT_ID"

docker exec pandafuzz-master sqlite3 /app/data/pandafuzz.db \
  "UPDATE jobs SET status='assigned', assigned_bot='$BOT_ID', started_at=datetime('now') WHERE id='$JOB_ID';"

echo "Job assigned. Waiting for bot to process..."
sleep 10

# Check if bot is processing
echo "Checking bot logs for job execution..."
docker logs pandafuzz-bot-1 2>&1 | grep "$JOB_ID" | tail -5

echo ""
echo "Waiting 30 seconds for job to complete..."
sleep 30

# Check for raw coverage files
echo ""
echo "=== Checking Raw Coverage Files ==="
echo "1. Bot Storage:"
docker exec pandafuzz-bot-1 ls -la work/coverage_data/corpus/coverage/ 2>/dev/null | head -5 || echo "No coverage directory"

echo ""
echo "2. Database Records:"
docker exec pandafuzz-master sqlite3 /app/data/pandafuzz.db \
  "SELECT job_id, format, file_type, 
   CASE WHEN fuzzer_stats_path IS NOT NULL THEN 'YES' ELSE 'NO' END as has_fuzzer_stats,
   CASE WHEN plot_data_path IS NOT NULL THEN 'YES' ELSE 'NO' END as has_plot_data,
   CASE WHEN fuzz_bitmap_path IS NOT NULL THEN 'YES' ELSE 'NO' END as has_bitmap
   FROM coverage_reports WHERE job_id='$JOB_ID';"

echo ""
echo "3. API Response:"
curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/coverage/raw" | jq '.count, .files[0]'

echo ""
echo "=== Test Complete ==="