## ADDED Requirements

### Requirement: Cancel a requested item
The webserver SHALL expose `POST /downloads/:id/cancel`, where `:id` is the item's notification tvdb id, and cancelling SHALL stop every piece of work for that item.

#### Scenario: Cancel a scheduled episode
- **WHEN** the item has a `pending` scheduled row with an armed timer
- **THEN** the row's status becomes `cancelled`, the timer is released, and the notification status becomes `cancelled`

#### Scenario: Cancel a downloading item
- **WHEN** the torrenter has a torrent in flight for the item
- **THEN** the webserver asks the torrenter to cancel it before marking the notification `cancelled`

#### Scenario: Nothing to cancel
- **WHEN** no row, torrent or notification exists for the id
- **THEN** the endpoint responds 404

### Requirement: Retry an item now
The webserver SHALL expose `POST /downloads/:id/retry` which re-arms the item's scheduled rows and dispatches them without waiting for the next poll.

#### Scenario: Retry during backoff
- **WHEN** the item's row is `pending` with a future `next_attempt_at`
- **THEN** `next_attempt_at` becomes now, the row is dispatched within seconds, and the notification reads `searching`

#### Scenario: Retry a permanently failed item
- **WHEN** the item's row is `failed`
- **THEN** the row returns to `pending` with `attempts` reset to 0 and is dispatched

#### Scenario: Retry while a dispatch is in progress
- **WHEN** the item's row is `queued`
- **THEN** the endpoint responds 409 and changes nothing

### Requirement: List and grab releases by hand
The webserver SHALL expose `GET /downloads/:id/releases` returning the torrenter's candidate list for the item and `POST /downloads/:id/releases` to download a chosen candidate.

#### Scenario: Grab replaces automatic work
- **WHEN** a candidate is posted for an item that has a pending row
- **THEN** the torrenter is asked to download exactly that release, the row is marked completed and its timer released, and the response carries the torrenter's result

### Requirement: Upcoming schedule
The webserver SHALL expose `GET /schedule/upcoming` returning every `pending` scheduled row ordered by its effective due time, with item key, title, season, episode, poster, release time, next attempt, attempts and last failure.

#### Scenario: Retry rows are included
- **WHEN** a row is waiting on a retry backoff
- **THEN** it appears with `next_attempt_at` set and its attempt count

### Requirement: Cancelled rows can be re-requested
Scheduling an item whose row is `cancelled` SHALL revive that row rather than report a duplicate.

#### Scenario: Request after cancel
- **WHEN** a cancelled episode is requested again
- **THEN** its row returns to `pending` with `attempts` 0 and no `next_attempt_at`, and the request summary counts it as scheduled

### Requirement: Cancelled dispatch results are ignored
A download result with code `cancelled` SHALL NOT create a retry row, mark the row failed, or change the notification.

#### Scenario: Cancel during search
- **WHEN** the torrenter reports `cancelled` for an item the webserver had dispatched
- **THEN** the row keeps its `cancelled` status and no retry is scheduled
