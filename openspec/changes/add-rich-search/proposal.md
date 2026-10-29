# Change: Rich Search (years, people, studios, IMDb ids)

## Why

Search only matched title text, and anything else typed into the box made it
worse. Searching for A24's "X" (2022) as `X (2022)` returned wrestling events
and K-pop concerts because the parentheses and year were sent to TVDB as part
of the title (unencoded, too); searching `X` alone buried the film at position
12 of 50 behind "Julia X" and "Malcolm X". There was no way to find a film by
its director or star, a show by its network, or to paste an IMDb link.

TVDB's search index supports more than Scout used: `year`, `director`
(movies), `network` (series), `country` and `language` filters; hits carry
`studios` and `director`; people are searchable (`type=person`) and a person's
extended record lists every credit with the title's name, poster and year;
and `/search/remoteid/{id}` resolves IMDb ids. This change exposes all of it
through the same search box.

## What Changes

- **Query syntax** (`backend/pkg/search`): the text is parsed into a title
  plus filters before anything is sent to TVDB.
  - A year in brackets (`X (2022)`) or trailing the title (`X 2022`) becomes a
    year filter. A bare trailing year also keeps the verbatim title as a
    fallback search so "Class of 1984" still finds the film named that.
  - `key:value` filters, value running to the next filter or the end, quotes
    optional: `year:`, `director:`/`dir:`, `actor:`/`cast:`/`starring:`,
    `person:` (any role), `studio:`/`company:`, `network:`, `country:`,
    `lang:`/`language:`, `imdb:`. Colons inside titles ("Mission: Impossible")
    are left alone because only known keys count.
  - A bare IMDb id or IMDb title URL is recognised as an IMDb lookup.
- **tvdb-proxy `/series`** accepts the structured query (`mediaName`,
  `altName`, `year`, `director`, `person`, `personRole`, `company`, `country`,
  `language`, `imdb`) and answers in ranked tiers: exact title/alias matches,
  then credits of a person whose name is the query, then the rest of the
  title hits, then verbatim-title hits. Native TVDB filters are passed
  through (`year`, `country`, `language`, `director` for movies, `network`
  for series); studios are filtered locally from the hits. `actor:`/`person:`
  (and `director:` where TVDB cannot filter) answer from the person's
  credits, narrowed by any title or year typed alongside. A plain name that
  exactly matches a TVDB person folds their acting and directing credits in
  after exact title matches, ranked by their dominant line of work with
  main-cast roles first. IMDb ids use `/search/remoteid`. People lookups are
  cached for an hour. Every series hit still gets its episode list.
- **Search hits carry more**: `Media` gains `director`, `studios`,
  `network`, `matchedVia` ("Mia Goth · Actor" on results found through a
  person); search hits now also populate genres, country and language, and a
  hit without an English translation falls back to its original name instead
  of showing up blank. The title parameter is URL-encoded properly.
- **webserver `/search/enriched`** returns `{ results, query }`, where
  `query` is the parsed reading of the text (title, filters, inferred flag)
  so the UI can display it. `/search` is unchanged.
- **UI search overlay**: placeholder and a `?` tooltip teach the syntax; the
  empty state offers clickable examples; once results arrive, the parsed
  title and filter chips appear under the box and a chip's × removes that
  filter from the text; cards show year and provenance on hover; empty
  results explain themselves (a filter without a title, an IMDb id of the
  other media type, a person with no credits of that type).

## Impact

- Affected specs: `ui-search` (query syntax, interpretation display, API
  response shape), `tvdb-proxy-api` (search endpoint parameters, people and
  remote-id lookups, hit mapping).
- Affected code: new `pkg/search`; `cmd/tvdb-proxy/search.go` (replaces
  `queryShow`); `pkg/media` types; webserver `TVDBProxyClient.Search`,
  `SearchInteractor`, `EnrichedSearchHandler`; UI `useEnrichedSearch`,
  `SearchDropdown`, `SearchResultsGrid`, `utils/searchQuery.ts`.
- Breaking for API consumers of `/api/search/enriched` (array → object). The
  UI is the only consumer and ships in the same release.
- Extra TVDB traffic per search: one people search (cached per name), plus
  one extended-record fetch per matched person (cached per id), plus one
  verbatim title search when a trailing year was inferred.

## Approval

Requested as a `/goal` on 2026-10-29 ("think of ways to enrichen the search
capabilities ... and then implement it"); implemented on that basis.
