# Test Resources

This directory contains all test-related resources for PandaFuzz.

## Directory Structure

### test-targets/
Example fuzzing targets for testing PandaFuzz functionality:

- **crashers/** - Programs designed to crash under specific inputs
  - `test-crasher.c` - Simple program that crashes on specific input patterns
  - `segfault.c` - Program that triggers segmentation faults

- **fuzzers/** - Fuzzer-compatible test programs
  - `libfuzzer-test.c` - C test harness for LibFuzzer
  - `libfuzzer-test.cc` - C++ test harness for LibFuzzer
  - `test-fuzzer.cc` - Generic fuzzer test target
  - `test-libfuzzer-harness.cc` - LibFuzzer harness implementation

- **vulnerable/** - Intentionally vulnerable programs for security testing

### test-data/
Test data and seed inputs for fuzzing:

- **seeds/** - Initial seed inputs for fuzzing
- **corpus/** - Test corpus files (generated during tests)

### test-corpus/
Sample corpus files for testing corpus management features:
- `crash_seed.txt` - Sample crash-triggering input

## Usage

### Compiling Test Targets

For AFL++:
```bash
# Using AFL++ compiler
afl-clang-fast -o test-binary test-targets/crashers/test-crasher.c

# Using regular compiler
gcc -o test-binary test-targets/crashers/test-crasher.c
```

For LibFuzzer:
```bash
# Requires clang with fuzzer support
clang -fsanitize=fuzzer,address -g -o libfuzzer-test test-targets/fuzzers/libfuzzer-test.cc
```

### Using Test Resources

1. **With the test script:**
   ```bash
   ./scripts/test-fuzzers.sh
   ```
   The script automatically uses binaries from this directory if compilation fails.

2. **Manual testing:**
   ```bash
   # Compile a test target
   gcc -o crasher test-targets/crashers/test-crasher.c
   
   # Upload and create job
   curl -X POST http://localhost:8080/api/v1/jobs/upload \
     -F "job_metadata={\"name\":\"Test\",\"type\":\"fuzzing\",\"fuzzer\":\"afl++\"}" \
     -F "target_binary=@crasher"
   ```

3. **Using seed corpus:**
   ```bash
   # Create archive of seed inputs
   cd test-data/seeds
   tar -czf seeds.tar.gz *
   
   # Upload with job
   curl -X POST http://localhost:8080/api/v1/jobs/upload \
     -F "job_metadata={...}" \
     -F "target_binary=@binary" \
     -F "seed_corpus=@seeds.tar.gz"
   ```

## Test Programs

### Crash Patterns

The test programs in `test-targets/crashers/` crash on these inputs:
- `"CRASH"` - Triggers abort()
- `"FAULT"` - Triggers segmentation fault
- `"DIVZERO"` - Triggers division by zero
- `"BUG"` - Triggers assertion failure

These patterns help verify that PandaFuzz correctly:
- Detects crashes during fuzzing
- Captures crash inputs
- Reports crashes to the master
- Stores crash information in the database