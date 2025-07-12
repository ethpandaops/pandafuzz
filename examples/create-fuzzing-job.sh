#!/bin/bash
# Self-contained script that creates a working fuzzing job with crash detection
# This script compiles and uploads a LibFuzzer binary that will find crashes quickly

set -e

MASTER_URL="${MASTER_URL:-http://localhost:8080}"

echo "================================================"
echo "PandaFuzz Example: Create Fuzzing Job with Crash"
echo "================================================"
echo ""

# Step 1: Create a simple vulnerable LibFuzzer target
echo "Step 1: Creating vulnerable LibFuzzer target..."
cat > /tmp/crash_example.c << 'EOF'
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
#include <stdio.h>

// Simple LibFuzzer harness that crashes on specific inputs
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    // Print to show it's running
    static int runs = 0;
    if (++runs % 1000 == 0) {
        fprintf(stderr, "Fuzzer has run %d times\n", runs);
    }
    
    // Crash on "BOOM" - easy to find
    if (size >= 4 && memcmp(data, "BOOM", 4) == 0) {
        fprintf(stderr, "FOUND CRASH TRIGGER: BOOM!\n");
        abort();
    }
    
    // Another crash on "FUZZ"
    if (size >= 4 && data[0] == 'F' && data[1] == 'U' && 
        data[2] == 'Z' && data[3] == 'Z') {
        fprintf(stderr, "FOUND CRASH TRIGGER: FUZZ!\n");
        __builtin_trap();
    }
    
    // Buffer overflow on long input
    if (size > 100) {
        char small[50];
        memcpy(small, data, size); // Overflow!
    }
    
    return 0;
}
EOF

echo "✓ Created crash_example.c with multiple vulnerabilities"
echo ""

# Step 2: Compile with LibFuzzer using Docker
echo "Step 2: Compiling with LibFuzzer..."
docker run --rm -v /tmp:/work -w /work ubuntu:22.04 bash -c '
    apt-get update -qq && apt-get install -y -qq clang > /dev/null 2>&1
    echo "Compiling..."
    clang -g -fsanitize=fuzzer,address crash_example.c -o crash_example_fuzzer
    echo "Testing binary..."
    ./crash_example_fuzzer -help=1 2>&1 | grep -q "libFuzzer" && echo "✓ Valid LibFuzzer binary"
' || exit 1

if [ ! -f "/tmp/crash_example_fuzzer" ]; then
    echo "❌ Failed to compile fuzzer"
    exit 1
fi

echo "✓ Binary compiled successfully"
ls -lh /tmp/crash_example_fuzzer
echo ""

# Step 3: Create seed corpus
echo "Step 3: Creating seed corpus..."
mkdir -p /tmp/corpus
echo -n "BOO" > /tmp/corpus/seed1   # Almost "BOOM"
echo -n "FUZ" > /tmp/corpus/seed2   # Almost "FUZZ"
echo -n "test" > /tmp/corpus/seed3
printf 'A%.0s' {1..50} > /tmp/corpus/seed4  # Medium size
echo "✓ Created 4 seed files"
echo ""

# Step 4: Upload binary and create job
echo "Step 4: Creating fuzzing job with binary upload..."

JOB_METADATA='{
  "name": "LibFuzzer Crash Example",
  "target": "/app/targets/crash_example",
  "fuzzer": "libfuzzer",
  "duration": 120000000000,
  "config": {
    "duration": 120000000000,
    "memory_limit": 512,
    "timeout": 5000000000,
    "output_dir": "/app/work/crash_example_output"
  }
}'

# Upload the binary and corpus
RESPONSE=$(curl -s -X POST \
    -F "job_metadata=$JOB_METADATA" \
    -F "target_binary=@/tmp/crash_example_fuzzer" \
    -F "seed_corpus=@/tmp/corpus/seed1" \
    -F "seed_corpus=@/tmp/corpus/seed2" \
    -F "seed_corpus=@/tmp/corpus/seed3" \
    -F "seed_corpus=@/tmp/corpus/seed4" \
    "$MASTER_URL/api/v1/jobs/upload")

# Clean up (ignore errors silently)
rm -f /tmp/crash_example.c /tmp/crash_example_fuzzer 2>/dev/null || true
rm -rf /tmp/corpus 2>/dev/null || true

