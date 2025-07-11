#!/bin/bash
# Script to validate crash storage in PandaFuzz database

# Default database path (adjust if needed)
DB_PATH="${PANDAFUZZ_DB:-./pandafuzz.db}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=== PandaFuzz Crash Storage Validation ==="
echo "Database: $DB_PATH"
echo

# Check if database exists
if [ ! -f "$DB_PATH" ]; then
    echo -e "${RED}Error: Database not found at $DB_PATH${NC}"
    echo "Set PANDAFUZZ_DB environment variable or run from directory with pandafuzz.db"
    exit 1
fi

# Function to run a query and display results
run_query() {
    local title="$1"
    local query="$2"
    echo -e "${GREEN}$title${NC}"
    sqlite3 -header -column "$DB_PATH" "$query"
    echo
}

# 1. Check schema
echo -e "${YELLOW}1. Database Schema Check${NC}"
sqlite3 "$DB_PATH" ".schema crash_inputs" 2>/dev/null
if [ $? -ne 0 ]; then
    echo -e "${RED}Warning: crash_inputs table may not exist${NC}"
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
NO_INPUT_COUNT=$(sqlite3 "$DB_PATH" "
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

# 6. Check specific job if provided as argument
if [ ! -z "$1" ]; then
    echo -e "${YELLOW}6. Checking specific job: $1${NC}"
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

# 7. Export a sample crash (optional)
echo -e "${YELLOW}To export a crash input to file:${NC}"
echo "sqlite3 $DB_PATH \"SELECT writefile('crash_output.bin', input) FROM crash_inputs WHERE crash_id = 'YOUR_CRASH_ID';\""
echo

echo "=== Validation Complete ==="