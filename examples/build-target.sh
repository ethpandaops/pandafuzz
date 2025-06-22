#!/bin/bash
# Build the vulnerable target program

echo "Building vulnerable target program..."

cd "$(dirname "$0")"

# Build the Docker image
docker build -t pandafuzz-target -f Dockerfile.target .

# Extract the compiled binaries and corpus
docker create --name temp-target pandafuzz-target
docker cp temp-target:/targets/vulnerable_afl ./
docker cp temp-target:/targets/vulnerable_libfuzzer ./
docker cp temp-target:/targets/vuln.dict ./
docker cp temp-target:/corpus ./
docker rm temp-target

echo "Target binaries built successfully!"
echo ""
echo "Files created:"
echo "  - vulnerable_afl (for AFL++ fuzzing)"
echo "  - vulnerable_libfuzzer (for LibFuzzer)"
echo "  - vuln.dict (dictionary file)"
echo "  - corpus/ (seed corpus directory)"
echo ""
echo "The vulnerable program has:"
echo "  1. Buffer overflow vulnerability (input > 64 chars)"
echo "  2. Crash trigger on input starting with 'CRASH'"
echo "  3. Array out-of-bounds on input containing 'BUG' at position 10-12"