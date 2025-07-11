-- Validate Crash Storage in PandaFuzz Database
-- Run these queries against the SQLite database to verify crash input storage

-- 1. Check if crash_inputs table exists and its structure
.schema crash_inputs

-- 2. Count total crashes vs crashes with input data
SELECT 
    'Total Crashes' as metric,
    COUNT(*) as count
FROM crashes
UNION ALL
SELECT 
    'Crashes with Input Data' as metric,
    COUNT(*) as count
FROM crash_inputs;

-- 3. List recent crashes and check if they have input data
SELECT 
    c.id,
    c.job_id,
    c.bot_id,
    c.timestamp,
    c.type,
    c.size,
    CASE 
        WHEN ci.crash_id IS NOT NULL THEN 'Yes'
        ELSE 'No'
    END as has_input_data,
    CASE 
        WHEN ci.crash_id IS NOT NULL THEN LENGTH(ci.input)
        ELSE 0
    END as input_size_bytes
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
ORDER BY c.timestamp DESC
LIMIT 20;

-- 4. Find crashes without input data (potential issues)
SELECT 
    c.id,
    c.job_id,
    c.bot_id,
    c.timestamp,
    c.type,
    c.file_path
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE ci.crash_id IS NULL
ORDER BY c.timestamp DESC
LIMIT 10;

-- 5. Check a specific crash by ID (replace with your crash ID)
-- Example: job_5083ac5c-f80e-4757-8989-7e2e9d725229_crash-a1f2925a2506ed803347072619283e2a3efd538f
SELECT 
    c.*,
    CASE 
        WHEN ci.crash_id IS NOT NULL THEN 'Present'
        ELSE 'Missing'
    END as input_status,
    LENGTH(ci.input) as input_size
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE c.job_id = 'job_5083ac5c-f80e-4757-8989-7e2e9d725229';

-- 6. Extract and save a crash input to file (SQLite specific)
-- This will write the crash input to a file for analysis
-- Replace 'your_crash_id' with actual crash ID
-- .output crash_sample.bin
-- SELECT input FROM crash_inputs WHERE crash_id = 'your_crash_id';
-- .output stdout

-- 7. Summary statistics by job
SELECT 
    c.job_id,
    COUNT(DISTINCT c.id) as total_crashes,
    COUNT(DISTINCT ci.crash_id) as crashes_with_input,
    ROUND(COUNT(DISTINCT ci.crash_id) * 100.0 / COUNT(DISTINCT c.id), 2) as percentage_with_input
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
GROUP BY c.job_id
ORDER BY total_crashes DESC
LIMIT 10;

-- 8. Check recent crashes by timestamp (last 24 hours)
SELECT 
    c.id,
    c.job_id,
    c.timestamp,
    CASE WHEN ci.crash_id IS NOT NULL THEN 'Yes' ELSE 'No' END as has_input
FROM crashes c
LEFT JOIN crash_inputs ci ON c.id = ci.crash_id
WHERE c.timestamp > datetime('now', '-1 day')
ORDER BY c.timestamp DESC;