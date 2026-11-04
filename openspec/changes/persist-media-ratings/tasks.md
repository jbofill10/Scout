# Tasks

## 1. Store

- [x] 1.1 Migration `015_add_ratings_cache.sql` (`RatingsCache`: key, JSONB value, expires_at, updated_at)
- [x] 1.2 `ratings.Store`, `Entry`, `PostgresStore` (`Get`, `Put` in key order, `Prune`, `Count`, `Close`);
  integration test gated on `DB_HOST`

## 2. Caches

- [x] 2.1 `asyncCache` persistence: read-through on a miss, write-through from the detached lookup, expired
  answers stand in; tests
- [x] 2.2 `Service`: `Options.Store`; OMDb and found-id caches persisted; per-title Wikidata scores restored
  and kept through the store (`restoreScores`, `keepScores`); tests
- [x] 2.3 `Resolver.SetStore`; test

## 3. Wiring

- [x] 3.1 Config `DB_HOST`/`DB_USER`/`DB_PASSWORD`/`DB_NAME` with `ConnStr`; test
- [x] 3.2 `main.go`: migrate, open, prune and wire the store when `DB_HOST` is set
- [x] 3.3 Compose template: tvdb-proxy `DB_*` and `postgres` dependency; README, DEPLOYMENT
