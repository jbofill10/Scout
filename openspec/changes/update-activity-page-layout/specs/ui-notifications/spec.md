## MODIFIED Requirements

### Requirement: Download Activity Page

The UI SHALL provide an Activity page showing what Scout is working on right now, laid out so the current state fits on one screen: a row of stage tiles, the indexer strip, and one tab at a time listing one line per show or movie.

#### Scenario: Show stage counts

- **WHEN** the Activity page loads
- **THEN** the page SHALL display a tile per stage for downloading, searching, scheduled and failed items, in one row from tablet width up
- **AND** each tile SHALL show the number of rows in that stage across the page's own lists, the scheduled tile counting the pending schedule rows
- **AND** clicking a tile SHALL open the tab that holds that stage, filtered to it, with a removable "<Stage> only" chip and the tile shown pressed
- **AND** clicking the pressed tile again SHALL clear the filter

#### Scenario: Tabs instead of stacked sections

- **WHEN** the page renders
- **THEN** it SHALL offer In flight, Upcoming and Finished tabs, each labelled with its row count when non-zero
- **AND** only the selected tab's list SHALL render
- **AND** switching tabs SHALL clear any stage filter

#### Scenario: Each item once

- **WHEN** a future episode has both a `scheduled` notification and a pending schedule row
- **THEN** it SHALL appear once, under Upcoming, as the schedule row
- **WHEN** a retry wait has both a `searching` notification and a pending schedule row
- **THEN** it SHALL appear once, under In flight, as the notification
- **WHEN** a `scheduled` notification has no pending schedule row
- **THEN** it SHALL still appear under In flight

#### Scenario: Group a show's episodes

- **WHEN** a tab holds several rows for the same show (same category and title, ignoring case)
- **THEN** they SHALL render as one collapsed line with the poster, title, season or season range, episode count, a count per stage, a proportional stage bar, the lead row's code and status, and the lead row's relative time
- **AND** the lead row SHALL be the most recently updated row under In flight and Finished, and the soonest due row under Upcoming
- **AND** opening the line SHALL list each episode in season and episode order on one row with its code, stage chip, status line, relative time, leading trace id characters and actions
- **AND** an Expand all / Collapse all control SHALL open or close every group on the current tab
- **WHEN** a show or movie has a single row in the tab
- **THEN** it SHALL render as one line with poster, title, code, stage chip, status line, relative time, trace prefix and actions, with no disclosure

#### Scenario: Status line

- **WHEN** a row is rendered
- **THEN** its status SHALL be, in priority: the failure reason, followed by "retrying in …" when the webserver has re-armed the item; the retry time with the attempt number and the last failure reason; the release time; otherwise the stage description
- **AND** failure reasons SHALL use the error colour and retry waits the searching colour

#### Scenario: Upcoming and Finished stay short

- **WHEN** Upcoming or Finished holds more than twelve groups
- **THEN** only the first twelve SHALL render, with a control to show all and to show fewer again
- **AND** In flight SHALL never be cut
- **AND** the Finished tab SHALL offer a Clear finished control that dismisses completed rows and failures the scheduler has given up on, then refreshes the activity and notification queries

#### Scenario: Poll for updates

- **WHEN** the Activity page is open
- **THEN** the page SHALL refetch the snapshot every 5 seconds
- **AND** indicate the refresh without shifting the layout
- **AND** provide a manual refresh control that also refreshes the upcoming rows

#### Scenario: Empty and error states

- **WHEN** the selected tab contains no rows
- **THEN** the page SHALL explain what would appear there (request a show or movie; future releases; completed, failed and cancelled downloads)
- **WHEN** a stage filter hides every row
- **THEN** the page SHALL say so and point at clearing the filter
- **WHEN** the request fails
- **THEN** the page SHALL show a retry control

### Requirement: Upcoming section

The Activity page SHALL list every pending scheduled row under the Upcoming tab, grouped by show and ordered by the soonest due row, each row with when it releases or retries and a Cancel action, plus Retry now and Pick release while a retry is pending, collapsed past twelve groups with a control to show all.

#### Scenario: Beyond the weekly widget

- **WHEN** an episode is scheduled more than seven days out
- **THEN** it appears under Upcoming even though the Home schedule widget does not show it

#### Scenario: Next release first

- **WHEN** a show has several pending episodes
- **THEN** its group line SHALL name the soonest one and when it releases
