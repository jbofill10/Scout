# Implementation Tasks: Add Download Notifications

**Change ID**: `add-download-notifications`

This document outlines the implementation tasks required to deliver the notifications feature. Tasks are ordered to deliver incremental, user-visible progress while managing dependencies.

## Phase 1: Database Schema & Migrations

These tasks establish the data foundation. Must complete before backend work.

- [x] **1.1** Create SQL migration script `sql/migrations/001_add_notifications_table.sql`
  - Create Notifications table with all required columns
  - Add indexes (tvdb_id, stage, status, created_at, is_read)
  - Add UNIQUE constraint on (tvdb_id, season, episode)
  - **Validation**: Run migration on local PostgreSQL, verify table creation
  - **Dependencies**: None
  - **Assigned**: webserver-maintainer agent

- [x] **1.2** Create migration script for backfilling existing data
  - Migrate ShowDownloadHistory entries to Notifications table
  - Map old status values to new stage/status values
  - **Validation**: Verify data integrity after migration
  - **Dependencies**: 1.1
  - **Assigned**: webserver-maintainer agent

- [x] **1.3** Update `sql/setup-postgres.sql` with Notifications table
  - Add CREATE TABLE statement for fresh installs
  - Add all indexes
  - **Validation**: Run setup script on fresh database
  - **Dependencies**: 1.1
  - **Assigned**: webserver-maintainer agent

- [x] **1.4** Document migration in DEPLOYMENT.md
  - Add instructions for running migration on existing deployments
  - Add rollback instructions
  - **Validation**: Manual review
  - **Dependencies**: 1.1, 1.2
  - **Assigned**: webserver-maintainer agent

## Phase 2: Webserver Backend (Repository Layer)

Implements data access layer for notifications. Can be tested independently.

- [x] **2.1** Create NotificationRepository interface
  - Define methods: CreateNotification, UpdateNotification, GetNotifications, GetGrouped, MarkAsRead, Dismiss, GetUnreadCount, CleanupOld
  - Add to `backend/internal/webserver/repository/repository.go`
  - **Validation**: Compiles successfully
  - **Dependencies**: 1.1
  - **Assigned**: webserver-maintainer agent

- [x] **2.2** Implement NotificationRepository in PostgreSQL
  - Create `backend/internal/webserver/repository/notification.go`
  - Implement all interface methods with proper context propagation
  - Use parameterized queries to prevent SQL injection
  - Include trace_id/span_id in all inserts/updates
  - **Validation**: Unit tests for each method
  - **Dependencies**: 2.1
  - **Assigned**: webserver-maintainer agent

- [x] **2.3** Write unit tests for NotificationRepository
  - Create `backend/internal/webserver/repository/notification_test.go`
  - Test all CRUD operations
  - Test grouping logic
  - Test cleanup logic
  - **Validation**: `go test ./internal/webserver/repository/...` passes
  - **Dependencies**: 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **2.4** Generate mocks for NotificationRepository
  - Run mockery to generate mocks in `repository/mocks/`
  - **Validation**: Mock files generated successfully
  - **Dependencies**: 2.1
  - **Assigned**: webserver-maintainer agent

## Phase 3: Webserver Backend (Handler & API Layer)

Exposes notifications via REST API. Builds on repository layer.

- [x] **3.1** Create NotificationHandler struct
  - Create `backend/internal/webserver/handlers/notification_handler.go`
  - Constructor with repository dependency injection
  - **Validation**: Compiles successfully
  - **Dependencies**: 2.1
  - **Assigned**: webserver-maintainer agent

- [x] **3.2** Implement GET /api/notifications endpoint
  - Fetch recent notifications (limit 100, ordered by created_at DESC)
  - Support query params: limit, offset, stage, status
  - Return JSON with proper error handling
  - **Validation**: Handler unit tests
  - **Dependencies**: 3.1, 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **3.3** Implement GET /api/notifications/grouped endpoint
  - Fetch notifications grouped by tvdb_id
  - Sort groups by latest notification timestamp
  - Return JSON with media metadata and episode list
  - **Validation**: Handler unit tests
  - **Dependencies**: 3.1, 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **3.4** Implement GET /api/notifications/unread/count endpoint
  - Return count of unread notifications
  - Simple JSON response: {"count": 5}
  - **Validation**: Handler unit tests
  - **Dependencies**: 3.1, 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **3.5** Implement PATCH /api/notifications/:id/read endpoint
  - Mark single notification as read (is_read = true)
  - Return 404 if notification not found
  - **Validation**: Handler unit tests
  - **Dependencies**: 3.1, 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **3.6** Implement DELETE /api/notifications/:id endpoint
  - Soft delete notification (set auto_dismissed = true)
  - Return 404 if notification not found
  - **Validation**: Handler unit tests
  - **Dependencies**: 3.1, 2.2
  - **Assigned**: webserver-maintainer agent

- [x] **3.7** Add routes to webserver main.go
  - Register all notification routes under /api/notifications
  - Wire up handler with repository
  - **Validation**: Server starts without errors
  - **Dependencies**: 3.1-3.6
  - **Assigned**: webserver-maintainer agent

