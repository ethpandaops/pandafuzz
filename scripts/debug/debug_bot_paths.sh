#!/bin/bash
# Debug script to check bot container paths and binary location

set -e

echo "=== Debugging Bot Container Paths ==="

# Function to run commands in bot container
run_in_bot() {
    docker-compose exec bot bash -c "$1" 2>/dev/null || docker exec pandafuzz-bot bash -c "$1" 2>/dev/null || echo "Failed to exec into bot container"
}

echo ""
echo "1. Checking container working directory:"
run_in_bot "pwd"

echo ""
echo "2. Checking /app directory contents:"
run_in_bot "ls -la /app/"

echo ""
echo "3. Checking work directory location:"
run_in_bot "ls -la /app/work/ 2>/dev/null || echo 'Work directory not found at /app/work'"
run_in_bot "ls -la ./work/ 2>/dev/null || echo 'Work directory not found at ./work'"

echo ""
echo "4. Finding all work directories:"
run_in_bot "find /app -name 'work' -type d 2>/dev/null | head -10"

echo ""
echo "5. Checking for job directories:"
run_in_bot "find /app -path '*/jobs/*' -type d 2>/dev/null | head -10"

echo ""
echo "6. Looking for target_binary files:"
run_in_bot "find /app -name 'target_binary' -type f 2>/dev/null | head -10"

echo ""
echo "7. Checking volume mounts:"
run_in_bot "mount | grep -E '(work|app)' || echo 'No relevant mounts found'"

echo ""
echo "8. Bot process working directory:"
run_in_bot "ps aux | grep pandafuzz-bot | grep -v grep | head -1"
run_in_bot "pwdx \$(pgrep pandafuzz-bot) 2>/dev/null || echo 'Cannot get process working directory'"

echo ""
echo "9. Checking relative work directory resolution:"
run_in_bot "cd /app && realpath ./work"

echo ""
echo "10. Recent job directories (if any):"
run_in_bot "find /app -name 'job*' -type d -mtime -1 2>/dev/null | head -10"

echo ""
echo "=== Manual Check Instructions ==="
echo "To manually check inside the container:"
echo "1. docker exec -it pandafuzz-bot bash"
echo "2. cd /app"
echo "3. ls -la work/"
echo "4. find . -name 'target_binary' -type f"
echo ""
echo "To check if a specific job exists:"
echo "Replace JOB_ID with actual job ID:"
echo "find /app -path '*JOB_ID*' -type d"