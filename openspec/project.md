# Project Context

## Purpose
Scout is an over-engineered microservices architecture for automated media torrenting with a web interface. The system coordinates between four microservices to search for TV shows and movies, schedule downloads based on air dates, and integrate with qBittorrent for downloading and Plex for library management. Key goals include:
- Automatic downloading of TV episodes as they air (with scheduling for future episodes)
- Smart torrent selection based on quality preferences and trusted uploaders
- Seamless integration with existing Plex Media Server libraries
- Duplicate detection to avoid re-downloading existing content
- Support for both regular shows and anime with proper episode numbering

## Tech Stack

### Backend Services (Go 1.23)
- **webserver** - Central REST API and coordinator (port 22920)
- **tvdb-proxy** - TVDB API v4 proxy with metadata enrichment (port 22000)
- **torrenter** - Download processor with qBittorrent/Plex/Prowlarr integration (port 22001)

### Frontend
- **React 19** with TypeScript
- **Material-UI v7** for UI components
- **Vite** as build tool (port 5173 dev, port 80 production)

### Infrastructure
- **PostgreSQL** - Primary database for scheduled downloads, history, and Plex library sync
- **Docker Compose** - Primary deployment method
- **Nginx** - Reverse proxy for frontend and API routing
- **OpenTelemetry** - Distributed tracing and structured logging

### External Integrations
- **TVDB API v4** - Media metadata and search
- **qBittorrent** - Torrent client
- **Plex Media Server** - Library management
- **Prowlarr** - Indexer aggregation (NYAA for anime, 1337x for shows)

## Project Conventions

### Code Style

**Go Services:**
- `golangci-lint` for all Go code with maximum line length of 120 characters
- Standard Go project layout: `cmd/` for entry points, `internal/` for private packages
- Interface-based design for all major components (services, repositories)
- Context propagation: Always pass `context.Context` through entire call chain
- HTTP clients: Always use `http.NewRequestWithContext(ctx, ...)`, never `client.Get(url)`
- Logging: Use context-aware structured logging: `logger.InfoContext(ctx, "message", "key", value)`
- Error handling: Return errors up the stack, log at the handler level

**Frontend:**
- **ESLint** for code quality
- TypeScript for all new code
- React 19 with functional components and hooks
- Material-UI v7 component patterns

**Naming Conventions:**
- Go packages: lowercase, single word when possible
- Interfaces: Descriptive names (e.g., `TorrentService`, `MediaProcessor`, `Repository`)
- Mock generation: Use mockery tool, output to `internal/service/mocks/` and `internal/repository/mocks/`

### Architecture Patterns

**Layered Architecture:**
All Go services follow consistent layers:
- **Handler** (HTTP layer) - Gin framework, extracts context, validates requests
- **Interactor** (orchestration layer) - Coordinates between services, implements business logic
- **Service** (domain layer) - External service integrations (qBittorrent, Plex, Prowlarr)
- **Repository** (data layer) - PostgreSQL database operations

Example flow: `DownloadHandler` → `DownloadInteractor` → `QbittHandler`/`Repository`

**Interface-Based Design:**
- All major components defined as interfaces for dependency injection and testing
- Mock generation via mockery tool
- Enables clean testing and loose coupling

**Service Communication:**
- HTTP APIs between microservices using Docker Compose DNS names
- `webserver` → `tvdb-proxy:22000` (search, metadata)
- `webserver` → `torrenter:22001` (download requests)
- `ui` → `webserver:22920` (proxied through nginx at `/api`)

**Key Patterns:**
- **Scheduling**: `Scheduler.Start()` goroutine queries `ScheduledDownloads` table, sends due episodes to media queue
- **Plex Integration**: Syncs library on startup, checks `EpisodeExists()` before downloading to avoid duplicates
- **Error Handling**: Download failures logged to `ShowDownloadHistory`, duplicates return `ErrDuplicateScheduled`
- **Observability**: OpenTelemetry tracing with trace-correlated structured logging

### Testing Strategy

**Go Services:**
- Standard `go test` framework for all services
- Mock interfaces generated with **mockery** tool
- Test files: `*_test.go` alongside source files
- Mock locations:
  - `internal/service/mocks/` - Service layer mocks
  - `internal/repository/mocks/` - Repository layer mocks

**MANDATORY: Test-Guardian Workflow**

**CRITICAL:** After **every code change**, you **must** proactively run the test-guardian agent before considering work complete. This is non-negotiable.

**When to use test-guardian:**
- After making any edits to code files (`.go`, `.ts`, `.tsx`, `.js`)
- After writing new code files
- After refactoring existing code
- After fixing bugs
- After adding new features
- After updating dependencies

**Workflow:**
1. Make code changes
2. **Immediately** run test-guardian agent
3. Review test results
4. Fix any failures or regressions
5. When new code is written, write new tests for it
6. Only then mark task as complete

**Never mark a coding task "completed" until:**
- ✅ Code changes are made
- ✅ test-guardian has been run
- ✅ All tests are passing
- ✅ Any failures have been addressed
- ✅ Any binaries created by `go build` are removed

**Exception:** Read-only operations (file reads, searches, documentation) don't require test-guardian.

**Frontend:**
- React Testing Library (via Vite)
- ESLint for code quality: `npm run lint`

**Integration Testing:**
- Local development: Run all services and test end-to-end
- Docker Compose: Use `./compose-deploy.sh` to deploy and test

### Git Workflow

**Branch Strategy:**
- Main branch: `main`
- All PRs target `main` branch

