# Frontend Issues

## PF-ISSUE-0201: Duplicate dashboard codebases (web vs pkg/web/dashboard)
- Status: fixed
- Severity: low
- Area: frontend
- Type: techdebt
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Two React dashboards exist with overlapping scope, creating ambiguity and split maintenance.
- Evidence: `Makefile:37` uses `web/` for builds; `pkg/web/dashboard/README.md:1` documents a separate dashboard.
- Repro: Search for dashboard directories and build targets.
- Proposed Fix: Pick a single source of truth or clearly mark one as deprecated.
- Test Plan: Verify the chosen dashboard builds and runs via documented commands.
- Notes: Consider consolidating API client usage and design tokens.
