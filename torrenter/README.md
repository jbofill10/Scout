# Torrenter

Download processor and Plex integration service for the Scout media torrenting system.

## Overview

The torrenter is responsible for the complete download lifecycle: searching for torrents via Prowlarr, downloading via qBittorrent, monitoring download progress, post-processing files for Plex compatibility, and maintaining a synchronized mirror of the Plex library in PostgreSQL for duplicate detection.

## Architecture

The torrenter follows a layered architecture with dependency injection:

```
┌─────────────────────────────────────────┐
│          HTTP Request (Gin)             │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│     Handlers (internal/handlers/)       │
│  - download_handler.go                  │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│   Interactors (internal/interactors/)   │
│  - download_interactor.go               │
│  (Orchestrates workflow)                │
└──────────────┬──────────────────────────┘
               │
        ┌──────┴──────┬───────────────┐
        │             │               │
┌───────▼──────┐  ┌───▼────────┐  ┌──▼────────────┐
│   Services   │  │ Repository │  │  FileSystem   │
│ - Torrent    │  │(PostgreSQL)│  │               │
│ - Plex       │  │            │  │               │
│ - MediaProc  │  │            │  │               │
└──────────────┘  └────────────┘  └───────────────┘
```

### Directory Structure

```
torrenter/
├── cmd/torrenter/
│   └── main.go                       # Entry point, dependency injection
├── internal/
│   ├── handlers/                     # HTTP request handlers
│   │   └── download_handler.go
│   ├── interactors/                  # Business logic orchestration
│   │   └── download_interactor.go
│   ├── service/                      # Domain services
│   │   ├── interfaces.go             # Service interfaces
│   │   ├── torrent.go                # QbittHandler - Prowlarr/qBittorrent
│   │   ├── plex.go                   # PlexHandler - Plex library sync
│   │   ├── mediaprocessor.go         # MediaProcessSvc - Post-processing
│   │   ├── filesystem.go             # FsSvc - File operations
│   │   └── mocks/                    # Generated mocks for testing
│   ├── repository/                   # Database operations
│   │   ├── repository.go
│   │   └── mocks/                    # Generated mocks for testing
│   ├── models/                       # Data structures
│   │   ├── torrent_match.go
│   │   └── events.go
│   └── config/                       # Configuration loading
│       └── config.go
├── go.mod
└── go.sum
```

## Responsibilities

1. **Torrent Search & Download**
   - Creates search strategies (multiple query variations per episode)
   - Searches Prowlarr with appropriate indexers (NYAA for anime, 1337x + NYAA for regular)
   - Validates torrents (correct episode, no batches, quality checks)
   - Applies uploader preferences from database
   - Sends magnet links to qBittorrent

2. **Download Monitoring**
   - Polls qBittorrent every 10 seconds for torrents tagged "scout"
   - Detects completion (state="stalledUP", completed=size)
   - Runs integrity recheck
   - Triggers post-processing on completion

3. **Plex Integration**
   - Syncs entire Plex library to PostgreSQL on startup
   - Provides duplicate detection before downloading
   - Determines preferred library paths for new media
   - Maintains mirror of Shows, Seasons, Episodes, Movies

4. **Media Post-Processing**
   - Prepares target paths for Plex
   - Constructs proper filenames (S01E01 format)
   - Plans file organization (hard-linking TODO)

## API Endpoints

### POST /download
Initiate download of media (show or movie).

**Request Body:**
```json
{
  "id": 81189,
  "title": "Breaking Bad",
  "type": "series",
  "episodes": [
    {
      "season": 1,
      "episode": 1,
      "title": "Pilot",
      "absoluteNumber": 0
    }
  ],
  "anime": false
}
```

**Response:**
```json
{
  "message": "Download initiated for Breaking Bad",
  "episodes_downloaded": 1,
  "episodes_skipped": 0
}
```

### GET /media/:hash
Check if media exists in Plex library (for duplicate detection).

**Parameters:**
- `hash` - Content hash of the media

**Response:**
```json
{
  "exists": true,
  "location": "/plex/TV Shows/Breaking Bad/Season 1/Breaking Bad - S01E01.mkv"
}
```

## Configuration

The torrenter is configured via environment variables and Kubernetes secrets:

### Prowlarr Configuration
- `PROWLARR_HOST` - Prowlarr API URL (default: `http://localhost:9696`)
- `/root/secrets/prowlarr-key` - Prowlarr API key (in K8s)

