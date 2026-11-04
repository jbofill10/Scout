## MODIFIED Requirements

### Requirement: Media Card Component

The UI SHALL provide a media card component that displays poster images with hover effects in a Netflix-style layout.

#### Scenario: Display poster image

- **WHEN** MediaCard component receives media data
- **THEN** the card SHALL display the poster image with 2:3 aspect ratio
- **AND** use object-fit: cover to fill the card area
- **AND** lazy load the image to improve performance

#### Scenario: Show the title's rating

- **WHEN** the media carries ratings
- **THEN** the card SHALL show one score as a pill in its top-left corner, next to the library badge in the
  top-right
- **AND** for a movie the pill SHALL show the Rotten Tomatoes Tomatometer as a percentage with a tomato (60%
  and up) or splat (below 60%) glyph, falling back to the IMDb rating and then the Metascore
- **AND** for a series the pill SHALL show the IMDb rating to one decimal with a star glyph, falling back to
  the Tomatometer and then the Metascore
- **AND** the pill SHALL name its source and value for assistive technology (e.g. "Tomatometer 97%")
- **AND** without ratings the card SHALL show no pill

#### Scenario: Detail dialog lists every score

- **WHEN** the media detail dialog opens for a title with ratings
- **THEN** it SHALL list every score (Tomatometer, IMDb, Metascore) as pills under the facts strip, the card's
  headline score first

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
