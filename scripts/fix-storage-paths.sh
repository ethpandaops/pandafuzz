#!/bin/bash
# Script to fix storage path issues in pandafuzz containers

set -e

echo "=== Fixing PandaFuzz Storage Paths ==="

# Check if containers are running
if ! docker compose ps | grep -q "pandafuzz-master"; then
    echo "Master container not running. Starting containers..."
    docker compose up -d
    sleep 5
fi

echo ""
echo "1. Creating storage directories in master container..."
docker compose exec master bash -c '
    # Create all required storage directories
    mkdir -p /app/data/{binaries,corpus,crashes,logs,backups,temp}
    
    # Set proper permissions
    chown -R pandafuzz:pandafuzz /app/data
    chmod -R 755 /app/data
    
    # List created directories
    echo "Created directories:"
    ls -la /app/data/
'

echo ""
echo "2. Checking for existing binaries..."
docker compose exec master bash -c '
    # Check both potential locations
    echo "Checking /app/storage/binaries:"
    ls -la /app/storage/binaries 2>/dev/null || echo "Directory not found"
    
    echo ""
    echo "Checking /app/data/binaries:"
    ls -la /app/data/binaries 2>/dev/null || echo "Directory not found"
'

echo ""
echo "3. Migrating any existing binaries..."
docker compose exec master bash -c '
    # If old storage directory exists, migrate files
    if [ -d "/app/storage/binaries" ] && [ -d "/app/data/binaries" ]; then
        echo "Migrating binaries from /app/storage to /app/data..."
        cp -rv /app/storage/binaries/* /app/data/binaries/ 2>/dev/null || echo "No binaries to migrate"
    fi
'

echo ""
echo "4. Creating work directories in bot container..."
docker compose exec bot bash -c '
    # Create work directory structure
    mkdir -p /app/work/jobs
    
    # Set permissions
    chown -R $(id -u):$(id -g) /app/work
    chmod -R 755 /app/work
    
    # List created directories
    echo "Created directories:"
    ls -la /app/work/
'

echo ""
echo "5. Testing binary download mechanism..."
# Create a test binary
cat > /tmp/test_fuzzer.cc << 'EOF'
#include <stdint.h>
#include <stddef.h>
extern "C" int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size) {
    return 0;
}
EOF

# Compile test binary on host if clang is available
if command -v clang++ &> /dev/null; then
    echo "Compiling test binary..."
    clang++ -fsanitize=fuzzer /tmp/test_fuzzer.cc -o /tmp/test_fuzzer
    
    # Copy to master container
    docker cp /tmp/test_fuzzer pandafuzz-master:/tmp/test_fuzzer
    
    # Move to binaries directory
    docker compose exec master bash -c '
        mv /tmp/test_fuzzer /app/data/binaries/test_fuzzer_$(date +%s)
        chmod 755 /app/data/binaries/test_fuzzer_*
        echo "Test binary stored at:"
        ls -la /app/data/binaries/test_fuzzer_*
    '
else
    echo "Clang not available on host, skipping test binary creation"
fi

echo ""
echo "6. Restarting containers to apply changes..."
docker compose restart master bot

echo ""
echo "7. Waiting for services to be ready..."
sleep 10

echo ""
echo "8. Verifying setup..."
echo "Master storage:"
docker compose exec master bash -c 'ls -la /app/data/'

echo ""
echo "Bot work directory:"
docker compose exec bot bash -c 'ls -la /app/work/'

echo ""
echo "=== Storage paths fixed! ==="
echo ""
echo "Next steps:"
echo "1. Upload a binary through the web UI at http://localhost:8088"
echo "2. Check that binary is stored in master at: /app/data/binaries/"
echo "3. Create and run a job to verify binary download to bot"
echo "4. Monitor logs with: docker compose logs -f bot"