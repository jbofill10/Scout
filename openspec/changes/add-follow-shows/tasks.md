## 1. Storage
- [x] 1.1 Migration `010_add_followed_shows.sql` (+ `setup-postgres.sql`)
- [x] 1.2 `FollowRepository` (upsert, delete, list, get, record check); `SchedulerRepository.TrackedItemKeys`; `NotificationRepository.ExistingTvdbIDs` (+ mocks/fakes)

## 2. Webserver
- [x] 2.1 `FollowInteractor`: Follow (snapshot + first check), Unfollow, List, CheckOne / CheckAll with the exclusion rules, background loop
- [x] 2.2 Handler + routes: `GET /follows`, `POST /follows`, `DELETE /follows/:tvdbId`, `POST /follows/check`, `POST /follows/:tvdbId/check`
- [x] 2.3 Config `FOLLOW_CHECK_INTERVAL`; compose template; `.env.template`
- [x] 2.4 Tests: exclusion rules (specials, pre-follow, tracked, in Plex), include_missing, check bookkeeping, handler codes

## 3. UI
- [x] 3.1 `useFollows` hooks (list, follow, unfollow, check)
- [x] 3.2 `FollowButton` in the show detail dialog with "new episodes only" / "also grab missing"
- [x] 3.3 Following tab on the Library page
- [x] 3.4 Tests: hooks, FollowButton

## 4. Verify
- [x] 4.1 `go vet`, `go test ./...`; `npm run lint`, `npx tsc -b`, `npm test`, `npm run build`
- [ ] 4.2 Deploy webserver + ui; follow a show and confirm the check runs and schedules nothing twice
