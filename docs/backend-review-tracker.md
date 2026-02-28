# Backend Core Pipeline Review Tracker

Last updated: 2026-02-07
Scope: backend core functionality only (`search`, `downloading`, `scheduling`)
Source of truth: `openspec/specs/webserver-downloads/spec.md`, `openspec/specs/torrenter-downloads/spec.md`, `CLAUDE.md`

Severity:
- `P0`: core flow broken for common usage
- `P1`: high-risk correctness or contract gap
- `P2`: meaningful reliability/confidence gap

Status:
- `Open`, `In Progress`, `Blocked`, `Done`

---

## Search Pipeline

### Intended Flow (spec/code intent)
1. UI calls `GET /api/search?query=...&media_type=...`.
2. webserver `SearchHandler` -> `SearchInteractor` -> `TVDBProxyClient.Search()`.
3. webserver calls tvdb-proxy `GET /series?mediaType=...&mediaName=...`.
4. tvdb-proxy queries TVDB and returns parsed media list.
5. webserver returns `200` media list.

Enriched path:
1. UI calls `GET /api/search/enriched?query=...&media_type=...`.
2. webserver `EnrichedSearchHandler` -> `EnrichedSearchInteractor`.
3. Interactor orchestrates search + status enrichment and returns `200` list.

### Verification Evidence
- Runtime checks on host:
  - `2026-02-07`: `GET /api/search?query=The%20Office&media_type=series` -> `200`.
  - `2026-02-07`: `GET /api/search/enriched?query=The%20Office&media_type=series` -> `200`.
- Direct tvdb-proxy checks:
  - `GET http://localhost:22000/series?mediaName=The%20Office&mediaType=series` -> `200`.
- webserver logs captured 500 for spaced query request.
- Unit tests that run in this environment:
  - `backend/internal/webserver/interactors` -> pass.
  - `backend/internal/webserver/handlers` -> pass.
- Environment limitation:
  - `backend/internal/webserver/clients` tests blocked by sandbox socket restrictions (`httptest` bind on `[::1]:0`).

### Findings

| ID | Severity | Status | Confidence | Evidence | Symptom/Risk | Expected Intent | Proposed Fix | Validation |
|---|---|---|---|---|---|---|---|---|
| SEARCH-001 | P0 | Done | Confirmed | `backend/internal/webserver/clients/tvdb_client.go:30` | Spaced series query fails (`500`) while one-word works. Core user search path is unreliable. | Any valid query string should be passed losslessly to tvdb-proxy and return consistent results. | Build request URL with `url.Values` / proper query escaping instead of string interpolation. | Live verified after redeploy: spaced query returns `200` on both `/search` and `/search/enriched`. |
| SEARCH-002 | P1 | In Progress | Confirmed | `backend/internal/webserver/handlers/search_handler.go:39` | Potential panic on empty result set due to `searchResults[0]` log access. | Empty results should return `200` with `[]`, never panic. | Guard `len(searchResults)` before indexing; remove debug-style logging dependency on first element. | Code fix landed; full handler empty-result test still pending due concrete interactor wiring constraints. |
| SEARCH-003 | P1 | In Progress | Confirmed | `backend/cmd/tvdb-proxy/main.go:199`, `:213`, `:225` | Concurrent goroutines append to shared `results` slice without synchronization; race/corruption risk under load. | Search aggregation should be concurrency-safe and deterministic. | Protect shared slice with mutex or channel fan-in and aggregate on single goroutine. | Mutex-based fix landed; run `go test -race` on tvdb-proxy search path in full CI/runtime environment. |

### Exit Criteria (Search)
1. Spaced and unspaced queries for both `series` and `movie` return `200` on `/api/search`.
2. `/api/search/enriched` returns `200` for same query set.
3. Empty-result scenario returns `200 []` with no panic path.
4. Search aggregation path is race-free in test evidence.

---

## Downloading Pipeline

### Intended Flow (spec/code intent)
1. webserver receives:
   - `POST /shows` for show/episode payloads.
   - `POST /movies` for movie payloads.
2. webserver download interactor:
   - Enriches metadata and anime classification.
   - For immediately eligible content, calls torrenter `POST /download`.
   - For future content, schedules in `ScheduledDownloads`.
3. torrenter processes download:
   - Duplicate checks (Plex/library/TVDB ID matching where applicable).
   - Search strategy -> Prowlarr -> qBittorrent.
   - Post-processing + history/notification updates.

### Verification Evidence
- Runtime checks on host:
  - `2026-02-07`: `POST /api/shows` with `{}` -> `400`.
  - Post-redeploy: `POST /movies` with non-movie category (`{"type":"series"}`) -> `400`.
  - `2026-02-07`: `POST /api/movies` with valid movie payload while torrenter path unavailable -> `500` and row persisted in `scheduleddownloads` for follow-up.
  - Intermediate post-redeploy: torrenter `GET /media/123` -> `500` with `pq: relation "media" does not exist`.
  - Intermediate post-redeploy: torrenter `POST /media/exists` -> `500` (`failed to check media existence`).
  - Final post-fix redeploy: torrenter `GET /media/123` -> `404`.
  - Final post-fix redeploy: torrenter `POST /media/exists` -> `200` with `{"exists":{"123":false,"456":false},"in_progress":{}}`.
