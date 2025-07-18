# PandaFuzz Repository Structure

## Core Directories

### `/cmd`
Contains the main application entry points:
- `master/` - Master server application
- `bot/` - Fuzzing bot application

### `/pkg`
Core package implementations:
- `bot/` - Bot implementation (includes AFL++ crash detection fixes)
- `fuzzer/` - Fuzzer implementations (AFL++, LibFuzzer, etc.)
- `master/` - Master server implementation
- `service/` - Business logic services
- `storage/` - Database and storage implementations
- `common/` - Shared types and interfaces

### `/web`
Web UI frontend application (React)

### `/scripts`
Utility scripts for operations:
- `test-fuzzers.sh` - Comprehensive test for AFL++ and LibFuzzer
- `build-docker.sh` - Build Docker images
- `create-job.sh` - Create fuzzing jobs
- See `scripts/README.md` for full list

### `/tests`
Test suites:
- `unit/` - Unit tests
- `integration/` - Integration tests
- `e2e/` - End-to-end tests

### `/test-resources`
Consolidated test resources directory containing:
- `test-targets/` - Example fuzzing targets for testing:
  - `crashers/` - Programs that crash on specific inputs
  - `fuzzers/` - Fuzzer-compatible test programs
  - `vulnerable/` - Intentionally vulnerable programs
- `test-data/` - Test data and seed inputs for fuzzing
- `test-corpus/` - Sample corpus files for testing

### `/test-documentation`
Documentation related to testing and fixes:
- `AFL_PLUS_PLUS_FIXES.md` - Details of AFL++ crash detection fixes
- `REPOSITORY_STRUCTURE.md` - This file

## Configuration Files

- `docker-compose.yml` - Docker Compose configuration
- `Dockerfile` - Container build configuration
- `master-docker.yaml` - Master server configuration
- `go.mod`, `go.sum` - Go module dependencies
- `package.json` - Node.js dependencies for web UI
- `playwright.config.ts` - E2E test configuration

## Key Code Changes for AFL++ Fixes

1. **`pkg/bot/agent.go`**
   - Fixed crash detection path for AFL++ (`output/afl_output/crashes/`)
   - Added backward compatibility check

2. **`pkg/fuzzer/aflplusplus.go`**
   - Added OutputWriter support for log capture
   - Implemented periodic crash checking during execution

## Testing the Fixes

To test AFL++ and LibFuzzer crash detection:
```bash
./scripts/test-fuzzers.sh
```

This script will:
- Create test binaries that crash on specific inputs
- Upload them as fuzzing jobs
- Monitor crash detection and log capture
- Verify both AFL++ and LibFuzzer work correctly