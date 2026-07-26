## ADDED Requirements

### Requirement: Download Request Confirmation

The UI SHALL confirm the outcome of every download request through an app-wide toast, so no download button click is silent.

#### Scenario: Confirm a request that queued episodes

- **WHEN** a download request returns `queued_now > 0`
- **THEN** the UI SHALL show a success toast naming the media and how many episodes are being searched for now
- **AND** include any scheduled count and the first release date
- **AND** offer a "View activity" action linking to `/activity`

#### Scenario: Confirm a request with nothing to do

- **WHEN** a download request returns no queued and no scheduled items
- **THEN** the UI SHALL show an informational toast stating the media is already tracked

#### Scenario: Report a failed request

- **WHEN** a download request fails
- **THEN** the UI SHALL show an error toast naming the media
- **AND** log the error with the originating component

#### Scenario: Show pending state on the download button

- **WHEN** a download request is in flight
- **THEN** the dialog's download button SHALL be disabled and show a spinner
- **AND** the dialog SHALL close once the request is accepted

#### Scenario: Refresh dependent views

- **WHEN** a download request succeeds
- **THEN** the UI SHALL invalidate the activity, notification and weekly schedule queries

### Requirement: Download Activity Page

The UI SHALL provide an Activity page showing what Scout is working on right now.

#### Scenario: Show stage counts

- **WHEN** the Activity page loads
- **THEN** the page SHALL display a tile per stage for downloading, searching, scheduled and failed items
- **AND** show the count returned by `/api/activity`

#### Scenario: List in-flight work

- **WHEN** the snapshot contains active items
- **THEN** the page SHALL list each item with its poster, title, episode or movie label, stage chip and stage progress track
- **AND** show the reason it is waiting, the retry time when one is set, or the release time for scheduled items
- **AND** show when it was last updated and the leading characters of its trace id

#### Scenario: List finished work

- **WHEN** the snapshot contains completed or failed items
- **THEN** the page SHALL list them in a separate section
- **AND** render failure reasons in the error colour

#### Scenario: Poll for updates

- **WHEN** the Activity page is open
- **THEN** the page SHALL refetch the snapshot every 5 seconds
- **AND** indicate the refresh without shifting the layout
- **AND** provide a manual refresh control

#### Scenario: Empty and error states

- **WHEN** the snapshot contains no items
- **THEN** the page SHALL explain that requesting a show or movie will populate it
- **WHEN** the request fails
- **THEN** the page SHALL show a retry control

### Requirement: Activity Navigation Badge

The UI SHALL surface in-flight download count in the top navigation.

#### Scenario: Badge the Activity link

- **WHEN** one or more downloads are in flight
- **THEN** the navbar Activity link SHALL display the count as a badge
- **AND** expose the count to assistive technology through the link's accessible name
- **AND** hide the badge when nothing is in flight

#### Scenario: Keep the count current

- **WHEN** the app is open on any page
- **THEN** the count SHALL refresh at least every 15 seconds
