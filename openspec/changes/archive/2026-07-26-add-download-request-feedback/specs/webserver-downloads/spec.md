## MODIFIED Requirements

### Requirement: Movie Download API Endpoint

The webserver service SHALL provide a REST API endpoint for initiating movie downloads that returns a summary of what was accepted without waiting for the torrent search.

#### Scenario: Download movie request

- **WHEN** a POST request is made to `/movies` with a movie media object
- **THEN** the service SHALL accept the request with `tvdb.Media` in JSON body
- **AND** validate that `media.Category == "movie"`
- **AND** call the download interactor to process the movie
- **AND** return HTTP 202 with a summary body containing `media_title`, `category`, `queued_now`, `scheduled`, `skipped`, optional `next_release` and `trace_id`

#### Scenario: Released movie reported as queued

- **WHEN** a released movie is requested
- **THEN** the service SHALL create its notification with status `searching`
- **AND** return `queued_now: 1` before the torrenter responds

#### Scenario: Unreleased movie reported as scheduled

- **WHEN** a movie whose `firstAired` date is in the future is requested
- **THEN** the service SHALL return `scheduled: 1`
- **AND** set `next_release` to the release date

#### Scenario: Already-scheduled movie is not an error

- **WHEN** a movie that already has a scheduled row is requested again
- **THEN** the service SHALL return HTTP 202 with `skipped: 1`
- **AND** NOT return an error status

#### Scenario: Invalid media type rejected

- **WHEN** a POST request is made to `/movies` with non-movie media
- **THEN** the service SHALL validate `media.Category == "movie"`
- **AND** return HTTP 400 with error message if category is not "movie"

#### Scenario: Missing required fields

- **WHEN** a POST request is made to `/movies` with incomplete media object
- **THEN** the service SHALL return HTTP 400 with validation error
- **AND** specify which required fields are missing

## ADDED Requirements

### Requirement: Show Download API Endpoint Summary

The webserver service SHALL report per-episode disposition when a show download is requested, and SHALL NOT block the request on the torrent search.

#### Scenario: Episodes classified before responding

- **WHEN** a POST request is made to `/shows` with a series media object
- **THEN** the service SHALL count aired episodes as `queued_now`
- **AND** count future episodes successfully scheduled as `scheduled`
- **AND** count specials, unparseable air dates, duplicates and scheduling errors as `skipped`
- **AND** return HTTP 202 with those counts

#### Scenario: Notifications exist before the response

- **WHEN** the response is returned
- **THEN** every queued episode SHALL already have a notification with status `searching`
- **AND** every scheduled episode SHALL already have a notification with status `scheduled`

#### Scenario: Torrent search runs after the response

- **WHEN** aired episodes are queued
- **THEN** the service SHALL dispatch them to the torrenter on a context detached from the request
- **AND** the request SHALL complete without waiting for that dispatch
- **AND** per-episode failures SHALL still be reconciled into retry rows or failed notifications

### Requirement: Download Activity Endpoint

The webserver service SHALL provide a REST API endpoint exposing in-flight and recently finished downloads with their stage and retry state.

#### Scenario: Return in-flight and finished work

- **WHEN** a GET request is made to `/activity`
- **THEN** the service SHALL return `active` containing notifications with status `scheduled`, `searching` or `downloading`
- **AND** return `recent` containing notifications with status `completed` or `failed`
- **AND** return `counts` with the number of items per stage
- **AND** order both lists most-recently-updated first

#### Scenario: Enrich items with retry state

- **WHEN** an item has a pending or queued scheduled download row
- **THEN** the service SHALL include `attempts`, `next_attempt_at`, `release_time`, `schedule_status` and the last failure code and reason
- **AND** match the row to the notification by episode id for shows and media id for movies

#### Scenario: Degrade when scheduling state is unavailable

- **WHEN** the scheduled downloads lookup fails
- **THEN** the service SHALL log the error
- **AND** still return the notification-derived snapshot without retry details

#### Scenario: Limit results

- **WHEN** a GET request is made to `/activity?limit={n}`
- **THEN** the service SHALL return at most {n} notifications
- **AND** default to 200 when the parameter is absent or invalid
