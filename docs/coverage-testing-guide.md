# Coverage Collection Testing Guide

## Quick Start

To fix the coverage collection issue and test it properly, run:

```bash
# Use the fixed test script that compiles binaries in the container
./scripts/run-coverage-test-fixed.sh afl++     # Test AFL++ coverage
./scripts/run-coverage-test-fixed.sh libfuzzer  # Test LibFuzzer coverage
```

## The Problem

The original test scripts compile binaries on the host without proper fuzzer instrumentation:
- Missing AFL++ instrumentation (`__afl_*` symbols)
- Missing LibFuzzer entry points (`LLVMFuzzerTestOneInput`)
- Missing coverage instrumentation (`__llvm_prof*` symbols)

## The Solution

The fixed script (`run-coverage-test-fixed.sh`) solves this by:

1. **Compiling inside the bot container** where all fuzzer tools are installed
2. **Using proper compiler flags** for coverage instrumentation
3. **Verifying instrumentation** before running the test
4. **Extracting the binary** for job submission

## How It Works

### Step 1: Container Compilation

The script runs compilation inside the bot container:

```bash
docker exec pandafuzz-bot-1 bash -c '
    # Write source code
    cat > /tmp/test.c << EOF
    // Your test code here
    EOF
    
    # Compile with AFL++ instrumentation
    afl-clang-fast -g -O0 \
        -fprofile-instr-generate \
        -fcoverage-mapping \
        -o /tmp/test /tmp/test.c
'
```

### Step 2: Binary Extraction

```bash
docker cp pandafuzz-bot-1:/tmp/test ./test_binary
```

### Step 3: Job Submission with Coverage

```json
{
  "config": {
    "coverage": {
      "enabled": true,
      "format": "lcov"
    }
  }
}
```

## Alternative Solutions

### Option 1: Install Tools on Host

```bash
# Ubuntu/Debian
sudo apt-get install -y clang-14 llvm-14 libfuzzer-14-dev lcov

# Install AFL++
git clone https://github.com/AFLplusplus/AFLplusplus.git
cd AFLplusplus && make && sudo make install
```

### Option 2: Create Compilation API

Add an endpoint to compile on the bot via API (requires code changes).

### Option 3: Use Pre-compiled Test Binaries

Store properly instrumented test binaries in the repository.

## Verification

Check that coverage tools are working:

```bash
docker exec pandafuzz-bot-1 bash -c "
    which llvm-profdata && echo '✓ llvm-profdata found'
    which llvm-cov && echo '✓ llvm-cov found'
    which afl-clang-fast && echo '✓ afl-clang-fast found'
    which lcov && echo '✓ lcov found'
"
```

## Troubleshooting

If coverage reports don't appear:

1. **Check binary instrumentation:**
   ```bash
   docker exec pandafuzz-bot-1 nm /tmp/test | grep "__llvm_prof"
   ```

2. **Check job logs:**
   ```bash
   curl -s "http://localhost:8080/api/v1/jobs/${JOB_ID}/logs" | jq
   ```

3. **Check coverage files:**
   ```bash
   docker exec pandafuzz-bot-1 ls -la /app/work/jobs/job_${JOB_ID}/coverage/
   ```

## Next Steps

1. Run the fixed test script to verify coverage works
2. Update CI/CD pipelines to use the fixed approach
3. Consider implementing a compilation service for production use