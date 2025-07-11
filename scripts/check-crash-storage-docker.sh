#!/bin/bash
# Script to validate crash storage in PandaFuzz database inside Docker container

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default container name (adjust if needed)
CONTAINER_NAME="${PANDAFUZZ_CONTAINER:-pandafuzz-master-1}"
DB_PATH="${PANDAFUZZ_DB:-/app/data/pandafuzz.db}"

echo "=== PandaFuzz Crash Storage Validation (Docker) ==="
echo "Container: $CONTAINER_NAME"
echo "Database: $DB_PATH"
echo

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}Error: Container '$CONTAINER_NAME' is not running${NC}"
    echo "Available containers:"
    docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
    echo
    echo "Set PANDAFUZZ_CONTAINER environment variable to the correct container name"
    exit 1
fi

# Function to run a query in the container
run_query() {
    local title="$1"
    local query="$2"
    echo -e "${GREEN}$title${NC}"
    docker exec "$CONTAINER_NAME" sqlite3 -header -column "$DB_PATH" "$query"
    echo
}

# Function to run a query and capture output
get_query_result() {
    docker exec "$CONTAINER_NAME" sqlite3 "$DB_PATH" "$1" 2>/dev/null
}

# 1. Check if database exists in container
echo -e "${YELLOW}1. Checking Database${NC}"
if ! docker exec "$CONTAINER_NAME" test -f "$DB_PATH"; then
    echo -e "${RED}Error: Database not found at $DB_PATH in container${NC}"
    echo "Checking for database files in container:"
    docker exec "$CONTAINER_NAME" find /app -name "*.db" -type f 2>/dev/null
    exit 1
fi
echo -e "${GREEN}Database found${NC}"
echo

# 2. Check schema
echo -e "${YELLOW}2. Database Schema Check${NC}"
docker exec "$CONTAINER_NAME" sqlite3 "$DB_PATH" ".schema crash_inputs" 2>/dev/null
if [ $? -ne 0 ]; then
    echo -e "${RED}Warning: crash_inputs table may not exist${NC}"
fi
echo

# 3. Overall statistics
run_query "3. Overall Crash Statistics" "
SELECT 
    'Total Crashes' as metric,
    COUNT(*) as count
FROM crashes
UNION ALL
SELECT 
    'Crashes with Input Data' as metric,
    COUNT(*) as count
FROM crash_inputs;"

# 4. Recent crashes
run_query "4. Recent Crashes (Last 10)" "
SELECT 
    substr(c.id, 1, 20) as crash_id,
    substr(c.job_id, 1, 20) as job_id,
    datetime(c.timestamp) as time,
    c.type,
    CASE 
        WHEN ci.crash_id IS NOT NULL THEN 'Yes'
        ELSE 'No'
    END as has_input,
    CASE 
        WHEN ci.crash_id IS NOT NULL THEN LENGTH(ci.input)
        ELSE 0
    END as input_bytes
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
ORDER BY c.timestamp DESC
LIMIT 10;"

# 5. Check for crashes without input
echo -e "${YELLOW}5. Crashes Without Input Data${NC}"
NO_INPUT_COUNT=$(get_query_result "
SELECT COUNT(*) 
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE ci.crash_id IS NULL;")

if [ "$NO_INPUT_COUNT" -gt 0 ]; then
    echo -e "${RED}Found $NO_INPUT_COUNT crashes without input data${NC}"
    run_query "First 5 crashes without input:" "
    SELECT 
        substr(c.id, 1, 30) as crash_id,
        datetime(c.timestamp) as time,
        c.type
    FROM crashes c
    LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
    WHERE ci.crash_id IS NULL
    ORDER BY c.timestamp DESC
    LIMIT 5;"
else
    echo -e "${GREEN}All crashes have input data stored!${NC}"
    echo
fi

# 6. Job summary
run_query "6. Crash Storage by Job (Top 5)" "
SELECT 
    substr(c.job_id, 1, 30) as job_id,
    COUNT(DISTINCT c.id) as total_crashes,
    COUNT(DISTINCT ci.crash_id) as with_input,
    ROUND(COUNT(DISTINCT ci.crash_id) * 100.0 / COUNT(DISTINCT c.id), 2) as pct_with_input
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
GROUP BY c.job_id
ORDER BY total_crashes DESC
LIMIT 5;"

# 7. Check specific job if provided as argument
if [ ! -z "$1" ]; then
    echo -e "${YELLOW}7. Checking specific job: $1${NC}"
    run_query "Crashes for job $1:" "
    SELECT 
        c.id,
        datetime(c.timestamp) as time,
        c.type,
        CASE WHEN ci.crash_id IS NOT NULL THEN 'Yes' ELSE 'No' END as has_input,
        LENGTH(ci.input) as input_size
    FROM crashes c
    LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
    WHERE c.job_id = '$1'
    ORDER BY c.timestamp DESC;"
fi

# 8. Interactive SQL mode
echo -e "${YELLOW}For interactive SQL queries:${NC}"
echo "docker exec -it $CONTAINER_NAME sqlite3 $DB_PATH"
echo

# 9. Export crash to host
echo -e "${YELLOW}To export a crash input to your host:${NC}"
echo "# First, export inside container:"
echo "docker exec $CONTAINER_NAME sqlite3 $DB_PATH \"SELECT writefile('/tmp/crash.bin', input) FROM crash_inputs WHERE crash_id = 'YOUR_CRASH_ID';\""
echo "# Then copy to host:"
echo "docker cp $CONTAINER_NAME:/tmp/crash.bin ./crash.bin"
echo

echo "=== Validation Complete ==="