- Relevant package tests passing in this environment:
  - `backend/internal/webserver/handlers`, `interactors`, `repository`.
  - `backend/internal/torrenter/repository`.
- Critical confidence gaps in tests:
  - `backend/internal/webserver/clients` tests blocked by environment sockets.
  - Full backend `go test ./...` previously showed torrenter service test compile mismatch due interface/mock drift.

### Findings

| ID | Severity | Status | Confidence | Evidence | Symptom/Risk | Expected Intent | Proposed Fix | Validation |
|---|---|---|---|---|---|---|---|---|
| DL-001 | P0 | Done | Confirmed | Runtime `POST /shows` with `{}` -> `200`; handler path `backend/internal/webserver/handlers/download_handler.go` | Invalid show payload acknowledged as success; core pipeline can silently no-op while reporting success. | Invalid/incomplete payload should be rejected with `400` and clear error. | Add explicit required-field validation in webserver show download path (ID/name/category/episodes contract). | Validation logic landed + tests in `backend/internal/webserver/handlers/download_handler_test.go`; live verification now returns `400`. |
| DL-002 | P1 | Done | Confirmed | `backend/internal/webserver/clients/torrenter_client.go:86-87`, `backend/internal/torrenter/repository/postgres.go:175` | Batch route existed but originally failed at runtime due repository query to non-existent `Media` table. | Service-client contracts should be aligned and backed by correct storage query model. | Implement matching torrenter batch endpoint and back it with schema-valid existence checks across `Shows`/`Movies`/`Episodes`. | Live verified after final redeploy: `GET /media/:hash` returns `404` when missing, and `POST /media/exists` returns `200` map response. |
| DL-003 | P2 | Done | Confirmed | `backend/internal/torrenter/service/interfaces.go:52` vs missing method in generated mock used by tests | Interface drift broke service test compile in broader run; reduces confidence in download core changes. | Core pipeline interfaces should stay test-covered and build-clean. | Regenerate/update mocks when repository interface changes; enforce in CI. | Missing mock method added in both repository mock files; compile checks pass in touched packages. |

### Exit Criteria (Downloading)
1. Invalid show/movie payloads are rejected deterministically (`400`).
2. Valid payloads route and return expected response semantics.
3. Service-to-service contracts used by download orchestration are aligned (no dead routes).
4. Download service unit tests compile cleanly with current interfaces.

---

## Scheduling Pipeline

### Intended Flow (spec/code intent)
1. webserver interactor decides immediate vs future execution using air/release date.
2. Future items stored in `ScheduledDownloads` with content hash dedup.
3. Scheduler polls due entries, marks queued, sends media to torrenter.
4. Weekly schedule endpoint exposes queued/pending upcoming items (`GET /schedule/weekly`).

### Verification Evidence
- Runtime checks on host:
  - `2026-02-07`: `GET /api/schedule/weekly` -> `200`.
  - Database snapshots show failed attempts tracked on `scheduleddownloads` with `retry_count`, `last_error`, `next_attempt_at`.
- webserver logs show periodic `/tvdb/batch/episodes` activity from refresh loop; scheduling infrastructure active.
- Test evidence:
  - `backend/internal/webserver/scheduler` -> pass.
  - `backend/internal/webserver/repository` (includes scheduler repository tests) -> pass.

### Findings

