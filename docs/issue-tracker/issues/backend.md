# Backend and API Issues

## PF-ISSUE-0001: Corpus uploads store metadata only; content never persisted
- Status: fixed
- Severity: high
- Area: api/corpus
- Type: bug/data-loss
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Uploaded corpus data is discarded; later downloads return empty or missing content.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:127` shows UploadCorpus only calls AddFile; `pkg/service/corpus_service.go:147` defines StoreFileContent but it is not invoked.
- Repro: Upload corpus via `/api/v1/corpus`, then attempt download; content is empty unless manually stored.
- Proposed Fix: Store file content during upload via `StoreFileContent` (or direct file storage) and align path usage.
- Test Plan: Add integration test covering upload + download round trip.
- Notes: Also affects corpus dedup and reuse across jobs. Unverified; tests not run.

## PF-ISSUE-0002: Corpus collection uploads store metadata only
- Status: fixed
- Severity: high
- Area: api/corpus
- Type: bug/data-loss
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Collection files are never persisted in file storage, making collections unusable for sync or download.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:1104` reads content but only calls AddCorpusCollectionFile.
- Repro: Upload collection files and attempt to retrieve content; only metadata exists.
- Proposed Fix: Save file content in file storage and persist path/hash alongside metadata.
- Test Plan: Add collection upload + retrieval tests.
- Notes: Consider aligning collection storage with hash-based corpus storage. Unverified; tests not run.

## PF-ISSUE-0003: Corpus download path mismatches hash-based storage; empty content on read errors
- Status: fixed
- Severity: high
- Area: api/corpus
- Type: bug/data-integrity
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Download looks in `corpus/<campaign>/<filename>` but storage uses hash-based paths; failed reads return empty content with 200 OK.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:643` builds filename-based path and returns empty content; `pkg/service/corpus_service.go:448` shows hash-based path convention.
- Repro: Upload corpus (or promote crash), then download; file not found or empty even when content exists.
- Proposed Fix: Use hash-based path for downloads and return a non-200 error on missing content.
- Test Plan: Round-trip test for upload/promote/download; verify content hash.
- Notes: Also hides storage failures by returning empty payloads. Unverified; tests not run.

## PF-ISSUE-0004: Corpus hash format inconsistent (`sha256:` prefix vs raw hex)
- Status: fixed
- Severity: medium
- Area: api/corpus
- Type: bug/consistency
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Deduplication and storage path logic may diverge between uploads and crash promotion.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:172` uses `sha256:` prefix; `pkg/service/corpus_service.go:459` returns raw hex.
- Repro: Upload a seed corpus and promote the same input from a crash; hashes differ.
- Proposed Fix: Standardize hash format across code paths and migrations.
- Test Plan: Dedup test across upload and crash promotion.
- Notes: Aligning hash format will also fix storage path consistency. Unverified; tests not run.

## PF-ISSUE-0005: UploadCorpus panics on invalid campaign ID
- Status: fixed
- Severity: high
- Area: api
- Type: bug
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Invalid `campaign_id` triggers server panic via `uuid.MustParse`.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:209` uses `uuid.MustParse(campaignID)` on user input.
- Repro: POST `/api/v1/corpus` with a non-UUID `campaign_id`.
- Proposed Fix: Validate `campaign_id` with `uuid.Parse` and return 400 on failure.
- Test Plan: Add request validation test for invalid campaign ID.
- Notes: Similar pattern may exist in other endpoints; audit for `MustParse` on user input. Unverified; tests not run.

## PF-ISSUE-0006: Corpus upload reads entire files into memory with no size enforcement
- Status: fixed
- Severity: medium
- Area: api
- Type: perf/security
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Large uploads can exhaust memory or crash the process.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:165` and `pkg/api/v1/adapters/corpus_adapter.go:1143` call `io.ReadAll` without size limits.
- Repro: Upload a very large file; process memory spikes.
- Proposed Fix: Stream to file storage with explicit size limits and early rejection.
- Test Plan: Add size limit tests for corpus and collection uploads.
- Notes: Tie limits to configuration values.

## PF-ISSUE-0007: Corpus collection upload defers file.Close in loop (FD leak risk)
- Status: fixed
- Severity: low
- Area: api
- Type: bug
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Many files in a single request can exhaust file descriptors.
- Evidence: `pkg/api/v1/adapters/corpus_adapter.go:1135` defers Close inside the loop.
- Repro: Upload a large number of files in a single collection request.
- Proposed Fix: Close each file immediately after reading.
- Test Plan: Add stress test for multi-file upload.
- Notes: Also applies to any future multi-file upload handlers.

## PF-ISSUE-0008: readRequestBody panics on unknown Content-Length; read errors swallowed
- Status: fixed
- Severity: medium
- Area: api
- Type: bug
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Chunked/unknown-length requests can panic; read errors return partial data with no error.
- Evidence: `pkg/api/v1/adapters/job_adapter.go:680` uses `make([]byte, 0, r.ContentLength)` and breaks on any read error.
- Repro: Send chunked request (Content-Length = -1).
- Proposed Fix: Handle unknown length safely, and return non-EOF read errors.
- Test Plan: Add tests for chunked and oversized request bodies.
- Notes: Use `io.ReadAll` over `io.LimitReader` or `io.ReadAll` with max bytes.

## PF-ISSUE-0009: SecurityConfig values are defined but not enforced
- Status: fixed
- Severity: medium
- Area: config/security
- Type: techdebt/security
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Configured limits and allowed extensions are ignored by upload endpoints and validation.
- Evidence: `pkg/common/config.go:131` defines security fields; `pkg/api/v1/api.go:40` uses a separate MaxRequestSize config path.
- Repro: Configure `security.allowed_file_extensions` and upload unsupported files; uploads still succeed.
- Proposed Fix: Wire `SecurityConfig` into API/middleware and enforce in upload handlers.
- Test Plan: Add validation tests using config-driven limits.
- Notes: Align with existing validation middleware defaults.

## PF-ISSUE-0010: API auth disabled by default with placeholder JWT secret
- Status: fixed
- Severity: high
- Area: security
- Type: security
- Breaking Change: yes
- Deprecation Notice: none
- Impact: Default configuration leaves the API unauthenticated unless explicitly configured.
- Evidence: `pkg/api/v1/middleware/stack.go:26` sets a placeholder secret; `pkg/api/v1/middleware/stack.go:162` disables JWT auth when placeholder is present.
- Repro: Start server with defaults; endpoints are accessible without auth.
- Proposed Fix: Require explicit opt-out, or fail startup when secret is unset in non-dev environments.
- Test Plan: Add startup/config tests for auth-required settings.
- Notes: Document clear production guidance in deployment docs.

## PF-ISSUE-0011: Debug messages emitted at Info level
- Status: open
- Severity: low
- Area: logging
- Type: techdebt
- Breaking Change: no
- Deprecation Notice: n/a
- Impact: Noisy logs in production and reduced signal-to-noise.
- Evidence: `pkg/service/job_service.go:81` and `pkg/storage/sqlite.go:1019` log "DEBUG" messages at Info level.
- Repro: Create a job and observe Info logs.
- Proposed Fix: Downgrade to Debug level or remove.
- Test Plan: Manual log inspection or unit tests for logging level (optional).
- Notes: Consider centralizing log-level controls.
