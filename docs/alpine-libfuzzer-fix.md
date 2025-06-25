# Alpine Linux LibFuzzer Compatibility Fix

## Problem Description

When running libFuzzer-instrumented binaries in Alpine Linux Docker containers, the fuzzer crashes immediately with a segmentation fault (SIGSEGV) at address 0x000000000000. This is caused by incompatibilities between:

1. Alpine's musl libc vs glibc
2. How libFuzzer's runtime is linked
3. Missing or incompatible runtime libraries

## Root Cause

The crash occurs because:
- LibFuzzer attempts to call a function pointer that is NULL (pc=0x000000000000)
- This happens during initialization before fuzzing even starts
- Alpine's musl libc has different symbol resolution and initialization behavior than glibc

## Solutions Implemented

### 1. Dockerfile Updates

Updated the bot Dockerfile to include additional runtime dependencies:
```dockerfile
# Additional runtime libraries for libfuzzer
clang-libs
libc++
libc++-dev
libgcc
```

### 2. Executor Code Updates

The executor now:
- Detects Alpine Linux environments
- Applies specific workarounds for libFuzzer binaries
- Falls back to simulated execution if the binary is incompatible
- Sets environment variables to help with sanitizer compatibility

### 3. Alternative Compilation Methods

Created a new Dockerfile (`Dockerfile.target.alpine.fixed`) that tries multiple compilation strategies:
- Standard compilation with explicit runtime linking
- Static linking to avoid runtime issues
- Using clang++ with libc++
- Minimal fuzzer without address sanitizer

## Usage

### Building Compatible Binaries

For Alpine containers, use one of these compilation methods:

```bash
# Method 1: Minimal fuzzer (most compatible)
clang -g -O1 -fsanitize=fuzzer harness.c target.c -o fuzzer

# Method 2: Static linking
clang -g -O1 -fsanitize=fuzzer,address -static-libasan -static-libgcc harness.c target.c -o fuzzer

# Method 3: With libc++ (for C++ code)
clang++ -g -O1 -fsanitize=fuzzer,address -stdlib=libc++ harness.cpp target.cpp -o fuzzer
```

### Running the Debug Script

To diagnose libfuzzer issues in Alpine:

```bash
docker run -it --rm -v $(pwd)/scripts:/scripts alpine:3.19 sh
apk add --no-cache clang clang-dev compiler-rt llvm llvm-dev libc++ libc++-dev
/scripts/debug-alpine-fuzzer.sh
```

### Fallback Behavior

If a libFuzzer binary is incompatible with the Alpine environment:
1. The executor detects the crash (exit codes -11, -4, or -6)
2. Logs diagnostic information about the failure
3. Automatically falls back to simulated fuzzing
4. Continues operation without blocking the job

## Best Practices

1. **Test binaries locally**: Before deploying, test fuzzer binaries in the target Alpine container
2. **Use minimal instrumentation**: Start with `-fsanitize=fuzzer` only, add sanitizers if they work
3. **Consider alternatives**: AFL++ works more reliably in Alpine than libFuzzer
4. **Monitor logs**: Check job logs for "falling back to simulated execution" messages

## Future Improvements

Consider:
1. Using a glibc-based image (e.g., debian:slim) for better libFuzzer compatibility
2. Building a custom Alpine image with glibc support
3. Creating a compatibility layer for libFuzzer binaries
4. Implementing automatic binary recompilation in the container