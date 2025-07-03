#!/bin/bash
# LibFuzzer target wrapper for test_crash program
# This wrapper is needed because libfuzzer expects the target to read from a file passed as argument

if [ $# -eq 0 ]; then
    echo "Usage: $0 <input_file>"
    exit 1
fi

# Read the input file and pass it to our test program via stdin
cat "$1" | /app/test_crash