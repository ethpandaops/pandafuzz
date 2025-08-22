# Real Coverage Instrumentation Guide for PandaFuzz

## Overview
This guide explains how to get real coverage instrumentation working for AFL++ and LibFuzzer in PandaFuzz. Coverage instrumentation allows fuzzers to track which parts of the code are exercised during fuzzing, enabling coverage-guided fuzzing and detailed coverage reports.

## AFL++ Real Instrumentation

### Requirements for AFL++ Coverage

1. **Binary Compilation with AFL++ Instrumentation**
   - Target binary MUST be compiled with AFL++ compiler wrappers
   - Use `afl-clang-fast` or `afl-gcc` for compilation
   - NOT regular clang/gcc

2. **Coverage-Specific Compilation Flags**
   ```bash
   # For edge coverage (standard AFL++)
   afl-clang-fast -o target target.c
   
   # For LLVM coverage (more detailed, compatible with llvm-cov)
   AFL_USE_ASAN=1 AFL_LLVM_INSTRUMENT=CLASSIC afl-clang-fast \
     -fprofile-instr-generate -fcoverage-mapping \
     -o target target.c
   
   # For maximum coverage information
   AFL_LLVM_DOCUMENT_IDS=1 AFL_LLVM_CMPLOG=1 afl-clang-fast \
     -g -O0 -fprofile-instr-generate -fcoverage-mapping \
     -o target target.c
   ```

3. **Environment Variables During Fuzzing**
   ```bash
   export AFL_SKIP_CPUFREQ=1           # Skip CPU frequency check
   export AFL_NO_AFFINITY=1            # Don't bind to specific CPU cores
   export AFL_LLVM_DOCUMENT_IDS=1      # Track edge IDs for coverage
   export AFL_LLVM_MAP_ADDR=0x10000    # Fixed map address for consistency
   export AFL_MAP_SIZE=65536            # Standard map size
   ```

### How AFL++ Coverage Works

1. **Bitmap Coverage**: AFL++ uses a 64KB bitmap to track edge coverage
   - Each edge (branch transition) maps to a byte in the bitmap
   - Bitmap shows which edges were hit during execution
   - Located at: `{output_dir}/fuzz_bitmap` (when saved)

2. **Plot Data**: Contains coverage statistics over time
   - Located at: `{output_dir}/plot_data`
   - Format: CSV with edges_found, map_size percentage

3. **Queue Directory**: Contains interesting inputs that increased coverage
   - Each file represents a unique coverage pattern
   - Can be analyzed to understand coverage growth

### Extracting Real AFL++ Coverage

```go
// In coverage_extractor.go, read actual AFL++ data:

func (ce *CoverageExtractor) ExtractBitmapCoverage(ctx context.Context, outputDir string) (*CoverageData, error) {
    // 1. Read plot_data for coverage statistics
    plotData := filepath.Join(outputDir, "plot_data")
    stats := parseAFLPlotData(plotData)
    
    // 2. Count queue entries for path coverage
    queueDir := filepath.Join(outputDir, "queue")
    queueCount := countQueueEntries(queueDir)
    
    // 3. If binary was compiled with LLVM coverage, extract profraw
    profraws := findProfrawFiles(outputDir)
    if len(profraws) > 0 {
        // Merge profraw files and generate LCOV
        mergedProfdata := mergeProfraws(profraws)
        lcovData := generateLCOV(mergedProfdata, targetBinary)
    }
    
    // 4. Calculate real coverage percentage
    coverage := float64(stats.EdgesFound) / float64(65536) * 100
    
    return &CoverageData{
        Edges:           stats.EdgesFound,
        TotalEdges:      65536,
        CoveragePercent: coverage,
        PathsTotal:      queueCount,
    }, nil
}
```

### Common AFL++ Coverage Issues & Solutions

| Issue | Cause | Solution |
|-------|-------|----------|
| 0% coverage shown | Binary not instrumented | Recompile with `afl-clang-fast` |
| No bitmap file | AFL++ not configured for bitmap export | Set `AFL_LLVM_DOCUMENT_IDS=1` |
| No profraw files | Missing LLVM coverage flags | Add `-fprofile-instr-generate -fcoverage-mapping` |
| Coverage not increasing | Binary too simple or optimized | Compile with `-O0` and add more branches |

## LibFuzzer Real Instrumentation

