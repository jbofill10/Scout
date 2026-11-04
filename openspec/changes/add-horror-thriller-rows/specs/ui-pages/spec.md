## MODIFIED Requirements

### Requirement: Shows Page Layout

The UI SHALL provide a Shows page that displays TV series organized by genre.

#### Scenario: Display shows page title

- **WHEN** user navigates to the Shows page (route "/shows")
- **THEN** the page SHALL display "TV Shows" as the page title
- **AND** use Typography variant h3 for the title

#### Scenario: Display genre rows for TV shows

- **WHEN** Shows page loads
- **THEN** the page SHALL fetch the genre list from the API
- **AND** display exactly 8 curated genres in this order: Action, Comedy, Drama, Sci-Fi, Horror, Thriller, Anime, Documentary
- **AND** render a GenreRow for each genre
- **AND** apply spacing between genre rows
- **AND** handle missing genres gracefully if not found in TVDB

#### Scenario: Handle genre loading state

- **WHEN** genres are being fetched
- **THEN** the page SHALL display loading indicators
- **AND** maintain layout stability during loading

#### Scenario: Fetch genre-specific shows

- **WHEN** a GenreRow is rendered on the Shows page
- **THEN** the row SHALL fetch popular shows filtered by that genre ID
- **AND** display the genre name as the row title
- **AND** render shows in a horizontal carousel

### Requirement: Movies Page Layout

The UI SHALL provide a Movies page that displays films organized by genre.

#### Scenario: Display movies page title

- **WHEN** user navigates to the Movies page (route "/movies")
- **THEN** the page SHALL display "Movies" as the page title
- **AND** use Typography variant h3 for the title

#### Scenario: Display genre rows for movies

- **WHEN** Movies page loads
- **THEN** the page SHALL fetch the genre list from the API
- **AND** display exactly 8 curated genres in this order: Action, Comedy, Drama, Sci-Fi, Horror, Thriller, Anime, Documentary
- **AND** render a GenreRow for each genre
- **AND** apply spacing between genre rows
- **AND** handle missing genres gracefully if not found in TVDB

#### Scenario: Handle genre loading state

- **WHEN** genres are being fetched
- **THEN** the page SHALL display loading indicators
- **AND** maintain layout stability during loading

#### Scenario: Fetch genre-specific movies

- **WHEN** a GenreRow is rendered on the Movies page
- **THEN** the row SHALL fetch popular movies filtered by that genre ID
- **AND** display the genre name as the row title
- **AND** render movies in a horizontal carousel
