# webserver-downloads Capability Delta

**Change ID**: `add-download-notifications`

This document specifies the changes to the `webserver-downloads` capability to support notification creation and management.

## ADDED Requirements

### Requirement: Notification Repository

The webserver service SHALL provide a repository interface for managing download notifications.

#### Scenario: Create notification
- **WHEN** `CreateNotification()` is called with notification details
- **THEN** the repository SHALL insert a new record into the Notifications table
- **AND** set created_at and updated_at to current timestamp
- **AND** include trace_id and span_id from context for observability
- **AND** return the created notification with generated ID

#### Scenario: Update notification stage
- **WHEN** `UpdateNotification()` is called with notification ID and new stage
- **THEN** the repository SHALL update the notification's stage field
- **AND** update the updated_at timestamp
- **AND** return the updated notification

#### Scenario: Fetch recent notifications
- **WHEN** `GetNotifications()` is called with limit parameter
- **THEN** the repository SHALL query notifications ordered by created_at DESC
- **AND** limit results to specified limit (default 100)
- **AND** return notifications that are not auto-dismissed
- **AND** include all metadata (poster_url, media_title, stage, etc.)

#### Scenario: Fetch notifications grouped by media
- **WHEN** `GetGroupedNotifications()` is called
- **THEN** the repository SHALL query notifications grouped by tvdb_id
- **AND** order groups by the latest notification timestamp within each group
- **AND** return each group with list of notifications for that media
- **AND** include media metadata (title, poster, category)

#### Scenario: Mark notification as read
- **WHEN** `MarkAsRead()` is called with notification ID
- **THEN** the repository SHALL update is_read to true
- **AND** update updated_at timestamp
- **AND** return success if notification exists
- **AND** return error if notification not found

#### Scenario: Dismiss notification
- **WHEN** `DismissNotification()` is called with notification ID
- **THEN** the repository SHALL update auto_dismissed to true
- **AND** update updated_at timestamp
- **AND** return success if notification exists
- **AND** return error if notification not found

#### Scenario: Get unread count
- **WHEN** `GetUnreadCount()` is called
- **THEN** the repository SHALL count notifications where is_read=false AND auto_dismissed=false
- **AND** return the count as an integer

#### Scenario: Cleanup old notifications
- **WHEN** `CleanupOld()` is called with retention days parameter
- **THEN** the repository SHALL delete notifications older than retention days
- **AND** return count of deleted notifications

#### Scenario: Auto-dismiss successful downloads
- **WHEN** `AutoDismissSuccessful()` is called with age threshold
- **THEN** the repository SHALL update auto_dismissed=true for notifications where stage='completed' AND status='success' AND created_at < threshold
- **AND** return count of auto-dismissed notifications

#### Scenario: Prevent duplicate notifications per episode
- **WHEN** creating a notification for an episode that already has a notification
- **THEN** the repository SHALL detect duplicate via UNIQUE constraint (tvdb_id, season, episode)
- **AND** update the existing notification instead of creating new
- **AND** preserve original created_at but update updated_at

### Requirement: Notification API Endpoints

The webserver service SHALL provide REST API endpoints for accessing and managing notifications.

#### Scenario: Fetch all notifications
- **WHEN** a GET request is made to `/api/notifications`
- **THEN** the handler SHALL call repository.GetNotifications()
- **AND** support query params: limit, offset, stage, status
- **AND** return JSON array of notifications with HTTP 200
- **AND** return empty array if no notifications exist

#### Scenario: Fetch notifications grouped by media
- **WHEN** a GET request is made to `/api/notifications/grouped`
- **THEN** the handler SHALL call repository.GetGroupedNotifications()
- **AND** return JSON object with groups keyed by tvdb_id
- **AND** each group contains media metadata and notification list
- **AND** return HTTP 200

#### Scenario: Get unread notification count
- **WHEN** a GET request is made to `/api/notifications/unread/count`
- **THEN** the handler SHALL call repository.GetUnreadCount()
- **AND** return JSON object {"count": N} with HTTP 200

#### Scenario: Mark notification as read
- **WHEN** a PATCH request is made to `/api/notifications/:id/read`
- **THEN** the handler SHALL parse notification ID from URL parameter
- **AND** call repository.MarkAsRead(id)
- **AND** return HTTP 200 with success message
- **AND** return HTTP 404 if notification not found

#### Scenario: Dismiss notification
- **WHEN** a DELETE request is made to `/api/notifications/:id`
- **THEN** the handler SHALL parse notification ID from URL parameter
- **AND** call repository.DismissNotification(id)
- **AND** return HTTP 200 with success message
- **AND** return HTTP 404 if notification not found

#### Scenario: Handle invalid notification ID
- **WHEN** a request is made with non-numeric notification ID
- **THEN** the handler SHALL return HTTP 400 with validation error
- **AND** specify that ID must be a valid integer

