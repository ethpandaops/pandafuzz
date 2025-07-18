# Scripts Overview

## Testing AFL++ and LibFuzzer

### Quick Start
```bash
# Fastest way to test both fuzzers
./scripts/quick-fuzzer-test.sh
```

### Comprehensive Test
```bash
# Full test with crash detection verification
./scripts/test-fuzzers.sh
```

### Manual Job Creation
```bash
# Create AFL++ job
./scripts/create-job.sh "AFL Test" afl++ 60 /path/to/binary

# Create LibFuzzer job
./scripts/create-job.sh "LibFuzzer Test" libfuzzer 60 /path/to/binary
```

## What Each Script Does

1. **`quick-fuzzer-test.sh`** (NEW)
   - Quick test for both AFL++ and LibFuzzer
   - Uses pre-built test binaries if available
   - Creates simple test binaries if needed
   - Shows immediate results
   - Best for: Quick verification that fuzzers work

2. **`test-fuzzers.sh`**
   - Comprehensive test suite
   - Creates test binaries with multiple crash patterns
   - Monitors job execution in real-time
   - Verifies crash detection and log capture
   - Best for: Thorough testing of AFL++ fixes

3. **`create-job.sh`** (UPDATED)
   - Create custom fuzzing jobs
   - Supports binary and seed corpus upload
   - Shows job status and early crash detection
   - Best for: Testing with custom binaries

## Example Workflow

```bash
# 1. Start services (if using Docker)
docker-compose up -d

# 2. Quick test to verify everything works
./scripts/quick-fuzzer-test.sh

# 3. Run comprehensive tests
./scripts/test-fuzzers.sh

# 4. Create custom jobs
gcc -o my-target test-resources/test-targets/crashers/test-crasher.c
./scripts/create-job.sh "My Test" afl++ 300 my-target
```

## Key Features Tested

✅ AFL++ crash detection in `output/afl_output/crashes/`
✅ LibFuzzer crash detection
✅ Job log capture and retrieval
✅ Real-time crash reporting during execution
✅ Binary and seed corpus upload