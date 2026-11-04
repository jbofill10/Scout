# Add Horror and Thriller rows to the Shows and Movies pages

## Why

The Shows and Movies pages suggest popular titles in six genres (Action, Comedy, Drama, Sci-Fi, Anime,
Documentary). Horror and Thriller are two of the most-browsed genres and TVDB carries both (ids 6 and 24), so
titles in them never surface unless the user searches for them by name.

## What Changes

- The curated genre list gains Horror and Thriller, giving eight rows per page.
- The list lives in one place in the UI (`ui/src/utils/curatedGenres.ts`); `GenreContext`, `Shows.tsx` and
  `Movies.tsx` read it instead of each holding a copy. Rows now render in that list's order (Action, Comedy,
  Drama, Sci-Fi, Horror, Thriller, Anime, Documentary) instead of TVDB's genre-id order.
- The tvdb-proxy warm list (`popular.DefaultRows`) and the webserver's `PopularRefresher` categories build the
  two new rows at startup like the others, so the pages are served from cache.

## Impact

- UI: `utils/curatedGenres.ts` (new), `contexts/GenreContext.tsx`, `pages/Shows.tsx`, `pages/Movies.tsx`.
- Backend: `internal/tvdbproxy/popular/popular.go`, `internal/webserver/cache/popular_refresher.go`.
- Four more popular rows (two kinds × two genres) are ranked at startup and refreshed hourly: more TVDB,
  Trakt and Wikipedia traffic in proportion.
- Spec: `ui-pages` Shows and Movies page requirements.

## Approval

Requested as a `/goal` on 2026-11-03 ("add horror and thriller sections to the suggestion pages");
implemented on that basis.
