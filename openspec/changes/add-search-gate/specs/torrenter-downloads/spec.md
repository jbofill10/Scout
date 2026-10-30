## ADDED Requirements

### Requirement: Paced, prioritised indexer searches
Every Prowlarr search SHALL pass through one gate that, per indexer, allows `search_rate_per_minute` sustained searches with a burst of `search_burst` (overridable per indexer through `indexer_rate_limits`) and one search in flight at a time. Searches that cannot run at once SHALL wait in a queue served by priority (manual, scheduled, follow, missing, upgrade) and then by arrival.

#### Scenario: A person's request jumps the queue
- **WHEN** ten follow-check searches are waiting on 1337x and a request from the UI arrives
- **THEN** the UI request's search runs next

#### Scenario: Sustained rate
- **WHEN** the rate is 6 per minute with a burst of 3
- **THEN** three searches may run back to back and further ones are spaced about ten seconds apart

### Requirement: Paused indexers are skipped
While Prowlarr reports an indexer disabled, the gate SHALL pause it: new and queued searches against it SHALL fail fast and the request SHALL continue with its other indexers or fall through to its fallback tier. The pause SHALL lift when the indexer is healthy again.

#### Scenario: Banned preferred indexer
- **WHEN** 1337x is disabled for an IP block and The Pirate Bay is healthy
- **THEN** a show's search goes to The Pirate Bay without querying 1337x

### Requirement: Search queue visibility
`GET /indexers/health` SHALL include, per indexer, the search in flight, the waiting count by priority with labels, searches in the last minute, the effective limits and any pause.

### Requirement: Search pacing settings
The settings SHALL include `search_rate_per_minute` (1 to 120, default 6), `search_burst` (1 to 30, default 3), `indexer_rate_limits` (per Prowlarr indexer id, same ranges) and `bulk_request_batch` (1 to 500, default 10), changeable at runtime from the Settings page.

#### Scenario: Override one indexer
- **WHEN** `indexer_rate_limits` is `{"2": {"per_minute": 4, "burst": 2}}`
- **THEN** indexer 2 runs at 4 per minute with a burst of 2 and every other indexer keeps the global values
