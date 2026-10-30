## MODIFIED Requirements

### Requirement: Page Furniture Adapts to Width

Page headers, summary tiles and list rows SHALL rearrange rather than squeeze at phone width.

#### Scenario: Page header with an action on a phone

- **WHEN** a page header has an action at phone width
- **THEN** the action SHALL render below the title instead of beside it

#### Scenario: Activity page on a phone

- **WHEN** the Activity page renders at phone width
- **THEN** the stage tiles SHALL render two per row
- **AND** the tabs SHALL fill the width
- **AND** each row SHALL wrap its status line under the title, show its relative time inline, and omit the time column, trace id and stage bar
- **AND** a group line SHALL show its stage summary under the title in place of the counts column
- **AND** the indexer strip SHALL omit the relay summary, keeping it behind the chevron

#### Scenario: Library page frame

- **WHEN** the Library page renders at any width
- **THEN** it SHALL use the same page frame and header as the other routes, clear of the fixed navbar
