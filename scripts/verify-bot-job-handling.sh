#!/bin/bash
# Script to verify and debug bot job handling

set -e

echo "=== Verifying Bot Job Handling ==="

# Check if bot is properly registered with master
echo ""
echo "1. Checking bot registration..."
BOTS=$(curl -s http://localhost:8088/api/v1/bots)
echo "Registered bots:"
echo "$BOTS" | python3 -m json.tool 2>/dev/null || echo "$BOTS"

# Get bot ID from the response
BOT_ID=$(echo "$BOTS" | grep -o '"id":"[^"]*' | head -1 | cut -d'"' -f4)
if [ -z "$BOT_ID" ]; then
    echo "No bots registered! Checking bot container..."
    docker-compose logs --tail=50 bot | grep -E "(register|error|failed)"
    exit 1
fi
echo "Found bot ID: $BOT_ID"

# Check bot capabilities
echo ""
echo "2. Checking bot capabilities..."
BOT_DETAILS=$(curl -s http://localhost:8088/api/v1/bots/${BOT_ID})
echo "$BOT_DETAILS" | python3 -m json.tool 2>/dev/null || echo "$BOT_DETAILS"

# Check if bot is receiving heartbeats
echo ""
echo "3. Checking bot heartbeat..."
docker-compose logs --tail=20 bot | grep -i "heartbeat" | tail -5

# Check job assignment mechanism
echo ""
echo "4. Checking job assignment logs..."
docker-compose logs --tail=50 master | grep -E "(assign|job.*bot|${BOT_ID})" | tail -10

# Test job pickup
echo ""
echo "5. Forcing job pickup check..."
docker-compose exec bot bash -c '
# Check if bot is trying to get jobs
echo "Bot process:"
ps aux | grep pandafuzz-bot | grep -v grep

# Check bot configuration
echo ""
echo "Bot work directory configuration:"
grep -E "(work_dir|WorkDir)" /app/bot-docker.yaml || cat /app/bot-docker.yaml | grep -A5 "fuzzing:"

# Create work directory if missing
if [ ! -d /app/work ]; then
    echo "Creating work directory..."
    mkdir -p /app/work/jobs
    chmod -R 755 /app/work
fi

# Check permissions
echo ""
echo "Work directory permissions:"
ls -la /app/work/
'

# Check for any error patterns
echo ""
echo "6. Checking for common errors..."
echo "Download errors:"
docker-compose logs bot | grep -i "download.*error" | tail -5 || echo "No download errors found"

echo ""
echo "Permission errors:"
docker-compose logs bot | grep -i "permission" | tail -5 || echo "No permission errors found"

echo ""
echo "Connection errors:"
docker-compose logs bot | grep -i "connection" | tail -5 || echo "No connection errors found"

# Restart bot to force re-registration
echo ""
echo "7. Restarting bot to force fresh registration..."
docker-compose restart bot
sleep 5

# Check new registration
echo ""
echo "8. Checking new registration..."
docker-compose logs --tail=20 bot | grep -E "(register|started|connected)"

echo ""
echo "=== Verification Complete ==="
echo ""
echo "If jobs are still not being picked up, check:"
echo "1. Master URL in bot config matches service name"
echo "2. Network connectivity between containers"
echo "3. Job assignment logic in master"
echo "4. Bot polling interval and job check mechanism"