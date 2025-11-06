# UI Search Specification

## ADDED Requirements

### Requirement: Media Type Selection

The search interface SHALL provide a toggle control for users to select between searching for TV shows or movies.

#### Scenario: Default media type is TV shows

- **WHEN** the search page loads for the first time
- **THEN** the media type SHALL default to "series"
- **AND** the toggle control SHALL display "TV Shows" as selected

#### Scenario: User switches to movie search

- **WHEN** the user clicks the "Movies" toggle button
- **THEN** the media type state SHALL update to "movie"
- **AND** the toggle control SHALL display "Movies" as selected
- **AND** subsequent searches SHALL use `media_type=movie` in API requests

#### Scenario: User switches back to TV show search

- **WHEN** the user clicks the "TV Shows" toggle button after selecting "Movies"
- **THEN** the media type state SHALL update to "series"
- **AND** the toggle control SHALL display "TV Shows" as selected
- **AND** subsequent searches SHALL use `media_type=series` in API requests

### Requirement: Dynamic Search API Endpoint

The search functionality SHALL dynamically construct API requests based on the selected media type.

#### Scenario: Search for TV shows

- **WHEN** the user submits a search query with media type "series"
- **THEN** the request SHALL include query parameter `media_type=series`
- **AND** the request SHALL be sent to `GET /api/search`
- **AND** results SHALL contain TV show metadata

#### Scenario: Search for movies

- **WHEN** the user submits a search query with media type "movie"
- **THEN** the request SHALL include query parameter `media_type=movie`
- **AND** the request SHALL be sent to `GET /api/search`
- **AND** results SHALL contain movie metadata

### Requirement: Dynamic Download API Endpoint

The download functionality SHALL route requests to the appropriate backend endpoint based on media type.

#### Scenario: Download TV show

- **WHEN** the user clicks download on a search result with media type "series"
- **THEN** the request SHALL POST to `/api/shows`
- **AND** the request body SHALL include the complete SearchResult object
- **AND** the backend SHALL process the show download request

#### Scenario: Download movie

- **WHEN** the user clicks download on a search result with media type "movie"
- **THEN** the request SHALL POST to `/api/movies`
- **AND** the request body SHALL include the complete SearchResult object
- **AND** the backend SHALL process the movie download request

### Requirement: Media-Specific Metadata Display

The search result dialog SHALL display metadata appropriate to the media type.

#### Scenario: Display TV show metadata

- **WHEN** a user opens the detail dialog for a TV show result
- **THEN** the dialog SHALL display the episode count
- **AND** the dialog SHALL display show-specific information (seasons, episodes)

#### Scenario: Display movie metadata

- **WHEN** a user opens the detail dialog for a movie result
- **THEN** the dialog SHALL display the release date if available
- **AND** the dialog SHALL display movie-specific information (runtime, year)
- **AND** the dialog SHALL handle missing metadata gracefully

#### Scenario: Handle missing movie metadata

- **WHEN** a movie result has no release date or runtime information
- **THEN** the dialog SHALL display the movie without errors
- **AND** the dialog SHALL show a placeholder or omit missing fields

### Requirement: UI Component Integration

The media type selection control SHALL integrate seamlessly with existing Material-UI v7 patterns.

#### Scenario: Toggle button styling

- **WHEN** the search page renders
- **THEN** the ToggleButtonGroup SHALL use Material-UI v7 components
- **AND** the styling SHALL match existing UI patterns using `sx` prop
- **AND** the layout SHALL be minimal and unintrusive

#### Scenario: Responsive layout

- **WHEN** the page is viewed on different screen sizes
- **THEN** the toggle control SHALL remain accessible and functional
- **AND** the control SHALL adapt to available space without breaking layout
