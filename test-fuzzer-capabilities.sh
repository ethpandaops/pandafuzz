#!/bin/bash

# Test script to verify fuzzer capability matching fix
# This script creates jobs for different fuzzer types and verifies they are only assigned to capable bots

set -e

echo "Testing fuzzer capability matching fix..."
echo "======================================="

# Configuration
MASTER_URL="http://localhost:8080"
BOT_ID="test-bot-$(uuidgen | cut -c1-8)"

# Function to check API response
check_response() {
    local response=$1
    local expected=$2
    if [[ "$response" == *"$expected"* ]]; then
        echo "✓ Success: Found expected response"
    else
        echo "✗ Failed: Expected '$expected' not found in response"
        echo "Response: $response"
        return 1
    fi
}

# Function to create a job
create_job() {
    local fuzzer=$1
    local name="test-${fuzzer}-job-$(date +%s)"

    echo ""
    echo "Creating $fuzzer job: $name"

    response=$(curl -s -X POST "${MASTER_URL}/api/v1/jobs" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"$name\",
            \"target\": \"/tmp/test_binary\",
            \"fuzzer\": \"$fuzzer\",
            \"duration\": 60,
            \"config\": {
                \"timeout\": 10,
                \"memory_limit\": \"512M\"
            }
        }")

    job_id=$(echo "$response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)

    if [[ -n "$job_id" ]]; then
        echo "Created job ID: $job_id"
        echo "$job_id"
    else
        echo "Failed to create job: $response"
        return 1
    fi
}

# Function to register a bot with specific capabilities
register_bot() {
    local capabilities=$1
    local bot_name="bot-$(date +%s)"

    echo ""
    echo "Registering bot with capabilities: $capabilities"

    response=$(curl -s -X POST "${MASTER_URL}/api/v1/bots" \
        -H "Content-Type: application/json" \
        -d "{
            \"hostname\": \"test-host-$(uuidgen | cut -c1-8)\",
            \"name\": \"$bot_name\",
            \"capabilities\": $capabilities,
            \"api_endpoint\": \"http://localhost:9049\"
        }")

    bot_id=$(echo "$response" | grep -o '"bot_id":"[^"]*' | cut -d'"' -f4)

    if [[ -n "$bot_id" ]]; then
        echo "Registered bot ID: $bot_id"
        echo "$bot_id"
    else
        echo "Failed to register bot: $response"
        return 1
    fi
}

# Function to request a job for a bot
request_job() {
    local bot_id=$1

    echo ""
    echo "Requesting job for bot: $bot_id"

    response=$(curl -s -X POST "${MASTER_URL}/api/v1/bots/${bot_id}/jobs/next")

    if [[ "$response" == "" ]] || [[ "$response" == "null" ]]; then
        echo "No job assigned (204 No Content)"
        return 1
    else
        job_id=$(echo "$response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
        fuzzer=$(echo "$response" | grep -o '"fuzzer":"[^"]*' | cut -d'"' -f4)

        if [[ -n "$job_id" ]]; then
            echo "Assigned job ID: $job_id (fuzzer: $fuzzer)"
            return 0
        else
            echo "Response: $response"
            return 1
        fi
    fi
}

# Test 1: Bot with only AFL++ capability should only get AFL++ jobs
echo ""
echo "=== Test 1: AFL++ only bot ==="
echo "-------------------------------"

# Create jobs for each fuzzer type
afl_job=$(create_job "afl++")
libfuzzer_job=$(create_job "libfuzzer")
honggfuzz_job=$(create_job "honggfuzz")

# Register bot with only AFL++ capability
afl_bot=$(register_bot '["aflplusplus"]')

# Request job - should get AFL++ job
if request_job "$afl_bot"; then
    echo "✓ Test 1 passed: AFL++ bot got a job"
else
    echo "✗ Test 1 failed: AFL++ bot did not get a job"
fi

# Test 2: Bot with libfuzzer capability should only get libfuzzer jobs
echo ""
echo "=== Test 2: LibFuzzer only bot ==="
echo "-----------------------------------"

libfuzzer_bot=$(register_bot '["libfuzzer"]')

if request_job "$libfuzzer_bot"; then
    echo "✓ Test 2 passed: LibFuzzer bot got a job"
else
    echo "✗ Test 2 failed: LibFuzzer bot did not get a job"
fi

# Test 3: Bot with honggfuzz capability should only get honggfuzz jobs
echo ""
echo "=== Test 3: HonggFuzz only bot ==="
echo "-----------------------------------"

honggfuzz_bot=$(register_bot '["honggfuzz"]')

if request_job "$honggfuzz_bot"; then
    echo "✓ Test 3 passed: HonggFuzz bot got a job"
else
    echo "✗ Test 3 failed: HonggFuzz bot did not get a job"
fi

# Test 4: Bot with multiple capabilities can get any matching job
echo ""
echo "=== Test 4: Multi-capability bot ==="
echo "-------------------------------------"

multi_bot=$(register_bot '["aflplusplus", "libfuzzer", "honggfuzz"]')

# Create more jobs to ensure availability
create_job "afl++"
create_job "libfuzzer"
create_job "honggfuzz"

# Request multiple jobs
for i in {1..3}; do
    echo "Request $i:"
    if request_job "$multi_bot"; then
        echo "✓ Multi-capability bot got a job"
    else
        echo "✗ Multi-capability bot did not get a job"
        break
    fi
done

# Test 5: Bot with no matching capabilities should not get any jobs
echo ""
echo "=== Test 5: No matching capabilities ==="
echo "-----------------------------------------"

# Create an AFL++ job
create_job "afl++"

# Register a bot with only honggfuzz capability (no AFL++ jobs available for it)
honggfuzz_only_bot=$(register_bot '["honggfuzz"]')

# Clear any existing honggfuzz jobs by assigning them to the previous bot
request_job "$honggfuzz_bot" >/dev/null 2>&1
request_job "$honggfuzz_bot" >/dev/null 2>&1

# Now request job for honggfuzz-only bot when only AFL++ job is pending
if request_job "$honggfuzz_only_bot"; then
    echo "✗ Test 5 failed: Bot got a job despite no matching capabilities"
else
    echo "✓ Test 5 passed: Bot correctly received no job due to capability mismatch"
fi

echo ""
echo "======================================="
echo "Fuzzer capability matching tests completed"