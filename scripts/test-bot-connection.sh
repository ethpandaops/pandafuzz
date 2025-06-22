#!/bin/bash
# Test script to verify bot can connect to master

set -e

echo "Testing bot connection to master..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default master URL
MASTER_URL="${MASTER_URL:-http://localhost:8080}"

echo -e "${YELLOW}Using Master URL: $MASTER_URL${NC}"

# Test 1: Check if master is reachable
echo -e "\n${YELLOW}Test 1: Checking if master is reachable...${NC}"
if curl -s -f "$MASTER_URL/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Master is reachable${NC}"
else
    echo -e "${RED}✗ Master is not reachable at $MASTER_URL${NC}"
    echo "Please ensure the master is running and accessible"
    exit 1
fi

# Test 2: Check master API version
echo -e "\n${YELLOW}Test 2: Checking master API version...${NC}"
API_RESPONSE=$(curl -s "$MASTER_URL/api/v1/version" || echo "{}")
if [ ! -z "$API_RESPONSE" ] && [ "$API_RESPONSE" != "{}" ]; then
    echo -e "${GREEN}✓ Master API is responding${NC}"
    echo "API Response: $API_RESPONSE"
else
    echo -e "${RED}✗ Failed to get API version${NC}"
fi

# Test 3: Test bot registration endpoint
echo -e "\n${YELLOW}Test 3: Testing bot registration endpoint...${NC}"
REGISTRATION_DATA='{
  "hostname": "test-bot-'$(date +%s)'",
  "capabilities": ["afl++", "libfuzzer"]
}'

REGISTRATION_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "User-Agent: PandaFuzz-Bot/test" \
    -d "$REGISTRATION_DATA" \
    "$MASTER_URL/api/v1/bots/register" || echo "{}")

if echo "$REGISTRATION_RESPONSE" | grep -q "bot_id"; then
    echo -e "${GREEN}✓ Bot registration successful${NC}"
    echo "Registration Response: $REGISTRATION_RESPONSE"
    BOT_ID=$(echo "$REGISTRATION_RESPONSE" | grep -o '"bot_id":"[^"]*"' | sed 's/"bot_id":"\([^"]*\)"/\1/')
    echo "Bot ID: $BOT_ID"
else
    echo -e "${RED}✗ Bot registration failed${NC}"
    echo "Response: $REGISTRATION_RESPONSE"
    exit 1
fi

# Test 4: Check if bot appears in bot list
echo -e "\n${YELLOW}Test 4: Checking if bot appears in list...${NC}"
sleep 2
BOT_LIST=$(curl -s "$MASTER_URL/api/v1/bots" || echo "[]")
if echo "$BOT_LIST" | grep -q "$BOT_ID"; then
    echo -e "${GREEN}✓ Bot appears in master's bot list${NC}"
else
    echo -e "${RED}✗ Bot does not appear in master's bot list${NC}"
    echo "Bot List: $BOT_LIST"
fi

# Test 5: Test heartbeat endpoint
echo -e "\n${YELLOW}Test 5: Testing heartbeat endpoint...${NC}"
HEARTBEAT_DATA='{
  "status": "idle",
  "resource_usage": {
    "cpu_percent": 10.5,
    "memory_used": 104857600,
    "disk_used": 209715200
  }
}'

HEARTBEAT_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -H "User-Agent: PandaFuzz-Bot/'$BOT_ID'" \
    -d "$HEARTBEAT_DATA" \
    "$MASTER_URL/api/v1/bots/$BOT_ID/heartbeat" || echo "{}")

if echo "$HEARTBEAT_RESPONSE" | grep -q "status"; then
    echo -e "${GREEN}✓ Heartbeat successful${NC}"
else
    echo -e "${RED}✗ Heartbeat failed${NC}"
    echo "Response: $HEARTBEAT_RESPONSE"
fi

echo -e "\n${GREEN}All connection tests completed!${NC}"
echo -e "\nTo test with docker-compose:"
echo "1. Start the master: docker-compose up -d master"
echo "2. Wait for master to be healthy: docker-compose ps"
echo "3. Start the bots: docker-compose up -d bot"
echo "4. Check logs: docker-compose logs bot"