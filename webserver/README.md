# Webserver

Central coordinator and REST API for the Scout media torrenting system.

## Overview

The webserver acts as the main orchestrator in the Scout microservices architecture. It receives search requests from the UI, coordinates with tvdb-proxy for media metadata, manages the episode scheduling system, and dispatches download requests to the torrenter service.

## Architecture

The webserver follows a layered architecture pattern:

```
┌─────────────────────────────────────────┐
│          HTTP Request (Gin)             │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│     Handlers (internal/handlers/)       │
│  - search_handler.go                    │
│  - download_handler.go                  │
└──────────────┬──────────────────────────┘
               │
┌──────────────▼──────────────────────────┐
│   Interactors (internal/interactors/)   │
│  - search_interactor.go                 │
│  - download_interactor.go               │
│  (Business logic orchestration)         │
└──────────────┬──────────────────────────┘
               │
        ┌──────┴──────┐
        │             │
┌───────▼──────┐  ┌──▼────────────────────┐
│   Clients    │  │  Repository           │
│ - TVDBProxy  │  │  (PostgreSQL)         │
│ - Torrenter  │  │                       │
└──────────────┘  └───────────────────────┘
```

### Directory Structure

```
webserver/
├── cmd/webserver/
│   └── main.go                # Entry point, dependency injection
├── internal/
│   ├── handlers/              # HTTP request handlers
│   │   ├── search_handler.go
│   │   └── download_handler.go
│   ├── interactors/           # Business logic orchestration
│   │   ├── search_interactor.go
│   │   └── download_interactor.go
│   ├── clients/               # HTTP clients for external services
│   │   ├── tvdb_proxy_client.go
│   │   └── torrenter_client.go
│   ├── repository/            # Database operations
│   │   ├── repository.go
│   │   └── scheduled_downloads.go
│   ├── scheduler/             # Episode scheduling system
│   │   └── scheduler.go
│   └── config/                # Configuration loading
│       └── config.go
├── go.mod
└── go.sum
```

## Responsibilities

1. **Search Coordination**
   - Receives search queries from UI
   - Forwards requests to tvdb-proxy for TVDB search
   - Returns enriched results (with episode metadata) to UI

2. **Download Management**
   - Receives download requests (shows/movies) from UI
   - Determines which episodes have aired vs. future episodes
   - Sends immediate downloads to torrenter
   - Schedules future episodes in PostgreSQL

3. **Episode Scheduling**
   - Runs background scheduler goroutine
   - Monitors `ScheduledDownloads` table for due episodes
   - Automatically sends episodes to torrenter when release_time is reached

4. **Anime Detection**
   - Queries tvdb-proxy for extended series info
   - Checks for "Anime" genre tag
   - Sets appropriate flags for torrenter (absolute numbering)

## API Endpoints

### GET /search
Search for TV shows or movies via TVDB.

**Query Parameters:**
- `query` (string, required) - Search term
- `media_type` (string, required) - "series" or "movie"

**Example:**
```bash
curl "http://localhost:22920/search?query=Breaking%20Bad&media_type=series"
```

**Response:** Array of `tvdb.Media` objects with episode information

### POST /shows
Schedule download of TV show episodes.

**Request Body:**
```json
{
  "id": 81189,
  "title": "Breaking Bad",
  "episodes": [
    {"season": 1, "episode": 1, "airDate": "2008-01-20"},
    {"season": 1, "episode": 2, "airDate": "2008-01-27"}
  ],
  "anime": false
}
```

**Response:**
```json
{
  "message": "Download initiated for Breaking Bad",
  "scheduled": 5,
  "immediate": 2
}
```

### POST /movies
Download a movie.

**Request Body:**
```json
{
  "id": 12345,
  "title": "The Matrix",
  "year": 1999
}
```

## Configuration

The webserver is configured via environment variables:

### Database Configuration
- `DB_HOST` - PostgreSQL host (default: `postgres-service`)
- `DB_PORT` - PostgreSQL port (default: `5432`)
- `DB_USER` - Database username (default: `scoutuser`)
- `DB_PASSWORD` - Database password (required)
- `DB_NAME` - Database name (default: `scoutdb`)

### Service Discovery
- `TVDB_PROXY_HOST` - TVDB Proxy service URL (default: `http://localhost:22000`)
- `TORRENTER_HOST` - Torrenter service URL (default: `http://localhost:22001`)

### Server
- `BIND_ADDRESS` - Server bind address and port (default: `0.0.0.0:22920`)

### Kubernetes Configuration

In Kubernetes, these are provided via ConfigMaps and Secrets:

```yaml
# ConfigMap
DB_HOST: postgres-service
TVDB_PROXY_HOST: http://tvdb-proxy-service:22000
TORRENTER_HOST: http://torrenter-service:22001

# Secret (base64 encoded)
DB_PASSWORD: <base64-encoded-password>
```

## Running Locally

