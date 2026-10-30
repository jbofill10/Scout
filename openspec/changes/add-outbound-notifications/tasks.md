## 1. Storage
- [x] 1.1 Migration `011_add_notification_targets.sql`: `NotificationTargets` table, `Notifications.notified_status` backfilled for existing terminal rows (+ `setup-postgres.sql`)
- [x] 1.2 Target repository (list/create/update/delete/record result); notification repository `PendingDeliveries` and `MarkNotified`

## 2. Dispatcher
- [x] 2.1 Senders: webhook (JSON, optional bearer token), ntfy (title/tags/priority headers), discord (content)
- [x] 2.2 Poll loop (15s): deliver each pending transition to the targets that want its event, record per-target result, mark notified
- [x] 2.3 Handler + routes incl. `POST /notification-targets/:id/test`
- [x] 2.4 Tests: payloads per kind (httptest), event filter, mark-notified bookkeeping, handler validation

## 3. UI
- [x] 3.1 `useNotificationTargets` hooks; Notifications section on Settings: list, add, enable/disable, test, delete
- [x] 3.2 Tests: hooks

## 4. Verify
- [x] 4.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 4.2 Deploy webserver + ui; add a target and send a test
