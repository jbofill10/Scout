## ADDED Requirements

### Requirement: Season packs on request
When `season_packs` is on and a show request has at least `season_pack_min_episodes` episodes to download from one season, the torrenter SHALL search for a pack of that season before searching per episode, and SHALL fall back to per-episode searches when no acceptable pack exists.

#### Scenario: Pack found
- **WHEN** "Show S01 1080p" is listed for a request of eight season-one episodes
- **THEN** one torrent is added, all eight notifications read downloading, all eight count as in flight, and no per-episode search runs for them

#### Scenario: No pack
- **WHEN** no pack title names the show and season at or above the quality floor
- **THEN** the episodes are searched for one by one as before

#### Scenario: Off by default
- **WHEN** `season_packs` is off
- **THEN** multi-episode torrents stay rejected and behaviour is unchanged

### Requirement: Pack landing
When a pack torrent completes, the torrenter SHALL link each video file whose own season and episode (or absolute number) match a wanted episode under that episode's Plex name, fetch subtitles per file, mark those episodes completed, and mark wanted episodes the pack did not contain as failed with the reason "not in the season pack".

#### Scenario: Partial pack
- **WHEN** a pack holds episodes 1 to 9 of a season and episode 10 was wanted too
- **THEN** episodes 1 to 9 complete and episode 10 is marked failed with that reason