### Requirements for LibFuzzer Coverage

1. **Binary Compilation with LibFuzzer**
   ```bash
   # Basic LibFuzzer with coverage
   clang++ -fsanitize=fuzzer,address \
           -fprofile-instr-generate -fcoverage-mapping \
           -g -O1 \
           -o fuzzer fuzzer.cc
   
   # Maximum coverage detail
   clang++ -fsanitize=fuzzer,address \
           -fprofile-instr-generate -fcoverage-mapping \
           -mllvm -runtime-counter-relocation \
           -g -O0 \
           -o fuzzer fuzzer.cc
   ```

2. **Required Function Signature**
   ```cpp
   extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
       // Your fuzzing logic here
       return 0;
   }
   ```

3. **Environment Variables**
   ```bash
   export LLVM_PROFILE_FILE="coverage-%p-%m.profraw"  # Profile output pattern
   export ASAN_OPTIONS=coverage=1:coverage_dir=/path/to/coverage
   ```

### How LibFuzzer Coverage Works

1. **Profraw Files**: Raw coverage data written during execution
   - Pattern: `coverage-{pid}-{hash}.profraw`
   - Contains hit counts for each basic block

2. **Profdata Merge**: Combine multiple profraw files
   ```bash
   llvm-profdata merge -sparse *.profraw -o coverage.profdata
   ```

3. **Coverage Reports**: Generate from profdata
   ```bash
   # LCOV format
   llvm-cov export -format=lcov \
            -instr-profile=coverage.profdata \
            ./fuzzer > coverage.lcov
   
   # JSON format
   llvm-cov export -format=json \
            -instr-profile=coverage.profdata \
            ./fuzzer > coverage.json
   ```

### Extracting Real LibFuzzer Coverage

```go
func (ce *CoverageExtractor) ExtractProfdataCoverage(profdataPath, binaryPath string) (*CoverageData, error) {
    // 1. Find all profraw files
    profraws, _ := filepath.Glob(filepath.Join(workDir, "*.profraw"))
    
    // 2. Merge profraw files
    cmd := exec.Command("llvm-profdata", "merge", "-sparse", "-o", "merged.profdata")
    cmd.Args = append(cmd.Args, profraws...)
    cmd.Run()
    
    // 3. Generate LCOV report
    cmd = exec.Command("llvm-cov", "export",
        "-format=lcov",
        "-instr-profile=merged.profdata",
        binaryPath)
    lcovData, _ := cmd.Output()
    
    // 4. Parse LCOV for statistics
    stats := parseLCOV(lcovData)
    
    return &CoverageData{
        LineCoverage:     stats.LinePercentage,
        FunctionCoverage: stats.FunctionPercentage,
        BranchCoverage:   stats.BranchPercentage,
    }, nil
}
```

### Common LibFuzzer Coverage Issues & Solutions

| Issue | Cause | Solution |
|-------|-------|----------|
| No profraw files | Missing instrumentation flags | Add `-fprofile-instr-generate -fcoverage-mapping` |
| llvm-profdata fails | Version mismatch | Use same LLVM version for compile and analysis |
| Empty coverage report | Binary stripped | Compile with `-g` flag |
| Coverage at 0% | Wrong binary path | Use absolute path to instrumented binary |

## Implementation Steps for Real Coverage

### 1. Update Dockerfile for Better Instrumentation Support

```dockerfile
# Already done in current Dockerfile, but ensure:
- LLVM 14 installed with all tools
- AFL++ built with LLVM support
- LibFuzzer development packages installed
- Coverage tools (lcov, llvm-cov, llvm-profdata) available
```

### 2. Modify Job Submission to Include Compilation Flags

```go
// In job creation, add compilation instructions
type JobConfig struct {
    // ... existing fields ...
    CompilationFlags []string `json:"compilation_flags,omitempty"`
    InstrumentationType string `json:"instrumentation_type"` // "afl++", "libfuzzer", "both"
}
```

### 3. Create Pre-Fuzzing Binary Instrumentation Step

