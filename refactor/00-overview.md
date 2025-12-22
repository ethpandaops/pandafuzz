# PandaFuzz Refactoring Plan

## Overview

This directory contains a sequenced refactoring plan to address usability and maintainability issues identified in the codebase. Each file is numbered to indicate execution order and designed to minimize duplicate work.

## Execution Order

| Sequence | File | Description | Estimated Impact |
|----------|------|-------------|------------------|
| 01 | `01-replace-panics-with-errors.md` | Replace panic() calls with error returns | Low risk, isolated changes |
| 02 | `02-consolidate-fuzzer-packages.md` | Merge duplicate fuzzer implementations | High impact, foundational |
| 03 | `03-refactor-common-package.md` | Split pkg/common/ into domain packages | High impact, many file changes |
| 04 | `04-consolidate-type-definitions.md` | Remove duplicate Job/Bot/etc types | Medium impact |
| 05 | `05-refactor-state-god-file.md` | Split pkg/master/state.go | Medium impact |
| 06 | `06-unify-api-versions.md` | Consolidate API v1 and v3 | Medium impact |
| 07 | `07-fix-broken-tests.md` | Update/remove broken test code | Low risk |
| 08 | `08-implement-analytics-stubs.md` | Complete analytics service | Low risk, additive |
| 09 | `09-configuration-cleanup.md` | Simplify configuration structure | Low risk |
| 10 | `10-documentation-updates.md` | Update docs and godoc comments | No risk |

## Key Principles

1. **Sequence Matters**: Complete earlier items before later ones to avoid rework
2. **Test After Each Step**: Run `make test` after each refactoring item
3. **Incremental Commits**: Commit after each file's changes are complete
4. **Preserve Invariants**: Each file lists invariants that must not change
5. **Feature Parity**: No functionality should be lost during refactoring

## Verification Commands

After each refactoring step, run:

```bash
# Build verification
make build

# Lint verification
make lint

# Test verification
make test

# Docker verification (for integration)
docker-compose build --no-cache
docker-compose up -d
./scripts/run-test-with-corpus.sh both
```

## Dependencies Between Items

```
01 (panics) ─────────────────────────────────────────┐
                                                      │
02 (fuzzer) ──┬──> 03 (common) ──> 04 (types) ──────>├──> 07 (tests)
              │                                       │
              └──> 05 (state) ──> 06 (api) ──────────┤
                                                      │
                                    08 (analytics) ───┤
                                                      │
                                    09 (config) ──────┤
                                                      │
                                    10 (docs) ────────┘
```

## Before Starting

1. Ensure all current tests pass: `make test`
2. Create a feature branch: `git checkout -b refactor/codebase-cleanup`
3. Back up the database if running in production
4. Review each plan file completely before starting that item

## Progress Tracking

Mark items as complete by updating this file:

- [ ] 01-replace-panics-with-errors
- [ ] 02-consolidate-fuzzer-packages
- [ ] 03-refactor-common-package
- [ ] 04-consolidate-type-definitions
- [ ] 05-refactor-state-god-file
- [ ] 06-unify-api-versions
- [ ] 07-fix-broken-tests
- [ ] 08-implement-analytics-stubs
- [ ] 09-configuration-cleanup
- [ ] 10-documentation-updates
