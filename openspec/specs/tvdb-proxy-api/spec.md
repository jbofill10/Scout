# tvdb-proxy-api Specification

## Purpose
TBD - created by archiving change add-movie-downloading. Update Purpose after archive.
## Requirements
### Requirement: Movie Extended Information Endpoint

The tvdb-proxy service SHALL support fetching extended information for movies by conditionally routing to TVDB's movie-specific endpoints.

#### Scenario: Fetch movie extended info

- **WHEN** a request is made to `/series/{id}/extended?mediaType=movie`
- **THEN** the service SHALL call TVDB API `/movies/{id}/extended`
- **AND** return genres, slug, and aliases for the movie

#### Scenario: Fetch series extended info

- **WHEN** a request is made to `/series/{id}/extended?mediaType=series`
- **THEN** the service SHALL call TVDB API `/series/{id}/extended`
- **AND** return genres, slug, and aliases for the series

#### Scenario: Default to series when mediaType omitted

- **WHEN** a request is made to `/series/{id}/extended` without mediaType parameter
- **THEN** the service SHALL default to series behavior for backwards compatibility
- **AND** call TVDB API `/series/{id}/extended`

### Requirement: Movie Translation Endpoint

The tvdb-proxy service SHALL support fetching translations and aliases for movies by conditionally routing to TVDB's movie-specific translation endpoints.

#### Scenario: Fetch movie translations

- **WHEN** `fetchTranslations()` is called with mediaType "movie"
- **THEN** the service SHALL call TVDB API `/movies/{id}/translations/{language}`
- **AND** extract aliases from the translation response

#### Scenario: Fetch series translations

- **WHEN** `fetchTranslations()` is called with mediaType "series"
- **THEN** the service SHALL call TVDB API `/series/{id}/translations/{language}`
- **AND** extract aliases from the translation response

### Requirement: Movie Metadata Enrichment

The tvdb-proxy service SHALL skip episode metadata fetching for movies and return empty episode arrays.

#### Scenario: Movie search returns no episodes

- **WHEN** a movie is returned from TVDB search results
- **THEN** the service SHALL NOT call `querySeriesMetadata()` for the movie
- **AND** SHALL set `Metadata.Episodes` to empty array
- **AND** return the movie data with empty episode metadata

#### Scenario: Series search returns episodes

- **WHEN** a series is returned from TVDB search results
- **THEN** the service SHALL call `querySeriesMetadata()` for the series
- **AND** return the series data with populated episode metadata

### Requirement: Anime Detection for Movies

The tvdb-proxy service SHALL detect anime classification for movies by checking both "Anime" and "Animation" genre tags.

#### Scenario: Movie with Anime genre

- **WHEN** extended information is fetched for a movie
- **AND** the TVDB genres include "Anime"
- **THEN** the service SHALL set `media.Anime = true`
- **AND** fetch both English and Japanese aliases

#### Scenario: Movie with Animation genre

- **WHEN** extended information is fetched for a movie
- **AND** the TVDB genres include "Animation" but not "Anime"
- **THEN** the service SHALL set `media.Anime = true`
- **AND** fetch both English and Japanese aliases

#### Scenario: Movie with neither Anime nor Animation genre

- **WHEN** extended information is fetched for a movie
- **AND** the TVDB genres do not include "Anime" or "Animation"
- **THEN** the service SHALL set `media.Anime = false`
- **AND** fetch only English aliases

#### Scenario: Series anime detection unchanged

- **WHEN** extended information is fetched for a series
- **THEN** the service SHALL only check for "Anime" genre (existing behavior)
- **AND** NOT check for "Animation" genre

### Requirement: Universal Search Endpoint

The tvdb-proxy service SHALL answer `GET /series` with ranked media for a structured query: a title (`mediaName`) and/or filters (`year`, `director`, `person`, `personRole`, `company`, `country`, `language`, `imdb`, `altName`) as defined by `pkg/search`, for `mediaType` `movie` or `series`.

#### Scenario: Search for movies

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=movie`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=movie` with the query URL-encoded
- **AND** return movie search results with Category="movie"