## MODIFIED Requirements

### Requirement: Movie Download Orchestration

The webserver download interactor SHALL orchestrate the movie download process including anime detection, release date checking, scheduling, **and notification creation**.

#### Scenario: Download released movie immediately
- **WHEN** `DownloadMovie()` is called with a movie that has already been released
- **THEN** the interactor SHALL fetch extended info from tvdb-proxy with mediaType="movie"
- **AND** determine anime classification from TVDB genres
- **AND** **create notification with stage='searching' and status='searching'**
- **AND** **extract poster_url from media object for notification**
- **AND** send the movie to torrenter service immediately via `/download` endpoint
- **AND** return success response

#### Scenario: Schedule unreleased movie
- **WHEN** `DownloadMovie()` is called with a movie that has a future release date
- **THEN** the interactor SHALL parse `media.Metadata.FirstAired` as release date
- **AND** compare release date to current date
- **AND** **create notification with stage='scheduled' and status='searching'**
- **AND** **include release_time in notification metadata**
- **AND** store movie in ScheduledDownloads table with future release_time
- **AND** return success response indicating movie is scheduled

#### Scenario: Handle notification creation failure gracefully
- **WHEN** notification creation fails during movie download orchestration
- **THEN** the interactor SHALL log the error with trace_id
- **AND** continue with download process (non-blocking)
- **AND** return success response for download

### Requirement: Show Download Orchestration (NEW SCENARIO)

The webserver download interactor SHALL create notifications when processing show downloads.

#### Scenario: Download aired episodes immediately
- **WHEN** `DownloadShow()` is called with aired episodes
- **THEN** the interactor SHALL create notification with stage='searching' for each episode
- **AND** extract poster_url, season, episode, absoluteEpisode from media object
- **AND** send episodes to torrenter service immediately
- **AND** handle notification creation failures gracefully (non-blocking)

#### Scenario: Schedule future episodes
- **WHEN** `DownloadShow()` is called with future episodes
- **THEN** the interactor SHALL create notification with stage='scheduled' for each future episode
- **AND** include air date in notification metadata
- **AND** store episodes in ScheduledDownloads table
- **AND** handle notification creation failures gracefully (non-blocking)

### Requirement: Notification Cleanup Job (NEW)

The webserver service SHALL run a background job to clean up old notifications and auto-dismiss successful downloads.

#### Scenario: Daily cleanup of old notifications
- **WHEN** the cleanup job runs (daily at 3 AM)
- **THEN** the job SHALL call repository.CleanupOld(30 days)
- **AND** log count of deleted notifications
- **AND** handle errors gracefully (retry next day)

#### Scenario: Auto-dismiss successful downloads after 24 hours
- **WHEN** the cleanup job runs
- **THEN** the job SHALL call repository.AutoDismissSuccessful(24 hours)
- **AND** log count of auto-dismissed notifications
- **AND** handle errors gracefully

#### Scenario: Cleanup job starts with webserver
- **WHEN** the webserver service starts
- **THEN** the cleanup job goroutine SHALL be spawned
- **AND** run on a daily ticker
- **AND** log startup message

## Related Capabilities

This change relates to:
- **torrenter-downloads**: Torrenter updates notification stages during download progress
- **ui-notifications**: UI components consume notification API endpoints

## Observability Requirements

All notification operations SHALL:
- Accept context.Context as first parameter
- Propagate trace_id and span_id from context to database
- Log errors with context for trace correlation
- Include notification ID in log messages
- Use structured logging with key-value pairs

## Data Model

The Notifications table schema:

```sql
CREATE TABLE IF NOT EXISTS Notifications (
    id SERIAL PRIMARY KEY,
    tvdb_id TEXT NOT NULL,
    media_title TEXT NOT NULL,
    category TEXT NOT NULL, -- 'series' or 'movie'
    season INTEGER,
    episode INTEGER,
    absolute_episode INTEGER,
    poster_url TEXT,
    is_anime BOOLEAN DEFAULT FALSE,
    stage TEXT NOT NULL, -- 'scheduled', 'searching', 'downloading', 'completed', 'failed'
    status TEXT NOT NULL, -- 'searching', 'downloading', 'success', 'failure'
    reason TEXT,
    torrent_hash TEXT,
    is_read BOOLEAN DEFAULT FALSE,
    auto_dismissed BOOLEAN DEFAULT FALSE,
    trace_id TEXT,
    span_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tvdb_id, season, episode)
);
```

## Non-Functional Requirements

- Notification API endpoints SHALL respond within 200ms (p95)
- Notification creation SHALL not block download orchestration
- Cleanup job SHALL run outside peak usage hours (3 AM)
- Database queries SHALL use indexes for performance
- Notification fetch SHALL limit to 100 results by default
