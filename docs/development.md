# PandaFuzz Development Guide

## Prerequisites

- Go 1.21+
- Node.js 18+ (web UI and e2e tests)
- Docker + Docker Compose (optional)
- AFL++/LibFuzzer/Honggfuzz (optional, for fuzzer integration tests)

## Quick Start

```bash
# Build binaries
make build

# Run master
./pandafuzz-master -config configs/master.yaml

# Run bot
./pandafuzz-bot -config configs/bot.example.yaml
```

## Authentication (Default On)

Authentication is required by default.

- Configure either `security.jwt_secret` or `security.api_keys` in the master config.
- For local dev without auth, set `security.allow_insecure: true` explicitly.
- Bots must set `api_key` when using API keys.
- The web UI reads `REACT_APP_API_KEY` at build time or `localStorage.api_key` at runtime.

## Build Commands

```bash
make build
make build-master
make build-bot
```

## Web UI

```bash
make web-dev     # Dev server
make build-web   # Production build
```

## Tests

```bash
# Go tests
go test ./...

# Unit tests
go test ./tests/unit/...

# Integration tests
go test ./tests/integration/...

# E2E tests (Playwright)
npm install
npm test
```

## Configuration

- Master config: `configs/master.yaml`
- Bot config: `configs/bot.example.yaml`
- Set `PANDAFUZZ_CONFIG` or pass `-config` to binaries.

## Issue Tracker

Use `docs/issue-tracker/` for current status, changes, and planned work. Avoid duplicating roadmaps in docs.
