# Change: Rank the popular rows by what people are watching now

## Why
The Home, Shows and Movies rows come from TVDB's `/series/filter` and `/movies/filter` sorted by `score`, and TVDB's score is a lifetime total. Every movie row opens with Titanic (1997), Avengers: Endgame and Frozen; the show rows are Stranger Things, Game of Thrones and Grey's Anatomy; and because the filter forces `country=usa`, the Anime rows hold Western animation (RWBY, Star Wars: Visions) instead of anime. Nothing in them says what is popular this week.

## What Changes
- tvdb-proxy gets a `popular` package: one cached service in front of two ranking sources, serving `/series/popular` and `/movies/popular` with the same response shape as before, so the webserver, its cache and the UI need no change.
- **Trakt source** (when `TRAKT_CLIENT_ID` is set): Trakt's most-watched list for the period (`TRAKT_PERIOD`, default `weekly`) or its trending list (`TRAKT_LIST=trending`), filtered by the row's genre. Shows are resolved to TVDB through the TVDB id Trakt carries, movies through their IMDb id and TVDB's remote-id lookup. Trakt's English title is the display name; TVDB supplies id, poster, status and score.
- **TVDB source** (no client id, or whenever Trakt fails or returns nothing): candidates from the last release years plus every continuing series, for US and UK releases (Japan first for Anime), ranked by TVDB score weighted towards new and currently airing titles. Series that have not aired and movies that are not released are dropped; non-English titles get their English name.
- **Wikipedia attention** (on by default, `POPULAR_WIKIPEDIA=false` turns it off): the TVDB ranking's best 50 titles are ordered by English Wikipedia pageviews over the last seven days, and the film and TV articles in yesterday's most-read list are resolved to TVDB and added, so the rows follow what people are looking up this week without any API key.
- **AniList** (on by default, `POPULAR_ANILIST=false` turns it off): the two Anime rows lead with AniList's keyless trending list, since Wikipedia barely measures anime.
- Every item carries `popularity` (`source`, `value`, `detail`) saying what ranked it.
- Rows are cached for an hour in the proxy and resolved TVDB records for a day, so the webserver's basic and enriched refreshes cost one ranking; the proxy warms the UI's rows at startup.
- New environment: `TRAKT_CLIENT_ID`, `TRAKT_LIST`, `TRAKT_PERIOD`, `POPULAR_WIKIPEDIA`, `POPULAR_ANILIST` (and `TRAKT_HOST`, `WIKIPEDIA_API_HOST`, `WIKIMEDIA_REST_HOST`, `ANILIST_HOST` for tests), in `.env.template` and the compose template.

## Impact
- Affected specs: `tvdb-proxy-api`
- Affected code: `backend/internal/tvdbproxy/popular` (new), `pkg/media` (`Popularity`), `handlers/popular_handler.go`, `config/config.go`, `cmd/tvdb-proxy/main.go`; webserver `cache/popular_refresher.go` (first refresh no longer blocks startup); `.env.template`, `docker-compose.yml.template`, `README.md`, `DEPLOYMENT.md`, `CLAUDE.md`
