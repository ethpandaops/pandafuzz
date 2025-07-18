#!/bin/bash

# PandaFuzz Integration Test Runner

set -e

echo "🐼 PandaFuzz Integration Test Runner"
echo "===================================="

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
VERBOSE=""
SHORT=""
SPECIFIC_TEST=""
COVERAGE=""
BENCH=""

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -s|--short)
            SHORT="-short"
            shift
            ;;
        -t|--test)
            SPECIFIC_TEST="-run $2"
            shift 2
            ;;
        -c|--coverage)
            COVERAGE="1"
            shift
            ;;
        -b|--bench)
            BENCH="1"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  -v, --verbose     Run tests with verbose output"
            echo "  -s, --short       Skip long-running tests"
            echo "  -t, --test NAME   Run specific test (e.g., TestMasterBot)"
            echo "  -c, --coverage    Generate coverage report"
            echo "  -b, --bench       Run benchmarks"
            echo "  -h, --help        Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Change to project directory
cd "$(dirname "$0")"

# Run unit tests first
echo -e "${YELLOW}Running unit tests...${NC}"
if go test ./tests/unit/... $VERBOSE $SHORT; then
    echo -e "${GREEN}✓ Unit tests passed${NC}"
else
    echo -e "${RED}✗ Unit tests failed${NC}"
    exit 1
fi

# Run integration tests
echo -e "\n${YELLOW}Running integration tests...${NC}"

if [ -n "$COVERAGE" ]; then
    # Run with coverage
    echo "Generating coverage report..."
    go test ./tests/integration/... $VERBOSE $SHORT $SPECIFIC_TEST -coverprofile=coverage.out
    go tool cover -html=coverage.out -o coverage.html
    echo -e "${GREEN}✓ Coverage report generated: coverage.html${NC}"
elif [ -n "$BENCH" ]; then
    # Run benchmarks
    echo "Running benchmarks..."
    go test ./tests/integration -bench=. -benchmem $VERBOSE
else
    # Normal test run
    if go test ./tests/integration/... $VERBOSE $SHORT $SPECIFIC_TEST; then
        echo -e "${GREEN}✓ Integration tests passed${NC}"
    else
        echo -e "${RED}✗ Integration tests failed${NC}"
        exit 1
    fi
fi

# Summary
echo -e "\n${GREEN}=== Test Summary ===${NC}"
echo "Unit Tests: ✓"
echo "Integration Tests: ✓"

if [ -n "$COVERAGE" ]; then
    echo -e "\nCoverage report available at: ${YELLOW}coverage.html${NC}"
    # Show coverage summary
    go tool cover -func=coverage.out | grep total | awk '{print "Total Coverage: " $3}'
fi

echo -e "\n${GREEN}All tests completed successfully! 🎉${NC}"