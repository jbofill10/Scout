## ADDED Requirements

### Requirement: Popular Shows API Endpoint

The webserver service SHALL provide a REST API endpoint for fetching popular TV shows from TVDB via the tvdb-proxy service.

#### Scenario: Fetch popular shows without genre

- **WHEN** a GET request is made to `/api/popular/shows`
- **THEN** the service SHALL proxy the request to tvdb-proxy `/series/popular`
- **AND** forward the response to the client
- **AND** return HTTP 200 with popular shows array

#### Scenario: Fetch popular shows with genre filter

- **WHEN** a GET request is made to `/api/popular/shows?genre={id}`
- **THEN** the service SHALL proxy the request to tvdb-proxy `/series/popular?genre={id}`
- **AND** include the genre query parameter in the tvdb-proxy request
- **AND** return HTTP 200 with genre-filtered shows array

#### Scenario: Handle tvdb-proxy errors

- **WHEN** tvdb-proxy returns an error for popular shows request
- **THEN** the service SHALL return HTTP 502 with error message
- **AND** log the error with OpenTelemetry context
- **AND** include details for debugging in logs

#### Scenario: Propagate context for observability

- **WHEN** processing a popular shows request
- **THEN** the service SHALL extract context from the incoming request
- **AND** use context.Context for the tvdb-proxy HTTP client call
- **AND** ensure trace propagation to tvdb-proxy service

### Requirement: Popular Movies API Endpoint

The webserver service SHALL provide a REST API endpoint for fetching popular movies from TVDB via the tvdb-proxy service.

#### Scenario: Fetch popular movies without genre

- **WHEN** a GET request is made to `/api/popular/movies`
- **THEN** the service SHALL proxy the request to tvdb-proxy `/movies/popular`
- **AND** forward the response to the client
- **AND** return HTTP 200 with popular movies array

#### Scenario: Fetch popular movies with genre filter

- **WHEN** a GET request is made to `/api/popular/movies?genre={id}`
- **THEN** the service SHALL proxy the request to tvdb-proxy `/movies/popular?genre={id}`
- **AND** include the genre query parameter in the tvdb-proxy request
- **AND** return HTTP 200 with genre-filtered movies array

#### Scenario: Handle tvdb-proxy errors

- **WHEN** tvdb-proxy returns an error for popular movies request
- **THEN** the service SHALL return HTTP 502 with error message
- **AND** log the error with OpenTelemetry context
- **AND** include details for debugging in logs

#### Scenario: Propagate context for observability

- **WHEN** processing a popular movies request
- **THEN** the service SHALL extract context from the incoming request
- **AND** use context.Context for the tvdb-proxy HTTP client call
- **AND** ensure trace propagation to tvdb-proxy service

### Requirement: Genres API Endpoint

The webserver service SHALL provide a REST API endpoint for fetching the list of available genres from TVDB.

#### Scenario: Fetch genre list

- **WHEN** a GET request is made to `/api/genres`
- **THEN** the service SHALL proxy the request to tvdb-proxy `/genres`
- **AND** forward the response to the client
- **AND** return HTTP 200 with genres array containing {id, name, slug}

#### Scenario: Handle tvdb-proxy errors

- **WHEN** tvdb-proxy returns an error for genres request
- **THEN** the service SHALL return HTTP 502 with error message
- **AND** log the error with OpenTelemetry context

#### Scenario: Propagate context for observability

- **WHEN** processing a genres request
- **THEN** the service SHALL extract context from the incoming request
- **AND** use context.Context for the tvdb-proxy HTTP client call

### Requirement: Weekly Schedule API Endpoint

The webserver service SHALL provide a REST API endpoint for fetching scheduled downloads for the next 7 days.

#### Scenario: Fetch weekly schedule

- **WHEN** a GET request is made to `/api/schedule/weekly`
- **THEN** the service SHALL query the ScheduledDownloads table
- **AND** filter for records with status="pending"
- **AND** filter for records with release_time between now and 7 days from now
- **AND** return HTTP 200 with schedule array

#### Scenario: Return schedule item format

- **WHEN** scheduled downloads are found
- **THEN** each item SHALL include: id, title, season, episode, releaseTime, posterUrl, isAnime
- **AND** items SHALL be sorted by releaseTime in ascending order
- **AND** response SHALL be formatted as JSON array

#### Scenario: Handle empty schedule

- **WHEN** no downloads are scheduled within the next 7 days
- **THEN** the service SHALL return HTTP 200 with empty array
- **AND** log the empty result for monitoring

#### Scenario: Handle database errors

- **WHEN** querying the ScheduledDownloads table fails
- **THEN** the service SHALL return HTTP 500 with error message
- **AND** log the error with OpenTelemetry context
- **AND** include database error details in logs

#### Scenario: Propagate context for observability

- **WHEN** processing a weekly schedule request
- **THEN** the service SHALL extract context from the incoming request
- **AND** pass context.Context to repository methods
- **AND** ensure trace spans are created for database queries
