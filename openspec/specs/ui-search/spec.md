# ui-search Specification

## Purpose
TBD - created by archiving change add-movie-search-ui. Update Purpose after archive.
## Requirements
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

The search functionality SHALL send the text as typed to `GET /api/search/enriched` with the selected media type, and SHALL read the response as `{ results, query }`.

#### Scenario: Search for TV shows

- **WHEN** the user submits a search query with media type "series"
- **THEN** the request SHALL include query parameter `media_type=series`
- **AND** the request SHALL be sent to `GET /api/search/enriched`
- **AND** `results` SHALL contain TV show metadata with library status

#### Scenario: Search for movies

- **WHEN** the user submits a search query with media type "movie"
- **THEN** the request SHALL include query parameter `media_type=movie`
- **AND** the request SHALL be sent to `GET /api/search/enriched`
- **AND** `results` SHALL contain movie metadata with library status

#### Scenario: Query interpretation is returned

- **WHEN** the webserver answers an enriched search
- **THEN** `query` SHALL describe how the text was read: `raw`, `title`, the parsed fields, and `filters` in typing order, each with `key`, `value` and `inferred` when the filter came from the title text

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

### Requirement: Search Dropdown Overlay

The search functionality SHALL provide a dropdown overlay that appears on top of the current page with a semi-transparent backdrop.

#### Scenario: Open search overlay from navbar

- **WHEN** user clicks the search icon in the navbar
- **THEN** the search dropdown SHALL slide down from the top
- **AND** a semi-transparent backdrop SHALL appear behind the dropdown
- **AND** the backdrop opacity SHALL be 0.85
- **AND** the search input SHALL receive automatic focus

#### Scenario: Close search overlay

- **WHEN** search overlay is open
- **AND** user clicks on the backdrop
- **THEN** the search dropdown SHALL close with animation
- **AND** return user to the current page

#### Scenario: Close search overlay with ESC key

- **WHEN** search overlay is open
- **AND** user presses the ESC key
- **THEN** the search dropdown SHALL close immediately
- **AND** return focus to the navbar search icon

#### Scenario: Search input debouncing

- **WHEN** user types in the search input field
- **THEN** the search SHALL be debounced by 300ms
- **AND** API request SHALL only fire after user stops typing
- **AND** minimize unnecessary API calls

#### Scenario: Display search results in dropdown

- **WHEN** search results are returned from the API
- **THEN** the results SHALL be displayed within the dropdown
- **AND** maintain infinite scroll pagination
- **AND** scroll vertically for additional results

#### Scenario: Open detail dialog from dropdown

- **WHEN** user clicks on a search result in the dropdown
- **THEN** the SearchResultDialog SHALL open
- **AND** the search dropdown SHALL remain open in the background
- **AND** closing the dialog SHALL return to the search dropdown

### Requirement: Search Query Syntax

The search box SHALL accept filters alongside a title, parsed by the webserver.

#### Scenario: Year in the text

- **WHEN** the user types `X (2022)`, `X [2022]` or `X 2022`
- **THEN** the search SHALL be for title "X" with year 2022
- **AND** for the bare trailing form the exact text SHALL also be searched so a title ending in a number is still found

#### Scenario: Numbers that are not years

- **WHEN** the user types a lone number ("1917"), a number outside 1880 to five years from now ("Blade Runner 2049"), or a number not at the end ("2001 a space odyssey")
- **THEN** it SHALL remain part of the title

#### Scenario: Key:value filters

- **WHEN** the user types `year:`, `director:` (or `dir:`), `actor:` (or `cast:`, `starring:`), `person:`, `studio:` (or `company:`), `network:`, `country:`, `lang:` (or `language:`) or `imdb:` followed by a value
- **THEN** the value SHALL run to the next filter key or the end of the text, or to the closing quote when quoted
- **AND** words before the first filter SHALL be the title

#### Scenario: Colons in titles

- **WHEN** the user types a title containing a colon after a word that is not a filter key ("Mission: Impossible")
- **THEN** the whole text SHALL be treated as the title

#### Scenario: IMDb id or link

- **WHEN** the user types an IMDb title id ("tt13560574") or pastes an IMDb title URL
- **THEN** the search SHALL look that title up by IMDb id

### Requirement: Query Interpretation Display

The search overlay SHALL show the user how their text was read, once results for it have arrived.

#### Scenario: Filter chips

- **WHEN** the interpreted query has one or more filters
- **THEN** a row under the search box SHALL show "Searching for {title}" (or "Searching" when there is no title) followed by one chip per filter, labelled with the filter's name and value

#### Scenario: Removing a filter

- **WHEN** the user clicks a chip's remove control
- **THEN** the search text SHALL be rebuilt from the title and the remaining filters as `key:value`, and searched immediately

#### Scenario: Result card provenance

- **WHEN** a result was found through a person rather than its title
- **THEN** the card's hover overlay SHALL show its year and `matchedVia` ("2022 · Mia Goth · Actor")
- **AND** other results SHALL show their year

### Requirement: Search Guidance

The search overlay SHALL teach the syntax without getting in the way.

#### Scenario: Empty state

- **WHEN** nothing has been typed
- **THEN** the overlay SHALL show example searches as clickable chips, each with a short hint, and clicking one SHALL fill the box and select the fitting media type

#### Scenario: Syntax help

- **WHEN** the search box is empty
- **THEN** a help icon in the box SHALL show the filter syntax on hover or tap
- **AND** the placeholder SHALL mention that years and people can be searched

#### Scenario: Filters without a title

- **WHEN** the interpreted query has only filters that need a title (year, studio, network, country, language) and no results
- **THEN** the empty state SHALL say that a title or a person is needed to go with them

#### Scenario: IMDb id of the other media type

- **WHEN** an IMDb lookup returns nothing
- **THEN** the empty state SHALL name the id and suggest switching to the other media type tab

#### Scenario: Person with no credits

- **WHEN** a person or director filter returns nothing
- **THEN** the empty state SHALL name the person and suggest checking the spelling or the other tab