## Phase 4: Webserver Backend (Notification Creation)

Integrates notification creation into download flow. First user-visible backend milestone.

- [x] **4.1** Modify DownloadInteractor to create notifications on show download
  - In `DownloadShow()`, create notification with stage "searching" when download starts
  - Extract poster_url from tvdb.Media object
  - Use context for trace_id/span_id
  - Handle errors gracefully (log but don't block download)
  - **Validation**: Integration test, verify notification created in DB
  - **Dependencies**: 2.2, 3.7
  - **Assigned**: webserver-maintainer agent

- [x] **4.2** Modify DownloadInteractor to create notifications on movie download
  - In `DownloadMovie()`, create notification with stage "searching" when download starts
  - Handle same way as shows
  - **Validation**: Integration test, verify notification created in DB
  - **Dependencies**: 2.2, 3.7
  - **Assigned**: webserver-maintainer agent

- [x] **4.3** Modify DownloadInteractor to create notifications on scheduling
  - When episode is added to ScheduledDownloads, create notification with stage "scheduled"
  - Include release_time metadata
  - **Validation**: Integration test, verify scheduled notification created
  - **Dependencies**: 2.2, 3.7
  - **Assigned**: webserver-maintainer agent

- [x] **4.4** Add cleanup job to webserver
  - Create goroutine in main.go that runs daily
  - Calls NotificationRepository.CleanupOld(30 days)
  - Also auto-dismisses successful notifications older than 24 hours
  - **Validation**: Manual test with old notifications
  - **Dependencies**: 2.2
  - **Assigned**: webserver-maintainer agent

## Phase 5: Torrenter Backend (Notification Updates)

Updates notifications as download progresses. Completes backend notification lifecycle.

- [x] **5.1** Add NotificationRepository to torrenter
  - Copy interface from webserver (consider moving to pkg/notifications)
  - Implement PostgreSQL version for torrenter
  - **Validation**: Compiles successfully
  - **Dependencies**: 2.1, 2.2
  - **Assigned**: torrenter-maintainer agent

- [x] **5.2** Update notification when download starts (torrent.go)
  - In `HandleDownload()`, after adding torrent to qBittorrent
  - Update notification stage to "downloading", save torrent_hash
  - Use context for trace correlation
  - **Validation**: Integration test, verify stage updated
  - **Dependencies**: 5.1
  - **Assigned**: torrenter-maintainer agent

- [x] **5.3** Update notification on completion (download_interactor.go)
  - In `handleDownloadCompletion()`, when processing succeeds
  - Update notification stage to "completed", status to "success"
  - **Validation**: Integration test, verify completion notification
  - **Dependencies**: 5.1
  - **Assigned**: torrenter-maintainer agent

- [x] **5.4** Update notification on failure (download_interactor.go, torrent.go)
  - When download fails at any stage, update stage to "failed"
  - Save descriptive reason (no torrents found, torrent client error, processing error, etc.)
  - **Validation**: Integration test, trigger failure and verify notification
  - **Dependencies**: 5.1
  - **Assigned**: torrenter-maintainer agent

- [x] **5.5** Write integration tests for torrenter notification flow
  - Test full flow: searching → downloading → completed
  - Test failure scenarios
  - Use repository mocks
  - **Validation**: `go test ./cmd/torrenter/...` passes
  - **Dependencies**: 5.2, 5.3, 5.4
  - **Assigned**: torrenter-maintainer agent

## Phase 6: UI Components (Notification Bell)

First user-visible UI milestone. Users can see notifications.

- [ ] **6.1** Create NotificationDropdown component
  - Create `ui/src/components/ui/NotificationDropdown.tsx`
  - Props: isOpen, onClose, notifications array
  - Use Material-UI Slide, Backdrop, Box components
  - Follow SearchDropdown pattern for animation/overlay
  - **Validation**: Component renders without errors
  - **Dependencies**: None (parallel with backend)
  - **Assigned**: ui-maintainer agent

- [ ] **6.2** Add TanStack Query hook for fetching notifications
  - Create `ui/src/hooks/useNotifications.ts`
  - Query `/api/notifications/grouped` endpoint
  - Auto-refresh every 30 seconds
  - Cache for 30 seconds
  - **Validation**: Hook fetches data successfully
  - **Dependencies**: 3.3, 3.7 (can stub API during dev)
  - **Assigned**: ui-maintainer agent

- [ ] **6.3** Add TanStack Query hook for unread count
  - Create `ui/src/hooks/useUnreadCount.ts`
  - Query `/api/notifications/unread/count` endpoint
  - Auto-refresh every 30 seconds
  - **Validation**: Hook fetches count successfully
  - **Dependencies**: 3.4, 3.7
  - **Assigned**: ui-maintainer agent

- [ ] **6.4** Implement notification grouping logic in UI
  - In NotificationDropdown, group notifications by media
  - Show media poster, title, latest stage
  - For shows: List episodes with individual statuses
  - Expandable/collapsible groups
  - **Validation**: Grouping displays correctly with mock data
  - **Dependencies**: 6.1
  - **Assigned**: ui-maintainer agent

- [ ] **6.5** Add mark as read functionality
  - Implement mutation for PATCH /api/notifications/:id/read
  - Call on notification click
  - Invalidate queries to refresh UI
  - **Validation**: Notification marked as read, badge updates
  - **Dependencies**: 3.5, 6.1, 6.2
  - **Assigned**: ui-maintainer agent

- [ ] **6.6** Add dismiss functionality
  - Implement mutation for DELETE /api/notifications/:id
  - Add dismiss button (X icon) to each notification
  - Invalidate queries to refresh UI
  - **Validation**: Notification dismissed, removed from list
  - **Dependencies**: 3.6, 6.1, 6.2
  - **Assigned**: ui-maintainer agent

- [ ] **6.7** Add bell icon to Navbar
  - Add NotificationsIcon from @mui/icons-material
  - Add Badge component showing unread count
  - Position in top-right, between nav buttons and search icon
  - Wire up click handler to open NotificationDropdown
  - **Validation**: Bell icon displays, badge shows count
  - **Dependencies**: 6.1, 6.3
  - **Assigned**: ui-maintainer agent

- [ ] **6.8** Style NotificationDropdown to match theme
  - Use warm dark theme colors (indigo/purple)
  - Match card styling from other components
  - Responsive layout (though desktop-focused)
  - Hover states, transitions
  - **Validation**: Visual review, matches existing UI
  - **Dependencies**: 6.1, 6.4
  - **Assigned**: ui-maintainer agent

## Phase 7: Testing & Polish

Ensures quality and catches regressions. Must complete before considering feature done.

- [x] **7.1** Run test-guardian on webserver changes
  - Verify all unit tests pass
  - Check for regressions
  - Ensure new tests have proper coverage
  - **Validation**: test-guardian report shows all passing
  - **Dependencies**: Phase 2, 3, 4 complete
  - **Assigned**: test-guardian agent

- [x] **7.2** Run test-guardian on torrenter changes
  - Verify all unit tests pass
  - Check for regressions
  - Ensure notification updates work correctly
  - **Validation**: test-guardian report shows all passing
  - **Dependencies**: Phase 5 complete
  - **Assigned**: test-guardian agent

- [ ] **7.3** Run UI linting and tests
  - `npm run lint` in ui/ directory
  - Fix any ESLint errors
  - Verify components follow React best practices
  - **Validation**: Lint passes with no errors
  - **Dependencies**: Phase 6 complete
  - **Assigned**: ui-maintainer agent

- [ ] **7.4** End-to-end testing
  - Deploy to local Docker Compose environment
  - Download a show with multiple episodes
  - Verify notifications appear at each stage
  - Test mark as read functionality
  - Test dismiss functionality
  - Verify auto-dismiss after 24 hours
  - **Validation**: Full user flow works as expected
  - **Dependencies**: All phases complete
  - **Assigned**: Manual testing

- [ ] **7.5** Update CLAUDE.md with notification system documentation
  - Add section on notification architecture
  - Document API endpoints
  - Document notification stages
  - Update database schema section
  - **Validation**: Manual review
  - **Dependencies**: All phases complete
  - **Assigned**: Documentation

- [x] **7.6** Update DEPLOYMENT.md with migration instructions
  - Document how to run migration on existing deployments
  - Add troubleshooting section
  - **Validation**: Manual review
  - **Dependencies**: 1.4
  - **Assigned**: Documentation

## Parallelization Opportunities

Tasks that can be worked on in parallel (no dependencies):

**Group A (Backend Foundation)**:
- Phase 1 (Database) → Phase 2 (Repository) → Phase 3 (Handlers)

**Group B (UI Foundation, parallel with backend)**:
- 6.1, 6.2, 6.3 (can use mocked API responses during development)

**Group C (Integration)**:
- Phase 4 (Webserver integration) and Phase 5 (Torrenter integration) can overlap if both use completed repository layer

**Group D (Polish)**:
- 6.4, 6.7, 6.8 (UI polish tasks) can overlap

## Definition of Done

A task is considered complete when:
1. Code is written and compiles successfully
2. Unit tests are written and passing
3. Integration tests are passing (where applicable)
4. test-guardian verification passes (for code changes)
5. Code follows Scout's architectural patterns (layered architecture, context propagation, observability)
6. golangci-lint passes (for Go code)
7. ESLint passes (for UI code)
8. Changes are manually tested in local environment

## Rollback Strategy

If issues are discovered after deployment:

1. **Database rollback**: Run migration down script to drop Notifications table
2. **Code rollback**: Revert to previous git commit
3. **Partial rollback**: Disable notification creation (feature flag) while keeping table and API

## Success Metrics

After implementation, verify:
- [ ] Notifications created for 100% of downloads (shows and movies)
- [ ] Notifications update correctly through all stages
- [ ] UI bell icon shows accurate unread count
- [ ] Failed downloads show descriptive error messages
- [ ] Auto-dismiss works for successful downloads
- [ ] Cleanup job removes notifications older than 30 days
- [ ] API response times <200ms for notification endpoints
- [ ] No regressions in existing download functionality
