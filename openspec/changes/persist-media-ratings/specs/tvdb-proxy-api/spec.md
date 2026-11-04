## ADDED Requirements

### Requirement: Rating Answers Persist Across Restarts

When `DB_HOST` is configured, the tvdb-proxy SHALL keep every rating answer it caches (OMDb scores, the
per-title Wikidata and Wikipedia scores, found IMDb ids and resolved TVDB-to-IMDb ids, misses included) in
the `RatingsCache` table of the shared PostgreSQL, under a key naming the cache and the id, with the time it
stops being current, and SHALL read it back on a memory miss, so a restart does not look the answers up
again. Without `DB_HOST` the proxy SHALL cache ratings in memory only.

#### Scenario: Library scores survive a restart

- **WHEN** the proxy restarts after rating the library's titles
- **THEN** the next `/ratings/batch` for those titles SHALL answer from the store without asking OMDb,
  Wikidata, Wikipedia or TVDB

#### Scenario: Expired answer stands in

- **WHEN** a title's stored or cached answer has expired and the fresh lookup has not landed within the
  caller's wait, or fails
- **THEN** the proxy SHALL return the expired answer, and the fresh one SHALL replace it when it lands

#### Scenario: Store unavailable

- **WHEN** the database cannot be migrated or reached at startup, or a read or write fails later
- **THEN** the proxy SHALL log it and serve ratings from memory as before

#### Scenario: Old answers pruned

- **WHEN** the proxy starts
- **THEN** it SHALL delete stored answers that expired more than 90 days ago
