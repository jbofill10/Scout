## ADDED Requirements

### Requirement: Missing episodes per library show
The torrenter SHALL expose `GET /library/missing` returning, for every library show, the aired non-special TVDB episodes that have no Plex episode, grouped by show with title and poster. The webserver SHALL relay it and mark each episode `requested` when a scheduled row or a notification exists for it.

#### Scenario: A show with gaps
- **WHEN** a show has two aired episodes absent from Plex, one of which is already scheduled
- **THEN** the show appears with both episodes, one marked requested

#### Scenario: Complete shows are omitted
- **WHEN** every aired episode of a show is in Plex
- **THEN** the show does not appear

### Requirement: Fill missing episodes
The webserver SHALL expose `POST /library/shows/:tvdbId/fill-missing` which requests the show's missing episodes that are not already requested through the normal show request path and returns its summary.

#### Scenario: Nothing left to request
- **WHEN** every missing episode is already requested
- **THEN** the response reports zero queued and zero scheduled and no notification is created

### Requirement: Missing tab
The Library page SHALL have a Missing tab listing shows with gaps, expandable to their episodes with requested ones marked, and a Download missing button per show.

#### Scenario: Fill from the tab
- **WHEN** the user presses Download missing on a show
- **THEN** a toast reports how many episodes were queued or scheduled and the tab refreshes
