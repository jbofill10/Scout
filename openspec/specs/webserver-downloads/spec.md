# webserver-downloads Specification

## Purpose
TBD - created by archiving change add-movie-downloading. Update Purpose after archive.
## Requirements
### Requirement: Movie Download API Endpoint

The webserver service SHALL provide a REST API endpoint for initiating movie downloads.

#### Scenario: Download movie request

- **WHEN** a POST request is made to `/movies` with a movie media object
- **THEN** the service SHALL accept the request with `tvdb.Media` in JSON body
- **AND** validate that `media.Category == "movie"`
- **AND** call the download interactor to process the movie
- **AND** return HTTP 200 with success message

#### Scenario: Invalid media type rejected

- **WHEN** a POST request is made to `/movies` with non-movie media
- **THEN** the service SHALL validate `media.Category == "movie"`
- **AND** return HTTP 400 with error message if category is not "movie"

#### Scenario: Missing required fields

- **WHEN** a POST request is made to `/movies` with incomplete media object
- **THEN** the service SHALL return HTTP 400 with validation error
- **AND** specify which required fields are missing

### Requirement: Movie Download Orchestration

The webserver download interactor SHALL orchestrate the movie download process including anime detection, release date checking, and scheduling.

#### Scenario: Download released movie immediately

- **WHEN** `DownloadMovie()` is called with a movie that has already been released
- **THEN** the interactor SHALL fetch extended info from tvdb-proxy with mediaType="movie"
- **AND** determine anime classification from TVDB genres
- **AND** send the movie to torrenter service immediately via `/download` endpoint
- **AND** return success response

#### Scenario: Schedule unreleased movie

- **WHEN** `DownloadMovie()` is called with a movie that has a future release date
- **THEN** the interactor SHALL parse `media.Metadata.FirstAired` as release date
- **AND** compare release date to current date
- **AND** store movie in ScheduledDownloads table with future release_time
- **AND** return success response indicating movie is scheduled

#### Scenario: Handle missing release date

- **WHEN** `DownloadMovie()` is called with a movie that has empty FirstAired
- **THEN** the interactor SHALL treat the movie as already released
- **AND** send the movie to torrenter service immediately
- **AND** return success response

#### Scenario: Handle invalid release date format

- **WHEN** `DownloadMovie()` is called with unparseable FirstAired date
- **THEN** the interactor SHALL treat the movie as already released
- **AND** send the movie to torrenter service immediately
- **AND** log warning about invalid date format

#### Scenario: Fetch anime status for movie

- **WHEN** processing a movie download
- **THEN** the interactor SHALL call TVDBProxyClient.GetExtendedInfo with mediaType="movie"
- **AND** extract anime classification from extended info response
- **AND** set `media.Anime` flag before sending to torrenter

### Requirement: TVDB Client Media Type Support

The webserver TVDB proxy client SHALL pass media type parameter when fetching extended information.

#### Scenario: Fetch extended info for movie

- **WHEN** `GetExtendedInfo()` is called with mediaType="movie"
- **THEN** the client SHALL include `?mediaType=movie` query parameter in request
- **AND** call tvdb-proxy endpoint `/series/{id}/extended?mediaType=movie`
- **AND** return extended info response

#### Scenario: Fetch extended info for series

- **WHEN** `GetExtendedInfo()` is called with mediaType="series"
- **THEN** the client SHALL include `?mediaType=series` query parameter in request
- **AND** call tvdb-proxy endpoint `/series/{id}/extended?mediaType=series`
- **AND** return extended info response

### Requirement: Movie Scheduling Integration

The webserver SHALL reuse the existing ScheduledDownloads table and scheduler for future movie releases.

#### Scenario: Store movie in schedule

- **WHEN** a movie with future release date is scheduled
- **THEN** the repository SHALL store the complete `tvdb.Media` object as JSONB
- **AND** set status="pending"
- **AND** set release_time to the movie's FirstAired date
- **AND** compute SHA256 content hash for deduplication

#### Scenario: Scheduler picks up due movie

- **WHEN** the scheduler detects a movie with release_time <= today
- **THEN** the scheduler SHALL retrieve the movie from ScheduledDownloads
- **AND** send the movie to torrenter service via `/download` endpoint
- **AND** update the record status to "queued"

#### Scenario: Prevent duplicate movie scheduling

- **WHEN** attempting to schedule a movie that already exists in ScheduledDownloads
- **THEN** the repository SHALL detect duplicate via content hash
- **AND** return `ErrDuplicateScheduled` error
- **AND** the interactor SHALL return error response to client

