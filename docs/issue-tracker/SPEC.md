# Issue and Change Tracking Spec

## Purpose
Provide a consistent, low-friction format for recording issues, tracking fixes, and proposing future changes.

## Directory Layout
- `docs/issue-tracker/ISSUES_INDEX.md`: summary list of all issues.
- `docs/issue-tracker/issues/*.md`: detailed issue records grouped by area.
- `docs/issue-tracker/PLAN.md`: current plan for high-impact changes.
- `docs/issue-tracker/changes/`: future change proposals (add as needed).
- `docs/issue-tracker/issues/python-client-docs.md`: appendix for large file lists.

## ID Conventions
- Issues: `PF-ISSUE-0001` (sequential, zero-padded to 4 digits).
- Changes: `PF-CHANGE-0001` (sequential, zero-padded to 4 digits).

## Status Values
- `open`
- `triaged`
- `in_progress`
- `blocked`
- `fixed`
- `verified`
- `wont_fix`

## Severity Levels
- `critical`: exploit/data loss, system outage, or security exposure
- `high`: significant data integrity or service reliability risk
- `medium`: user-facing defects, test gaps, or maintainability risks
- `low`: polish, log noise, or small paper cuts

## Breaking Change Policy
- When compatibility is unclear, assume breaking and proceed without deprecation notices.
- Record breaking changes explicitly in issues and change records.
- No deprecation windows; plan and execute breaking changes directly.

## Issue Record Template
Use this in the appropriate `issues/*.md` file.

```md
## PF-ISSUE-0000: Short title
- Status: open
- Severity: medium
- Area: api/storage/docs/tests/frontend
- Type: bug/security/perf/docs/test/techdebt
- Breaking Change: yes/no (assume yes if in doubt)
- Deprecation Notice: none/n-a
- Impact: What breaks and who it affects
- Evidence: `path/to/file.go:123` (add multiple lines if needed)
- Repro: Minimal steps or conditions, or "not observed" if purely static
- Proposed Fix: High-level approach (no implementation detail)
- Test Plan: Expected tests or validation steps
- Notes: Optional context or follow-ups
```

## Change Record Template
Use this for future changes and refactors under `docs/issue-tracker/changes/`.

```md
## PF-CHANGE-0000: Short title
- Status: proposed
- Type: feature/refactor/docs/infra
- Motivation: Why this change is needed
- Scope: What is in/out of scope
- Design Notes: Key design decisions and constraints
- Backwards Compatibility: Impact on APIs, configs, or storage
- Breaking Change: yes/no (assume yes if in doubt)
- Deprecation Notice: none/n-a
- Rollout Plan: Steps and migration notes
- Test Plan: Required tests and validation
- Risks: Known risks or unknowns
```

## Update Rules
- Add every new issue to `ISSUES_INDEX.md`.
- Keep the issue ID stable; update status instead of creating duplicates.
- Include file references with line numbers when evidence exists.
- Close issues only after test/verification evidence is recorded.
