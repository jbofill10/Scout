# Change: Send download outcomes to webhooks, ntfy and Discord

## Why
The only place a finished or failed download shows up is the bell inside the app. Nothing reaches a phone, a chat or Home Assistant, so a failure can sit unnoticed for days.

## What Changes
- A `NotificationTargets` table: name, kind (`webhook`, `ntfy`, `discord`), url, optional token, the events it wants (`completed`, `failed`, `cancelled`), enabled flag, last result.
- A dispatcher in the webserver that polls the Notifications table for terminal status transitions not yet delivered (`notified_status` column, backfilled so history is not replayed) and posts them to every enabled target that wants the event. Home Assistant is a `webhook` target (its webhook URL accepts a JSON POST).
- Endpoints `GET/POST /notification-targets`, `PUT/DELETE /notification-targets/:id`, `POST /notification-targets/:id/test`, and a Notifications section on the Settings page to manage targets and send a test.

## Impact
- Affected specs: new `outbound-notifications` capability
- Affected code: migration `011_add_notification_targets.sql`; webserver `internal/webserver/outbound` (dispatcher, senders), `repository/notification.go` (pending deliveries, mark notified), `repository/target_repository.go`, `handlers/notification_target_handler.go`, main wiring; UI `hooks/useNotificationTargets.ts`, `pages/Settings.tsx`
