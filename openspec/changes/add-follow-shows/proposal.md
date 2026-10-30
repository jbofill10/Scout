# Change: Follow a show so new episodes are requested automatically

## Why
A request only covers the episodes TVDB listed at the time. When a new season is announced nothing happens until the show is requested again, so an ongoing show has to be babysat.

## What Changes
- A `FollowedShows` table and `GET/POST/DELETE /follows` endpoints on the webserver. Following stores a snapshot of the show's media and, optionally, requests the episodes that are already missing.
- A follow check that runs on start, on a timer (`FOLLOW_CHECK_INTERVAL`, default 6h) and on demand (`POST /follows/check`). For each followed show it fetches the current episode list from TVDB, drops episodes that are in Plex, already tracked by a scheduled row or a notification, specials, and (unless missing episodes were included) episodes that aired before the follow began, then hands what remains to the existing show request path, so aired episodes are searched for and future ones scheduled exactly as a manual request would.
- UI: a Follow button in the show detail dialog (with a choice to also grab missing episodes) and a Following tab on the Library page with last check, episodes added, Unfollow and Check now.

## Impact
- Affected specs: new `webserver-follows` capability
- Affected code: migration `010_add_followed_shows.sql`; webserver `repository/follow_repository.go`, `interactors/follow_interactor.go`, `handlers/follow_handler.go`, notification repository (`ExistingTvdbIDs`), scheduler repository (`TrackedItemKeys`), main wiring; UI `hooks/useFollows.ts`, `components/ui/FollowButton.tsx`, `pages/Library.tsx`, `MediaStatusDialog.tsx`
