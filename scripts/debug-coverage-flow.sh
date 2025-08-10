#!/bin/bash

# Script to add debug logging and test coverage data flow
# This will help identify where coverage data is being lost

set -e

echo "=== Adding Debug Logging for Coverage Flow ==="
echo "This script will add temporary debug logging to trace coverage data"
echo

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Backup original files
echo -e "${YELLOW}Creating backups of files to be modified...${NC}"
cp pkg/fuzzer/aflplusplus.go pkg/fuzzer/aflplusplus.go.debug-backup
cp pkg/bot/executor_fuzzer.go pkg/bot/executor_fuzzer.go.debug-backup
cp pkg/bot/client.go pkg/bot/client.go.debug-backup
cp pkg/master/api_coverage_simple.go pkg/master/api_coverage_simple.go.debug-backup

echo -e "${GREEN}✓${NC} Backups created"

# Function to add debug logging
add_debug_logging() {
    echo -e "${BLUE}Adding debug logging to trace coverage data flow...${NC}"
    
    # 1. Add logging to AFL++ CollectCoverageData
    cat > /tmp/afl_debug_patch.txt << 'EOF'
--- a/pkg/fuzzer/aflplusplus.go
+++ b/pkg/fuzzer/aflplusplus.go
@@ CollectCoverageData
+	a.logger.WithFields(logrus.Fields{
+		"method": "CollectCoverageData",
+		"output_dir": a.outputDir,
+	}).Debug("DEBUG: Starting coverage collection")
+	
 	// Try multiple possible locations for the stats file
 	possibleStatsFiles := []string{
@@ after statsData != nil
+			a.logger.WithFields(logrus.Fields{
+				"key": key,
+				"value": value,
+				"type": fmt.Sprintf("%T", value),
+			}).Debug("DEBUG: Parsed stat from file")
@@ after edges_found parsing
+		a.logger.WithFields(logrus.Fields{
+			"edges_found_raw": val,
+			"edges_parsed": edges,
+			"line_coverage_calculated": float64(edges) * 2.0,
+		}).Debug("DEBUG: Edges found and coverage calculated")
@@ before return
+	a.logger.WithFields(logrus.Fields{
+		"coverage_data": coverageData,
+		"data_types": fmt.Sprintf("%+v", getTypes(coverageData)),
+	}).Debug("DEBUG: Final coverage data being returned")
EOF

    # 2. Add logging to executor_fuzzer.go
    cat > /tmp/executor_debug_patch.txt << 'EOF'
--- a/pkg/bot/executor_fuzzer.go
+++ b/pkg/bot/executor_fuzzer.go
@@ after CollectCoverageData call
+		fje.logger.WithFields(logrus.Fields{
+			"coverage_data": coverageData,
+			"data_length": len(coverageData),
+			"data_keys": getKeys(coverageData),
+		}).Debug("DEBUG: Coverage data collected from fuzzer")
@@ in coverage extraction
+		fje.logger.WithFields(logrus.Fields{
+			"line_coverage_raw": coverageData["line_coverage"],
+			"line_coverage_type": fmt.Sprintf("%T", coverageData["line_coverage"]),
+			"coverage_percent_raw": coverageData["coverage_percent"],
+			"coverage_percent_type": fmt.Sprintf("%T", coverageData["coverage_percent"]),
+			"line_coverage_extracted": lineCoverage,
+			"function_coverage_extracted": functionCoverage,
+		}).Debug("DEBUG: Coverage percentage extraction")
@@ before ReportCoverageData
+		fje.logger.WithFields(logrus.Fields{
+			"report_id": reportID,
+			"coverage_report": coverageReport,
+			"report_keys": getKeys(coverageReport),
+		}).Debug("DEBUG: About to report coverage data to master")
EOF

    # 3. Add logging to client.go
    cat > /tmp/client_debug_patch.txt << 'EOF'
--- a/pkg/bot/client.go
+++ b/pkg/bot/client.go
@@ in ReportCoverageData
+	rc.logger.WithFields(logrus.Fields{
+		"endpoint": "/api/v1/results/coverage-report",
+		"coverage_data": coverageData,
+		"data_keys": getKeys(coverageData),
+		"job_id": coverageData["job_id"],
+		"line_coverage": coverageData["line_coverage"],
+	}).Debug("DEBUG: Sending coverage data to master")
@@ after successful send
+	rc.logger.Debug("DEBUG: Coverage data successfully sent to master")
EOF

    # 4. Add logging to master API handler
    cat > /tmp/master_debug_patch.txt << 'EOF'
--- a/pkg/master/api_coverage_simple.go
+++ b/pkg/master/api_coverage_simple.go
@@ in handleSubmitCoverageReport
+	s.logger.WithFields(logrus.Fields{
+		"request_body": req,
+		"job_id": req.JobID,
+		"line_coverage": req.LineCoverage,
+		"coverage_data_keys": getKeys(req.CoverageData),
+		"coverage_data": req.CoverageData,
+	}).Debug("DEBUG: Received coverage report from bot")
@@ after edges extraction
+	s.logger.WithFields(logrus.Fields{
+		"edges": edges,
+		"exec_count": execCount,
+		"coverage_record": coverageRecord,
+	}).Debug("DEBUG: Processed coverage record for storage")
EOF

    echo -e "${GREEN}✓${NC} Debug patches prepared"
}

# Function to compile helper functions
add_helper_functions() {
    echo -e "${BLUE}Adding helper functions for debug output...${NC}"
    
    cat > /tmp/debug_helpers.go << 'EOF'
// Add these helper functions to each file that needs them

func getKeys(m map[string]interface{}) []string {
    keys := make([]string, 0, len(m))
    for k := range m {
        keys = append(keys, k)
    }
    return keys
}

func getTypes(m map[string]interface{}) map[string]string {
    types := make(map[string]string)
    for k, v := range m {
        types[k] = fmt.Sprintf("%T", v)
    }
    return types
}
EOF
    
    echo -e "${GREEN}✓${NC} Helper functions prepared"
}

# Create test script
create_test_script() {
    echo -e "${BLUE}Creating test script...${NC}"
    
    cat > test-coverage-debug.sh << 'SCRIPT'
#!/bin/bash

echo "=== Coverage Debug Test ==="
echo "This will create a test job and monitor the coverage data flow"
echo

# Set very verbose logging
export LOG_LEVEL=debug

# Create test binary and corpus
TEST_DIR="/tmp/coverage-debug-test-$(date +%s)"
mkdir -p "$TEST_DIR"

cat > "$TEST_DIR/test.c" << 'EOF'
#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>

int main() {
    char buf[10];
    if (read(0, buf, 10) < 1) return 0;
    
    if (buf[0] == 'A') {
        if (buf[1] == 'B') {
            if (buf[2] == 'C') {
                abort();
            }
        }
    }
    return 0;
}
EOF

gcc -o "$TEST_DIR/test_binary" "$TEST_DIR/test.c"
mkdir -p "$TEST_DIR/corpus"
echo "test" > "$TEST_DIR/corpus/seed1"
echo "Atest" > "$TEST_DIR/corpus/seed2"
echo "ABtest" > "$TEST_DIR/corpus/seed3"

# Copy to containers
docker cp "$TEST_DIR/test_binary" pandafuzz-master:/app/data/binaries/debug-test
docker cp "$TEST_DIR/corpus" pandafuzz-master:/app/data/corpus/debug-corpus

# Start tailing logs
echo "Starting log monitoring..."
docker logs -f pandafuzz-bot-1 2>&1 | grep -E "DEBUG:|ERROR:" > bot-debug.log &
BOT_LOG_PID=$!

docker logs -f pandafuzz-master 2>&1 | grep -E "DEBUG:|ERROR:" > master-debug.log &
MASTER_LOG_PID=$!

# Create job
echo "Creating test job..."
JOB_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v3/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coverage Debug Test",
    "fuzzer": "afl++",
    "target": "/app/data/binaries/debug-test",
    "duration": 30000000000,
    "config": {
      "memory_limit": 256,
      "timeout": 1000
    }
  }')

JOB_ID=$(echo "$JOB_RESPONSE" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
echo "Created job: $JOB_ID"

# Wait for job to complete
echo "Waiting 45 seconds for job to complete..."
sleep 45

# Get job status
echo "Checking job status..."
curl -s http://localhost:8080/api/v3/jobs/$JOB_ID | jq '.status'

# Get coverage
echo "Checking coverage..."
curl -s http://localhost:8080/api/v3/jobs/$JOB_ID/coverage | jq '.'

# Stop log tailing
kill $BOT_LOG_PID $MASTER_LOG_PID 2>/dev/null

# Check fuzzer stats directly
echo "Checking fuzzer stats in container..."
docker exec pandafuzz-bot-1 find /app/work/jobs -name "fuzzer_stats" -exec cat {} \; 2>/dev/null | grep -E "edges_found|bitmap_cvg|paths_total"

echo
echo "=== Debug Output Summary ==="
echo "Bot debug log: bot-debug.log"
echo "Master debug log: master-debug.log"
echo
echo "Key things to look for:"
echo "1. 'DEBUG: Starting coverage collection' - confirms collection started"
echo "2. 'DEBUG: Edges found and coverage calculated' - shows edge parsing"
echo "3. 'DEBUG: Coverage data collected from fuzzer' - shows data received by executor"
echo "4. 'DEBUG: Coverage percentage extraction' - shows type conversion attempts"
echo "5. 'DEBUG: Sending coverage data to master' - shows what's being sent"
echo "6. 'DEBUG: Received coverage report from bot' - shows what master received"

# Cleanup
rm -rf "$TEST_DIR"
SCRIPT

    chmod +x test-coverage-debug.sh
    echo -e "${GREEN}✓${NC} Test script created: test-coverage-debug.sh"
}

# Main execution
echo
echo "=== Setup Instructions ==="
echo
echo "1. The debug patches have been prepared in /tmp/"
echo "2. A test script 'test-coverage-debug.sh' has been created"
echo
echo "To apply debug logging and test:"
echo
echo -e "${YELLOW}Step 1: Apply the debug logging manually${NC}"
echo "Edit the following files and add debug logging at key points:"
echo "  - pkg/fuzzer/aflplusplus.go (CollectCoverageData method)"
echo "  - pkg/bot/executor_fuzzer.go (coverage collection section)"
echo "  - pkg/bot/client.go (ReportCoverageData method)"
echo "  - pkg/master/api_coverage_simple.go (handleSubmitCoverageReport)"
echo
echo -e "${YELLOW}Step 2: Rebuild and restart containers${NC}"
echo "  make docker && docker-compose down && docker-compose up -d"
echo
echo -e "${YELLOW}Step 3: Run the test${NC}"
echo "  ./test-coverage-debug.sh"
echo
echo -e "${YELLOW}Step 4: Analyze the logs${NC}"
echo "  grep 'DEBUG:' bot-debug.log | less"
echo "  grep 'DEBUG:' master-debug.log | less"
echo
echo "The debug output will show:"
echo "  - What data AFL++ is collecting"
echo "  - Data types at each stage"
echo "  - What's being sent to the master"
echo "  - What the master is receiving"
echo "  - Where the data might be getting lost or corrupted"

# Generate the patches and helper functions
add_debug_logging
add_helper_functions
create_test_script

echo
echo -e "${GREEN}Setup complete!${NC}"
echo "Follow the steps above to identify where coverage data is being lost."