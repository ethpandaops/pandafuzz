# Documentation and Test Issues

## PF-ISSUE-0101: Project structure doc lists non-existent paths
- Status: fixed
- Severity: medium
- Area: docs
- Type: docs
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: New contributors get incorrect guidance about module locations and expected directories.
- Evidence: `docs/project-structure.md:7` includes paths such as `ai_plans/`, `pkg/auth/`, `pkg/db/`, `pkg/job/`, and `pkg/queue/` that do not exist in the repo.
- Repro: Compare documented tree to the repo root.
- Proposed Fix: Update the directory listing to match the current layout and remove stale entries.
- Test Plan: N/A (documentation update).
- Notes: Also update any references to example config filenames if needed.

## PF-ISSUE-0102: Development doc roadmap/TODO claims are stale
- Status: fixed
- Severity: medium
- Area: docs
- Type: docs
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Readers are told that TODOs are addressed and a roadmap is pending, but TODOs and skipped tests remain.
- Evidence: `docs/development.md:200` lists an implementation schedule; `docs/development.md:228` claims "All TODO comments addressed".
- Repro: Locate TODOs in tests and docs that contradict the statement.
- Proposed Fix: Update the development guide to reflect current status and remove outdated timeline claims.
- Test Plan: N/A (documentation update).
- Notes: Consider linking to the issue tracker instead of hardcoding schedules.

## PF-ISSUE-0103: Python client docs contain TODO placeholder examples
- Status: open
- Severity: medium
- Area: docs
- Type: docs
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: API client docs are incomplete and misleading for users.
- Evidence: `pkg/clients/python/docs/AnalyticsResponse.md:20` (representative); full file list in `docs/issue-tracker/issues/python-client-docs.md`.
- Repro: Open any of the listed docs; see "TODO update the JSON string below".
- Proposed Fix: Regenerate docs or replace TODO placeholders with real examples.
- Test Plan: Regenerate docs in CI or add a lint check for TODO placeholders.
- Notes: Track generation inputs to avoid manual drift.

## PF-ISSUE-0104: Python client mypy strict mode still TODO
- Status: open
- Severity: low
- Area: tooling
- Type: techdebt
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Type checking is less strict than intended; potential issues slip through.
- Evidence: `pkg/clients/python/pyproject.toml:54` has TODO for mypy strict mode.
- Repro: N/A (static config).
- Proposed Fix: Enable strict or document why it is disabled and track remaining violations.
- Test Plan: Run mypy in CI with strict mode once violations are resolved.
- Notes: This is a tooling quality improvement.

## PF-ISSUE-0105: Integration tests skipped/outdated
- Status: fixed
- Severity: medium
- Area: tests
- Type: test
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Critical scenarios (reconnection, auth, corpus handling) are untested or disabled.
- Evidence: `tests/integration/master_bot_test.go:242`, `tests/integration/api_test.go:548`, `tests/integration/fuzzer_test.go:518`.
- Repro: Run tests; these are skipped or commented out.
- Proposed Fix: Redesign tests with controllable retries, update auth tests, and use public APIs for libFuzzer corpus.
- Test Plan: Re-enable the tests and run integration suite.
- Notes: Link fixes to retry configuration and public corpus API changes.

## PF-ISSUE-0106: Duplicate e2e suites/configs make test entrypoints unclear
- Status: fixed
- Severity: low
- Area: tests
- Type: test
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Contributors may run the wrong e2e suite or duplicate coverage work.
- Evidence: `tests/e2e/README.md` and `test/api/e2e/README.md`, plus separate `playwright.config.ts` files.
- Repro: Search for Playwright configs and e2e test directories.
- Proposed Fix: Consolidate or clearly document the intended suite and remove deprecated configs.
- Test Plan: Ensure a single documented command runs the preferred e2e suite.
- Notes: Align with CI setup once consolidated.
