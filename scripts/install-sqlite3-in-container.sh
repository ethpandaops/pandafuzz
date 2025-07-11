#!/bin/bash
# Script to install sqlite3 in the running container (temporary solution)

CONTAINER="${PANDAFUZZ_CONTAINER:-pandafuzz-master}"

echo "Installing sqlite3 in container: $CONTAINER"
echo "Note: This is temporary and will be lost when container restarts"
echo

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER}$"; then
    echo "Error: Container '$CONTAINER' is not running"
    exit 1
fi

# Install sqlite3
echo "Updating package list..."
docker exec "$CONTAINER" apt-get update -qq

echo "Installing sqlite3..."
docker exec "$CONTAINER" apt-get install -y sqlite3

if [ $? -eq 0 ]; then
    echo "Success! sqlite3 installed in container"
    echo "You can now use the Docker query scripts"
else
    echo "Failed to install sqlite3"
    exit 1
fi