#### Scenario: Search for series

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=series`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=series` with the query URL-encoded
- **AND** return series search results with Category="series"

#### Scenario: Native TVDB filters are passed through

- **WHEN** the request carries `year`, `country` or `language`
- **THEN** the service SHALL pass each as the same-named TVDB `/search` parameter
- **AND** `director` SHALL be passed to TVDB for movies only
- **AND** `company` SHALL be passed as TVDB's `network` parameter for series only

#### Scenario: Studio filter is applied locally

- **WHEN** the request is for movies and carries `company`
- **THEN** the service SHALL keep only hits whose `studios` include a name containing the value, compared case-insensitively
- **AND** SHALL NOT send TVDB's `company` parameter

#### Scenario: Exact title matches lead

- **WHEN** a hit's name, original name or an alias equals the title after normalisation (case, punctuation and spacing ignored)
- **THEN** it SHALL be ranked before hits that merely contain the title

#### Scenario: Inferred year keeps a verbatim search

- **WHEN** the request carries `altName`
- **THEN** the service SHALL additionally search TVDB for `altName` without the year filter
- **AND** append those hits after the year-filtered hits, without duplicates

#### Scenario: Series hits include episodes

- **WHEN** any result, from any source, is a series
- **THEN** the service SHALL fetch its episode list and score as before
- **AND** a failed episode fetch SHALL leave that series in the results with an empty episode list

#### Scenario: Filters without anything to search

- **WHEN** the request has no `mediaName`, `person`, `director` or `imdb`
- **THEN** the service SHALL answer 400

#### Scenario: Result caps

- **WHEN** the merged results exceed 60
- **THEN** only the first 60 SHALL be returned

### Requirement: Popular Series Endpoint

The tvdb-proxy service SHALL provide an endpoint to fetch popular TV series using TVDB's filter API with score-based sorting.

#### Scenario: Fetch popular series without genre filter

- **WHEN** a request is made to `/series/popular` without genre parameter
- **THEN** the service SHALL call TVDB API `/series/filter?country=usa&lang=eng&sort=score&sortType=desc`
- **AND** return up to 20 popular series (default limit)
- **AND** enrich results with episode metadata (same as search endpoint)

#### Scenario: Fetch popular series with genre filter

- **WHEN** a request is made to `/series/popular?genre={id}`
- **THEN** the service SHALL call TVDB API `/series/filter` with genre parameter
- **AND** include `country=usa&lang=eng&sort=score&sortType=desc&genre={id}`
- **AND** return series filtered by the specified genre
- **AND** enrich results with episode metadata

#### Scenario: Limit popular series results

- **WHEN** a request is made to `/series/popular?limit={n}`
- **THEN** the service SHALL return at most {n} results
- **AND** enforce a maximum limit of 50 results
- **AND** default to 20 results if limit not specified

#### Scenario: Handle TVDB filter API errors

- **WHEN** TVDB API `/series/filter` returns an error
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with context for debugging
- **AND** include TVDB API response details in logs

### Requirement: Popular Movies Endpoint

The tvdb-proxy service SHALL provide an endpoint to fetch popular movies using TVDB's filter API with score-based sorting.

#### Scenario: Fetch popular movies without genre filter

- **WHEN** a request is made to `/movies/popular` without genre parameter
- **THEN** the service SHALL call TVDB API `/movies/filter?country=usa&lang=eng&sort=score&sortType=desc`
- **AND** return up to 20 popular movies (default limit)
- **AND** set Category="movie" for each result

#### Scenario: Fetch popular movies with genre filter

- **WHEN** a request is made to `/movies/popular?genre={id}`
- **THEN** the service SHALL call TVDB API `/movies/filter` with genre parameter
- **AND** include `country=usa&lang=eng&sort=score&sortType=desc&genre={id}`
- **AND** return movies filtered by the specified genre

#### Scenario: Limit popular movies results

- **WHEN** a request is made to `/movies/popular?limit={n}`
- **THEN** the service SHALL return at most {n} results
- **AND** enforce a maximum limit of 50 results
- **AND** default to 20 results if limit not specified

#### Scenario: Handle TVDB filter API errors

