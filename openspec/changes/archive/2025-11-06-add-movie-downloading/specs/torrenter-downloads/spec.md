# Capability: Torrenter Downloads

## ADDED Requirements

### Requirement: Movie Download Processing

The torrenter service SHALL accept and process movie download requests through the existing download endpoint.

#### Scenario: Accept movie download request

- **WHEN** a download request is received with `media.Category == "movie"`
- **THEN** the service SHALL accept the request
- **AND** process the movie download
- **AND** NOT return early or skip processing

#### Scenario: Check movie exists in Plex

- **WHEN** processing a movie download request
- **THEN** the service SHALL query the repository with `MovieExistsByTvdbId(media.Id)`
- **AND** skip download if movie already exists in Plex library
- **AND** return success (no error) to indicate duplicate skip

#### Scenario: Movie not in Plex proceeds to search

- **WHEN** movie existence check returns false
- **THEN** the service SHALL proceed to create movie search strategy
- **AND** search for torrents via Prowlarr
- **AND** download the best matching torrent

### Requirement: Movie Search Strategy

The torrenter service SHALL generate appropriate search queries for movies based on title and year.

#### Scenario: Create movie search query

- **WHEN** `createMovieSearchStrategy()` is called with a movie
- **THEN** the service SHALL create query string "{MovieName} {Year}"
- **AND** set Prowlarr category to 2000 (movies)
- **AND** set episode number to 0 to indicate movie (not episode)
- **AND** return array with single SearchStrategy

#### Scenario: Movie search query format

- **WHEN** generating movie search query
- **THEN** the service SHALL use movie name from `media.Name`
- **AND** append year from `media.Year` if available
- **AND** format as "Movie Title YYYY" or "Movie Title (YYYY)"

### Requirement: Movie Indexer Selection

The torrenter service SHALL select appropriate Prowlarr indexers for movies based on anime classification.

#### Scenario: Anime movie uses NYAA and 1337x

- **WHEN** processing a movie download with `media.Anime == true`
- **THEN** the service SHALL set indexers to `[NYAA, 1337]` (both NYAA and 1337x)
- **AND** search both indexers for anime movie torrents

#### Scenario: Regular movie uses only 1337x

- **WHEN** processing a movie download with `media.Anime == false`
- **THEN** the service SHALL set indexers to `[1337]` (1337x only)
- **AND** search only 1337x indexer for movie torrents

#### Scenario: Indexer constants defined

- **WHEN** indexer selection is performed
- **THEN** the service SHALL use defined constants for indexer IDs
- **AND** NYAA constant SHALL map to correct Prowlarr indexer ID
- **AND** 1337 constant SHALL map to 1337x Prowlarr indexer ID

### Requirement: Movie Repository Existence Check

The torrenter repository SHALL provide a method to check if a movie exists in the Plex library by TVDB ID.

#### Scenario: Check movie exists by TVDB ID

- **WHEN** `MovieExistsByTvdbId()` is called with a TVDB movie ID
- **THEN** the repository SHALL query Movies table with `WHERE tvdb_id = $1`
- **AND** return true if movie record exists
- **AND** return false if movie record does not exist

#### Scenario: Handle empty TVDB ID

- **WHEN** `MovieExistsByTvdbId()` is called with empty TVDB ID
- **THEN** the repository SHALL return false
- **AND** NOT query the database

#### Scenario: Handle database error

- **WHEN** `MovieExistsByTvdbId()` encounters database error
- **THEN** the repository SHALL return false and the error
- **AND** allow caller to handle error appropriately

### Requirement: Movie Torrent Search

The torrenter service SHALL search for movie torrents using the generated search strategy.

#### Scenario: Search Prowlarr for movie

- **WHEN** movie search strategy is executed
- **THEN** the service SHALL call Prowlarr API with movie category (2000)
- **AND** use configured indexers based on anime classification
- **AND** apply quality filtering to results
- **AND** return matching torrents

#### Scenario: No movie torrents found

- **WHEN** Prowlarr search returns no results for movie
- **THEN** the service SHALL log warning with movie name and search query
- **AND** return error indicating no torrents found
- **AND** NOT add torrent to qBittorrent

#### Scenario: Movie torrent quality selection

- **WHEN** multiple movie torrents are found
- **THEN** the service SHALL sort torrents by quality and seeders
- **AND** apply uploader preference filtering
- **AND** select the best matching torrent
- **AND** download to qBittorrent with "scout" category tag

### Requirement: Movie Download Monitoring

The torrenter service SHALL monitor movie torrent downloads and trigger post-processing upon completion.

#### Scenario: Monitor movie torrent download

- **WHEN** a movie torrent is added to qBittorrent
- **THEN** the service SHALL poll qBittorrent every 10 seconds
- **AND** check torrent completion status
- **AND** send TorrentCompleteEvent when download finishes

#### Scenario: Process completed movie download

- **WHEN** a movie torrent completes downloading
- **THEN** the service SHALL call MediaProcessor.ProcessDownloadedTorrent
- **AND** determine movie base directory from Plex library
- **AND** construct save path as `{baseDir}/{filename}`
- **AND** create symbolic link from qBittorrent download location to save path

#### Scenario: Movie already has base directory

- **WHEN** processing completed movie download
- **AND** movie exists in Movies table with base_directory
- **THEN** the service SHALL use existing base_directory for save path
- **AND** NOT query Plex API for directory

#### Scenario: Movie missing base directory

- **WHEN** processing completed movie download
- **AND** movie does not exist in Movies table
- **THEN** the service SHALL use default movie library path
- **AND** create subdirectory for movie: `{defaultPath}/{MovieName} ({Year})`
- **AND** save movie file to constructed path
