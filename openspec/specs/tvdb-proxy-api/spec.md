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

The tvdb-proxy service SHALL continue to support the universal search endpoint for both movies and series.

#### Scenario: Search for movies

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=movie`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=movie`
- **AND** return movie search results with Category="movie"

#### Scenario: Search for series

- **WHEN** a request is made to `/series?mediaName={query}&mediaType=series`
- **THEN** the service SHALL call TVDB API `/search?query={query}&type=series`
- **AND** return series search results with Category="series"

