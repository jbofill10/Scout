## ADDED Requirements

### Requirement: Follow and unfollow a show
The webserver SHALL expose `POST /follows` (body: the show's media and `include_missing`), `DELETE /follows/:tvdbId` and `GET /follows`. Following SHALL store a snapshot of the show's media without episodes and run a first check immediately.

#### Scenario: Follow with missing episodes
- **WHEN** a show is followed with `include_missing` true
- **THEN** every aired, non-special episode that is not in Plex and not already tracked is requested, and future episodes are scheduled

#### Scenario: Follow new episodes only
- **WHEN** a show is followed with `include_missing` false
- **THEN** only episodes airing on or after the follow date are requested or scheduled

### Requirement: Periodic follow check
The webserver SHALL re-check every followed show on start, every `FOLLOW_CHECK_INTERVAL` (default 6h) and on `POST /follows/check`, requesting only episodes that are new to Scout.

#### Scenario: New season announced
- **WHEN** TVDB lists episodes for a followed show that have no scheduled row, no notification and are not in Plex
- **THEN** they are scheduled (future) or searched for (aired), once, and the follow records when it was checked and how many episodes were added

#### Scenario: Nothing new
- **WHEN** every listed episode is in Plex, tracked, a special, or aired before the follow began
- **THEN** no request is made and no duplicate notification is created

### Requirement: Follow controls in the UI
The show detail dialog SHALL offer Follow (with the include-missing choice) or Unfollow, and the Library page SHALL list followed shows with last check time, episodes added, Unfollow and Check now.

#### Scenario: Follow from a search result
- **WHEN** the user follows a show from its dialog
- **THEN** a toast confirms it and the button reads Following
