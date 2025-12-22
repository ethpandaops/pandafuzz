# Contributing to PandaFuzz

Thank you for your interest in contributing to PandaFuzz! This document provides guidelines and information for contributors.

## Getting Started

### Prerequisites

- Go 1.23+
- Node.js 16+ (for web UI and E2E tests)
- Docker and Docker Compose (for integration testing)
- golangci-lint (for linting)

### Development Setup

1. Clone the repository:
```bash
git clone https://github.com/ethpandaops/pandafuzz.git
cd pandafuzz
```

2. Install dependencies:
```bash
go mod download
cd web && npm install && cd ..
```

3. Build all binaries:
```bash
make build
```

4. Run tests:
```bash
make test
```

## Development Workflow

### Building

```bash
# Build all binaries
make build

# Build only master
make build-master

# Build only bot
make build-bot

# Build web UI
make build-web

# Build Docker images
make docker
```

### Testing

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run integration tests
make test-integration

# Run tests with coverage
make test-coverage

# Run with race detector
go test -race ./...

# Run E2E tests
npm test
```

### Linting

```bash
# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet
```

### Running Locally

```bash
# Run master with config
make run-master

# Run bot with config
make run-bot

# Run with Docker Compose
docker-compose up -d
```

## Code Style

### Go Code

- Follow standard Go conventions and idioms
- Use `gofmt` for formatting
- Run `make lint` before committing
- Add godoc comments to all exported types and functions
- Avoid generic package names (`utils`, `helpers`, `common`)
- Use meaningful variable and function names

### Testing

- Write table-driven tests when testing multiple scenarios
- Use `testify/require` for assertions that should stop the test
- Use `testify/assert` for non-critical assertions
- Mock interfaces, not implementations
- Aim for 70% coverage of critical paths

### Error Handling

- Always handle errors explicitly
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use typed errors from `pkg/errors` for domain-specific errors
- Log errors at appropriate levels

### Documentation

- Add package-level doc.go for each package
- Document all exported types, functions, and methods
- Update docs/ when changing user-facing behavior
- Keep CLAUDE.md up to date with build commands

## Package Structure

The codebase follows domain-driven design:

- `pkg/common/` - Shared types (Job, Bot, CrashResult)
- `pkg/config/` - Configuration management
- `pkg/database/` - Database interfaces
- `pkg/errors/` - Error types
- `pkg/retry/` - Retry logic and circuit breaker
- `pkg/master/` - Master server
- `pkg/bot/` - Bot agent
- `pkg/domain/` - Business logic domains
- `pkg/service/` - Application services
- `pkg/infrastructure/` - Storage, monitoring

## Pull Request Process

1. Create a feature branch from `master`:
```bash
git checkout -b feature/your-feature
```

2. Make your changes with clear, focused commits

3. Ensure all tests pass:
```bash
make test
make lint
```

4. Update documentation if needed

5. Create a pull request with:
   - Clear title describing the change
   - Description of what and why
   - Any breaking changes noted
   - Test plan or verification steps

### PR Review Guidelines

- Keep PRs focused and small when possible
- Respond to feedback constructively
- Request re-review after making changes
- Squash commits before merging if needed

## Commit Messages

Use clear, descriptive commit messages:

```
feat: add crash deduplication using SHA256

Implement hash-based deduplication for crash results to reduce
storage usage and improve analysis efficiency.

- Add Hash field to CrashResult type
- Implement checkCrashDuplicateTx in state_crash.go
- Update crash processing to skip duplicates
```

Prefixes:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Test changes
- `chore:` - Maintenance tasks

## Issue Reporting

When reporting issues, include:

1. **Environment**: OS, Go version, Docker version
2. **Steps to reproduce**: Clear, minimal reproduction steps
3. **Expected behavior**: What should happen
4. **Actual behavior**: What happens instead
5. **Logs**: Relevant log output
6. **Configuration**: Relevant config (sanitized)

## Architecture Decisions

Major architectural decisions should be discussed in issues before implementation. Consider:

- Impact on existing functionality
- Performance implications
- Backward compatibility
- Testing requirements

## Questions?

- Open an issue for questions about the codebase
- Check existing issues and documentation first
- Join discussions in PR reviews

## License

By contributing to PandaFuzz, you agree that your contributions will be licensed under the project's license.