**Commit Conventions:**
- Concise commit messages (1-2 sentences) focusing on "why" rather than "what"
- All commits include attribution footer:
  ```
  🤖 Generated with [Claude Code](https://claude.com/claude-code)

  Co-Authored-By: Claude <noreply@anthropic.com>
  ```

**Git Safety:**
- Never update git config
- Never run destructive/irreversible git commands without explicit user request
- Never skip hooks (--no-verify, --no-gpg-sign) unless explicitly requested
- Never force push to main/master
- Avoid `git commit --amend` unless explicitly requested or adding pre-commit hook edits

## Domain Context

**Media Torrenting Domain:**
- **TV Shows vs Movies**: Different handling for episodic content vs single files
- **Anime vs Regular Shows**: Critical distinction affecting search and numbering
  - Anime: Absolute numbering (1, 2, 3...), uses `AbsoluteNumber` field
  - Regular shows: Season/episode format (S01E05), uses `SeasonNumber`/`EpisodeNumber` fields
  - Detection: TVDB genre tags containing "Anime"
- **Torrent Quality Selection**: Prioritize by quality tier (4K > 1080p > 720p), then by trusted uploaders
- **Indexer Selection**: NYAA for anime (specialized), 1337x for regular shows, both searched via Prowlarr
- **Episode Air Dates**: Future episodes stored in `ScheduledDownloads` table, downloaded when air date arrives

**TVDB API Integration:**
- TVDB API v4 provides metadata and episode information
- Episode enrichment: Fetching `/series/:id/extended` to get full episode list with air dates
- Search endpoint: `/series?mediaName=...&mediaType=...`

**Plex Library Management:**
- Plex library structure: Libraries > Shows > Seasons > Episodes (with EpisodeMedia files)
- Library sync: On torrenter startup, sync entire Plex library to PostgreSQL
- Duplicate detection: Check `Repository.EpisodeExists()` before initiating download
- Hard-linking: Not yet implemented, files currently copied

**Download Workflow:**
1. User selects media in UI
2. Check if anime via TVDB genre tags
3. Split episodes into aired vs future
4. Aired: Send immediately to torrenter
5. Future: Store in `ScheduledDownloads` with release_time
6. Scheduler goroutine watches for due episodes
7. Torrenter searches Prowlarr, selects best torrent, sends to qBittorrent
8. Monitor download progress, trigger post-processing on completion

## Important Constraints

**Technical Constraints:**
- **Context Propagation**: Must always pass `context.Context` through entire call chain for observability (OpenTelemetry tracing and trace-correlated logging)
- **Service Communication**: Services communicate via Docker Compose DNS names in production, localhost in development
- **Database**: PostgreSQL required (previously SQLite, migrated for better concurrency)
- **Hard-linking**: Not yet implemented - media files are copied rather than hard-linked to Plex library locations
- **Line Length**: Go code must not exceed 120 characters per line (golangci-lint enforcement)

**Architectural Constraints:**
- **Layered Architecture**: Must follow Handler → Interactor → Service/Repository pattern
- **Interface-Based Design**: All major components must be defined as interfaces for testing
- **No Direct HTTP Clients**: Always use `http.NewRequestWithContext(ctx, ...)`, never `client.Get(url)` directly

**Deployment Constraints:**
- **Docker Compose**: Primary deployment method, all services must be containerized
- **Configuration**: Template files (`.env.template`, `docker-compose.yml.template`) provided, actual files gitignored for security
- **Port Allocation**: Fixed ports (22920, 22000, 22001, 80, 30030) - changes require coordination across services

**Security Constraints:**
- **Secrets Management**: All secrets via `.env` file, never hardcoded
- **Network Exposure**: LAN access only via port 30030, not internet-facing
- **API Keys**: Required for TVDB, qBittorrent, Plex, Prowlarr

## External Dependencies

**Required External Services:**

1. **TVDB API v4** (`https://api4.thetvdb.com`)
   - Purpose: Media metadata, search, episode information
   - Authentication: API key + bearer token
   - Rate limits: Standard TVDB API limits
   - Configuration: `TVDB_HOST`, `TVDB_API_KEY`, `TVDB_PIN`

2. **qBittorrent** (WebUI API)
   - Purpose: Torrent downloading and management
   - Authentication: Username/password
   - Configuration: `QBITT_HOST`, `QBITT_USER`, `QBITT_PASSWORD`
   - Used by: torrenter service
   - Features: Category tagging ("scout"), progress monitoring

3. **Plex Media Server** (API)
   - Purpose: Media library management and sync
   - Authentication: X-Plex-Token
   - Configuration: `PLEX_HOST`, `PLEX_KEY`, `PLEX_MOVIE_SECTIONS`, `PLEX_SHOW_SECTIONS`
   - Used by: torrenter service
   - Features: Library sync, duplicate detection

4. **Prowlarr** (Indexer Manager)
   - Purpose: Torrent indexer aggregation and search
   - Authentication: API key
   - Configuration: `PROWLARR_HOST`, `PROWLARR_KEY`
   - Used by: torrenter service
   - Indexers: NYAA (anime), 1337x (shows/movies)

**Infrastructure Dependencies:**

5. **PostgreSQL 17**
   - Purpose: Scheduled downloads, download history, Plex library mirror
   - Configuration: `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`
   - Databases: webserver DB, torrenter DB
   - Schema: SQL files in `sql/` directory

6. **OpenTelemetry Collector** (Optional)
   - Purpose: Distributed tracing and structured logging
   - Configuration: `OTEL_EXPORTER_OTLP_ENDPOINT` (default: `localhost:4317`)
   - Protocol: OTLP over gRPC
   - Used by: All Go services
