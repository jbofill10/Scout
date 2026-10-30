## ADDED Requirements

### Requirement: Retry of asynchronous failures
The webserver SHALL keep a completed schedule row for every item the torrenter accepts into a download, including direct requests. It SHALL periodically find failed notifications whose `failure_code` is transient and that have no pending or queued row, and re-arm the item's newest completed row with the standard backoff, incrementing its attempts; when the attempts are exhausted it SHALL mark the row and the notification permanently failed. A lost movie with no row SHALL get a retry row rebuilt from the notification.

#### Scenario: Monitor lost after a direct request
- **WHEN** a movie requested from the UI was accepted, then marked failed by the torrenter with code `monitor_lost`
- **THEN** within one reconciliation pass the item is pending again with a next attempt one hour out and its notification reads searching with the attempt count

#### Scenario: Already retrying
- **WHEN** a failed notification's item already has a pending row
- **THEN** the reconciler leaves it alone

#### Scenario: Permanent code
- **WHEN** the failure code is `processing_error`
- **THEN** nothing is re-armed

## MODIFIED Requirements

### Requirement: Download controls
Retry now SHALL re-arm the newest completed row of an item that has no live rows, and Pick release SHALL use that row's media. Cancel SHALL ignore completed rows. The Activity list SHALL offer Retry for a failed item whose only row is completed.
