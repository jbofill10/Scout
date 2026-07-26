# Outstanding Work

Known issues that are **still open**, found while merging the library page and the
download-request-feedback work (PRs #17–#32, 2026-07-26).

This file lists only open items. When one is fixed, delete its section rather than
marking it done — a list of things that are no longer true is worse than no list.

Each item includes a command to re-check it, so nothing here has to be taken on trust.

---

## 1. `.env.template` is gitignored, so a fresh clone cannot follow the setup

**Severity: high — new-machine setup is broken.**

`README.md` (lines 35 and 108), `CLAUDE.md`, and `DEPLOYMENT.md` all instruct:

```bash
cp .env.template .env
```

But `.gitignore:22` has a broad `.env.*` rule that also catches `.env.template`, so the
file has never been committed. On a fresh clone there is nothing to copy and the
documented setup path dead-ends.

```bash
git check-ignore -v .env.template     # -> .gitignore:22:.env.*
git ls-files .env.template            # -> empty
```

**Fix:** negate the rule and commit the template.

```gitignore
.env.*
!.env.template
```

**Before committing it, read the local `.env.template` in full and confirm every value is
a `CHANGE_ME` placeholder.** It is currently an untracked local file, so it may have
accumulated real values. This is the reason the fix was not applied automatically.

Note the local copy was corrected to say `TVDB_PIN` (see item 2) but that change is
invisible to git while the file stays ignored.

---

## 2. Archive the deployed openspec change

`openspec/changes/add-download-request-feedback/` is at **21/21 tasks complete** and has
been deployed. Per `openspec/AGENTS.md` it should now be archived so it stops appearing
as active work.

```bash
grep -c '^- \[ \]' openspec/changes/add-download-request-feedback/tasks.md   # -> 0
ls openspec/changes/ | grep -v archive
```

**Fix:** run the archive flow (`/openspec:archive`, or `openspec archive
add-download-request-feedback`), which moves it under `openspec/changes/archive/` and
folds the deltas into `openspec/specs/`.

`add-netflix-style-ui-overhaul` is also still listed as active and is worth a look, but it
has a large task list that was not audited — check whether it is genuinely finished before
archiving it.

---

## 3. Gin trusts all proxies in every service

**Severity: medium — affects the correctness of client IPs, and gin flags it itself.**

All three Go services call `gin.Default()` without ever calling `SetTrustedProxies`, so
gin trusts `X-Forwarded-For` from any source. Everything sits behind the nginx container,
so the practical exposure is limited to the LAN, but any client IP used for logging or
rate limiting is currently attacker-settable.

```bash
grep -rn "SetTrustedProxies" backend/ --include=*.go        # -> no matches
docker compose logs torrenter | grep "trusted all proxies"  # -> warning present
```

**Fix:** in each of `backend/cmd/{webserver,torrenter,tvdb-proxy}/main.go`, restrict to the
compose network after creating the router:

```go
if err := r.SetTrustedProxies([]string{"172.19.0.0/16"}); err != nil {
    logger.Error("Failed to set trusted proxies", "error", err)
}
```

`172.19.0.0/16` is the current `scout_scout-network` subnet. Docker assigns it
dynamically, so re-confirm before relying on it, and prefer the wider `172.16.0.0/12`
if you would rather not have the value drift:

```bash
docker network inspect scout_scout-network --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
```

---

## 4. `fmt.Printf` bypasses structured logging in tvdb-proxy

**Severity: low — one line, but it breaks the tracing contract.**

`backend/cmd/tvdb-proxy/main.go:627` writes to stdout directly:

```go
fmt.Printf("Fetching extended information for media ID: %s, type: %s\n", mediaId, mediaType)
```

`OBSERVABILITY.md` requires context-aware logging so entries carry `trace_id`/`span_id` and
reach the collector. This line is invisible in SigNoz and cannot be correlated to a request.
A `ctx` is available on the very next line.

```bash
grep -rn "fmt.Print" backend/ --include=*.go | grep -v _test
```

**Fix:**

```go
ctx := c.Request.Context()
logger.InfoContext(ctx, "Fetching extended information", "media_id", mediaId, "media_type", mediaType)
```

Two sibling debug logs (`"Is the URL here?"`, `"Is URL here?"`) were already removed in
#27/#28; this is the last one of that family.

---

## 5. `ReleaseYear` is not carried on `SearchStrategy`

`backend/internal/torrenter/service/mediaprocessor.go:106`:

```go
ReleaseYear: media.Req.ReleaseYear, // TODO: add to SearchStrategy
```

The processor reads the release year off the original request rather than the search
strategy that actually produced the download. For movies the target directory is
`{MovieName} ({Year})/`, so if the two ever disagree the file lands in a differently-named
folder than Plex expects.

Worth pairing with the `IsMovie` cleanup in #26, which fixed the same class of problem
(the processor inferring things instead of being told them).

---

## 6. The UI ships as one 800 KB chunk

Vite warns on every build:

```
dist/assets/index-*.js   800.67 kB │ gzip: 244.79 kB
(!) Some chunks are larger than 500 kB after minification.
```

Everything — all pages, MUI, TanStack Query — is in a single bundle, so the first paint
waits on code for pages the user may never open. The `Library` and `Activity` pages added
in #17 and #25 are both good candidates for route-level splitting.

```bash
cd ui && npm run build
```

**Fix:** lazy-load the route components with `React.lazy` + `Suspense` in `App.tsx`, and/or
set `build.rollupOptions.output.manualChunks` to split the vendor bundle.

---

## 7. `MovieDownloadHistory` is dead schema

The table is created in `sql/setup-postgres.sql` but nothing reads or writes it — shows
have `ShowDownloadHistory`, movies have no equivalent in code.

```bash
grep -rn "MovieDownloadHistory" backend/ --include=*.go   # -> no matches
```

`CLAUDE.md` already describes it as "currently unused (future feature)", so this is a
deliberate placeholder rather than a bug. Either wire it up alongside the movie download
path or drop it, so the schema stops implying a feature that does not exist.

---

## 8. `fix/torrent-download-scheduling-bugs` can be deleted

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
