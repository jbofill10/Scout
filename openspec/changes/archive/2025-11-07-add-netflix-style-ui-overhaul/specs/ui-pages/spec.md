## ADDED Requirements

### Requirement: Home Page Layout

The UI SHALL provide a Home page that displays popular content and upcoming scheduled downloads.

#### Scenario: Display page title

- **WHEN** user navigates to the Home page (route "/")
- **THEN** the page SHALL display "Scout" as the page title
- **AND** use Typography variant h3 for the title
- **AND** apply appropriate spacing below the navbar

#### Scenario: Display schedule widget

- **WHEN** Home page loads
- **THEN** the page SHALL display a ScheduleWidget component below the title
- **AND** the widget SHALL show scheduled downloads for the next 7 days
- **AND** apply spacing between the schedule widget and popular content sections

#### Scenario: Display popular TV shows

- **WHEN** Home page loads
- **THEN** the page SHALL display a GenreRow for popular TV shows
- **AND** use title "Popular TV Shows"
- **AND** fetch shows without genre filter (top 20 popular)
- **AND** render shows in a horizontal carousel

#### Scenario: Display popular movies

- **WHEN** Home page loads
- **THEN** the page SHALL display a GenreRow for popular movies
- **AND** use title "Popular Movies"
- **AND** fetch movies without genre filter (top 20 popular)
- **AND** render movies in a horizontal carousel

#### Scenario: Handle empty schedule

- **WHEN** no downloads are scheduled for the next 7 days
- **THEN** the ScheduleWidget SHALL display "No scheduled downloads this week"
- **AND** maintain layout without the schedule section collapsing

### Requirement: Shows Page Layout

The UI SHALL provide a Shows page that displays TV series organized by genre.

#### Scenario: Display shows page title

- **WHEN** user navigates to the Shows page (route "/shows")
- **THEN** the page SHALL display "TV Shows" as the page title
- **AND** use Typography variant h3 for the title

#### Scenario: Display genre rows for TV shows

- **WHEN** Shows page loads
- **THEN** the page SHALL fetch the genre list from the API
- **AND** display exactly 6 curated genres (Action, Comedy, Drama, Sci-Fi, Anime, Documentary)
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
- **AND** display exactly 6 curated genres (Action, Comedy, Drama, Sci-Fi, Anime, Documentary)
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

### Requirement: Navbar Navigation

The UI SHALL provide a fixed navigation bar at the top of all pages with links to Home, Shows, and Movies pages.

#### Scenario: Display navbar on all pages

- **WHEN** user is on any page
- **THEN** the navbar SHALL be fixed at the top of the viewport
- **AND** remain visible when scrolling
- **AND** apply theme background color (#141414)

#### Scenario: Navigate between pages

- **WHEN** user clicks Home button in navbar
- **THEN** the application SHALL navigate to the Home page (route "/")
- **AND** the Home button SHALL be highlighted as active

#### Scenario: Highlight active page

- **WHEN** user is on the Shows page
- **THEN** the Shows button SHALL be visually highlighted
- **AND** other navigation buttons SHALL use default styling

#### Scenario: Open search overlay

- **WHEN** user clicks the search icon in the navbar
- **THEN** the search dropdown overlay SHALL open
- **AND** the navbar SHALL remain visible
- **AND** the search icon SHALL indicate active state

### Requirement: Schedule Widget Display

The Home page SHALL display a schedule widget that shows upcoming downloads for the next 7 days.

#### Scenario: Fetch weekly schedule

- **WHEN** ScheduleWidget component mounts
- **THEN** the component SHALL fetch data from /api/schedule/weekly
- **AND** refresh data every 5 minutes
- **AND** use TanStack Query for caching and background updates

#### Scenario: Display scheduled items

- **WHEN** scheduled downloads are available
- **THEN** the widget SHALL display items in a horizontal layout
- **AND** show poster thumbnail for each item
- **AND** display title, season/episode format (S01E03), and release date
- **AND** enable horizontal scrolling if more than 6 items

#### Scenario: Click scheduled item

- **WHEN** user clicks on a scheduled download item
- **THEN** the SearchResultDialog SHALL open with the item details
- **AND** allow user to view or modify the scheduled download

#### Scenario: Handle empty schedule

- **WHEN** no downloads are scheduled for the next 7 days
- **THEN** the widget SHALL display "No scheduled downloads this week" message
- **AND** use appropriate styling for empty state

#### Scenario: Handle schedule fetch error

- **WHEN** fetching weekly schedule fails
- **THEN** the widget SHALL display an error message
- **AND** provide a retry button
- **AND** log the error for observability

## REMOVED Requirements

### Requirement: Schedule Page

**Reason**: Schedule functionality has been consolidated into the Home page as a widget. The dedicated /schedule route is no longer needed.

**Migration**: Users who navigate to /schedule should be redirected to the Home page. The schedule widget on the Home page provides equivalent functionality with improved UX.
