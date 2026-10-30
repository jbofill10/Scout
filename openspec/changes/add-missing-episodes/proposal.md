# Change: Show missing episodes and fill them in one click

## Why
The Library page already knows how many episodes a show is missing (TVDB count minus Plex count) but shows only a number. Getting the gaps filled means opening each show, working out which episodes are absent, and requesting them by hand.

## What Changes
- Torrenter `GET /library/missing`: every library show with aired, non-special episodes that are not in Plex, with the episode list, in one query.
- Webserver `GET /library/missing` relays that and marks each episode as already requested when a scheduled row or a notification exists for it; `POST /library/shows/:tvdbId/fill-missing` requests the missing episodes that are not already requested through the normal show request path.
- UI: a Missing tab on the Library page listing shows with gaps, their episodes (requested ones marked), and a Download missing button per show.

## Impact
- Affected specs: new `library-missing` capability
- Affected code: torrenter repository + library handler/interactor; webserver `interactors/missing_interactor.go`, `handlers/library_handler.go`, torrenter client; UI `hooks/useMissing.ts`, `pages/Library.tsx`