- **WHEN** TVDB API `/movies/filter` returns an error
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with context for debugging
- **AND** include TVDB API response details in logs

### Requirement: Genre List Endpoint

The tvdb-proxy service SHALL provide an endpoint to fetch the complete list of genres from TVDB.

#### Scenario: Fetch all genres

- **WHEN** a request is made to `/genres`
- **THEN** the service SHALL call TVDB API `/genres`
- **AND** return a JSON array of genre objects with {id, name, slug}
- **AND** cache the response for 24 hours to reduce API calls

#### Scenario: Handle TVDB genres API errors

- **WHEN** TVDB API `/genres` returns an error
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with context for debugging
- **AND** attempt to return cached genres if available

#### Scenario: Return cached genres

- **WHEN** genres have been fetched within the last 24 hours
- **AND** a request is made to `/genres`
- **THEN** the service SHALL return cached genres without calling TVDB API
- **AND** reduce load on TVDB API and improve response time

### Requirement: Search Hit Mapping

Search hits SHALL carry the extra fields TVDB's index provides.

#### Scenario: Movie hit fields

- **WHEN** a movie hit is mapped to Media
- **THEN** `director`, `studios`, `genres`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: Series hit fields

- **WHEN** a series hit is mapped to Media
- **THEN** `network`, `aliases`, `originalCountry` and `originalLanguage` SHALL be populated from the hit when present

#### Scenario: Missing English translation

- **WHEN** a hit has no English translated name
- **THEN** `mediaName` SHALL fall back to the hit's original name

### Requirement: People Credits Search

The tvdb-proxy service SHALL answer person queries from TVDB's people index and the person's credits.

#### Scenario: Explicit actor filter

- **WHEN** the request carries `person` with `personRole=actor`
- **THEN** the service SHALL search TVDB `/search?query={person}&type=person`, take the top hit (and further hits with the same normalised name, up to 3 people), fetch `/people/{id}/extended` for each
- **AND** return only their credits of the requested media type with peopleType "Actor" or "Guest Star", with no title search
- **AND** each result SHALL carry `matchedVia` as "{person name} · {peopleType}"

#### Scenario: Explicit person filter

- **WHEN** the request carries `person` without `personRole`
- **THEN** credits of every role SHALL be returned

#### Scenario: Director filter where TVDB cannot filter

- **WHEN** the request carries `director` and the media type is series, or there is no `mediaName`
- **THEN** the service SHALL answer from the person's credits with peopleType "Director"

#### Scenario: Title and year narrow credits

- **WHEN** a person request also carries `mediaName` or `year`
- **THEN** only credits whose title contains the name (normalised) and whose year equals the year SHALL be kept

#### Scenario: Implicit person match

- **WHEN** a title search is made and a TVDB person's name equals the title after normalisation
- **THEN** that person's "Actor" and "Director" credits of the media type SHALL be appended after exact title matches and before the remaining title hits
- **AND** people whose name merely contains the title SHALL NOT be expanded

#### Scenario: Credit ranking

- **WHEN** a person's credits are returned
- **THEN** they SHALL be ordered by the person's dominant line of work (acting or directing, by credit count) first, main-cast acting credits before supporting ones and guest spots, then the other line of work, then every other role; ties by TVDB score then year descending
- **AND** a title with several roles for the person SHALL appear once, showing the strongest role
- **AND** at most 40 credits per person SHALL be returned

#### Scenario: People lookups are cached and non-fatal

- **WHEN** a people search or extended record was fetched within the last hour
- **THEN** it SHALL be served from memory
- **AND** a failed people lookup during a title search SHALL leave the title results intact

### Requirement: IMDb Id Lookup

The tvdb-proxy service SHALL resolve IMDb title ids.

#### Scenario: IMDb id of the requested type

- **WHEN** the request carries `imdb={ttid}`
- **THEN** the service SHALL call TVDB `/search/remoteid/{ttid}` and return the matched record of the requested media type, with `imdbId` set, and its episodes if a series

#### Scenario: IMDb id of the other type

- **WHEN** the remote-id match is a series but `mediaType=movie`, or the reverse
- **THEN** the service SHALL return an empty list
