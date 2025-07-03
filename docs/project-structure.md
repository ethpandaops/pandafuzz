# PandaFuzz Project Structure

This document describes the organization of the PandaFuzz codebase after the cleanup and reorganization.

## Directory Structure

```
pandafuzz/
├── ai_plans/               # AI-generated implementation plans
│   ├── archived/          # Completed implementation plans
│   └── *.md              # Active implementation plans
│
├── cmd/                   # Application entry points
│   ├── bot/              # Fuzzing bot executable
│   └── master/           # Master server executable
│
├── configs/               # Example configuration files
│   ├── bot.example.yaml  # Example bot configuration
│   └── bot.docker.example.yaml  # Example Docker configuration
│
├── data/                  # Runtime data (gitignored)
│   ├── jobs/             # Job artifacts and results
│   ├── campaigns/        # Campaign data
│   └── *.db             # SQLite databases
│
├── docs/                  # Documentation
│   ├── api.md           # API reference
│   ├── architecture.md   # System architecture
│   ├── development.md    # Development guide
│   ├── project-structure.md  # This file
│   └── archive/         # Historical documentation
│
├── pkg/                   # Go packages
│   ├── analysis/         # Crash analysis
│   ├── api/             # REST API definitions
│   ├── auth/            # Authentication
│   ├── bot/             # Bot implementation
│   ├── config/          # Configuration management
│   ├── db/              # Database abstraction
│   ├── errors/          # Error handling
│   ├── fuzzer/          # Fuzzer interfaces
│   ├── httputil/        # HTTP utilities
│   ├── job/             # Job management
│   ├── master/          # Master server
│   ├── monitoring/      # Metrics collection
│   ├── queue/           # Job queue
│   ├── retry/           # Retry logic
│   ├── storage/         # File storage
│   └── types/           # Shared types
│
├── scripts/               # Shell scripts
│   ├── create_job.sh    # Unified job creation script
│   ├── run-e2e-tests.sh # End-to-end test runner
│   ├── run_tests.sh     # Unit test runner
│   └── test_crash_report.sh  # Crash reporting test
│
├── test_data/             # Test data
│   ├── seeds/           # Fuzzing seed inputs
│   └── corpus/          # Test corpus (generated)
│
├── test_targets/          # Test programs
│   ├── crashers/        # Programs that crash
│   ├── fuzzers/         # Fuzzer test harnesses
│   └── vulnerable/      # Vulnerable test programs
│
├── tests/                 # Integration tests
│   └── e2e/             # End-to-end tests
│
└── web/                   # Web UI
    ├── public/          # Static assets
    ├── src/             # React source code
    └── package.json     # Node.js dependencies
```

## File Organization Guidelines

### Where to Put New Files

1. **Go Code**
   - Application logic: `pkg/<package>/`
   - Entry points: `cmd/<app>/`
   - Shared types: `pkg/types/`

2. **Test Files**
   - Unit tests: Same directory as code (`*_test.go`)
   - Integration tests: `tests/`
   - Test programs: `test_targets/<category>/`
   - Test data: `test_data/`

3. **Scripts**
   - All shell scripts: `scripts/`
   - Name clearly with `.sh` extension

4. **Documentation**
   - User documentation: `docs/`
   - API documentation: `docs/api.md`
   - Old/outdated docs: `docs/archive/`

5. **Configuration**
   - Example configs: `configs/*.example.yaml`
   - Runtime configs: Root directory (gitignored)

## Naming Conventions

### Files
- Go files: `lowercase_with_underscores.go`
- Test files: `*_test.go`
- Scripts: `descriptive-name.sh`
- Documentation: `UPPERCASE.md` or `lowercase.md`

### Packages
- Use singular nouns: `storage` not `storages`
- Be descriptive: `monitoring` not `mon`
- Avoid generic names: `fuzzer` not `utils`

## Development Workflow

### Adding New Features
1. Create feature branch from `master`
2. Add code in appropriate `pkg/` subdirectory
3. Add tests in same directory
4. Update documentation if needed
5. Run tests: `go test ./...`
6. Submit PR with clear description

### Running Tests
```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Integration tests
./scripts/run-e2e-tests.sh
```

### Building
```bash
# Build master
go build -o master ./cmd/master

# Build bot
go build -o bot ./cmd/bot

# Build with Docker
docker-compose build
```

## Important Notes

1. **Never commit**:
   - Compiled binaries (covered by .gitignore)
   - Actual config files (only examples)
   - Database files
   - Crash artifacts

2. **Always include**:
   - Tests for new features
   - Documentation updates
   - Example configs for new options

3. **Before committing**:
   - Run `go mod tidy`
   - Run `go fmt ./...`
   - Run tests
   - Update relevant documentation