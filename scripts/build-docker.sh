#!/bin/bash
# Build Docker images with version information

set -e

# Get version info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building PandaFuzz Docker images..."
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME"
echo "Git Commit: $GIT_COMMIT"

# Export for docker-compose
export VERSION
export BUILD_TIME
export GIT_COMMIT

# Build with docker-compose
docker-compose build "$@"

echo "Build complete!"