```bash
#!/bin/bash
# pre-instrument.sh - Run before fuzzing

BINARY=$1
FUZZER_TYPE=$2
OUTPUT=$3

case $FUZZER_TYPE in
    "afl++")
        AFL_LLVM_DOCUMENT_IDS=1 afl-clang-fast \
            -g -O0 \
            -fprofile-instr-generate -fcoverage-mapping \
            -o "$OUTPUT" "$BINARY"
        ;;
    "libfuzzer")
        clang++ -fsanitize=fuzzer,address \
            -fprofile-instr-generate -fcoverage-mapping \
            -g -O1 \
            -o "$OUTPUT" "$BINARY"
        ;;
esac
```

### 4. Update Coverage Extractors to Use Real Data

In `aflplusplus/coverage_extractor.go`:
```go
// Check for real coverage data first
if plotDataExists {
    // Parse real AFL++ plot_data
    coverage = parseRealAFLCoverage(plotDataPath)
} else {
    // Fall back to synthetic data
    coverage = generateSyntheticCoverage()
}
```

### 5. Add Coverage Validation Tests

```bash
# validate-coverage.sh
#!/bin/bash

# Test AFL++ instrumentation
echo "Testing AFL++ instrumentation..."
cat > test.c << 'EOF'
#include <unistd.h>
int main() {
    char buf[10];
    read(0, buf, 10);
    if (buf[0] == 'A') return 1;
    return 0;
}
EOF

afl-clang-fast -o test_afl test.c
echo "A" | AFL_SKIP_CPUFREQ=1 afl-fuzz -i in -o out -V 1 -- ./test_afl
if [ -f out/plot_data ]; then
    echo "✓ AFL++ instrumentation working"
else
    echo "✗ AFL++ instrumentation failed"
fi

# Test LibFuzzer instrumentation
echo "Testing LibFuzzer instrumentation..."
cat > test_fuzz.cc << 'EOF'
#include <stdint.h>
#include <stddef.h>
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    if (size > 0 && data[0] == 'A') return 1;
    return 0;
}
EOF

clang++ -fsanitize=fuzzer -fprofile-instr-generate -fcoverage-mapping -o test_libfuzzer test_fuzz.cc
LLVM_PROFILE_FILE="test.profraw" timeout 1 ./test_libfuzzer -runs=100 2>/dev/null
if [ -f test.profraw ]; then
    echo "✓ LibFuzzer instrumentation working"
else
    echo "✗ LibFuzzer instrumentation failed"
fi
```

## Quick Start: Enable Real Coverage

### For AFL++:
1. Ensure target is compiled with `afl-clang-fast`
2. Set environment: `export AFL_LLVM_DOCUMENT_IDS=1`
3. Run fuzzing normally
4. Coverage data will be in plot_data and queue/

### For LibFuzzer:
1. Ensure target is compiled with `-fsanitize=fuzzer -fprofile-instr-generate -fcoverage-mapping`
2. Set environment: `export LLVM_PROFILE_FILE="coverage-%p.profraw"`
3. Run fuzzing normally
4. Merge profraw files with `llvm-profdata merge`
5. Generate reports with `llvm-cov export`

## Testing Coverage Locally

```bash
# Test with the fixed script using real instrumentation
./scripts/run-coverage-test-fixed.sh afl++

# Verify real coverage files
docker exec pandafuzz-bot-1 cat /app/work/jobs/job_*/output/afl_output/plot_data | grep edges_found

# For LibFuzzer
./scripts/run-coverage-test-fixed.sh libfuzzer
docker exec pandafuzz-bot-1 ls /app/work/jobs/job_*/*.profraw
```

## Troubleshooting Checklist

- [ ] Is the binary compiled with the fuzzer's compiler wrapper?
- [ ] Are coverage flags included during compilation?
- [ ] Are environment variables set correctly?
- [ ] Is the LLVM version consistent across tools?
- [ ] Are coverage tools installed and in PATH?
- [ ] Is the binary not stripped (has debug symbols)?
- [ ] Is the output directory writable?
- [ ] Are profraw files being generated?
- [ ] Can llvm-profdata merge the files?
- [ ] Can llvm-cov read the instrumented binary?

## Summary

Real coverage instrumentation requires:
1. **Proper compilation** with fuzzer-specific tools and flags
2. **Correct environment** variables during execution
3. **Post-processing tools** to extract and format coverage data
4. **Consistent toolchain** versions (especially LLVM)

The current PandaFuzz setup has all necessary tools installed. The main requirement is ensuring submitted binaries are properly instrumented, or implementing automatic re-instrumentation before fuzzing.