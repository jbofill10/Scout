## ADDED Requirements

### Requirement: Due rows of one show dispatched together
When a scheduled row for a series comes due, the webserver SHALL claim every other pending row of the same series due within the next 15 minutes, cancel their timers, and send them to the torrenter as one request: pack-first when season packs are on and the group has at least `season_pack_min_episodes` episodes, then one episode at a time for whatever no pack covered. Each row SHALL be settled on its own episode's outcome (completed, rescheduled with backoff, or failed) exactly as if it had been dispatched alone. Movie rows and rows with no due sibling SHALL be dispatched alone as before.

#### Scenario: Seventy retries come due together
- **WHEN** seventy retry rows for one show are due within the same minute
- **THEN** one pack-only request carrying all seventy episodes is sent before any per-episode request, and each row's status follows its own episode's result

#### Scenario: Pack covers part of the group
- **WHEN** the pack-only answer covers the season 1 rows and answers the season 7 rows with `no_pack`
- **THEN** the season 1 rows are completed and the season 7 episodes are sent one at a time

#### Scenario: Lone row
- **WHEN** no other row of the show is due within fifteen minutes
- **THEN** the row is dispatched alone

### Requirement: Shorter retry ladder for old content with no release
When a download fails with `no_torrent_found` and the content aired more than 30 days before the attempt, the webserver SHALL allow at most two attempts in total before marking the row failed with `max_retries_exceeded`, instead of the usual five. Other transient failure codes, and content aired within the last 30 days, SHALL keep the full ladder.

#### Scenario: Old episode, nothing listed
- **WHEN** a 2007 episode's first attempt finds no torrent
- **THEN** one retry is scheduled an hour later, and when it also finds nothing the row is marked failed

#### Scenario: Recent episode
- **WHEN** an episode that aired yesterday finds no torrent
- **THEN** the full ladder of five attempts applies

#### Scenario: Other transient failure on old content
- **WHEN** a 2007 episode's download fails because the indexer was unreachable
- **THEN** the full ladder of five attempts applies