### qBittorrent Configuration
- `QBITT_HOST` - qBittorrent URL (default: `http://localhost:8080`)
- `/root/secrets/qbitt-user` - qBittorrent username (in K8s)
- `/root/secrets/qbitt-password` - qBittorrent password (in K8s)

### Plex Configuration
- `PLEX_HOST` - Plex server URL (default: `http://localhost:32400`)
- `/root/secrets/plex-key` - Plex API token (in K8s)
- `PLEX_MOVIE_SECTIONS` - Comma-separated movie library section IDs
- `PLEX_SHOW_SECTIONS` - Comma-separated TV library section IDs

### Database Configuration
- `DB_HOST` - PostgreSQL host (default: `postgres-service`)
- `DB_PORT` - PostgreSQL port (default: `5432`)
- `DB_USER` - Database username (default: `scoutuser`)
- `DB_PASSWORD` - Database password (required)
- `DB_NAME` - Database name (default: `scoutdb`)

### Other
- `UI_ENDPOINT` - UI service URL for callbacks
- `BIND_ADDRESS` - Server bind address (default: `0.0.0.0:22001`)

## Running Locally

### Prerequisites
- Go 1.23+
- PostgreSQL running and initialized
- Prowlarr running with indexers configured
- qBittorrent running
- Plex Media Server running

### Start the Server

```bash
# Set required environment variables
export DB_PASSWORD="scoutpassword"
export PROWLARR_HOST="http://localhost:9696"
export QBITT_HOST="http://localhost:8080"
export PLEX_HOST="http://localhost:32400"

# For K8s-style secrets (optional for local dev):
# mkdir -p /root/secrets
# echo "your-prowlarr-key" > /root/secrets/prowlarr-key
# echo "admin" > /root/secrets/qbitt-user
# echo "adminpass" > /root/secrets/qbitt-password
# echo "your-plex-token" > /root/secrets/plex-key

# Or set as environment variables:
export PROWLARR_KEY="your-prowlarr-key"
export QBITT_USER="admin"
export QBITT_PASSWORD="adminpass"
export PLEX_KEY="your-plex-token"

# Set library sections
export PLEX_MOVIE_SECTIONS="3"
export PLEX_SHOW_SECTIONS="4"

# Run from torrenter directory
cd torrenter
go run cmd/torrenter/main.go
```

Server will start on `http://0.0.0.0:22001`

## Download Workflow

When a download request is received:

### 1. Pre-Download Filtering (`download_interactor.go`)
- For series, checks if episodes already exist in Plex via `Repository.EpisodeExists()`
- Filters out existing episodes
- If all episodes exist, returns success without downloading

### 2. Search Strategy Creation (`service/torrent.go:createSearchStrategy`)
Generates multiple search query variations:

**For Anime:**
- `"ShowName 01"` format (absolute numbering)
- Excludes "season"/"episode" keywords
- Variations with/without spaces in show name

**For Regular Shows:**
- `"ShowName S01E01"` format
- `"ShowName Season 1 Episode 1"` format
- Variations with/without spaces

### 3. Torrent Search (`service/torrent.go:HandleDownload`)
- Searches Prowlarr with appropriate indexers:
  - Anime: NYAA (ID 1)
  - Regular: NYAA + 1337x (IDs 1, 5)
- Validates via `isCorrectTorrent()`:
  - Excludes batch torrents (multiple episodes)
  - Ensures episode number in title
  - Matches show name
- Collects matching torrents as `TorrentMatch` objects

### 4. Quality Sorting (`service/torrent.go:sortTorrentsByQuality`)
- Sorts by quality: 4K > 2160p > 1080p > 720p > 480p
- Applies uploader preferences from `UploaderPreferences` table
- Falls back to "SubsPlease" for anime
- Returns highest quality from preferred uploader

### 5. Download Initiation (`service/torrent.go:downloadTorrent`)
- Creates save directory: `/data/Downloads/{TorrentTitle}`
- Adds magnet/torrent to qBittorrent
- Tags with "scout" category for monitoring
- Spawns goroutine to watch progress

### 6. Monitoring (goroutine in `HandleDownload`)
- Polls qBittorrent every 10 seconds
- Filters for torrents with "scout" category tag
- On completion (state="stalledUP", completed=size):
  - Runs recheck for integrity
  - Sends `TorrentCompleteEvent` to done channel

