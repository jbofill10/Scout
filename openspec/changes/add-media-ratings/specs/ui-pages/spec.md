## ADDED Requirements

### Requirement: Library Card Ratings

The Library page SHALL show each show and movie card's rating, looked up by TVDB id since Plex items carry
no IMDb id.

#### Scenario: Ratings load in chunks

- **WHEN** the TV Shows or Movies tab is shown
- **THEN** the page SHALL post the visible items' ids to `/api/ratings/batch` in chunks of 40
- **AND** each card SHALL show its rating pill (IMDb for shows; Tomatometer, else IMDb, for movies) as its
  chunk answers
- **AND** chunks SHALL be cached for an hour so switching tabs does not ask again

#### Scenario: Title without a score

- **WHEN** the backend returns no `ratings` for an item
- **THEN** the card SHALL show no rating pill and otherwise render as before
