# PandaFuzz Scripts

This directory contains utility scripts for PandaFuzz operations.

## Fuzzing Test Scripts

### 🧪 `test-fuzzers.sh`
Comprehensive test script for AFL++ and LibFuzzer crash detection.
```bash
./scripts/test-fuzzers.sh

# With custom settings
MASTER_URL=http://localhost:8088 TEST_DURATION=60 ./scripts/test-fuzzers.sh
```
- Tests both AFL++ and LibFuzzer
- Verifies crash detection works correctly
- Checks log capture functionality
- Creates test binaries automatically

### 🚀 `quick-fuzzer-test.sh`
Quick test to start AFL++ and LibFuzzer jobs.
```bash
./scripts/quick-fuzzer-test.sh

# With custom master URL
MASTER_URL=http://localhost:8088 ./scripts/quick-fuzzer-test.sh
```
- Simplified version for quick testing
- Uses pre-built binaries if available
- Shows immediate results

## Job Management

### 📝 `create-job.sh`
Create a fuzzing job with custom parameters.
```bash
# Basic usage
./scripts/create-job.sh "My Test" afl++ 60 /path/to/binary

# With seed corpus
./scripts/create-job.sh "Fuzz Test" libfuzzer 120 /path/to/binary /path/to/seeds.tar.gz

# Examples with test binaries
./scripts/create-job.sh "AFL Test" afl++ 60 test-resources/test-targets/crashers/test-crasher
./scripts/create-job.sh "LibFuzz Test" libfuzzer 60 test-resources/test-targets/fuzzers/libfuzzer-test
```

## Development Tools

### 🔨 `build-docker.sh`
Build Docker images for master and bot.
```bash
./scripts/build-docker.sh
```

### 🔧 `fix-storage-paths.sh`
Fix storage paths in Docker setup (useful after fresh setup).
```bash
./scripts/fix-storage-paths.sh
```

## Testing Tools

### 🧪 `run-tests.sh`
Run the complete test suite (unit + integration tests).
```bash
./scripts/run-tests.sh
```

### 🌐 `run-e2e-tests.sh`
Run end-to-end tests with Playwright.
```bash
./scripts/run-e2e-tests.sh
```

### 🔌 `test-bot-connection.sh`
Test bot connectivity to master server.
```bash
./scripts/test-bot-connection.sh
```

## Quick Start Examples

### Test AFL++ and LibFuzzer
```bash
# Quick test (recommended for first-time users)
./scripts/quick-fuzzer-test.sh

# Comprehensive test
./scripts/test-fuzzers.sh
```

### Create Custom Jobs
```bash
# Compile a test target
gcc -o my-fuzzer test-resources/test-targets/crashers/test-crasher.c

# Create AFL++ job
./scripts/create-job.sh "My AFL++ Test" afl++ 300 my-fuzzer

# Create LibFuzzer job (requires clang-compiled binary)
clang -fsanitize=fuzzer -o my-libfuzzer test-resources/test-targets/fuzzers/libfuzzer-test.cc
./scripts/create-job.sh "My LibFuzzer Test" libfuzzer 300 my-libfuzzer
```

### Docker Workflow
```bash
# Build images
./scripts/build-docker.sh

# Start services
docker-compose up -d

# Fix storage if needed
./scripts/fix-storage-paths.sh

# Run tests
./scripts/test-fuzzers.sh
```

## Environment Variables

- `MASTER_URL` - Master server URL (default: `http://localhost:8080`)
- `TEST_DURATION` - Test duration in seconds for test scripts
- `BOT_ID` - Bot identifier for connection tests

## Tips

1. Always ensure master and bot services are running before running test scripts
2. Use `quick-fuzzer-test.sh` for rapid testing
3. Use `test-fuzzers.sh` for thorough verification
4. Check logs at `$MASTER_URL/api/v1/jobs/{job_id}/logs` for debugging