if ! echo "$RESPONSE" | grep -q '"id"'; then
    echo "❌ Failed to create job:"
    echo "$RESPONSE" | jq '.' || echo "$RESPONSE"
    exit 1
fi

echo "✓ Job created successfully!"
JOB_ID=$(echo "$RESPONSE" | jq -r '.id')
echo "Job ID: $JOB_ID"
echo ""

# Step 5: Monitor the job
echo "================================================"
echo "📊 MONITORING YOUR FUZZING JOB"
echo "================================================"
echo ""
echo "Web UI: $MASTER_URL/jobs/$JOB_ID"
echo ""

# Check initial status
echo "Checking job status..."
sleep 3
STATUS=$(curl -s "$MASTER_URL/api/v1/jobs/$JOB_ID" | jq -r '.status')
echo "Status: $STATUS"

if [ "$STATUS" = "running" ]; then
    echo "✓ Job is running! LibFuzzer should find crashes within seconds."
    echo ""
    echo "⏳ Waiting 20 seconds for crashes..."
    sleep 20
    
    # Check for crashes
    echo ""
    echo "🔍 Checking for crashes..."
    # Try the job-specific crash endpoint first
    CRASHES=$(curl -s "$MASTER_URL/api/v1/jobs/$JOB_ID/crashes" 2>/dev/null || echo "")
    
    if [ -z "$CRASHES" ] || [ "$CRASHES" = "null" ]; then
        # Try global crashes endpoint
        CRASHES=$(curl -s "$MASTER_URL/api/v1/crashes" 2>/dev/null || echo "")
    fi
    
    if [ ! -z "$CRASHES" ] && [ "$CRASHES" != "null" ] && [ "$CRASHES" != "[]" ]; then
        CRASH_COUNT=$(echo "$CRASHES" | jq 'length' 2>/dev/null || echo "0")
        if [ "$CRASH_COUNT" -gt "0" ]; then
            echo "🎉 SUCCESS! Found $CRASH_COUNT crash(es)!"
            echo ""
            echo "Crash details:"
            echo "$CRASHES" | jq '.[] | {id, type, signal, hash}' 2>/dev/null || echo "$CRASHES"
            echo ""
            echo "📥 To download a crash:"
            FIRST_CRASH=$(echo "$CRASHES" | jq -r '.[0].id' 2>/dev/null || echo "")
            if [ ! -z "$FIRST_CRASH" ]; then
                echo "curl $MASTER_URL/api/v1/crashes/$FIRST_CRASH/download -o crash.input"
            fi
        fi
    else
        echo "No crashes found yet. This could mean:"
        echo "1. The fuzzer needs more time (check the web UI)"
        echo "2. The crashes endpoint might be at a different path"
        echo ""
        echo "Try checking:"
        echo "- Web UI: $MASTER_URL/crashes"
        echo "- Job logs: $MASTER_URL/jobs/$JOB_ID"
    fi
elif [ "$STATUS" = "failed" ]; then
    echo "❌ Job failed to start"
    echo "This might be due to LibFuzzer binary validation."
    echo ""
    echo "Check bot logs:"
    echo "docker logs pandafuzz-bot-1 --tail 50"
else
    echo "Job status: $STATUS"
    echo "The job might be pending assignment to a bot."
fi

echo ""
echo "================================================"
echo "📚 NEXT STEPS"
echo "================================================"
echo ""
echo "1. View job in web UI: $MASTER_URL/jobs/$JOB_ID"
echo "2. Check crashes: $MASTER_URL/crashes"
echo "3. Monitor job: curl $MASTER_URL/api/v1/jobs/$JOB_ID | jq '.'"
echo ""
echo "The fuzzer should find crashes quickly because:"
echo "- Seeds are close to crash triggers ('BOO' → 'BOOM')"
echo "- Crash conditions are simple (4-byte comparisons)"
echo "- LibFuzzer will mutate seeds to find exact matches"
echo ""
echo "🎯 SUCCESS CRITERIA:"
echo "The script is working when:"
echo "1. Job status shows 'running' or 'completed'"
echo "2. Bot logs show 'libfuzzer_crash' entries"
echo "3. Crash files appear in the job's work directory"
echo ""
echo "Even if the web UI doesn't show crashes (due to API issues),"
echo "you can verify crashes were found by checking:"
echo "docker exec pandafuzz-bot-1 find /app/work -name '*crash*' | grep $JOB_ID"