# Issue Index

Plan and breaking-change policy live in `docs/issue-tracker/PLAN.md` and `docs/issue-tracker/SPEC.md`.

| ID | Severity | Area | Title | Status | Details |
| --- | --- | --- | --- | --- | --- |
| PF-ISSUE-0001 | high | api/corpus | Corpus uploads store metadata only; content never persisted | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0002 | high | api/corpus | Corpus collection uploads store metadata only | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0003 | high | api/corpus | Corpus download path mismatches hash-based storage; empty content on read errors | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0004 | medium | api/corpus | Corpus hash format inconsistent (`sha256:` prefix vs raw hex) | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0005 | high | api | UploadCorpus panics on invalid campaign ID | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0006 | medium | api | Corpus upload reads entire files into memory with no size enforcement | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0007 | low | api | Corpus collection upload defers file.Close in loop (FD leak risk) | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0008 | medium | api | readRequestBody panics on unknown Content-Length; read errors swallowed | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0009 | medium | config/security | SecurityConfig values are defined but not enforced | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0010 | high | security | API auth disabled by default with placeholder JWT secret | fixed | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0011 | low | logging | Debug messages emitted at Info level | open | `docs/issue-tracker/issues/backend.md` |
| PF-ISSUE-0101 | medium | docs | Project structure doc lists non-existent paths | fixed | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0102 | medium | docs | Development doc roadmap/TODO claims are stale | fixed | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0103 | medium | docs | Python client docs contain TODO placeholder examples | open | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0104 | low | tooling | Python client mypy strict mode still TODO | open | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0105 | medium | tests | Integration tests skipped/outdated | fixed | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0106 | low | tests | Duplicate e2e suites/configs make test entrypoints unclear | fixed | `docs/issue-tracker/issues/docs-and-tests.md` |
| PF-ISSUE-0201 | low | frontend | Duplicate dashboard codebases (web vs pkg/web/dashboard) | fixed | `docs/issue-tracker/issues/frontend.md` |
