# PandaFuzz Coverage Testing Scripts

This directory contains scripts for testing the coverage collection functionality in PandaFuzz.

## Scripts

### 1. `run-test-with-coverage.sh`

A comprehensive test script that demonstrates coverage collection with both AFL++ and LibFuzzer.

**Features:**
- Builds test binaries with coverage instrumentation
- Creates coverage-oriented seed corpus
- Runs fuzzing jobs with coverage enabled
- Fetches and displays coverage statistics
- Supports downloading coverage reports

**Usage:**
```bash
# Test both AFL++ and LibFuzzer (default)
./run-test-with-coverage.sh

# Test only LibFuzzer
./run-test-with-coverage.sh libfuzzer

# Test only AFL++
./run-test-with-coverage.sh afl++
```

**Requirements:**
- For LibFuzzer: `clang++` with LLVM coverage support
- For AFL++: `afl-clang-fast` (preferred) or `afl-gcc`
- Fallback: Regular `gcc`/`g++` with gcov

### 2. `quick-coverage-test.sh`

A simplified test script for quick coverage verification using existing test resources.

**Features:**
- Uses test binaries from `test-resources/`
- Minimal setup time
- Focuses on verifying coverage collection works
- JSON format coverage reports

**Usage:**
```bash
./quick-coverage-test.sh
```

## Coverage Instrumentation

### LibFuzzer (Clang)

For LibFuzzer with Clang, coverage instrumentation uses:
```bash
clang++ -fsanitize=fuzzer -fprofile-instr-generate -fcoverage-mapping
```

The coverage data is collected via:
- `LLVM_PROFILE_FILE` environment variable
- `llvm-profdata merge` to process raw profile data
- `llvm-cov export` to generate reports

### AFL++ 

AFL++ coverage can use two approaches:

1. **LLVM Mode** (recommended):
```bash
AFL_USE_ASAN=1 afl-clang-fast -fprofile-instr-generate -fcoverage-mapping
```

2. **GCC Mode**:
```bash
afl-gcc --coverage
```

### Coverage Formats

PandaFuzz supports multiple coverage report formats:
- **JSON**: Machine-readable format with detailed coverage data
- **LCOV**: Standard format compatible with many tools
- **HTML**: Human-readable reports with source code annotation
- **Cobertura**: XML format for CI/CD integration

## Test Binaries

The scripts create test binaries with various code paths to demonstrate coverage:

1. **Branch Coverage**: Multiple if/else conditions
2. **Function Coverage**: Several functions with different call patterns
3. **Line Coverage**: Sequential code execution paths
4. **Loop Coverage**: Iteration-based code paths

Example test patterns:
- Command processing (ADD, DEL, MOD, GET)
- Binary format detection (UTF-8/16 BOMs)
- Pattern matching (ABC, XYZ sequences)
- Error conditions (crashes, aborts)

## Viewing Coverage Results

### Via API

```bash
# List coverage reports
curl http://localhost:8080/api/v3/jobs/{job_id}/coverage

# Get coverage metadata
curl http://localhost:8080/api/v3/jobs/{job_id}/coverage/{report_id}/metadata

# Download coverage report
curl http://localhost:8080/api/v3/jobs/{job_id}/coverage/{report_id} -o report.json
```

### Via Web UI

1. Navigate to the Jobs page
2. Click on a completed job with coverage enabled
3. View the Coverage tab to see reports and statistics
4. Download reports in your preferred format

## Troubleshooting

### No Coverage Reports Generated

1. **Check fuzzer binary has coverage instrumentation:**
   ```bash
   # For LibFuzzer
   ./binary -help=1  # Should mention coverage if instrumented
   
   # Check for profraw files
   ls /path/to/job/coverage/*.profraw
   ```

2. **Verify coverage tools are installed:**
   ```bash
   which llvm-profdata
   which llvm-cov
   which afl-cov-fast
   ```

3. **Check job configuration has coverage enabled:**
   ```bash
   curl http://localhost:8080/api/v1/jobs/{job_id} | jq .config.coverage
   ```

4. **Review job logs for coverage errors:**
   ```bash
   curl http://localhost:8080/api/v1/jobs/{job_id}/logs | jq '.logs[] | select(.message | contains("coverage"))'
   ```

### Coverage Processing Timeout

If coverage processing takes too long:
- Reduce the corpus size
- Use a shorter fuzzing duration
- Check available disk space
- Verify bot has sufficient resources

### Invalid Coverage Data

For issues with coverage accuracy:
- Ensure binary was built with `-O0` or `-O1` (not `-O2`/`-O3`)
- Verify source files are accessible to coverage tools
- Check that instrumentation matches the fuzzer type

## Example Output

Successful coverage collection shows:
```
✓ Found 1 coverage report(s)!

Coverage Report #1:
  ID: cov_abc123
  Format: json
  Size: 45678 bytes
  Created: 2024-01-15T10:30:00Z

Coverage Statistics:
  Line Coverage: 78.5%
  Function Coverage: 85.0%
  Branch Coverage: 72.3%
  Total Lines: 234
  Covered Lines: 184
  Total Functions: 20
  Covered Functions: 17
```

## Notes

- Coverage collection adds ~5-15% overhead to fuzzing performance
- Large binaries may take longer to process coverage data
- HTML reports can be quite large for complex codebases
- Some fuzzer configurations may not support all coverage formats