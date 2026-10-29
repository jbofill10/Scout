# Outstanding Work

Known issues that are **still open**, found while merging the library page and the
download-request-feedback work (PRs #17–#32, 2026-07-26).

This file lists only open items. When one is fixed, delete its section rather than
marking it done — a list of things that are no longer true is worse than no list.

Each item includes a command to re-check it, so nothing here has to be taken on trust.

---

## 1. `fix/torrent-download-scheduling-bugs` can be deleted

The branch is fully triaged. Every commit is either merged into `main` (by content, via
cherry-pick — the SHAs differ, so git still reports them as unmerged) or deliberately
superseded per the table below.

```bash
git log --oneline main..origin/fix/torrent-download-scheduling-bugs
git branch -D fix/torrent-download-scheduling-bugs
git push origin --delete fix/torrent-download-scheduling-bugs
```

### Do not re-port these — they are superseded

Kept here so the same commits are not "rediscovered" and merged later, which would revert
newer work.

| Commit | Superseded by |
|---|---|
| `ff35ca3` scheduler rewrite | `main` already does DB-driven retry (`retry/policy.go`, `ScheduleRetry`, `next_attempt_at`) and recovers crashes via `ResetStaleQueued` at startup. The in-memory `scheduledMap` it deletes is only in-process timer dedupe, not state of record. Merging it would revert #16's retry engine and #20's async dispatch. |
| `5556476` `ErrNoTorrentFound` | `main`'s per-episode `dlstatus.CodeNoTorrentFound` (classified transient) driving the retry engine is strictly more granular than a whole-request sentinel error. Its `didTorrentComplete` change is also superseded by `main`'s broader `torrent.Progress == 1` check. **One third of this commit was still needed** — a built-then-dropped log — landed as #32. |
| `e3b6b01` OTLP scheme stripping | `main`'s `normalizeOTLPEndpoint` does the same and also handles `https://` (insecure flag) and path components. The same commit's nginx/compose template fixes **were** needed and landed as #30. |
| `7fe6100` URL encoding | `main` already encodes with `url.QueryEscape`. The same commit's tvdb-proxy data race, semaphore leak, and malformed-id panic **were** needed and landed as #27; its empty-result guard landed as #28. |
| `efdf5a7` movie content-hash stability | `main`'s `ScheduleRetry` already hashes on `mediaReleaseTime(media)` so retries dedupe against the canonical row. Its scan-variable bug **was** still present and landed as #29. |
| `5b66be4` backend review tracker | A 2026-02-07 point-in-time snapshot whose statuses are now wrong. This file replaces it. |

---

## Verifying the whole list

```bash
cd backend && go build ./... && go vet ./... && go test ./...
cd backend && golangci-lint run ./cmd/... ./internal/... ./pkg/...   # currently 0 issues
cd ui && npx tsc --noEmit && npm run lint && npm run build
```

As of 2026-07-26 the backend passes 13/13 test packages and lints clean at 0 issues; the UI
typechecks, lints, and builds clean. Nothing in this file is a regression — these are
pre-existing gaps that the merge work surfaced but did not cause.
