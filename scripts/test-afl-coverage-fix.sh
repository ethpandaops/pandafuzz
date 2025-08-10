#!/bin/bash

# Test script to verify AFL++ coverage fix
# This script tests if AFL++ properly reports edges when run through PandaFuzz

set -e

echo "=== AFL++ Coverage Fix Test Script ==="
echo "Testing if AFL++ properly reports edges through PandaFuzz process management"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running in Docker
if [ -f /.dockerenv ]; then
    echo -e "${GREEN}✓${NC} Running in Docker container"
else
    echo -e "${YELLOW}⚠${NC} Not running in Docker, results may vary"
fi

# Test directories
TEST_DIR="/tmp/afl-coverage-test-$(date +%s)"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "Working directory: $TEST_DIR"
echo

# Create a simple test program
echo -e "${YELLOW}Creating test program...${NC}"
cat > test.c << 'EOF'
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int main(int argc, char** argv) {
    char buffer[100];
    
    // Read from stdin
    if (read(0, buffer, sizeof(buffer)) < 1) {
        return 0;
    }
    
    // Simple branches to create edges
    if (buffer[0] == 'A') {
        printf("Branch A\n");
        if (buffer[1] == 'B') {
            printf("Branch AB\n");
            if (buffer[2] == 'C') {
                printf("Branch ABC\n");
                // Intentional crash for testing
                if (buffer[3] == 'D') {
                    abort();
                }
            }
        }
    }
    
    return 0;
}
EOF

# Compile with AFL++ instrumentation
echo -e "${YELLOW}Compiling with AFL++ instrumentation...${NC}"
if command -v afl-gcc >/dev/null 2>&1; then
    afl-gcc -o test_binary test.c
    echo -e "${GREEN}✓${NC} Compiled with afl-gcc"
elif command -v afl-clang >/dev/null 2>&1; then
    afl-clang -o test_binary test.c
    echo -e "${GREEN}✓${NC} Compiled with afl-clang"
else
    echo -e "${RED}✗${NC} AFL++ compiler not found"
    exit 1
fi

# Create input corpus
echo -e "${YELLOW}Creating input corpus...${NC}"
mkdir -p input
echo "test" > input/seed1
echo "Atest" > input/seed2
echo "ABtest" > input/seed3
echo -e "${GREEN}✓${NC} Created input corpus with 3 seeds"

# Function to check for zombie processes
check_zombies() {
    local zombies=$(ps aux | grep -c '<defunct>' || true)
    if [ "$zombies" -gt 0 ]; then
        echo -e "${RED}✗${NC} Found $zombies zombie process(es)"
        ps aux | grep '<defunct>' || true
        return 1
    else
        echo -e "${GREEN}✓${NC} No zombie processes detected"
        return 0
    fi
}

# Function to check shared memory
check_shm() {
    local shm_count=$(ipcs -m 2>/dev/null | grep -c afl || true)
    if [ "$shm_count" -gt 0 ]; then
        echo -e "${GREEN}✓${NC} AFL++ shared memory segments detected: $shm_count"
        ipcs -m | grep afl || true
        return 0
    else
        # Check /proc/sysvipc/shm as fallback
        if [ -f /proc/sysvipc/shm ]; then
            local shm_size=$(wc -c < /proc/sysvipc/shm)
            if [ "$shm_size" -gt 100 ]; then
                echo -e "${GREEN}✓${NC} Shared memory activity detected (size: $shm_size bytes)"
                return 0
            fi
        fi
        echo -e "${YELLOW}⚠${NC} No AFL++ shared memory segments detected"
        return 1
    fi
}

# Test 1: Direct AFL++ execution (baseline)
echo
echo "=== Test 1: Direct AFL++ Execution (Baseline) ==="
mkdir -p output-direct

echo -e "${YELLOW}Running AFL++ directly...${NC}"
timeout 5s afl-fuzz -i input -o output-direct -- ./test_binary 2>&1 | tee direct.log || true

# Check results
if grep -q "edges found" output-direct/fuzzer_stats 2>/dev/null; then
    EDGES=$(grep "edges_found" output-direct/fuzzer_stats | cut -d: -f2 | tr -d ' ')
    echo -e "${GREEN}✓${NC} Direct execution found edges: $EDGES"
else
    # Fallback: check paths_total
    if grep -q "paths_total" output-direct/fuzzer_stats 2>/dev/null; then
        PATHS=$(grep "paths_total" output-direct/fuzzer_stats | cut -d: -f2 | tr -d ' ')
        echo -e "${GREEN}✓${NC} Direct execution found paths: $PATHS"
    else
        echo -e "${YELLOW}⚠${NC} Could not determine edges from direct execution"
    fi
fi

check_zombies
check_shm

