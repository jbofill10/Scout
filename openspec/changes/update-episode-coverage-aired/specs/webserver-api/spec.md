## ADDED Requirements

### Requirement: Library coverage counts aired episodes
Every show status the webserver returns SHALL distinguish the episodes that have aired from those still to
come, so that a show mid-run is never reported as missing episodes nobody could download. The enriched
status (`/search/enriched`, `/popular/*`) SHALL carry `aired` next to `downloaded` and `total`: the number of
non-special TVDB episodes whose air date is today or earlier, plus any episode the library holds regardless
of its date. An episode with no air date SHALL count as unaired. The relayed
`/library/shows/:tvdbId/metadata-status` SHALL carry `upcomingCount` (TVDB episodes not yet aired or
undated), its other counts covering aired episodes only, and `/library/shows/:tvdbId` SHALL list every TVDB
episode with its air date, aired or not.

#### Scenario: Show mid-run
- **WHEN** TVDB lists 20 non-special episodes of which 11 have aired and all 11 are in Plex
- **THEN** the enriched status reports downloaded 11, aired 11, total 20
- **AND** the metadata status reports tvdbEpisodeCount 11, plexEpisodeCount 11, missingCount 0, upcomingCount 9

#### Scenario: Library ahead of TVDB's date
- **WHEN** an episode is in Plex but TVDB dates it in the future or not at all
- **THEN** it counts as both downloaded and aired, so downloaded never exceeds aired

#### Scenario: Older client
- **WHEN** a client reads `total` without `aired`
- **THEN** `total` still means every non-special episode TVDB lists
