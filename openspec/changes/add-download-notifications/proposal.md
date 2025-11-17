# Proposal: Add Download Notifications

**Change ID**: `add-download-notifications`
**Status**: Proposed
**Author**: Claude Code
**Date**: 2025-11-15

## Problem Statement

Scout currently lacks visibility into download progress and status. Users have no way to:
- Track when scheduled downloads are queued for future air dates
- See which torrents are actively being searched for
- Monitor download progress for in-flight torrents
- Know when downloads complete successfully or fail
- Understand why downloads fail (no torrents found, torrent client error, etc.)

Additionally, there's a critical database inconsistency: the torrenter service expects a `DownloadHistory` table that doesn't exist in the schema, causing runtime errors when trying to record download status.

## Proposed Solution

Implement a comprehensive notifications system that tracks downloads through five stages:

1. **Scheduled** - Media added to schedule, waiting for air/release date
2. **Searching** - Actively searching Prowlarr for torrents
3. **Downloading** - Torrent added to qBittorrent, downloading
4. **Completed** - Download finished and media processed successfully
5. **Failed** - Error at any stage with descriptive reason

### User Experience

A bell icon in the top-right corner of the Navbar displays a badge with the count of unread notifications. Clicking the bell opens a dropdown showing recent notifications grouped by media item (show/movie). Each group displays:
- Media poster and title
- Latest notification stage/status
- For shows: List of episodes with individual statuses
- Ability to mark as read or dismiss

Successful downloads auto-dismiss after 24 hours to keep the list focused on active downloads and failures.

### Technical Approach

**Database**: Create a unified `Notifications` table with proper columns for UI display (tvdb_id, poster_url, stage, status, is_read, etc.) and indexes for performance. This fixes the existing DownloadHistory inconsistency.

**Backend**:
- Webserver creates initial notifications when downloads are initiated or scheduled
- Torrenter updates notifications as downloads progress through stages
- RESTful API endpoints for fetching, marking read, and dismissing notifications
- Automatic cleanup job removes notifications older than 30 days

**Frontend**:
- NotificationDropdown component using Material-UI, following existing SearchDropdown patterns
- Auto-refresh every 30 seconds using TanStack Query
- Grouping logic to consolidate multiple episodes of the same show
- Notification badge with unread count

## Affected Capabilities

### webserver-downloads (MODIFIED)
- Add NotificationRepository interface and PostgreSQL implementation
- Add NotificationHandler for API endpoints
- Modify DownloadInteractor to create notifications on download initiation
- Add routes for notification management

### torrenter-downloads (MODIFIED)
- Add NotificationRepository to torrenter service
- Update notifications in torrent.go when download starts
- Update notifications in download_interactor.go when download completes/fails
- Fix DownloadHistory table reference to use Notifications table

### ui-notifications (NEW CAPABILITY)
- Bell icon component in Navbar with unread badge
- NotificationDropdown component with slide animation and backdrop
- Notification grouping logic (by media, then by stage)
- TanStack Query integration for auto-refresh
- Mark as read and dismiss interactions

## Out of Scope

- Real-time notifications via WebSocket/SSE (deferred to future enhancement)
- Push notifications to mobile devices
- Email/SMS notifications
- Notification preferences/filtering (all notifications shown by default)

## Database Schema Changes

### New Table: Notifications

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
    UNIQUE(tvdb_id, season, episode) -- Prevent duplicate notifications per episode
);

CREATE INDEX idx_notifications_tvdb_id ON Notifications(tvdb_id);
CREATE INDEX idx_notifications_stage ON Notifications(stage);
CREATE INDEX idx_notifications_status ON Notifications(status);
CREATE INDEX idx_notifications_created_at ON Notifications(created_at DESC);
CREATE INDEX idx_notifications_is_read ON Notifications(is_read);
```

### Migration Strategy

1. Create Notifications table with new schema
2. Migrate existing ShowDownloadHistory entries to Notifications (backfill)
3. Update all code references from ShowDownloadHistory to Notifications
4. Keep ShowDownloadHistory and MovieDownloadHistory for backwards compatibility (deprecated)

## API Changes

### New Endpoints (webserver)

- `GET /api/notifications` - Fetch all notifications (limit 100, ordered by created_at DESC)
- `GET /api/notifications/grouped` - Fetch notifications grouped by media
- `GET /api/notifications/unread/count` - Get count of unread notifications
- `PATCH /api/notifications/:id/read` - Mark notification as read
- `DELETE /api/notifications/:id` - Dismiss notification

## Implementation Phases

1. **Database & Schema** - Create Notifications table, indexes, migration script
2. **Webserver Backend** - Repository, handlers, routes, notification creation logic
3. **Torrenter Backend** - Repository, notification update logic at each stage
4. **UI Components** - Bell icon, dropdown, grouping, auto-refresh
5. **Testing & Polish** - test-guardian verification, end-to-end testing

## Risks & Mitigation

**Risk**: Database schema change may require downtime
**Mitigation**: Use migration script, create new table without dropping old ones

**Risk**: Notification table grows large over time
**Mitigation**: Automatic cleanup job (30 days retention), indexed queries

**Risk**: Frequent polling (30s) may increase server load
**Mitigation**: Efficient queries with indexes, limit to 100 notifications, use TanStack Query caching

**Risk**: Notifications may not update if torrenter crashes
**Mitigation**: Observability with trace_id/span_id, status can be reconciled on restart

## Success Criteria

- [ ] Users can see all downloads in progress from the UI
- [ ] Notifications accurately reflect download stages (scheduled, searching, downloading, completed, failed)
- [ ] Failed downloads show descriptive error messages
- [ ] Notification bell shows unread count badge
- [ ] Notifications group by media item for shows with multiple episodes
- [ ] Successful downloads auto-dismiss after 24 hours
- [ ] Old notifications (>30 days) are automatically cleaned up
- [ ] All existing tests pass (test-guardian verification)
- [ ] Database inconsistency (DownloadHistory table) is resolved

## Future Enhancements

- Real-time notifications via WebSocket/SSE
- Notification preferences (filter by category, stage, etc.)
- Notification history view (show dismissed notifications)
- Email notifications for failures
- Retry failed downloads from notification
- Download progress percentage in notification
