# PandaFuzz Project Structure

This document describes the current PandaFuzz layout. For status and roadmap, see `docs/issue-tracker/`.

## Directory Structure

```
pandafuzz/
├── cmd/                # Go entrypoints (master, bot)
├── pkg/                # Go packages (api, bot, common, domain, service, storage, etc.)
├── web/                # React dashboard
├── configs/            # Example YAML configs
├── docs/               # Documentation and issue tracker
├── scripts/            # Utilities and tooling scripts
├── tests/              # Unit, integration, and e2e tests
├── test-resources/     # Test programs, seeds, and fixtures
├── migrations/         # Database migrations
├── docker/             # Docker assets
└── ...
```

## File Organization Guidelines

### Where to Put New Files

1. **Go Code**
   - Application logic: `pkg/<package>/`
   - Entry points: `cmd/<app>/`

2. **Tests**
   - Unit tests: alongside code (`*_test.go`) or `tests/unit/`
   - Integration tests: `tests/integration/`
   - E2E tests (Playwright): `tests/e2e/`

3. **Scripts**
   - Shell scripts and helpers: `scripts/`

4. **Documentation**
   - Docs: `docs/`
   - Issue tracker: `docs/issue-tracker/`

5. **Configuration**
   - Examples: `configs/`
   - Local configs: repo root (gitignored)

## Development Workflow

1. Create a branch from `master`.
2. Add code in the appropriate `pkg/` package or `cmd/` entrypoint.
3. Add or update tests.
4. Update docs/configs if behavior changes.
5. Run tests and linting.

## Running Tests

```bash
# Go tests
go test ./...

# Integration tests
go test ./tests/integration/...

# E2E tests (Playwright)
npm install
npm test
```

## Building

```bash
# Build master
make build-master

# Build bot
make build-bot

# Build web UI
make build-web
```

## Notes

- Use `docs/issue-tracker/` for the current state, issues, and changes.
- Keep example configs in `configs/` in sync with defaults and breaking changes.
