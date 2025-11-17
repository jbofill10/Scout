# torrenter-downloads Capability Delta

**Change ID**: `add-download-notifications`

This document specifies the changes to the `torrenter-downloads` capability to support notification updates during download progress.

## ADDED Requirements

### Requirement: Notification Repository Integration

The torrenter service SHALL integrate a notification repository for updating download notifications.

#### Scenario: Initialize notification repository
- **WHEN** the torrenter service starts
- **THEN** the service SHALL create NotificationRepository instance
- **AND** connect to shared PostgreSQL Notifications table
- **AND** inject repository into download interactor and torrent handler

#### Scenario: Update notification by torrent hash
- **WHEN** `UpdateNotificationByHash()` is called with torrent hash and new stage
- **THEN** the repository SHALL query Notifications table WHERE torrent_hash = $1
- **AND** update stage and status fields
- **AND** update updated_at timestamp
- **AND** return success if notification found

#### Scenario: Update notification by media identifiers
- **WHEN** `UpdateNotificationByMedia()` is called with tvdb_id, season, episode, and new stage
- **THEN** the repository SHALL query Notifications table WHERE tvdb_id = $1 AND season = $2 AND episode = $3
- **AND** update stage and status fields
- **AND** update updated_at timestamp
- **AND** return success if notification found

#### Scenario: Set failure reason
- **WHEN** `SetFailureReason()` is called with notification ID and error message
- **THEN** the repository SHALL update stage to 'failed'
- **AND** set status to 'failure'
- **AND** set reason field to error message
- **AND** update updated_at timestamp

### Requirement: Notification Update on Download Start

The torrenter service SHALL update notifications when torrents are added to qBittorrent.

#### Scenario: Update notification when download starts
- **WHEN** a torrent is successfully added to qBittorrent
- **THEN** the service SHALL update notification stage to 'downloading'
- **AND** update status to 'downloading'
- **AND** save torrent_hash to notification record
- **AND** use context for trace correlation
- **AND** log notification update with trace_id

#### Scenario: Handle notification update failure on download start
- **WHEN** notification update fails when download starts
- **THEN** the service SHALL log error with trace_id
- **AND** continue with download process (non-blocking)
- **AND** NOT fail the download due to notification error

### Requirement: Notification Update on Download Completion

The torrenter service SHALL update notifications when downloads complete successfully.

#### Scenario: Update notification when processing succeeds
- **WHEN** media processing completes successfully after download
- **THEN** the service SHALL update notification stage to 'completed'
- **AND** update status to 'success'
- **AND** use context for trace correlation
- **AND** log success with trace_id

#### Scenario: Handle notification update failure on success
- **WHEN** notification update fails on successful completion
- **THEN** the service SHALL log error with trace_id
- **AND** continue with completion flow (non-blocking)
- **AND** NOT fail the download due to notification error

### Requirement: Notification Update on Download Failure

The torrenter service SHALL update notifications when downloads fail at any stage.

#### Scenario: Update notification when torrent search fails
- **WHEN** Prowlarr search returns no results
- **THEN** the service SHALL update notification stage to 'failed'
- **AND** set status to 'failure'
- **AND** set reason to "No suitable torrent found"
- **AND** include search query details in reason
- **AND** log failure with trace_id

#### Scenario: Update notification when qBittorrent add fails
- **WHEN** adding torrent to qBittorrent fails
- **THEN** the service SHALL update notification stage to 'failed'
- **AND** set status to 'failure'
- **AND** set reason to qBittorrent error message
- **AND** log failure with trace_id

#### Scenario: Update notification when processing fails
- **WHEN** media processing fails after download
- **THEN** the service SHALL update notification stage to 'failed'
- **AND** set status to 'failure'
- **AND** set reason to processing error message
- **AND** log failure with trace_id

#### Scenario: Handle notification update failure on failure
- **WHEN** notification update fails when recording a failure
- **THEN** the service SHALL log error with trace_id
- **AND** continue with error handling flow
- **AND** NOT suppress original error

## MODIFIED Requirements

### Requirement: Movie Download Processing

The torrenter service SHALL accept and process movie download requests through the existing download endpoint **and update notifications at each stage**.

