#!/bin/bash

# Unified job creation script for pandafuzz
# Usage: create_job.sh [options]

set -e

# Default values
JOB_NAME="test-job-$(date +%s)"
TARGET="test_fuzzer"
FUZZER="libfuzzer"
DURATION=30000000000  # 30 seconds in nanoseconds
TIMEOUT=10000000000   # 10 seconds in nanoseconds
MEMORY_LIMIT=1024
DICTIONARY=""
API_URL="http://localhost:8088"
BINARY_PATH=""
SEED_CORPUS=""
USE_UPLOAD_ENDPOINT=false

# Help function
show_help() {
    cat << EOF
Usage: $0 [OPTIONS]

Create a fuzzing job in pandafuzz.

OPTIONS:
    -h, --help              Show this help message
    -n, --name NAME         Job name (default: test-job-TIMESTAMP)
    -t, --target TARGET     Target binary name (default: test_fuzzer)
    -f, --fuzzer TYPE       Fuzzer type: libfuzzer, afl, honggfuzz (default: libfuzzer)
    -d, --duration SEC      Duration in seconds (default: 30)
    -T, --timeout SEC       Timeout in seconds (default: 10)
    -m, --memory MB         Memory limit in MB (default: 1024)
    -D, --dictionary PATH   Dictionary file path (default: empty)
    -u, --url URL           API URL (default: http://localhost:8088)
    -b, --binary PATH       Path to target binary (enables upload endpoint)
    -s, --seeds PATH        Path to seed corpus zip file
    --type TYPE             Job type: test, binary, libfuzzer (deprecated, use specific options)

EXAMPLES:
    # Simple test job (no binary upload)
    $0 --name "quick-test" --duration 60

    # Upload job with binary
    $0 --name "crash-test" --binary ./test_fuzzer --duration 300

    # Upload job with binary and seeds
    $0 --name "seeded-test" --binary ./test_fuzzer --seeds ./seed_corpus.zip

    # LibFuzzer job with custom settings
    $0 --name "libfuzzer-run" --binary ./libfuzzer_test --fuzzer libfuzzer --memory 2048
EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_help
            exit 0
            ;;
        -n|--name)
            JOB_NAME="$2"
            shift 2
            ;;
        -t|--target)
            TARGET="$2"
            shift 2
            ;;
        -f|--fuzzer)
            FUZZER="$2"
            shift 2
            ;;
        -d|--duration)
            DURATION=$(($2 * 1000000000))  # Convert seconds to nanoseconds
            shift 2
            ;;
        -T|--timeout)
            TIMEOUT=$(($2 * 1000000000))  # Convert seconds to nanoseconds
            shift 2
            ;;
        -m|--memory)
            MEMORY_LIMIT="$2"
            shift 2
            ;;
        -D|--dictionary)
            DICTIONARY="$2"
            shift 2
            ;;
        -u|--url)
            API_URL="$2"
            shift 2
            ;;
        -b|--binary)
            BINARY_PATH="$2"
            USE_UPLOAD_ENDPOINT=true
            shift 2
            ;;
        -s|--seeds)
            SEED_CORPUS="$2"
            shift 2
            ;;
        --type)
            # Deprecated option for backward compatibility
            echo "Warning: --type is deprecated. Use specific options instead."
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Create job metadata JSON
JOB_METADATA=$(cat <<EOF
{
    "name": "$JOB_NAME",
    "target": "$TARGET",
    "fuzzer": "$FUZZER",
    "duration": $DURATION,
    "config": {
        "duration": $DURATION,
        "timeout": $TIMEOUT,
        "memory_limit": $MEMORY_LIMIT,
        "dictionary": "$DICTIONARY"
    }
}
EOF
)

echo "Creating job: $JOB_NAME"
echo "Job metadata:"
echo "$JOB_METADATA" | jq . 2>/dev/null || echo "$JOB_METADATA"

# Submit job
if [ "$USE_UPLOAD_ENDPOINT" = true ]; then
    # Use upload endpoint when binary is provided
    echo "Uploading job with binary..."
    
    # Build curl command
    CURL_CMD="curl -X POST $API_URL/api/v1/jobs/upload"
    CURL_CMD="$CURL_CMD -F 'job_metadata=$JOB_METADATA'"
    
    if [ -n "$BINARY_PATH" ]; then
        if [ ! -f "$BINARY_PATH" ]; then
            echo "Error: Binary file not found: $BINARY_PATH"
            exit 1
        fi
        CURL_CMD="$CURL_CMD -F 'target_binary=@$BINARY_PATH'"
    fi
    
    if [ -n "$SEED_CORPUS" ]; then
        if [ ! -f "$SEED_CORPUS" ]; then
            echo "Error: Seed corpus file not found: $SEED_CORPUS"
            exit 1
        fi
        CURL_CMD="$CURL_CMD -F 'seed_corpus=@$SEED_CORPUS'"
    fi
    
    CURL_CMD="$CURL_CMD -v"
    
    # Execute curl command
    eval $CURL_CMD
else
    # Use simple JSON endpoint
    echo "Creating job via JSON API..."
    curl -X POST $API_URL/api/v1/jobs \
        -H "Content-Type: application/json" \
        -d "$JOB_METADATA" \
        -v
fi

echo -e "\n\nJob creation completed!"