### Prerequisites
- Go 1.23+
- PostgreSQL running and initialized with `sql/setup-postgres.sql`
- tvdb-proxy service running on port 22000
- torrenter service running on port 22001

### Start the Server

```bash
# Set required environment variables
export DB_PASSWORD="scoutpassword"
export DB_HOST="localhost"
export DB_USER="scoutuser"
export DB_NAME="scoutdb"
export TVDB_PROXY_HOST="http://localhost:22000"
export TORRENTER_HOST="http://localhost:22001"

# Run from webserver directory
cd webserver
go run cmd/webserver/main.go
```

Server will start on `http://0.0.0.0:22920`

## Development

### Running Tests
```bash
cd webserver
go test ./...
```

### Running Linter
```bash
cd webserver
golangci-lint run
```

### Building
```bash
cd webserver
go build -o webserver cmd/webserver/main.go
```

### Generating Mocks (if needed)
```bash
cd webserver
mockery --all --dir internal/repository --output internal/repository/mocks
```

## Database Tables

The webserver manages the following PostgreSQL tables:

### ScheduledDownloads
Stores future episode downloads with complete media metadata.

```sql
CREATE TABLE ScheduledDownloads (
    id SERIAL PRIMARY KEY,
    media JSONB NOT NULL,
    content_hash TEXT UNIQUE NOT NULL,
    release_time TIMESTAMP NOT NULL,
    schedule_status TEXT NOT NULL DEFAULT 'pending'
);
```

- `media` - Complete `tvdb.Media` object as JSONB
- `content_hash` - SHA256 hash for deduplication
- `release_time` - When the episode should be downloaded
- `schedule_status` - 'pending' or 'queued'

### ShowDownloadHistory
Tracks all show download attempts for auditing and debugging.

```sql
CREATE TABLE ShowDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    season INTEGER NOT NULL,
    episode INTEGER NOT NULL,
    downloadDate TIMESTAMP NOT NULL,
    reason TEXT,
    status TEXT NOT NULL
);
```

### MovieDownloadHistory
Tracks movie download attempts.

```sql
CREATE TABLE MovieDownloadHistory (
    id SERIAL PRIMARY KEY,
    mediaTitle TEXT NOT NULL,
    downloadDate TIMESTAMP NOT NULL,
    reason TEXT,
    status TEXT NOT NULL
);
```

## Key Workflows

### Search Flow
1. UI sends GET /search request
2. Handler receives request, delegates to SearchInteractor
3. SearchInteractor calls TVDBProxyClient.Search()
4. TVDBProxyClient makes HTTP request to tvdb-proxy
5. Results enriched with episode metadata, returned to UI

### Download Flow (Show)
1. UI sends POST /shows with selected episodes
2. Handler receives request, delegates to DownloadInteractor
3. DownloadInteractor calls TVDBProxyClient for extended info (anime check)
4. For each episode:
   - If aired: add to immediate batch
   - If future: call Repository.Schedule() to store in DB
5. Send immediate batch to TorrenterClient.Download()
6. Return summary to UI

### Scheduler Flow
1. Scheduler goroutine runs on startup via `scheduler.Start()`
2. Polls `ScheduledDownloads` table every minute
3. Queries for episodes where `release_time <= NOW()` and `status = 'pending'`
4. For each due episode:
   - Send to mediaQueue channel
   - Mark as 'queued' in database
5. Worker goroutine consumes mediaQueue, sends to TorrenterClient

## Dependencies

### Internal Dependencies
- `shared/media` - TVDB data structures (tvdb.Media, tvdb.Episode)
- `shared/status` - Download status constants

### External Dependencies
- `github.com/gin-gonic/gin` - HTTP framework
- `github.com/lib/pq` - PostgreSQL driver
- `database/sql` - Database interface

## Troubleshooting

### Connection Refused to tvdb-proxy
- Ensure tvdb-proxy is running: `curl http://localhost:22000/health`
- Check `TVDB_PROXY_HOST` environment variable

### Connection Refused to torrenter
- Ensure torrenter is running: `curl http://localhost:22001/health`
- Check `TORRENTER_HOST` environment variable

### Database Connection Errors
- Verify PostgreSQL is running
- Check credentials: `psql -U scoutuser -d scoutdb`
- Ensure database is initialized: `psql -U scoutuser -d scoutdb -f sql/setup-postgres.sql`

### Scheduler Not Processing Episodes
- Check logs for scheduler startup message
- Verify episodes exist in DB: `SELECT * FROM ScheduledDownloads WHERE schedule_status = 'pending';`
- Check release_time: `SELECT id, release_time, schedule_status FROM ScheduledDownloads;`

## Contributing

When adding new features:
1. Add handler in `internal/handlers/`
2. Add business logic in `internal/interactors/`
3. Add external calls in `internal/clients/` or `internal/repository/`
4. Register routes in `cmd/webserver/main.go`
5. Write tests in `*_test.go` files
6. Update this README with new endpoints/configuration

## License

MIT
