#!/bin/bash
# Script to validate crash storage by copying database from container to local

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CONTAINER_NAME="${PANDAFUZZ_CONTAINER:-pandafuzz-master}"
DB_PATH_IN_CONTAINER="/app/data/pandafuzz.db"
LOCAL_DB="/tmp/pandafuzz_copy.db"

echo "=== PandaFuzz Crash Storage Validation (Local Copy) ==="
echo "Container: $CONTAINER_NAME"
echo "Remote DB: $DB_PATH_IN_CONTAINER"
echo "Local copy: $LOCAL_DB"
echo

# Check if container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}Error: Container '$CONTAINER_NAME' is not running${NC}"
    echo "Available containers:"
    docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
    exit 1
fi

# Check if sqlite3 is installed locally
if ! command -v sqlite3 &> /dev/null; then
    echo -e "${RED}Error: sqlite3 is not installed locally${NC}"
    echo "Please install sqlite3: apt-get install sqlite3 (or equivalent for your OS)"
    exit 1
fi

# Copy database from container
echo -e "${YELLOW}Copying database from container...${NC}"
docker cp "$CONTAINER_NAME:$DB_PATH_IN_CONTAINER" "$LOCAL_DB"
if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to copy database from container${NC}"
    exit 1
fi
echo -e "${GREEN}Database copied successfully${NC}"
echo

# Function to run a query
run_query() {
    local title="$1"
    local query="$2"
    echo -e "${GREEN}$title${NC}"
    sqlite3 -header -column "$LOCAL_DB" "$query"
    echo
}

# 1. Check schema
echo -e "${YELLOW}1. Database Schema Check${NC}"
sqlite3 "$LOCAL_DB" ".schema crash_inputs" 2>/dev/null
if [ $? -ne 0 ]; then
    echo -e "${RED}Warning: crash_inputs table may not exist${NC}"
    echo "Available tables:"
    sqlite3 "$LOCAL_DB" ".tables"
fi
echo

# 2. Overall statistics
run_query "2. Overall Crash Statistics" "
SELECT 
    'Total Crashes' as metric,
    COUNT(*) as count
FROM crashes
UNION ALL
SELECT 
    'Crashes with Input Data' as metric,
    COUNT(*) as count
FROM crash_inputs;"

# 3. Recent crashes
run_query "3. Recent Crashes (Last 10)" "
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

# 4. Check for crashes without input
echo -e "${YELLOW}4. Crashes Without Input Data${NC}"
NO_INPUT_COUNT=$(sqlite3 "$LOCAL_DB" "
SELECT COUNT(*) 
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE ci.crash_id IS NULL;" 2>/dev/null)

if [ "$NO_INPUT_COUNT" -gt 0 ]; then
    echo -e "${RED}Found $NO_INPUT_COUNT crashes without input data${NC}"
    run_query "First 5 crashes without input:" "
    SELECT 
        substr(c.id, 1, 50) as crash_id,
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

# 5. Job summary
run_query "5. Crash Storage by Job (Top 5)" "
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

# 6. Check specific job if provided
JOB_ID="${1:-job_5083ac5c-f80e-4757-8989-7e2e9d725229}"
echo -e "${YELLOW}6. Checking specific job: $JOB_ID${NC}"
run_query "Crashes for job:" "
SELECT 
    c.id,
    datetime(c.timestamp) as time,
    c.type,
    CASE WHEN ci.crash_id IS NOT NULL THEN 'Yes' ELSE 'No' END as has_input,
    LENGTH(ci.input) as input_size
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE c.job_id = '$JOB_ID'
ORDER BY c.timestamp DESC;"

# 7. Export sample crash
echo -e "${YELLOW}Sample crash IDs with input data:${NC}"
sqlite3 -box "$LOCAL_DB" "
SELECT substr(crash_id, 1, 60) as crash_id
FROM crash_inputs
ORDER BY crash_id DESC
LIMIT 5;"

echo -e "${YELLOW}To export a crash to file:${NC}"
echo "sqlite3 $LOCAL_DB \"SELECT writefile('crash_sample.bin', input) FROM crash_inputs WHERE crash_id = 'CRASH_ID';\""
echo

# 8. Interactive mode
echo -e "${YELLOW}For interactive queries:${NC}"
echo "sqlite3 $LOCAL_DB"
echo

# Cleanup reminder
echo -e "${YELLOW}Note: Database copy is at $LOCAL_DB${NC}"
echo "Remove it when done: rm $LOCAL_DB"
echo

echo "=== Validation Complete ===="