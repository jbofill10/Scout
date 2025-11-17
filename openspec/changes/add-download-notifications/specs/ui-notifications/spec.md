# ui-notifications Capability Specification

**Change ID**: `add-download-notifications`

This is a NEW capability that provides user interface components for viewing and managing download notifications.

## Purpose

The ui-notifications capability provides a visual notification system in the Scout UI that allows users to track download progress, view completed downloads, and manage failures. Notifications are displayed via a bell icon in the top navigation bar that opens a dropdown with grouped notifications.

## ADDED Requirements

### Requirement: Notification Bell Icon

The UI SHALL display a notification bell icon in the top navigation bar with an unread badge.

#### Scenario: Display bell icon in Navbar
- **WHEN** the Navbar component renders
- **THEN** the UI SHALL display a NotificationsIcon (bell) from @mui/icons-material
- **AND** position the bell icon in the top-right corner between navigation buttons and search icon
- **AND** style the icon to match the Navbar theme (indigo/purple palette)
- **AND** make the icon clickable to toggle notification dropdown

#### Scenario: Display unread count badge
- **WHEN** the bell icon is rendered
- **THEN** the UI SHALL fetch unread count from `/api/notifications/unread/count`
- **AND** display Badge component with count if count > 0
- **AND** hide badge if count = 0
- **AND** style badge with primary color (#4F46E5) and white text
- **AND** position badge in top-right corner of bell icon

#### Scenario: Badge updates when notifications change
- **WHEN** a notification is marked as read or dismissed
- **THEN** the UI SHALL invalidate the unread count query
- **AND** refetch the updated count
- **AND** update the badge display automatically

#### Scenario: Bell icon click opens dropdown
- **WHEN** user clicks the bell icon
- **THEN** the UI SHALL toggle the NotificationDropdown open state
- **AND** close SearchDropdown if open (only one dropdown at a time)
- **AND** fetch notifications data if not already loaded

### Requirement: Notification Dropdown Component

The UI SHALL provide a NotificationDropdown component that displays notifications in a slide-down overlay.

#### Scenario: Dropdown renders with backdrop
- **WHEN** NotificationDropdown is opened
- **THEN** the UI SHALL render a semi-transparent Backdrop component
- **AND** set backdrop color to rgba(0, 0, 0, 0.5)
- **AND** set z-index to 1200 (below dropdown, above page content)
- **AND** close dropdown when backdrop is clicked

#### Scenario: Dropdown slides down from top
- **WHEN** NotificationDropdown opens
- **THEN** the UI SHALL use Material-UI Slide component with direction="down"
- **AND** animate from top of screen
- **AND** set position to fixed with top: 0
- **AND** set z-index to 1300 (above backdrop)
- **AND** set max-width to 600px and center horizontally

#### Scenario: Dropdown closes on ESC key
- **WHEN** NotificationDropdown is open and user presses ESC key
- **THEN** the UI SHALL close the dropdown
- **AND** restore focus to bell icon

#### Scenario: Dropdown has header with title and close button
- **WHEN** NotificationDropdown renders
- **THEN** the UI SHALL display header with "Notifications" title
- **AND** display close button (X icon) in top-right
- **AND** style header with dark background matching theme
- **AND** make close button clickable to dismiss dropdown

#### Scenario: Dropdown body displays notification groups
- **WHEN** NotificationDropdown renders with notifications
- **THEN** the UI SHALL display notification groups in scrollable container
- **AND** set max-height to 80vh to prevent overflow
- **AND** enable vertical scrolling if content exceeds max-height
- **AND** style scrollbar to match theme

#### Scenario: Empty state when no notifications
- **WHEN** NotificationDropdown renders with zero notifications
- **THEN** the UI SHALL display empty state message "No notifications"
- **AND** style message with secondary text color
- **AND** center message in dropdown body

#### Scenario: Loading state while fetching notifications
- **WHEN** NotificationDropdown is opened before notifications are loaded
- **THEN** the UI SHALL display Skeleton loading components
- **AND** show 3 skeleton cards matching notification card dimensions
- **AND** replace with actual notifications when data loads

#### Scenario: Error state when fetch fails
- **WHEN** notifications fetch fails with error
- **THEN** the UI SHALL display error message
- **AND** display retry button
- **AND** retry fetch when button clicked

### Requirement: Notification Data Fetching

The UI SHALL fetch notifications from the webserver API using TanStack Query.

#### Scenario: Fetch notifications on dropdown open
- **WHEN** NotificationDropdown opens
- **THEN** the UI SHALL call GET /api/notifications/grouped endpoint
- **AND** use TanStack Query useQuery hook
- **AND** cache results for 30 seconds
- **AND** auto-refresh every 30 seconds while dropdown is open

#### Scenario: Fetch unread count periodically
- **WHEN** the UI is active
- **THEN** the UI SHALL fetch unread count every 30 seconds
- **AND** use TanStack Query useQuery with refetchInterval: 30000
- **AND** cache results for 30 seconds
- **AND** update badge when count changes

#### Scenario: Stop auto-refresh when dropdown closed
- **WHEN** NotificationDropdown closes
- **THEN** the UI SHALL stop auto-refreshing notifications
- **AND** keep cached data for 30 seconds
- **AND** resume auto-refresh if dropdown reopens within cache window

### Requirement: Notification Grouping

The UI SHALL group notifications by media item (tvdb_id) and display them hierarchically.

#### Scenario: Group notifications by media
- **WHEN** rendering notification list
- **THEN** the UI SHALL group notifications by tvdb_id
- **AND** display one card per media item
- **AND** show media poster and title in card header
- **AND** list all episodes/notifications for that media in card body

#### Scenario: Display latest stage per group
- **WHEN** rendering a notification group
- **THEN** the UI SHALL find the most recent notification in the group
- **AND** display the latest stage (scheduled, searching, downloading, completed, failed) prominently
- **AND** use color coding: blue for scheduled, yellow for searching/downloading, green for completed, red for failed
- **AND** show timestamp of latest notification

#### Scenario: Sort groups by latest notification
- **WHEN** rendering notification groups
- **THEN** the UI SHALL sort groups by the latest notification timestamp within each group
- **AND** display most recently updated groups at the top
- **AND** maintain sort order as notifications update

#### Scenario: Expandable episode list for shows
- **WHEN** rendering a notification group for a TV show with multiple episodes
- **THEN** the UI SHALL display collapsed view by default showing latest episode
- **AND** display expand/collapse button
- **AND** show full episode list when expanded
- **AND** display each episode with season/episode number and individual status

#### Scenario: Episode status indicators
- **WHEN** displaying episode list in notification group
- **THEN** the UI SHALL show status icon for each episode
- **AND** use checkmark icon for completed
- **AND** use download icon for downloading
- **AND** use search icon for searching/scheduled
- **AND** use error icon for failed
- **AND** display episode number (S01E01 format or absolute number for anime)

### Requirement: Notification Interactions

The UI SHALL allow users to mark notifications as read and dismiss them.

#### Scenario: Mark notification as read on click
- **WHEN** user clicks on a notification card
- **THEN** the UI SHALL call PATCH /api/notifications/:id/read for all unread notifications in the group
- **AND** use TanStack Query useMutation hook
- **AND** optimistically update local state (mark as read immediately)
- **AND** invalidate queries on success (refetch unread count and notifications)
- **AND** revert optimistic update on failure

#### Scenario: Dismiss notification
- **WHEN** user clicks dismiss button (X icon) on notification card
- **THEN** the UI SHALL call DELETE /api/notifications/:id for the notification
- **AND** use TanStack Query useMutation hook
- **AND** optimistically remove notification from UI
- **AND** invalidate queries on success
- **AND** revert optimistic update on failure

#### Scenario: Dismiss all notifications in group
- **WHEN** user clicks "Dismiss all" button on notification group
- **THEN** the UI SHALL call DELETE /api/notifications/:id for each notification in group
- **AND** use Promise.all to batch requests
- **AND** optimistically remove entire group from UI
- **AND** invalidate queries on success
- **AND** revert optimistic update on failure

#### Scenario: Show loading state during mutation
- **WHEN** a mark as read or dismiss mutation is in flight
- **THEN** the UI SHALL disable the button
- **AND** show loading spinner on button
- **AND** prevent duplicate clicks

#### Scenario: Show error toast on mutation failure
- **WHEN** a mark as read or dismiss mutation fails
- **THEN** the UI SHALL display error toast message
- **AND** describe the error (e.g., "Failed to mark as read")
- **AND** auto-dismiss toast after 5 seconds
- **AND** allow manual dismissal of toast

### Requirement: Notification Card Styling

The UI SHALL style notification components to match Scout's warm dark theme.

#### Scenario: Card styling matches theme
- **WHEN** rendering notification cards
- **THEN** the UI SHALL use Material-UI Card component
- **AND** apply dark background (#1a1a2e or similar from theme)
- **AND** use indigo/purple accents (#4F46E5, #8B5CF6)
- **AND** match border radius and shadows from other cards (MediaCard, ScheduleWidget)

#### Scenario: Poster image display
- **WHEN** rendering notification group with poster_url
- **THEN** the UI SHALL display poster image in card header
- **AND** use 2:3 aspect ratio (same as MediaCard)
- **AND** set max-width to 80px for compact display
- **AND** apply border radius to poster

#### Scenario: Typography hierarchy
- **WHEN** rendering notification card
- **THEN** the UI SHALL use Material-UI Typography components
- **AND** display media title with variant="h6" in white
- **AND** display stage/status with variant="body2" in secondary color
- **AND** display episode numbers with variant="caption"
- **AND** use Inter font family from theme

#### Scenario: Color coding for notification stages
- **WHEN** displaying notification stage indicator
- **THEN** the UI SHALL apply color based on stage:
- **AND** blue (#4F46E5) for 'scheduled'
- **AND** yellow (#FFA500) for 'searching'
- **AND** purple (#8B5CF6) for 'downloading'
- **AND** green (#10B981) for 'completed'
- **AND** red (#EF4444) for 'failed'

#### Scenario: Hover effects
- **WHEN** user hovers over notification card
- **THEN** the UI SHALL apply hover effect
- **AND** lighten background slightly
- **AND** show dismiss button if not already visible
- **AND** animate transition smoothly

### Requirement: Accessibility

The UI SHALL make notifications accessible via keyboard and screen readers.

#### Scenario: Keyboard navigation
- **WHEN** NotificationDropdown is open
- **THEN** the UI SHALL support TAB key to navigate between notifications
- **AND** support ENTER key to mark as read
- **AND** support DELETE key to dismiss
- **AND** support ESC key to close dropdown
- **AND** trap focus within dropdown (don't tab to page elements behind)

#### Scenario: Screen reader support
- **WHEN** rendering notifications for screen readers
- **THEN** the UI SHALL use semantic HTML (header, article, button)
- **AND** add aria-label to bell icon ("Notifications, X unread")
- **AND** add aria-label to dismiss buttons ("Dismiss notification")
- **AND** add aria-live region for dynamic updates
- **AND** announce when notifications are marked as read or dismissed

#### Scenario: Focus management
- **WHEN** NotificationDropdown opens
- **THEN** the UI SHALL move focus to dropdown header
- **AND** restore focus to bell icon when dropdown closes
- **AND** maintain focus visibility with outline styles

### Requirement: Responsive Behavior

The UI SHALL adapt notification display for different screen sizes (though desktop-focused).

#### Scenario: Desktop display (1920x1080)
- **WHEN** rendering on desktop screen (>1280px width)
- **THEN** the UI SHALL set dropdown max-width to 600px
- **AND** center dropdown horizontally
- **AND** display full poster images and details

#### Scenario: Tablet display (768-1280px)
- **WHEN** rendering on tablet screen
- **THEN** the UI SHALL set dropdown max-width to 500px
- **AND** reduce poster size to 60px
- **AND** maintain all functionality

#### Scenario: Mobile display (<768px)
- **WHEN** rendering on mobile screen
- **THEN** the UI SHALL set dropdown width to 100vw
- **AND** reduce poster size to 50px
- **AND** stack elements vertically for compact display
- **AND** maintain touch-friendly button sizes (min 44px)

## Related Capabilities

This capability relates to:
- **webserver-downloads**: Consumes notification API endpoints
- **torrenter-downloads**: Displays notification updates from torrenter

## Non-Functional Requirements

- Notification dropdown SHALL render within 200ms after bell click
- Notification fetch SHALL complete within 1 second (network dependent)
- UI animations SHALL be smooth (60 FPS)
- Notification badge SHALL update within 5 seconds of status change (due to 30s polling)
- Dropdown SHALL support 100+ notifications without performance degradation
- All interactive elements SHALL have min touch target size of 44x44px

## Design Patterns

The ui-notifications capability SHALL follow these patterns:

- **Material-UI v7 components**: Use MUI components exclusively
- **TanStack Query**: Use for all API data fetching and caching
- **Optimistic updates**: Update UI immediately, rollback on error
- **Component composition**: NotificationDropdown composed of smaller components (NotificationGroup, NotificationCard, etc.)
- **Hooks**: Extract reusable logic into custom hooks (useNotifications, useUnreadCount, useMarkAsRead, etc.)
- **TypeScript**: Full type safety with interfaces for notification data
- **Context-free**: No global state, all state local to components or via TanStack Query cache
