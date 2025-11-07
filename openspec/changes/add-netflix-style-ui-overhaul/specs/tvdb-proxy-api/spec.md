## ADDED Requirements

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