#### Scenario: Accept movie download request
- **WHEN** a download request is received with `media.Category == "movie"`
- **THEN** the service SHALL accept the request
- **AND** process the movie download
- **AND** **update notification to stage='searching' if notification exists**
- **AND** NOT return early or skip processing

#### Scenario: Movie not in Plex proceeds to search
- **WHEN** movie existence check returns false
- **THEN** the service SHALL proceed to create movie search strategy
- **AND** search for torrents via Prowlarr
- **AND** **update notification to stage='downloading' when torrent is added to qBittorrent**
- **AND** **save torrent_hash to notification**
- **AND** download the best matching torrent

### Requirement: Movie Torrent Search

The torrenter service SHALL search for movie torrents using the generated search strategy **and update notifications on failure**.

#### Scenario: No movie torrents found
- **WHEN** Prowlarr search returns no results for movie
- **THEN** the service SHALL log warning with movie name and search query
- **AND** **update notification stage to 'failed' with reason "No suitable torrent found for {MovieName} ({Year})"**
- **AND** return error indicating no torrents found
- **AND** NOT add torrent to qBittorrent

### Requirement: Movie Download Monitoring

The torrenter service SHALL monitor movie torrent downloads and trigger post-processing upon completion **and update notifications on success or failure**.

#### Scenario: Process completed movie download
- **WHEN** a movie torrent completes downloading
- **THEN** the service SHALL call MediaProcessor.ProcessDownloadedTorrent
- **AND** determine movie base directory from Plex library
- **AND** construct save path as `{baseDir}/{filename}`
- **AND** create symbolic link from qBittorrent download location to save path
- **AND** **update notification stage to 'completed' with status 'success'**
- **AND** **log completion with trace_id**

#### Scenario: Movie processing fails
- **WHEN** media processing encounters an error
- **THEN** the service SHALL log error with trace_id
- **AND** **update notification stage to 'failed' with reason from error message**
- **AND** return error to caller

### Requirement: Show Download Processing (EXISTING, ADD NOTIFICATION UPDATES)

The torrenter service SHALL update notifications for show downloads at each stage.

#### Scenario: Show episode not in Plex proceeds to search
- **WHEN** episode existence check returns false
- **THEN** the service SHALL proceed to create episode search strategy
- **AND** search for torrents via Prowlarr
- **AND** **update notification to stage='downloading' when torrent is added to qBittorrent**
- **AND** **save torrent_hash to notification**
- **AND** download the best matching torrent

#### Scenario: No episode torrents found
- **WHEN** Prowlarr search returns no results for episode
- **THEN** the service SHALL log warning with show name, season, and episode
- **AND** **update notification stage to 'failed' with reason "No suitable torrent found for {ShowName} S{season}E{episode}"**
- **AND** return error indicating no torrents found

#### Scenario: Process completed episode download
- **WHEN** an episode torrent completes downloading
- **THEN** the service SHALL call MediaProcessor.ProcessDownloadedTorrent
- **AND** determine episode save path
- **AND** create symbolic link from qBittorrent download location to save path
- **AND** **update notification stage to 'completed' with status 'success'**
- **AND** **log completion with trace_id**

#### Scenario: Episode processing fails
- **WHEN** episode processing encounters an error
- **THEN** the service SHALL log error with trace_id
- **AND** **update notification stage to 'failed' with reason from error message**
- **AND** return error to caller

## Related Capabilities

This change relates to:
- **webserver-downloads**: Webserver creates initial notifications with stage='searching' or stage='scheduled'
- **ui-notifications**: UI components display notification updates

## Observability Requirements

All notification update operations SHALL:
- Accept context.Context as first parameter
- Extract trace_id and span_id from context
- Log notification updates with structured logging
- Include media identifiers (tvdb_id, season, episode) or torrent_hash in logs
- Use context-aware logging: `logger.InfoContext(ctx, "message", "key", value)`
- Handle errors gracefully without blocking download flow

## Error Handling

Notification update failures SHALL:
- Be logged with ERROR level and trace_id
- NOT block or fail the download process
- Preserve the original download error if both download and notification fail
- Allow download to proceed even if notification system is unavailable

## Performance Considerations

- Notification updates SHALL use database indexes (torrent_hash, tvdb_id)
- Notification updates SHALL use prepared statements to prevent SQL injection
- Notification repository SHALL reuse database connection pool
- Update operations SHALL timeout after 5 seconds to prevent blocking
