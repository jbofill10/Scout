# Change: Count library coverage against aired episodes

## Why

Every "what you have" number in Scout divided the library by everything TVDB lists. A show mid-run, such as
Dexter: Resurrection with season 1 in Plex and season 2 announced, read "11/20" on its poster, "11 of 20
episodes in your library" in the dialog, and nine "Missing" rows nobody could download. The Library page's
own counts already left unaired episodes out, so the same show read differently depending on where it was
opened.

## What Changes

- The webserver's enriched status (`MediaStatusInfo`) carries `aired`: how many of `total` have aired by
  today (an episode in the library counts as aired whatever its date says; a blank TVDB date is unaired, the
  same rule the download flow applies). `total` is unchanged.
- The torrenter's `GET /library/shows/:tvdbId/metadata-status` adds `upcomingCount`; `hasTvdbData` is true
  when TVDB lists anything, aired or not. `GET /library/shows/:tvdbId` lists every TVDB episode, aired or
  not, so the Library dialog shows the same upcoming episodes the search dialog does.
- The poster pill reads `downloaded/aired` ("11/11", green once everything aired is in) with the full
  sentence in its tooltip. The dialog header reads "11 of 11 aired episodes in your library · 9 upcoming",
  the progress bar measures against aired episodes, season tabs read "Season 2 · 1/1 · 9 upcoming" (or
  "Season 3 · 10 upcoming" before any of it airs), and unaired rows are labelled "Upcoming" (dated) or
  "TBA" (no date) instead of "Missing". The all-downloaded banner adds "9 more are yet to air."
- UI helpers: `utils/airDates.ts` (parse, format, `hasAired`), `utils/episodeCoverage.ts` (`summarizeCoverage`,
  `episodeState`, the pill, sentence and tab labels). `MediaStatusBadge.episodeCount` is now
  `{ downloaded, aired, upcoming }`.

## Impact

- Affected specs: `webserver-api`, `ui-search`
- Affected code: `backend/pkg/media/status.go`, `backend/pkg/library/types.go`,
  `backend/internal/webserver/interactors/{episode_counts,enriched_search_interactor,popular_enriched_interactor}.go`,
  `backend/internal/torrenter/{repository/postgres.go,interactors/library_interactor.go,service/interfaces.go}`
  and the repository mocks, `ui/src/utils/{airDates,episodeCoverage,statusHelpers}.ts`,
  `ui/src/components/ui/{MediaCard,MediaStatusDialog}.tsx`, `ui/src/pages/Library.tsx`, `ui/src/hooks/useLibrary.ts`
- No migration; the new JSON fields are additive.
