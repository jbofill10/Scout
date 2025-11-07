## ADDED Requirements

### Requirement: Horizontal Carousel Component

The UI SHALL provide a reusable horizontal carousel component for displaying media content in a Netflix-style scrollable layout.

#### Scenario: Render items in horizontal scroll container

- **WHEN** HorizontalCarousel component receives an array of items
- **THEN** the component SHALL render items in a horizontally scrollable container
- **AND** use CSS scroll-snap for smooth snapping behavior
- **AND** hide the default scrollbar with CSS

#### Scenario: Navigate carousel with buttons

- **WHEN** user hovers over the carousel
- **THEN** left and right navigation buttons SHALL appear
- **AND** clicking the left button SHALL scroll backward by one viewport width
- **AND** clicking the right button SHALL scroll forward by one viewport width
- **AND** buttons SHALL be disabled at scroll boundaries

#### Scenario: Navigate carousel with keyboard

- **WHEN** carousel has focus
- **AND** user presses left or right arrow keys
- **THEN** the carousel SHALL scroll in the corresponding direction
- **AND** maintain keyboard accessibility standards

#### Scenario: Support custom item rendering

- **WHEN** HorizontalCarousel receives a renderItem prop
- **THEN** the component SHALL use the provided render function for each item
- **AND** pass item data and click handler to renderItem function
- **AND** maintain consistent spacing between items

### Requirement: Media Card Component

The UI SHALL provide a media card component that displays poster images with hover effects in a Netflix-style layout.

#### Scenario: Display poster image

- **WHEN** MediaCard component receives media data
- **THEN** the card SHALL display the poster image with 2:3 aspect ratio
- **AND** use object-fit: cover to fill the card area
- **AND** lazy load the image to improve performance

#### Scenario: Show title on hover

- **WHEN** user hovers over the media card
- **THEN** a dark overlay SHALL appear with opacity transition
- **AND** the media title SHALL be displayed on the overlay
- **AND** a download icon SHALL be displayed on the overlay

#### Scenario: Handle click interaction

- **WHEN** user clicks on the media card
- **THEN** the onClick callback SHALL be invoked with media data
- **AND** the card SHALL provide visual feedback (scale or elevation change)

#### Scenario: Keyboard accessibility

- **WHEN** media card receives focus via Tab key
- **THEN** the card SHALL display focus indicator
- **AND** pressing Enter key SHALL trigger the onClick callback
- **AND** the card SHALL have appropriate ARIA labels

### Requirement: Genre Row Component

The UI SHALL provide a genre row component that combines a genre title with a horizontal carousel of media content.

#### Scenario: Fetch and display genre content

- **WHEN** GenreRow component receives genre and mediaType props
- **THEN** the component SHALL fetch popular content from the appropriate API endpoint
- **AND** pass genre ID as query parameter if provided
- **AND** render the fetched items in a HorizontalCarousel

#### Scenario: Display genre title

- **WHEN** GenreRow component receives a title prop
- **THEN** the component SHALL display the title above the carousel
- **AND** use Typography variant h5 for consistent styling
- **AND** apply appropriate spacing between title and carousel

#### Scenario: Show loading state

- **WHEN** genre content is being fetched
- **THEN** the component SHALL display skeleton loaders
- **AND** maintain layout stability during loading
- **AND** match the expected card dimensions

#### Scenario: Handle fetch errors

- **WHEN** fetching genre content fails
- **THEN** the component SHALL display an error message
- **AND** provide a retry button for transient errors
- **AND** log the error for observability

#### Scenario: Render media cards

- **WHEN** genre content is successfully loaded
- **THEN** the component SHALL render each item as a MediaCard
- **AND** pass appropriate click handlers to open detail dialog
- **AND** maintain consistent spacing and layout
