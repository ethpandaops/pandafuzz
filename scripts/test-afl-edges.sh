#!/bin/bash

# Script to test AFL++ edge detection in Docker container
# This demonstrates that AFL++ can successfully detect edges when run directly

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}=================================================${NC}"
echo -e "${BLUE}     AFL++ Edge Detection Test in Docker        ${NC}"
echo -e "${BLUE}=================================================${NC}"

# Check if docker container is running
if ! docker ps | grep -q pandafuzz-bot-1; then
    echo -e "${RED}Error: pandafuzz-bot-1 container is not running${NC}"
    echo "Please start the container with: docker compose up -d"
    exit 1
fi

echo -e "\n${YELLOW}Running full AFL++ edge detection test...${NC}\n"

docker exec pandafuzz-bot-1 bash -c '
set -e

RED="\033[0;31m"
GREEN="\033[0;32m"
YELLOW="\033[1;33m"
BLUE="\033[0;34m"
NC="\033[0m"

echo -e "${YELLOW}Step 1: Creating test program${NC}"
cat > /tmp/test_edges.c << "EOF"
#include <stdio.h>
#include <unistd.h>

int main() {
    char buf[100];
    ssize_t len = read(0, buf, sizeof(buf));
    
    if (len > 0) {
        switch(buf[0]) {
            case 48: printf("Zero\n"); break;     // ASCII 0
            case 49: printf("One\n"); break;      // ASCII 1
            case 50: printf("Two\n"); break;      // ASCII 2
            case 51: printf("Three\n"); break;    // ASCII 3
            case 52: printf("Four\n"); break;     // ASCII 4
            case 53: printf("Five\n"); break;     // ASCII 5
            case 65: printf("Letter A\n"); break; // ASCII A
            case 66: printf("Letter B\n"); break; // ASCII B
            default: printf("Other\n"); break;
        }
    }
    return 0;
}
EOF
echo -e "${GREEN}✓ Test program created${NC}"

echo -e "\n${YELLOW}Step 2: Compiling with afl-gcc${NC}"
/usr/local/bin/afl-gcc -g -O2 -o /tmp/test_edges /tmp/test_edges.c 2>&1 | head -5
echo -e "${GREEN}✓ Compilation complete${NC}"

echo -e "\n${YELLOW}Step 3: Verifying AFL++ instrumentation${NC}"
AFL_SYMBOLS=$(nm /tmp/test_edges | grep -c __afl || echo 0)
echo "AFL symbols found: ${AFL_SYMBOLS}"
if [ "$AFL_SYMBOLS" -gt 0 ]; then
    echo -e "${GREEN}✓ Binary is instrumented (${AFL_SYMBOLS} AFL symbols)${NC}"
else
    echo -e "${RED}✗ Binary is NOT instrumented${NC}"
    exit 1
fi

echo -e "\n${YELLOW}Step 4: Testing binary functionality${NC}"
echo "Testing with input \"1\":"
echo "1" | /tmp/test_edges
echo "Testing with input \"A\":"
echo "A" | /tmp/test_edges
echo -e "${GREEN}✓ Binary executes correctly${NC}"

echo -e "\n${YELLOW}Step 5: Creating test corpus${NC}"
rm -rf /tmp/afl_test
mkdir -p /tmp/afl_test/in
echo "0" > /tmp/afl_test/in/seed1
echo "1" > /tmp/afl_test/in/seed2
echo "2" > /tmp/afl_test/in/seed3
echo "A" > /tmp/afl_test/in/seed4
echo "B" > /tmp/afl_test/in/seed5
echo "X" > /tmp/afl_test/in/seed6
echo -e "${GREEN}✓ Created 6 seed inputs${NC}"

echo -e "\n${YELLOW}Step 6: Running AFL++ fuzzer for 10 seconds${NC}"
echo "Starting AFL++ (this will run for 10 seconds)..."
timeout 10 afl-fuzz -i /tmp/afl_test/in -o /tmp/afl_test/out /tmp/test_edges 2>&1 | tail -15

echo -e "\n${YELLOW}Step 7: Analyzing results${NC}"
if [ -f /tmp/afl_test/out/default/plot_data ]; then
    # Extract data from plot_data
    PLOT_LINE=$(tail -1 /tmp/afl_test/out/default/plot_data)
    EDGES=$(echo "$PLOT_LINE" | cut -d, -f13)
    COVERAGE=$(echo "$PLOT_LINE" | cut -d, -f7)
    EXECS=$(echo "$PLOT_LINE" | cut -d, -f12)
    CRASHES=$(echo "$PLOT_LINE" | cut -d, -f5)
    
    echo -e "\n${BLUE}========== RESULTS ==========${NC}"
    echo -e "Edges found:       ${GREEN}${EDGES}${NC}"
    echo -e "Coverage:          ${GREEN}${COVERAGE}${NC}"
    echo -e "Total executions:  ${GREEN}${EXECS}${NC}"
    echo -e "Crashes found:     ${GREEN}${CRASHES}${NC}"
    
    if [ "$EDGES" -gt 0 ]; then
        echo -e "\n${GREEN}✅ SUCCESS: AFL++ detected ${EDGES} edges!${NC}"
    else
        echo -e "\n${RED}❌ PROBLEM: AFL++ detected 0 edges${NC}"
    fi
else
    echo -e "${RED}❌ ERROR: No plot_data file generated${NC}"
    exit 1
fi

echo -e "\n${YELLOW}Step 8: Testing coverage map detection${NC}"
echo "Using afl-showmap to verify coverage detection..."
echo "1" | afl-showmap -o /tmp/map1.txt -- /tmp/test_edges 2>&1 | grep "Captured"
echo "A" | afl-showmap -o /tmp/mapA.txt -- /tmp/test_edges 2>&1 | grep "Captured"

if ! diff -q /tmp/map1.txt /tmp/mapA.txt >/dev/null 2>&1; then
    echo -e "${GREEN}✓ Different inputs produce different coverage maps${NC}"
else
    echo -e "${YELLOW}⚠ Same coverage for different inputs${NC}"
fi
'

echo -e "\n${BLUE}=================================================${NC}"
echo -e "${BLUE}           Test Complete                        ${NC}"
echo -e "${BLUE}=================================================${NC}"
echo ""
echo "This test demonstrates that AFL++ CAN detect edges when"
echo "run directly in the Docker container. If edges were found"
echo "(typically 20-30 for this program), then the issue with"
echo "PandaFuzz showing 0 edges is in the PandaFuzz execution,"
echo "not with AFL++ or the binary instrumentation."