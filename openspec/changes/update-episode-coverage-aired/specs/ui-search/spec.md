## MODIFIED Requirements

### Requirement: Media-Specific Metadata Display

The search result dialog SHALL display metadata appropriate to the media type, and SHALL measure a show's
library coverage against the episodes that have aired.

#### Scenario: Display TV show metadata

- **WHEN** a user opens the detail dialog for a TV show result
- **THEN** the dialog SHALL display the episode count as downloaded of aired episodes, with the number still
  to air alongside (e.g. "11 of 11 aired episodes in your library · 9 upcoming")
- **AND** the coverage progress bar SHALL be full once every aired episode is in the library
- **AND** each season tab SHALL carry that season's downloaded/aired count and its upcoming count
- **AND** an episode that has not aired SHALL be labelled "Upcoming" (dated) or "TBA" (undated), never "Missing"
- **AND** the dialog SHALL display show-specific information (seasons, episodes)

#### Scenario: Poster pill

- **WHEN** a show card carries a status badge
- **THEN** the pill SHALL read "Caught up" once every aired episode is in the library and more are still to air
- **AND** otherwise read downloaded/aired, green when complete and amber while aired episodes are missing
- **AND** its tooltip SHALL carry the full sentence including the upcoming count
- **AND** a show Scout follows SHALL carry a bookmark mark beside the pill, on every card that shows it

#### Scenario: Display movie metadata

- **WHEN** a user opens the detail dialog for a movie result
- **THEN** the dialog SHALL display the release date if available
- **AND** the dialog SHALL display movie-specific information (runtime, year)
- **AND** the dialog SHALL handle missing metadata gracefully

#### Scenario: Handle missing movie metadata

- **WHEN** a movie result has no release date or runtime information
- **THEN** the dialog SHALL display the movie without errors
- **AND** the dialog SHALL show a placeholder or omit missing fields
