#!/bin/bash
# Quick queries for crash validation in Docker

# Configuration
CONTAINER="${1:-pandafuzz-master-1}"
DB_PATH="/app/data/pandafuzz.db"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Quick Crash Validation ===${NC}"
echo "Container: $CONTAINER"
echo

# Quick check: Do we have crashes with input data?
echo -e "${YELLOW}Crash Input Summary:${NC}"
docker exec "$CONTAINER" sqlite3 -box "$DB_PATH" "
SELECT 
    COUNT(*) as total_crashes,
    SUM(CASE WHEN ci.crash_id IS NOT NULL THEN 1 ELSE 0 END) as with_input,
    SUM(CASE WHEN ci.crash_id IS NULL THEN 1 ELSE 0 END) as without_input
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id;"

echo -e "\n${YELLOW}Latest Crashes:${NC}"
docker exec "$CONTAINER" sqlite3 -box "$DB_PATH" "
SELECT 
    substr(c.id, 1, 40) as crash_id,
    CASE WHEN ci.crash_id IS NOT NULL THEN '✓' ELSE '✗' END as input,
    LENGTH(ci.input) as bytes
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
ORDER BY c.timestamp DESC
LIMIT 5;"

# Check the specific job mentioned in the issue
JOB_ID="job_5083ac5c-f80e-4757-8989-7e2e9d725229"
echo -e "\n${YELLOW}Checking job: $JOB_ID${NC}"
docker exec "$CONTAINER" sqlite3 -box "$DB_PATH" "
SELECT 
    COUNT(*) as crashes_in_job,
    SUM(CASE WHEN ci.crash_id IS NOT NULL THEN 1 ELSE 0 END) as with_input
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE c.job_id = '$JOB_ID';"