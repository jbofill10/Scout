## MODIFIED Requirements

### Requirement: Popular Series Endpoint

The tvdb-proxy service SHALL provide `GET /series/popular` returning series ranked by current popularity: Trakt's most-watched or trending shows when a Trakt client id is configured, otherwise TVDB's lifetime score weighted towards recently started and currently airing series. Results SHALL be TVDB records (TVDB id, poster, status) in the same shape as before.

#### Scenario: Ranked by Trakt

- **WHEN** `TRAKT_CLIENT_ID` is set and a request is made to `/series/popular?genre=Drama`
- **THEN** the service SHALL call Trakt `/shows/watched/{period}` (or `/shows/trending` when `TRAKT_LIST=trending`) with `genres=drama`, `extended=full` and the `trakt-api-key` and `trakt-api-version: 2` headers
- **AND** resolve each hit through its TVDB id to `/series/{id}` for the poster, status and score
- **AND** return the hits in Trakt's order with Trakt's title as the display name and TVDB's name as the original name
- **AND** drop hits with no TVDB id or that TVDB does not know

#### Scenario: Ranked from TVDB alone

- **WHEN** no Trakt client id is configured and a request is made to `/series/popular`
- **THEN** the service SHALL gather candidates from `/series/filter` for the current and previous release year and for continuing series, for US and UK releases
- **AND** rank them by score multiplied by 0.5 per year since first airing, with a floor of 0.02 for series that aired or are scheduled to air within 45 days
- **AND** drop series that have not aired yet

#### Scenario: Ordered by current attention

- **WHEN** no Trakt client id is configured and `POPULAR_WIKIPEDIA` is not turned off
- **THEN** the best 50 titles of the TVDB ranking SHALL be ordered by their English Wikipedia pageviews over the last seven days, most viewed first, with unmeasured titles after them in TVDB order
- **AND** film or TV articles from Wikipedia's most-read list of the previous day SHALL be resolved to TVDB records and added when they are released or aired, within the release window and in the row's genre

#### Scenario: Anime row

- **WHEN** a request is made to `/series/popular?genre=Anime` without Trakt
- **THEN** the TVDB candidates SHALL come from Japanese releases first (`country=jpn&lang=jpn`) and US releases second
- **AND** titles whose original language is not English SHALL be returned under their English TVDB translation
- **AND** unless `POPULAR_ANILIST` is turned off, up to half the row SHALL be AniList's trending anime (released or airing, found on TVDB by title), in AniList's order, ahead of the attention-ranked titles

#### Scenario: Ranking basis is visible

- **WHEN** a popular row is returned
- **THEN** each item SHALL carry `popularity` with `source` (`trakt`, `wikipedia`, `anilist` or `tvdb`), `value` and `detail`

#### Scenario: Trakt unavailable

- **WHEN** Trakt is configured and answers with an error, a rate limit or no hits
- **THEN** the service SHALL answer the request from the TVDB ranking and log a warning

#### Scenario: Limit popular series results

- **WHEN** a request is made to `/series/popular?limit={n}`
- **THEN** the service SHALL return at most {n} results
- **AND** enforce a maximum limit of 50 results
- **AND** default to 20 results if limit not specified

#### Scenario: Rows are cached

- **WHEN** the same kind, genre and limit are requested again within an hour
- **THEN** the service SHALL answer from its cache without calling Trakt or TVDB
- **AND** resolved TVDB records SHALL be reused for a day

#### Scenario: Unknown genre

- **WHEN** a request names a genre TVDB does not list
- **THEN** the service SHALL return HTTP 400

#### Scenario: Handle TVDB filter API errors

- **WHEN** every TVDB query behind a TVDB-ranked row fails
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with context for debugging

### Requirement: Popular Movies Endpoint

The tvdb-proxy service SHALL provide `GET /movies/popular` returning movies ranked by current popularity: Trakt's most-watched or trending movies when a Trakt client id is configured, otherwise TVDB's lifetime score normalised within each release year and weighted towards recent releases. Results SHALL be TVDB records with Category="movie".

#### Scenario: Ranked by Trakt

- **WHEN** `TRAKT_CLIENT_ID` is set and a request is made to `/movies/popular?genre=Sci-Fi`
- **THEN** the service SHALL call Trakt `/movies/watched/{period}` (or `/movies/trending`) with `genres=science-fiction`
- **AND** resolve each hit through its IMDb id with TVDB `/search/remoteid/{imdb}` to the TVDB movie record
- **AND** drop hits with no IMDb id or no TVDB movie

#### Scenario: Ranked from TVDB alone

- **WHEN** no Trakt client id is configured and a request is made to `/movies/popular`
- **THEN** the service SHALL gather candidates from `/movies/filter` for the current and previous release year, for US and UK releases
- **AND** rank each movie by its score relative to the best-scored movie of its release year, multiplied by 0.5 per year of age
- **AND** drop movies whose TVDB status is not Released or whose release year is older than the previous year

#### Scenario: Ordered by current attention

- **WHEN** no Trakt client id is configured and `POPULAR_WIKIPEDIA` is not turned off
- **THEN** the best 50 movies of the TVDB ranking SHALL be ordered by their English Wikipedia pageviews over the last seven days
- **AND** films from Wikipedia's most-read list SHALL be added when TVDB knows them, they are released and from the current or previous year

#### Scenario: Anime row

- **WHEN** a request is made to `/movies/popular?genre=Anime` without Trakt
- **THEN** the TVDB candidates SHALL come from Japanese releases first and US releases second, with English names for non-English titles

#### Scenario: Limit popular movies results

- **WHEN** a request is made to `/movies/popular?limit={n}`
- **THEN** the service SHALL return at most {n} results
- **AND** enforce a maximum limit of 50 results
- **AND** default to 20 results if limit not specified

#### Scenario: Handle TVDB filter API errors

- **WHEN** every TVDB query behind a TVDB-ranked row fails
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with context for debugging