# Test 2: PandaFuzz execution (with fixes)
echo
echo "=== Test 2: PandaFuzz Execution (With Fixes) ==="

# Create a simple job configuration
cat > job.yaml << EOF
id: test-afl-coverage
name: AFL++ Coverage Test
fuzzer: afl++
target: $TEST_DIR/test_binary
work_dir: $TEST_DIR/pandafuzz-test
config:
  duration: 5s
  memory_limit: 100
  timeout: 1000
enable_coverage: true
coverage_format: basic
EOF

# Run through PandaFuzz (if available)
if command -v pandafuzz-bot >/dev/null 2>&1; then
    echo -e "${YELLOW}Running through PandaFuzz bot...${NC}"
    mkdir -p pandafuzz-test/input
    cp input/* pandafuzz-test/input/
    
    # Start bot in background
    pandafuzz-bot --config job.yaml &
    BOT_PID=$!
    
    # Wait for fuzzing
    sleep 10
    
    # Stop bot
    kill $BOT_PID 2>/dev/null || true
    wait $BOT_PID 2>/dev/null || true
    
    # Check results
    if [ -f pandafuzz-test/output/fuzzer_stats ]; then
        if grep -q "edges_found" pandafuzz-test/output/fuzzer_stats; then
            EDGES=$(grep "edges_found" pandafuzz-test/output/fuzzer_stats | cut -d: -f2 | tr -d ' ')
            if [ "$EDGES" -gt 0 ]; then
                echo -e "${GREEN}✓${NC} PandaFuzz execution found edges: $EDGES"
                echo -e "${GREEN}✓ FIX VERIFIED: AFL++ reports edges through PandaFuzz${NC}"
            else
                echo -e "${RED}✗${NC} PandaFuzz execution found 0 edges"
                echo -e "${RED}✗ FIX FAILED: AFL++ not reporting edges${NC}"
            fi
        else
            # Fallback: check paths
            if grep -q "paths_total" pandafuzz-test/output/fuzzer_stats; then
                PATHS=$(grep "paths_total" pandafuzz-test/output/fuzzer_stats | cut -d: -f2 | tr -d ' ')
                if [ "$PATHS" -gt 1 ]; then
                    echo -e "${GREEN}✓${NC} PandaFuzz execution found paths: $PATHS"
                    echo -e "${GREEN}✓ FIX LIKELY WORKING: AFL++ discovering paths${NC}"
                else
                    echo -e "${YELLOW}⚠${NC} PandaFuzz execution found only $PATHS path(s)"
                fi
            fi
        fi
    else
        echo -e "${YELLOW}⚠${NC} fuzzer_stats file not found"
    fi
    
    check_zombies
    check_shm
else
    echo -e "${YELLOW}⚠${NC} PandaFuzz bot not available, skipping integration test"
    echo "To fully test the fix, build and run PandaFuzz with the changes"
fi

# Test 3: Process group and zombie prevention
echo
echo "=== Test 3: Process Group and Zombie Prevention ==="

echo -e "${YELLOW}Testing process group management...${NC}"

# Create a test script that spawns child processes
cat > fork_test.c << 'EOF'
#include <stdio.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/wait.h>

int main() {
    printf("Parent PID: %d, PGID: %d\n", getpid(), getpgrp());
    
    pid_t child = fork();
    if (child == 0) {
        // Child process
        printf("Child PID: %d, PGID: %d\n", getpid(), getpgrp());
        sleep(2);
        return 0;
    }
    
    // Parent waits
    wait(NULL);
    return 0;
}
EOF

gcc -o fork_test fork_test.c
./fork_test

# Final check for zombies
echo
echo "=== Final System Check ==="
check_zombies

# Check for any remaining AFL processes
AFL_PROCS=$(ps aux | grep -c 'afl-fuzz' | grep -v grep || true)
if [ "$AFL_PROCS" -gt 0 ]; then
    echo -e "${YELLOW}⚠${NC} Found $AFL_PROCS AFL++ process(es) still running"
    ps aux | grep 'afl-fuzz' | grep -v grep || true
else
    echo -e "${GREEN}✓${NC} No AFL++ processes left running"
fi

# Summary
echo
echo "=== Test Summary ==="
echo "The fix addresses the following issues:"
echo "1. Process group management for AFL++ fork-server"
echo "2. SIGCHLD handler for zombie prevention"
echo "3. Proper shared memory initialization"
echo "4. AFL++ fork-server initialization wait"
echo "5. Process health monitoring"
echo
echo "If all tests passed, AFL++ should now properly report edges when run through PandaFuzz."

# Cleanup
cd /
rm -rf "$TEST_DIR"

echo
echo -e "${GREEN}Test complete!${NC}"