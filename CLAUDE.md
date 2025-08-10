# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

### Go Backend
- `make build`: Build both master and bot binaries
- `make build-master`: Build only the master binary  
- `make build-bot`: Build only the bot binary
- `make test`: Run all tests using run_tests.sh script
- `make test-unit`: Run unit tests only
- `make test-integration`: Run integration tests only
- `make test-coverage`: Run tests with coverage report
- `make lint`: Run golangci-lint
- `make fmt`: Format Go code
- `make vet`: Run go vet

### Web UI
- `make build-web`: Build the React web UI
- `make web-dev`: Run web UI in development mode (port 3000, proxies to 8080)
- `cd web && npm run lint`: Run ESLint on web code
- `cd web && npm run type-check`: Run TypeScript type checking

### Docker
- `make docker`: Build Docker images for master and bot
- `make docker-compose`: Start services with docker-compose
- `docker-compose up -d`: Start all services
- `docker-compose logs -f`: View logs

### E2E Testing
- `npm test`: Run Playwright E2E tests
- `npm run test:ui`: Run Playwright tests with UI
- `npm run e2e`: Run E2E tests with Docker

### Development Tools
- `make run-master`: Run master locally with master.yaml config
- `make run-bot`: Run bot locally with bot.yaml config
- `./scripts/run-test-with-corpus.sh [afl++|libfuzzer|both]`: Test fuzzer integration
- `./scripts/run-test-with-coverage.sh [afl++|libfuzzer|both]`: Test coverage collection

## Architecture Overview

PandaFuzz is a distributed fuzzing orchestration system designed for reliability and fault tolerance:

### Core Components

1. **Master Node** (`cmd/master/`, `pkg/master/`)
   - Single coordination point with exclusive write access to storage
   - SQLite database for state management and persistence
   - RESTful API v3 with OpenAPI spec at `pkg/master/api_v3/openapi.yaml`
   - Job assignment, bot registration, and result collection
   - Automatic recovery from crashes with full state restoration

2. **Bot Agent** (`cmd/bot/`, `pkg/bot/`)
   - Executes fuzzing jobs with AFL++, LibFuzzer, or Honggfuzz
   - Heartbeat mechanism with configurable timeouts (default 30s)
   - Reports results via API to master
   - Automatic cleanup and resource monitoring

3. **Storage Backend** (`pkg/infrastructure/storage/`)
   - Abstracted interface supporting filesystem and S3
   - Master-exclusive write pattern prevents conflicts
   - Structured layout: `/corpus/{job_id}/`, `/crashes/{job_id}/`
   - SHA256-based deduplication for corpus and crashes

4. **Domain Layer** (`pkg/domain/`)
   - Clean architecture with separated business logic
   - Key packages:
     - `campaign/`: Campaign orchestration and lifecycle
     - `job/`: Job execution with async queue support
     - `bot/`: Bot registry and scheduler
     - `corpus/`: Corpus selection, sync, and quarantine
     - `crash/`: Crash deduplication and minimization
     - `fuzzer/`: Fuzzer factory and engine interfaces

### Key Design Patterns

- **Master-Only Writes**: Prevents filesystem conflicts
- **Atomic Operations**: Race-condition-free job assignment  
- **Timeout Everything**: All operations have configurable timeouts
- **Persistent State**: Complete recovery from any failure
- **Repository Pattern**: Clean data access abstraction

### API Endpoints

The system provides RESTful APIs (v1 for core, v3 for extended features):
- `/api/v3/bots`: Bot management and monitoring
- `/api/v3/campaigns`: Campaign orchestration
- `/api/v3/jobs`: Job submission and monitoring
- `/api/v3/jobs/{id}/coverage`: Coverage report access
- `/api/v3/corpus`: Corpus management and sync
- `/api/v3/crashes`: Crash analysis and deduplication

### Fuzzer Engines

All fuzzers implement `types.Fuzzer` interface (`pkg/domain/fuzzer/types/interface.go`):
- **LibFuzzer** (`pkg/domain/fuzzer/engines/libfuzzer/`): In-process, coverage-guided
- **AFL++** (`pkg/domain/fuzzer/engines/aflplusplus/`): Fork-based, instrumentation
- **Honggfuzz** (`pkg/domain/fuzzer/engines/honggfuzz/`): Multi-threaded, hardware-based

### Testing

- Unit tests: Alongside code (`*_test.go`)
- Integration tests: `tests/integration/`
- E2E tests: Playwright in `tests/e2e/`
- Test corpus: `test-resources/`