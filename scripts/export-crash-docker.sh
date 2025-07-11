#!/bin/bash
# Export a specific crash from Docker container to local file

if [ $# -lt 1 ]; then
    echo "Usage: $0 <crash_id> [container_name]"
    echo "Example: $0 crash-a1f2925a2506ed803347072619283e2a3efd538f"
    exit 1
fi

CRASH_ID="$1"
CONTAINER="${2:-pandafuzz-master-1}"
DB_PATH="/app/data/pandafuzz.db"
OUTPUT_FILE="crash_${CRASH_ID}.bin"

echo "Exporting crash: $CRASH_ID"
echo "From container: $CONTAINER"
echo "To file: $OUTPUT_FILE"
echo

# Check if crash exists
EXISTS=$(docker exec "$CONTAINER" sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM crash_inputs WHERE crash_id = '$CRASH_ID';")

if [ "$EXISTS" -eq 0 ]; then
    echo "Error: Crash input not found for ID: $CRASH_ID"
    echo
    echo "Available crashes with input:"
    docker exec "$CONTAINER" sqlite3 -box "$DB_PATH" "
    SELECT substr(crash_id, 1, 50) as crash_id 
    FROM crash_inputs 
    ORDER BY crash_id DESC 
    LIMIT 10;"
    exit 1
fi

# Export the crash
echo "Exporting crash data..."
docker exec "$CONTAINER" sqlite3 "$DB_PATH" "SELECT writefile('/tmp/crash_export.bin', input) FROM crash_inputs WHERE crash_id = '$CRASH_ID';"

# Copy to host
docker cp "$CONTAINER:/tmp/crash_export.bin" "./$OUTPUT_FILE"

# Clean up container
docker exec "$CONTAINER" rm -f /tmp/crash_export.bin

# Show file info
if [ -f "$OUTPUT_FILE" ]; then
    echo "Success! Crash exported to: $OUTPUT_FILE"
    echo "File size: $(ls -lh "$OUTPUT_FILE" | awk '{print $5}')"
    echo "File type: $(file -b "$OUTPUT_FILE")"
else
    echo "Error: Failed to export crash"
    exit 1
fi