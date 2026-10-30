## ADDED Requirements

### Requirement: Download actions on the Activity page
Each Activity row SHALL offer the actions the item supports: Cancel while it is scheduled, searching or downloading; Retry now while it is waiting on a retry or has failed with a row; Pick release when a row or an active torrent exists.

#### Scenario: Cancel from the row
- **WHEN** the user cancels a downloading item
- **THEN** a toast confirms it, the row moves to Finished with the `cancelled` stage, and the activity, notification and schedule queries refresh

#### Scenario: Pick a release
- **WHEN** the user opens Pick release
- **THEN** a dialog lists candidates with quality, size, seeders, indexer and the accept or reject reason, accepted ones first, and a Download button per candidate

### Requirement: Cancelled stage
The stage vocabulary SHALL include `cancelled`, rendered with a neutral colour, a block icon and the description "Cancelled".

#### Scenario: Cancelled in the bell and the page
- **WHEN** a notification has status `cancelled`
- **THEN** both the bell dropdown and the Activity page render the cancelled chip

### Requirement: Upcoming section
The Activity page SHALL show an Upcoming section listing every pending scheduled row, each with when it releases or retries and a Cancel action, collapsed past ten rows with a control to show all.

#### Scenario: Beyond the weekly widget
- **WHEN** an episode is scheduled more than seven days out
- **THEN** it appears in Upcoming even though the Home schedule widget does not show it
