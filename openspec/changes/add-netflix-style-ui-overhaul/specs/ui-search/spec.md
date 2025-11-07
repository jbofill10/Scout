## ADDED Requirements

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
