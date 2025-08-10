#!/bin/bash
# Simple script to compile binaries with coverage instrumentation on local machine
# Uses the working afl-gcc-fast for AFL++ and clang for LibFuzzer

set -e

SOURCE_FILE="$1"
OUTPUT_FILE="${2:-coverage_test}"
FUZZER_TYPE="${3:-afl++}"

if [ -z "$SOURCE_FILE" ]; then
    echo "Usage: $0 <source_file> [output_file] [afl++|libfuzzer]"
    echo ""
    echo "Compiles binaries with real coverage instrumentation for PandaFuzz"
    exit 1
fi

echo "Compiling $SOURCE_FILE for $FUZZER_TYPE..."

case "$FUZZER_TYPE" in
    "afl++")
        # Use afl-gcc-fast which works and provides real coverage
        if [ -f /usr/local/bin/afl-gcc-fast ]; then
            echo "Using afl-gcc-fast (provides real GCC coverage)"
            AFL_DONT_OPTIMIZE=1 /usr/local/bin/afl-gcc-fast \
                -g -O0 \
                -fprofile-arcs -ftest-coverage \
                -o "$OUTPUT_FILE" "$SOURCE_FILE"
        elif [ -f /usr/local/bin/afl-gcc ]; then
            echo "Using afl-gcc (basic instrumentation)"
            /usr/local/bin/afl-gcc -g -O0 -o "$OUTPUT_FILE" "$SOURCE_FILE"
        else
            echo "Error: No AFL++ compiler found"
            exit 1
        fi
        
        echo "✓ Compiled with AFL++ instrumentation"
        if nm "$OUTPUT_FILE" | grep -q "__gcov"; then
            echo "✓ GCC coverage instrumentation detected"
        fi
        ;;
        
    "libfuzzer")
        # Use clang with LibFuzzer
        COMPILER="clang++"
        if [[ "$SOURCE_FILE" == *.c ]]; then
            COMPILER="clang"
        fi
        
        if ! grep -q "LLVMFuzzerTestOneInput" "$SOURCE_FILE"; then
            echo "Error: Source must contain LLVMFuzzerTestOneInput function"
            exit 1
        fi
        
        $COMPILER -fsanitize=fuzzer,address \
            -fprofile-instr-generate -fcoverage-mapping \
            -g -O1 \
            -o "$OUTPUT_FILE" "$SOURCE_FILE"
        
        echo "✓ Compiled with LibFuzzer instrumentation"
        if nm "$OUTPUT_FILE" | grep -q "__llvm_prof"; then
            echo "✓ LLVM coverage instrumentation detected"
        fi
        ;;
        
    *)
        echo "Error: Use 'afl++' or 'libfuzzer'"
        exit 1
        ;;
esac

echo ""
echo "Binary ready: $OUTPUT_FILE"
echo "Upload to PandaFuzz for fuzzing with coverage!"