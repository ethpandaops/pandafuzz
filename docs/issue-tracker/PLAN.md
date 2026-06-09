# Hard Changes Plan

This plan lists the high-impact, breaking changes we will execute immediately. No deprecation notices will be issued; breaking changes are expected.

## Priorities
1. Storage and corpus behavior consistency (data integrity)
2. Security posture (auth by default, config enforcement)
3. API contract cleanup (hashes, downloads, error semantics)
4. Frontend consolidation (single dashboard source)
5. Documentation/test truthfulness (remove stale claims)

## Planned Work
- PF-CHANGE-0001: Unify corpus storage paths and hash format (completed)
  - Issues: PF-ISSUE-0001, PF-ISSUE-0003, PF-ISSUE-0004
- PF-CHANGE-0002: Enforce authentication by default (completed)
  - Issues: PF-ISSUE-0010
- PF-CHANGE-0003: Enforce security config limits in API/middleware (completed)
  - Issues: PF-ISSUE-0006, PF-ISSUE-0008, PF-ISSUE-0009
- PF-CHANGE-0004: Consolidate dashboard to a single source of truth (completed)
  - Issues: PF-ISSUE-0201
- PF-CHANGE-0005: Normalize docs/test guidance to match reality (completed)
  - Issues: PF-ISSUE-0101, PF-ISSUE-0102, PF-ISSUE-0105, PF-ISSUE-0106

## Policy Note
If a fix has any reasonable chance of being breaking, mark it as breaking and proceed without deprecation notices.
