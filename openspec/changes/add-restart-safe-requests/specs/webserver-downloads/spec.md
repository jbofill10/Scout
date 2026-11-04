## ADDED Requirements

### Requirement: Requests survive a restart
The webserver SHALL write a `ScheduledDownloads` row with status `queued` for every aired episode (or released movie) it accepts for immediate download, keyed like the row a future air date would create for the item, before the torrenter is asked for it, and SHALL settle that row on the torrenter's answer (completed, rescheduled with backoff, or failed) through the same path a due scheduled row takes. On start, before its first poll, the webserver SHALL return every queued row to pending so the interrupted dispatches are sent again at once. While a group of rows is dispatched one episode at a time, the rows still waiting SHALL have their queued time refreshed after each answer so the stale-queued sweep does not dispatch them a second time.

#### Scenario: Webserver restarts mid-batch
- **WHEN** a request for twenty aired episodes is accepted and the webserver restarts after the torrenter has answered for five of them
- **THEN** the fifteen unanswered episodes have queued rows, the new process re-arms them before its first poll, and they are dispatched again as one group with no user action

#### Scenario: Row already queued
- **WHEN** an episode is requested while a dispatch already holds its queued row
- **THEN** no second row or notification is created and the summary counts the episode as in progress

#### Scenario: Earlier row for the episode
- **WHEN** an episode is requested whose row from an earlier request is completed, failed, cancelled or pending
- **THEN** that row is taken over: queued, with this request's payload, a fresh retry ladder and any armed timer cancelled

#### Scenario: Row cannot be written
- **WHEN** the row insert fails
- **THEN** the episode is counted as skipped with a history row and is not sent to the torrenter

### Requirement: Unreachable torrenter costs no attempt
When a dispatch fails because the torrenter could not be reached at all (a transport error that is not a timeout), the webserver SHALL return the rows of that dispatch to pending due two minutes later without incrementing their attempts, record the unreachable code and reason on the row and the notification, wake the scheduler so the timers are armed at once, and, within a group, SHALL defer the remaining rows after the first refused call rather than asking the torrenter for each. An answer with an error status, a timeout and a cancelled dispatch SHALL be handled on the usual retry ladder.

#### Scenario: Torrenter restarting
- **WHEN** the torrenter is restarting as a group of ten rows is dispatched and the first call is refused
- **THEN** one call is made, all ten rows are due two minutes later with their attempt counts unchanged, and their notifications read that the download service is unreachable

#### Scenario: Torrenter answers with an error
- **WHEN** the torrenter answers a dispatch with status 500
- **THEN** the row takes its next step on the retry ladder as before

## MODIFIED Requirements

### Requirement: Show Download API Endpoint Summary

The webserver service SHALL report per-episode disposition when a show download is requested, and SHALL NOT block the request on the torrent search.

#### Scenario: Episodes classified before responding

- **WHEN** a POST request is made to `/shows` with a series media object
- **THEN** the service SHALL count aired episodes whose rows were written as `queued_now`
- **AND** count future episodes successfully scheduled as `scheduled`
- **AND** count specials, unparseable air dates, duplicates, scheduling errors and episodes whose row could not be written as `skipped`
- **AND** count episodes already being searched for, downloaded, held for release or dispatched as `in_progress`
- **AND** return HTTP 202 with those counts

#### Scenario: Notifications and rows exist before the response

- **WHEN** the response is returned
- **THEN** every queued episode SHALL already have a notification with status `searching` and a `ScheduledDownloads` row with status `queued`
- **AND** every scheduled episode SHALL already have a notification with status `scheduled` and a pending row

#### Scenario: Torrent search runs after the response

- **WHEN** aired episodes are queued
- **THEN** the service SHALL dispatch their rows to the torrenter on a context detached from the request, pack-first when season packs are on, then one episode per call
- **AND** the request SHALL complete without waiting for that dispatch
- **AND** each row SHALL be settled on its own episode's answer: completed, rescheduled with backoff, or failed with a notification and history entry
