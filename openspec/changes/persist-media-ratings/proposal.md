# Change: Keep rating answers across restarts

## Why

Every rating tvdb-proxy looks up (the Tomatometer and Metascore from Wikidata, Wikipedia and OMDb, the IMDb
ids resolved through TVDB and found on Wikidata) lives only in the proxy's memory. Each container restart
asks every source for every popular card and every library title again: a free OMDb key allows 1,000
lookups a day, the library alone is hundreds of titles, and Wikidata's query service is shared by everyone.
The user asked why the results are not simply saved so the library's ratings build up.

## What Changes

- A `RatingsCache` table (migration 015: `key`, `value` JSONB, `expires_at`, `updated_at`) in the PostgreSQL
  the other services share. tvdb-proxy runs the migrations at startup when `DB_HOST` is set, like the
  webserver and torrenter do.
- `ratings.Store` (`store.go`, `PostgresStore`): the caches write every kept answer through to it under a
  namespaced key (`omdb:`, `wikidata:`, `found:`, `imdb-id:`) and read it back on a memory miss, so a
  restart finds the library's scores at once. Misses are stored too, so unreleased titles are not asked for
  again.
- An expired answer, in memory or in the store, stands in while its refresh runs or fails: a week-old
  Tomatometer beats none. The startup prune drops answers expired for over 90 days.
- tvdb-proxy reads `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`; without `DB_HOST` it keeps ratings in
  memory only, as before. The compose template gives it the variables and a `postgres` dependency.

## Impact

- Affected specs: `tvdb-proxy-api`
- Affected code: `backend/internal/tvdbproxy/ratings/{store,cache,service,resolver}.go`,
  `backend/internal/tvdbproxy/config`, `backend/cmd/tvdb-proxy/main.go`,
  `backend/pkg/dbmigrate/migrations/015_add_ratings_cache.sql`, `docker-compose.yml.template`
- Deployments on an older local `docker-compose.yml` must add the `DB_*` lines to the tvdb-proxy block.