| ID | Severity | Status | Confidence | Evidence | Symptom/Risk | Expected Intent | Proposed Fix | Validation |
|---|---|---|---|---|---|---|---|---|
| SCH-001 | P1 | In Progress | Confirmed | `backend/internal/webserver/interactors/download_interactor.go:134-138` | Special-episode path `continue` skips `episodeSpan.End()`; trace/span leak risk in scheduling-related flow telemetry. | Every created span should end on all control paths. | Ensure `episodeSpan.End()` always executes (defer per-iteration or explicit end before continue). | Span-end fix landed; verify with SignOz trace sampling on special-episode requests in deployed env. |
| SCH-002 | P1 | In Progress | Confirmed | `backend/internal/webserver/interactors/download_interactor.go:295-312` | Date comparisons use local `time.Now()` + parsed date without explicit timezone normalization; boundary-day scheduling ambiguity possible. | Release/air-date comparisons should be deterministic across host timezone and date boundaries. | Normalize to date-only UTC (or explicit configured timezone) before compare. | `normalizeDateUTC` tests added in `backend/internal/webserver/interactors/download_interactor_test.go` and same-day movie scheduling test passing; still needs live scheduling boundary verification. |
| SCH-003 | P2 | Done | Confirmed | `sql/setup-postgres.sql:103` and `:107` duplicate index statement | Schema script duplication erodes migration hygiene/confidence for scheduling DB bootstrap. | Bootstrap SQL should be idempotent and free from accidental duplicates/noise. | Remove duplicate index declaration and tighten bootstrap review checks. | Duplicate index declaration removed. |
| SCH-004 | P0 | Done | Confirmed | `backend/internal/webserver/repository/scheduler_repository.go:145`, `backend/internal/webserver/scheduler/scheduler.go:62` | Jobs were marked `queued` before execution and relied on in-memory timers. On restart/crash, scheduled jobs could be stranded forever as `queued`. | Scheduler should be crash-recoverable with DB as source of truth for execution claim/due state. | Query only due-now jobs; atomically claim rows in DB (`FOR UPDATE SKIP LOCKED`) and recover stale `queued` jobs after timeout. | New repository transaction logic + scheduler tests pass (`go test ./internal/webserver/repository ./internal/webserver/scheduler`). |
| SCH-005 | P0 | Done | Confirmed | `backend/internal/webserver/scheduler/scheduler.go` (prior logic) + `scheduleddownloads.next_attempt_at` behavior | Retry jobs used metadata aired date in scheduler instead of DB retry timestamp, causing early/incorrect execution and retry storms. | Retry due-time must come from DB fields (`next_attempt_at`/`release_time`) only. | Move due-time decision entirely into repository query; scheduler only dequeues rows already due. | Verified by code path and passing scheduler/repository tests after refactor. |
| SCH-006 | P1 | Done | Confirmed | `backend/internal/webserver/repository/scheduler_repository.go:107`, `:183`, `:226` | Movie follow-up retries could hash by retry date at schedule time but by first-air-date at completion/requeue time, causing orphaned rows and failed retry bookkeeping. | Content hash for a logical movie item must be stable across schedule/retry/complete stages. | Normalize movie hash to prefer `metadata.firstAired` across all repository operations. | Verified by repository tests and live DB behavior (duplicate follow-up rows no longer multiplying for same movie request). |

### Exit Criteria (Scheduling)
1. Future content scheduling behavior is deterministic at date boundaries.
2. Due-media execution flow remains observable without span leaks.
3. Scheduler jobs remain recoverable across restarts/crashes (no permanent `queued` dead state).
4. Retry timing is respected from DB state (`next_attempt_at`) and not derived from media metadata.
5. DB bootstrap/migration scripts are clean and consistent for scheduling tables/indexes.
6. `/api/schedule/weekly` remains healthy with representative pending/queued data.

---

## Cross-Pipeline Notes

### Environment Constraints (during this review)
1. `backend/internal/webserver/clients` tests could not run in this sandbox due `httptest` socket bind restrictions.
2. Some broader test failures seen earlier were environment-related, but interface drift and runtime API evidence above are real findings.

### Execution Order
1. Fix `SEARCH-001` and `DL-001` first (P0 core-breakers).
2. Then resolve P1 contract/correctness issues (`SEARCH-002`, `SEARCH-003`, `DL-002`, `SCH-001`, `SCH-002`).
3. Close P2 confidence items (`DL-003`, `SCH-003`).

### Verification Log
- 2026-02-06: Initial core review seeded with live endpoint checks, logs, and runnable backend tests.
- 2026-02-06: Implemented fixes for SEARCH-001/002/003, DL-001/002/003, SCH-001/002/003.
- 2026-02-06: Added tests `backend/internal/webserver/handlers/download_handler_test.go`, `backend/internal/torrenter/handlers/media_handler_test.go`, and `backend/internal/webserver/interactors/download_interactor_test.go`; package tests passing for handlers/interactors in sandbox.
- 2026-02-06: `backend/internal/webserver/clients` full test suite still blocked in sandbox due `httptest` listener restrictions on `[::1]:0`; compile check executed with `-run TestThisDoesNotExist`.
- 2026-02-06: Rebuilt/redeployed `webserver`, `tvdb-proxy`, and `torrenter` via `docker compose up -d --build`.
- 2026-02-06: Live post-deploy checks confirmed SEARCH-001 and DL-001 fixes.
- 2026-02-06: Live checks discovered DL-002 backend storage issue (`pq: relation "media" does not exist`) on torrenter media existence endpoints.
- 2026-02-06: Patched `MediaExists` query to schema-valid tables and redeployed `torrenter`; live checks now pass for both single and batch media existence endpoints.
- 2026-02-07: Implemented deep reliability fixes for scheduling and retry orchestration (`SCH-004`, `SCH-005`, `SCH-006`) and redeployed `webserver`.
- 2026-02-07: Re-verified core API health: `/api/search` `200`, `/api/search/enriched` `200`, `/api/schedule/weekly` `200`, invalid `/api/shows` payload rejected with `400`.
