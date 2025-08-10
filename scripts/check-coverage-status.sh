#!/bin/bash
# Script to check coverage status for a job on the bot

JOB_ID="${1:-$(docker exec pandafuzz-master-1 sqlite3 /app/pandafuzz.db "SELECT id FROM jobs ORDER BY created_at DESC LIMIT 1;" 2>/dev/null)}"

if [ -z "$JOB_ID" ]; then
    echo "Usage: $0 [job_id]"
    echo "No job ID provided and couldn't find latest job"
    exit 1
fi

echo "========================================="
echo "Coverage Status for Job: $JOB_ID"
echo "========================================="

# Check if job directory exists
if ! docker exec pandafuzz-bot-1 test -d /app/work/jobs/job_${JOB_ID} 2>/dev/null; then
    echo "Job directory not found on bot. Checking master storage..."
    
    # Check master storage
    docker exec pandafuzz-master-1 ls -la /app/data/coverage/${JOB_ID}/ 2>/dev/null || echo "No coverage files in master storage"
    exit 1
fi

# Get fuzzer type
FUZZER_TYPE=$(docker exec pandafuzz-master-1 sqlite3 /app/pandafuzz.db "SELECT fuzzer FROM jobs WHERE id='$JOB_ID';" 2>/dev/null)
echo "Fuzzer Type: $FUZZER_TYPE"
echo ""

# Check AFL++ coverage
if [[ "$FUZZER_TYPE" == *"afl"* ]]; then
    echo "=== AFL++ Coverage ==="
    
    # Check plot_data
    echo "Edge Coverage:"
    docker exec pandafuzz-bot-1 bash -c "
        if [ -f /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data ]; then
            tail -1 /app/work/jobs/job_${JOB_ID}/output/afl_output/plot_data | \
            awk -F',' '{print \"  Edges found: \" \$13 \" | Map coverage: \" \$7 \" | Queue size: \" \$4}'
        else
            echo '  No plot_data file found'
        fi
    " 2>/dev/null
    
    # Check for GCC coverage files
    echo ""
    echo "GCC Coverage Files (.gcda):"
    GCDA_COUNT=$(docker exec pandafuzz-bot-1 find /app/work/jobs/job_${JOB_ID} -name "*.gcda" 2>/dev/null | wc -l)
    echo "  Found $GCDA_COUNT .gcda files"
    
    if [ "$GCDA_COUNT" -gt 0 ]; then
        docker exec pandafuzz-bot-1 find /app/work/jobs/job_${JOB_ID} -name "*.gcda" -exec basename {} \; 2>/dev/null | head -3
    fi
fi

# Check LibFuzzer coverage
if [[ "$FUZZER_TYPE" == *"libfuzzer"* ]]; then
    echo "=== LibFuzzer Coverage ==="
    
    # Check for profraw files
    echo "LLVM Profile Files:"
    PROFRAW_COUNT=$(docker exec pandafuzz-bot-1 find /app/work/jobs/job_${JOB_ID} -name "*.profraw" 2>/dev/null | wc -l)
    echo "  Found $PROFRAW_COUNT .profraw files"
    
    if [ "$PROFRAW_COUNT" -gt 0 ]; then
        docker exec pandafuzz-bot-1 find /app/work/jobs/job_${JOB_ID} -name "*.profraw" -exec basename {} \; 2>/dev/null | head -3
    fi
fi

# Check generated coverage reports
echo ""
echo "=== Generated Coverage Reports ==="

# Check bot work directory
echo "In bot work directory:"
docker exec pandafuzz-bot-1 bash -c "
    cd /app/work/jobs/job_${JOB_ID}
    if ls coverage.* >/dev/null 2>&1; then
        ls -lh coverage.* 2>/dev/null
    else
        echo '  No coverage files generated'
    fi
" 2>/dev/null

# Check master storage
echo ""
echo "In master storage:"
docker exec pandafuzz-master-1 bash -c "
    if [ -d /app/data/coverage/${JOB_ID} ]; then
        ls -lh /app/data/coverage/${JOB_ID}/ 2>/dev/null | tail -n +2
    else
        echo '  No coverage directory'
    fi
" 2>/dev/null

# Check database records
echo ""
echo "=== Database Records ==="
docker exec pandafuzz-master-1 sqlite3 /app/pandafuzz.db "
    SELECT 'Coverage Enabled: ' || enable_coverage,
           'Coverage Format: ' || coverage_format
    FROM jobs WHERE id='$JOB_ID';
" 2>/dev/null

echo ""
docker exec pandafuzz-master-1 sqlite3 /app/pandafuzz.db "
    SELECT 'Report ID: ' || id,
           'Format: ' || format,
           'Size: ' || size || ' bytes',
           'Created: ' || created_at
    FROM coverage_reports WHERE job_id='$JOB_ID';
" 2>/dev/null || echo "No coverage reports in database"

echo ""
echo "========================================="
echo "To view report content:"
echo "  docker exec pandafuzz-bot-1 cat /app/data/coverage/${JOB_ID}/coverage-*.lcov | head -50"
echo "  docker exec pandafuzz-bot-1 cat /app/data/coverage/${JOB_ID}/coverage-*.json | python3 -m json.tool"