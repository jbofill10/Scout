## ADDED Requirements

### Requirement: Phone Navigation

The UI SHALL provide navigation that fits a portrait phone without horizontal overflow.

#### Scenario: Bottom tab bar below the md breakpoint

- **WHEN** the viewport is narrower than the `md` breakpoint (900px)
- **THEN** the navbar's centre navigation pills SHALL be hidden
- **AND** a fixed bottom tab bar SHALL show Home, Shows, Movies, Library and Activity
- **AND** the current route's tab SHALL be highlighted
- **AND** the Activity tab SHALL carry the in-flight download count badge

#### Scenario: Pages clear the tab bar

- **WHEN** the bottom tab bar is shown
- **THEN** page content and toasts SHALL not be covered by it
- **AND** the bar SHALL sit above the device's bottom safe-area inset

#### Scenario: Desktop navigation unchanged

- **WHEN** the viewport is at least the `md` breakpoint
- **THEN** the bottom tab bar SHALL not render
- **AND** the navbar pills SHALL render as before

### Requirement: Responsive Poster Rows and Grids

Poster carousels and grids SHALL size their items to the viewport so posters never collapse to a single column or overflow the screen.

#### Scenario: Carousel on a phone

- **WHEN** a genre or popular row renders at phone width
- **THEN** roughly 2.6 posters SHALL be visible with the last one partially cut off
- **AND** the row SHALL extend to the screen edge and scroll by touch
- **AND** the pointer-only scroll buttons SHALL not render on devices that cannot hover

#### Scenario: Grids on a phone

- **WHEN** the search results grid or the Library grid renders at phone width
- **THEN** it SHALL show two columns

#### Scenario: Hover effects on touch screens

- **WHEN** a poster is tapped on a device that cannot hover
- **THEN** the card SHALL NOT remain raised or ringed after the tap

### Requirement: Full-Screen Overlays on Phones

Overlays that need vertical room SHALL take the whole screen on phones.

#### Scenario: Search overlay on a phone

- **WHEN** the search overlay opens at phone width
- **THEN** it SHALL fill the viewport
- **AND** the media-type toggle SHALL render full width on its own row below the search field

#### Scenario: Episode dialog on a phone or short viewport

- **WHEN** the media status dialog for a show opens at phone width or on a viewport 520px tall or shorter
- **THEN** the dialog SHALL be full screen
- **AND** the season tabs SHALL stay fixed while only the episode list scrolls
- **AND** the action buttons SHALL sit above the bottom safe-area inset

#### Scenario: Smaller dialogs on a phone

- **WHEN** a non-full-screen dialog opens at phone width
- **THEN** it SHALL keep a 16px gutter on each side rather than 32px

### Requirement: Page Furniture Adapts to Width

Page headers, summary tiles and list rows SHALL rearrange rather than squeeze at phone width.

#### Scenario: Page header with an action on a phone

- **WHEN** a page header has an action at phone width
- **THEN** the action SHALL render below the title instead of beside it

#### Scenario: Activity page on a phone

- **WHEN** the Activity page renders at phone width
- **THEN** the stage tiles SHALL render two per row
- **AND** each row SHALL show its relative time inline and omit the right-hand column

#### Scenario: Library page frame

- **WHEN** the Library page renders at any width
- **THEN** it SHALL use the same page frame and header as the other routes, clear of the fixed navbar
