## Context

The ratings package holds four in-memory caches: OMDb answers and found IMDb ids (`asyncCache`), the
per-title Wikidata scores (a map filled per chunk query) and the TVDB id resolver's cache (`asyncCache`).
All are keyed by a string and hold a JSON-friendly value with an expiry.

## Goals / Non-Goals

- Goals: no repeated lookups after a restart; the library's ratings accumulate; stale answers beat none.
- Non-goals: persisting the IMDb table (a 7 MB download, refreshed daily anyway); sharing the store between
  proxy instances (there is one); a UI for the table.

## Decisions

- One key-value table rather than a typed `MediaRatings` table: the four caches hold different shapes
  (score lists, id strings) and the table is a cache, not a model. Keys carry the cache's prefix.
- Write-through from the detached lookup, read-through on a memory miss. `asyncCache` gets an optional
  `persistence` (store, prefix, logger); the Wikidata map gets the same through `restoreScores` and
  `keepScores`, reading the store once per batch of never-seen ids.
- Serve stale: `asyncCache.get` returns an expired answer when the lookup does not land in time or fails,
  and `fromWikidata` answers from an expired entry likewise. An expired store entry is brought into memory
  as expired, so later askers find the stand-in without a read.
- Store failures are logged and the caches carry on in memory; a missing or unreachable database at startup
  leaves the proxy memory-only with a warning, since ratings are decoration.
- Puts are one transaction in key order, so concurrent chunk writes touching the same keys cannot deadlock.

## Risks / Trade-offs

- A score that changed is served stale for up to its TTL after a restart, as it was before within one
  process. Rating TTLs are unchanged (7 days, misses 1 day).
- The integration test needs a PostgreSQL (`DB_HOST`, `DB_USER`, `DB_NAME`); it skips otherwise.