### 7. Post-Processing (`service/mediaprocessor.go:ProcessDownloadedTorrent`)
- Determines Plex library path via `GetPreferredLibrary()`
- Constructs target path:
  - Shows: `{LibraryPath}/{ShowName}/Season {N}/{ShowName} - S{N}E{N}.{ext}`
  - Movies: `{LibraryPath}/{MovieName} ({Year}).{ext}`
- **TODO:** Hard-linking implementation pending

### 8. Plex Library Sync (`service/plex.go:syncPlexLibrary`)
- Runs once on startup
- Fetches all libraries, shows, seasons, episodes, movies
- Upserts to PostgreSQL
- Used for duplicate detection in step 1

## Database Tables

### Plex Library Mirror Tables

**Libraries**
```sql
CREATE TABLE Libraries (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    location TEXT NOT NULL,
    type TEXT NOT NULL
);
```

**Shows / Seasons / Episodes / EpisodeMedia**
Mirror structure from Plex for TV shows with season/episode hierarchy.

**Movies / MovieMedia**
Mirror structure from Plex for movies.

### UploaderPreferences
User-defined preferences for torrent uploaders:

```sql
CREATE TABLE UploaderPreferences (
    id SERIAL PRIMARY KEY,
    uploader_name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    is_anime BOOLEAN NOT NULL DEFAULT false,
    preference_order INTEGER NOT NULL
);
```

### Download History
Shared with webserver for tracking download attempts.

## Development

### Running Tests
```bash
cd torrenter
go test ./...
```

### Running Linter
```bash
cd torrenter
golangci-lint run
```

### Building
```bash
cd torrenter
go build -o torrenter cmd/torrenter/main.go
```

### Generating Mocks
```bash
cd torrenter
mockery --all --dir internal/service --output internal/service/mocks
mockery --all --dir internal/repository --output internal/repository/mocks
```

## Interface-Based Design

The torrenter uses interfaces for all major components to enable testing:

**Service Interfaces** (`internal/service/interfaces.go`):
- `TorrentService` - Torrent search, download, monitoring
- `PlexService` - Plex library sync and queries
- `MediaProcessor` - Post-download file processing
- `FileSystem` - File system operations
- `Repository` - Database operations

This allows easy mocking in tests and swapping implementations.

## Troubleshooting

### Prowlarr Connection Issues
```bash
# Test Prowlarr connectivity
curl "http://localhost:9696/api/v1/indexer" -H "X-Api-Key: YOUR_KEY"
```

### qBittorrent Authentication Errors
```bash
# Test qBittorrent login
curl -X POST "http://localhost:8080/api/v2/auth/login" \
  --data "username=admin&password=adminpass"
```

### Plex API Errors
```bash
# Test Plex connection
curl "http://localhost:32400/library/sections?X-Plex-Token=YOUR_TOKEN"
```

### Database Connection Errors
```bash
# Verify PostgreSQL
psql -U scoutuser -d scoutdb -c "SELECT * FROM Libraries;"
```

### Downloads Not Starting
- Check Prowlarr indexers are configured and working
- Verify torrents match quality/uploader preferences
- Check qBittorrent is not at download limit
- Review logs for search strategy and validation failures

### Plex Sync Not Working
- Verify Plex token is correct
- Check section IDs match library IDs in Plex
- Ensure database tables exist and are accessible

## Key Features

### Anime Handling
- Uses absolute numbering (1, 2, 3...) instead of S01E01
- Searches NYAA indexer exclusively
- Excludes "season"/"episode" keywords from search
- Prefers "SubsPlease" uploader by default

### Quality Preferences
Automatically selects highest quality available:
1. 4K
2. 2160p
3. 1080p
4. 720p
5. 480p

### Uploader Preferences
Configurable via `UploaderPreferences` table:
- Set preferred uploaders per media type
- Different preferences for anime vs regular shows
- Automatic fallback to "SubsPlease" for anime

### Duplicate Detection
- Checks Plex library before downloading
- Filters out episodes that already exist
- Saves bandwidth and prevents duplicates

## Contributing

When adding new features:
1. Define interfaces in `internal/service/interfaces.go`
2. Implement services in `internal/service/`
3. Add business logic in `internal/interactors/`
4. Create handlers in `internal/handlers/`
5. Generate mocks with mockery
6. Write unit tests with mocks
7. Update this README

## License

MIT