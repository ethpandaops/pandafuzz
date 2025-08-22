# PandaFuzz Coverage Collection Guide

## Overview

PandaFuzz supports comprehensive code coverage collection across multiple fuzzing engines (AFL++, LibFuzzer, and Honggfuzz). This guide explains how to enable coverage collection, interpret results, and troubleshoot common issues.

## Table of Contents

1. [Coverage Overview](#coverage-overview)
2. [Requirements by Fuzzer Type](#requirements-by-fuzzer-type)
3. [Enabling Coverage Collection](#enabling-coverage-collection)
4. [Coverage Formats](#coverage-formats)
5. [Running Coverage Tests](#running-coverage-tests)
6. [Interpreting Coverage Reports](#interpreting-coverage-reports)
7. [Troubleshooting](#troubleshooting)
8. [Performance Considerations](#performance-considerations)

## Coverage Overview

Coverage collection in PandaFuzz works through a multi-stage process:

1. **Binary Instrumentation**: Target binaries are compiled with coverage instrumentation
2. **Runtime Collection**: Fuzzers collect coverage data during execution
3. **Data Processing**: Coverage data is processed into standard formats
4. **Report Generation**: Human-readable reports are generated
5. **API Access**: Coverage data is accessible via the PandaFuzz API

### Coverage Metrics

PandaFuzz tracks several coverage metrics:
- **Line Coverage**: Percentage of source code lines executed
- **Function Coverage**: Percentage of functions called
- **Branch Coverage**: Percentage of conditional branches taken (when available)
- **Basic Block Coverage**: Low-level execution path coverage

## Requirements by Fuzzer Type

### AFL++

**Required Tools**:
- `afl-clang-fast` or `afl-gcc` (AFL++ compilers)
- `llvm-profdata` and `llvm-cov` (for LLVM mode coverage)
- `afl-cov` (optional, for web-based reports)

**Compilation**:
```bash
# LLVM mode (recommended)
afl-clang-fast -g -O0 -o target target.c

# GCC mode
afl-gcc -g -O0 -o target target.c
```

**Environment Variables**:
- `AFL_LLVM_DOCUMENT_IDS=1` - Better coverage tracking in LLVM mode
- `AFL_LLVM_CMPLOG=1` - Enhanced edge coverage

### LibFuzzer

**Required Tools**:
- `clang++` with LibFuzzer support (`-fsanitize=fuzzer`)
- `llvm-profdata` and `llvm-cov` (must match clang version)

**Compilation**:
```bash
# With coverage instrumentation
clang++ -g -O1 -fsanitize=fuzzer,address \
        -fprofile-instr-generate -fcoverage-mapping \
        -o target target.cc
```

**Environment Variables**:
- `LLVM_PROFILE_FILE` - Set by PandaFuzz to specify profile output location

### Honggfuzz

**Required Tools**:
- `honggfuzz` compiler wrapper
- `sancov` tool (for sanitizer coverage)
- Sanitizer libraries (ASan, MSan, etc.)

**Compilation**:
```bash
# With sanitizer coverage
hfuzz-clang++ -g -O1 -fsanitize=address \
              -fsanitize-coverage=trace-pc-guard \
              -o target target.cc
```

## Enabling Coverage Collection

### 1. Validate Coverage Tools

Before running coverage tests, validate your environment:

```bash
./scripts/validate-coverage-tools.sh
```

This script checks for:
- Required compiler tools
- Coverage processing utilities
- Version compatibility
- Optional enhancement tools

### 2. Compile with Coverage Instrumentation

Use the appropriate compiler flags for your fuzzer:

```bash
# AFL++ (avoid mixing with LLVM coverage flags)
afl-clang-fast -g -O0 -o fuzzer fuzzer.c

# LibFuzzer (always include fuzzer runtime)
clang++ -g -O1 -fsanitize=fuzzer -fprofile-instr-generate \
        -fcoverage-mapping -o fuzzer fuzzer.cc
```

### 3. Create Job with Coverage Enabled

When creating a fuzzing job, enable coverage in the configuration:

```json
{
  "name": "Coverage Test Job",
  "fuzzer": "libfuzzer",
  "config": {
    "coverage": {
      "enabled": true,
      "format": "lcov"
    }
  }
}
```

## Coverage Formats

PandaFuzz supports multiple coverage report formats:

### LCOV Format
- **Extension**: `.lcov`
- **Description**: Standard text format, compatible with many tools
- **Use Case**: Integration with CI/CD, genhtml for HTML reports
- **View**: Use `genhtml coverage.lcov -o html_report/`

### JSON Format
- **Extension**: `.json`
- **Description**: Machine-readable format with detailed metrics
- **Use Case**: Programmatic analysis, custom tooling
- **Structure**:
  ```json
  {
    "files": [{
      "filename": "src/main.c",
      "line_coverage": 67.5,
      "covered_lines": 135,
      "total_lines": 200
    }]
  }
  ```

### HTML Format
- **Extension**: `.html`
- **Description**: Interactive web-based reports
- **Use Case**: Manual review, team sharing
- **Features**: Source code annotation, sortable metrics

### Profdata Format
- **Extension**: `.profdata`
- **Description**: Raw LLVM profile data
- **Use Case**: Advanced analysis with LLVM tools
- **Process**: Merge with `llvm-profdata`, analyze with `llvm-cov`

## Running Coverage Tests

### Docker vs Local Environment

PandaFuzz provides two ways to run coverage tests:

1. **Docker (Recommended)**: All tools pre-installed
2. **Local**: Requires manual tool installation

### Quick Test Script

#### Option 1: Automatic (Docker/Local)

Use the wrapper script that automatically detects and uses Docker if local tools are missing:

```bash
# Automatically uses Docker if needed
./scripts/coverage-test-wrapper.sh

# Test specific fuzzer
./scripts/coverage-test-wrapper.sh afl++
./scripts/coverage-test-wrapper.sh libfuzzer
```

#### Option 2: Direct Local Execution

If you have all tools installed locally:

```bash
# Test both AFL++ and LibFuzzer
./scripts/run-test-with-coverage.sh

# Test specific fuzzer
./scripts/run-test-with-coverage.sh afl++
./scripts/run-test-with-coverage.sh libfuzzer
```

#### Option 3: Docker Execution

To explicitly use Docker:

```bash
# Run in Docker container
docker run --rm \
    -v $(pwd):/pandafuzz \
    -w /pandafuzz \
    pandafuzz-bot:latest \
    /pandafuzz/scripts/run-test-with-coverage.sh
```

### Installing Tools Locally (Optional)

If you prefer to install AFL++ and coverage tools locally:

```bash
# Install AFL++ and coverage tools on your system
sudo ./scripts/install-afl-local.sh

# Verify installation
./scripts/validate-coverage-tools.sh
```

### Manual Coverage Test

1. **Create test binary** using templates:
   ```bash
   # Copy and customize template
   cp scripts/templates/libfuzzer_coverage_harness.cc mytest.cc
   # Add your test logic
   vim mytest.cc
   # Compile with coverage
   clang++ -g -O1 -fsanitize=fuzzer,address \
           -fprofile-instr-generate -fcoverage-mapping \
           -o mytest mytest.cc
   ```

2. **Create corpus collection**:
   ```bash
   curl -X POST http://localhost:8080/api/v1/corpus/collections \
        -H "Content-Type: application/json" \
        -d '{"name": "Coverage Test Corpus"}'
   ```

3. **Upload test binary and create job**:
   ```bash
   curl -X POST http://localhost:8080/api/v1/jobs/upload \
        -F "job_metadata={\"fuzzer\":\"libfuzzer\",\"config\":{\"coverage\":{\"enabled\":true,\"format\":\"lcov\"}}}" \
        -F "target_binary=@mytest"
   ```

### Docker Environment

The PandaFuzz Docker image includes all coverage tools:

```bash
# Run bot with coverage support
docker run -e COVERAGE_ENABLED=true pandafuzz/bot:latest

# Interactive coverage testing
docker run -it pandafuzz/bot:dev bash
# Inside container:
./scripts/run-test-with-coverage.sh
```

## Interpreting Coverage Reports

### Understanding Metrics

1. **Line Coverage**:
   - Shows which source lines were executed
   - Target: 70-80% for well-tested code
   - Red lines: Never executed
   - Green lines: Executed at least once

2. **Function Coverage**:
   - Shows which functions were called
   - Important for API completeness
   - Uncalled functions may be dead code

3. **Branch Coverage**:
   - Shows which conditional paths were taken
   - Critical for thorough testing
   - Both true/false branches should be covered

### Reading LCOV Reports

```bash
# Generate HTML from LCOV
genhtml coverage.lcov --output-directory coverage_report/

# View summary
lcov --summary coverage.lcov
```

Output example:
```
Reading data file coverage.lcov
Summary coverage rate:
  lines......: 72.3% (1234 of 1706 lines)
  functions..: 85.7% (42 of 49 functions)
  branches...: 61.2% (234 of 382 branches)
```

### Analyzing JSON Reports

```python
import json

with open('coverage.json') as f:
    data = json.load(f)
    
for file in data['files']:
    print(f"{file['filename']}: {file['line_coverage']:.1f}% coverage")
    if file['line_coverage'] < 50:
        print(f"  WARNING: Low coverage in {file['filename']}")
```

## Troubleshooting

### Common Issues and Solutions

#### 1. "No coverage reports found"

**Causes**:
- Coverage tools not installed on bot
- Binary not properly instrumented
- Job finished too quickly

**Solutions**:
- Run `validate-coverage-tools.sh` to check environment
- Verify compilation flags match fuzzer requirements
- Increase job duration (minimum 30 seconds recommended)

#### 2. "Failed to compile with coverage"

**AFL++ Issues**:
```bash
# Wrong: Mixing instrumentation types
afl-clang-fast -fprofile-instr-generate -fcoverage-mapping

# Correct: Use AFL++ native instrumentation
afl-clang-fast -g -O0
```

**LibFuzzer Issues**:
```bash
# Wrong: Missing fuzzer runtime
clang++ -fprofile-instr-generate -fcoverage-mapping

# Correct: Include fuzzer runtime
clang++ -fsanitize=fuzzer -fprofile-instr-generate -fcoverage-mapping
```

#### 3. "Coverage data corrupted"

**Causes**:
- Version mismatch between compiler and coverage tools
- Incomplete profile data

**Solutions**:
- Ensure LLVM tools are from same version:
  ```bash
  clang++ --version
  llvm-profdata --version
  llvm-cov --version
  ```
- Check for profile data corruption:
  ```bash
  llvm-profdata show --counts profile.profraw
  ```

#### 4. "Low coverage despite long fuzzing"

**Optimization Tips**:
- Use coverage-guided corpus:
  ```bash
  # Create diverse seed inputs
  echo "TEST" > corpus/seed1
  echo "ABCDEF" > corpus/seed2
  printf "\x00\x01\x02" > corpus/seed3
  ```
- Enable CMPLOG for AFL++:
  ```bash
  AFL_LLVM_CMPLOG=1 afl-clang-fast -o target target.c
  ```
- Check for initialization issues:
  ```c
  // Ensure fuzzer can reach all code paths
  if (size < 4) return 0;  // Don't exit too early
  ```

### Bot Configuration Issues

If coverage isn't collected on the bot:

1. **Check bot logs**:
   ```bash
   curl http://localhost:8080/api/v1/jobs/{job_id}/logs | jq '.logs[] | select(.message | contains("coverage"))'
   ```

2. **Verify bot environment**:
   - Bot should have coverage tools in PATH
   - Coverage directory must be writable
   - Sufficient disk space for profile data

3. **Test locally with Docker**:
   ```bash
   # Use same image as production bot
   docker run -it pandafuzz/bot:latest bash
   ./scripts/validate-coverage-tools.sh
   ```

## Performance Considerations

### Coverage Overhead

Coverage collection impacts performance:
- **Compilation**: 2-3x slower with instrumentation
- **Runtime**: 10-30% execution overhead
- **Memory**: Additional memory for coverage counters
- **Disk**: Profile data can be large (MB to GB)

### Optimization Strategies

1. **Sampling Coverage**:
   - Collect coverage periodically, not continuously
   - Use job configuration to control frequency

2. **Incremental Coverage**:
   - Focus on new code paths
   - Use differential coverage between runs

3. **Storage Management**:
   - Compress coverage reports
   - Retain only recent reports
   - Use cleanup policies

### Best Practices

1. **Development Workflow**:
   - Run coverage during development
   - Set coverage targets (e.g., 70% line coverage)
   - Review uncovered code in PR reviews

2. **CI/CD Integration**:
   - Automate coverage collection
   - Fail builds below coverage threshold
   - Track coverage trends over time

3. **Fuzzing Strategy**:
   - Start with coverage-guided fuzzing
   - Use coverage to identify weak areas
   - Create targeted test cases for gaps

## Advanced Topics

### Custom Coverage Processing

Create custom coverage processors:

```go
// Implement CoverageProcessor interface
type CustomProcessor struct{}

func (p *CustomProcessor) Process(data []byte) (*CoverageReport, error) {
    // Parse coverage data
    // Generate custom metrics
    // Return standardized report
}
```

### Coverage Diff Analysis

Compare coverage between runs:

```bash
# Generate coverage for baseline
llvm-cov export -format=lcov baseline.profdata > baseline.lcov

# Generate coverage for new version  
llvm-cov export -format=lcov new.profdata > new.lcov

# Compare coverage
lcov -a baseline.lcov -a new.lcov -o diff.lcov
```

### Integration with IDEs

Most IDEs support coverage visualization:
- **VSCode**: Coverage Gutters extension
- **CLion**: Built-in coverage support
- **Vim**: vim-coverage plugin

Configure your IDE to read PandaFuzz coverage reports for inline visualization.

## References

- [AFL++ Coverage Documentation](https://github.com/AFLplusplus/AFLplusplus/blob/stable/docs/fuzzing_in_depth.md#coverage)
- [LibFuzzer Coverage Guide](https://llvm.org/docs/LibFuzzer.html#coverage)
- [LCOV Documentation](https://github.com/linux-test-project/lcov)
- [PandaFuzz API Documentation](../api/README.md)