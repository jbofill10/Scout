# TVDB Proxy

TVDB API v4 proxy service for media search and metadata enrichment in the Scout system.

## Overview

The TVDB Proxy acts as an intermediary between the Scout webserver and The TVDB API v4. It handles TVDB authentication, enriches search results with complete episode metadata, and determines if media is anime based on genre tags. This service is intentionally kept simple with a monolithic structure (`main.go`).

## Architecture

Unlike the other Scout services, tvdb_proxy uses a simple monolithic structure:

```
tvdb_proxy/
├── main.go          # Single file containing all logic
├── api.toml         # Optional local config file (not in repo)
└── go.mod
```

**Why monolithic?**
- Simple, stateless proxy functionality
- No complex business logic requiring layers
- Easy to understand and maintain
- Future refactoring possible if complexity grows

## Responsibilities

1. **TVDB Authentication**
   - Manages TVDB API v4 authentication
   - Handles token refresh and session management
   - Provides authenticated requests to TVDB API

2. **Media Search**
   - Receives search queries from webserver
   - Queries TVDB API for shows/movies
   - Enriches results with complete episode information
   - Returns structured `tvdb.Media` objects

3. **Extended Series Information**
   - Fetches detailed series metadata
   - Determines if series is anime based on genre tags
   - Provides episode lists with air dates

4. **Episode Metadata Enrichment**
   - For each search result, fetches all episodes
   - Includes season, episode number, title, air date
   - Calculates absolute numbering for anime

## API Endpoints

### GET /series
Search for TV shows or movies.

**Query Parameters:**
- `mediaType` (string, required) - "series" or "movie"
- `mediaName` (string, required) - Search query

**Example:**
```bash
curl "http://localhost:22000/series?mediaType=series&mediaName=Breaking%20Bad"
```

**Response:**
```json
[
  {
    "id": 81189,
    "title": "Breaking Bad",
    "type": "series",
    "year": 2008,
    "overview": "A high school chemistry teacher...",
    "episodes": [
      {
        "season": 1,
        "episode": 1,
        "title": "Pilot",
        "airDate": "2008-01-20",
        "absoluteNumber": 1
      }
    ],
    "anime": false
  }
]
```

### GET /series/:id/extended
Get extended information for a specific series.

**Path Parameters:**
- `id` (integer, required) - TVDB series ID

**Example:**
```bash
curl "http://localhost:22000/series/81189/extended"
```

**Response:**
```json
{
  "id": 81189,
  "title": "Breaking Bad",
  "genres": ["Drama", "Crime", "Thriller"],
  "episodes": [...],
  "anime": false
}
```

**Anime Detection:**
- Checks if "Anime" appears in the `genres` array
- Sets `anime: true` in response if detected
- Used by webserver to apply anime-specific handling

## Configuration

The tvdb_proxy loads configuration in this priority order:

1. **Environment Variables** (highest priority)
   - `TVDB_HOST` - TVDB API base URL
   - `TVDB_API_KEY` - TVDB API key
   - `TVDB_TOKEN` - TVDB authentication token (optional)
   - `BIND_ADDRESS` - Server bind address (default: `localhost:22000`)

2. **Kubernetes Secrets** (if running in K8s)
   - `/root/secrets/tvdb-api-key`
   - `/root/secrets/tvdb-token`

3. **Local TOML File** (for local development only)
   - `api.toml` in the tvdb_proxy directory

### Configuration Options

#### Environment Variables
```bash
export TVDB_HOST="https://api4.thetvdb.com/v4"
export TVDB_API_KEY="your-api-key-here"
export TVDB_TOKEN="your-token-here"  # Optional
export BIND_ADDRESS="0.0.0.0:22000"
```

#### TOML File (api.toml)
Create `tvdb_proxy/api.toml` for local development:

```toml
host = "https://api4.thetvdb.com/v4"
api-key = "your-api-key-here"
token = "your-token-here"  # Optional
```

**Note:** The `api.toml` file is `.gitignored` to prevent credential leaks.

#### Kubernetes Secrets
In production, create secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tvdb-proxy-secrets
type: Opaque
stringData:
  tvdb-api-key: "your-api-key-here"
  tvdb-token: "your-token-here"
```

Mounted at `/root/secrets/` in the pod.

## Getting a TVDB API Key

1. Go to https://thetvdb.com/
2. Create an account (free)
3. Navigate to API Keys section in your account settings
4. Generate a new API v4 key
5. Use the key in your configuration

## Running Locally

### Prerequisites
- Go 1.23+
- Valid TVDB API key

### Start the Server

**Using environment variables:**
```bash
cd tvdb_proxy

export TVDB_HOST="https://api4.thetvdb.com/v4"
export TVDB_API_KEY="your-api-key-here"
export BIND_ADDRESS="localhost:22000"

go run main.go
```

**Using TOML file:**
```bash
cd tvdb_proxy

# Create api.toml with your credentials
cat > api.toml << EOF
host = "https://api4.thetvdb.com/v4"
api-key = "your-api-key-here"
EOF

go run main.go
```

Server will start on `http://localhost:22000`

## Development

### Running Tests
```bash
cd tvdb_proxy
go test -v
```

### Running Linter
```bash
cd tvdb_proxy
golangci-lint run
```

### Building
```bash
cd tvdb_proxy
go build -o tvdb_proxy main.go
```

### Testing the API

**Test series search:**
```bash
curl "http://localhost:22000/series?mediaType=series&mediaName=Cowboy%20Bebop"
```

**Test extended series info:**
```bash
curl "http://localhost:22000/series/76885/extended"
```

**Test anime detection:**
```bash
# Cowboy Bebop should have anime=true
curl "http://localhost:22000/series/76885/extended" | jq '.anime'
```

## Key Features

### Episode Enrichment
Every search result includes complete episode metadata:
- Season and episode numbers
- Episode titles
- Air dates
- Absolute numbering (for anime)

This saves the webserver from making additional API calls.

### Anime Detection
Automatically detects anime based on TVDB genre tags:
- Checks for "Anime" in genres array
- Sets `anime` flag in response
- Used by torrenter for special handling

### Token Management
- Authenticates with TVDB API on startup
- Caches authentication token
- Refreshes token as needed
- Handles API rate limiting

### Error Handling
- Returns appropriate HTTP status codes
- Provides error messages in response
- Logs errors for debugging
- Handles TVDB API failures gracefully

## Troubleshooting

### Authentication Errors
```bash
# Verify your API key is valid
curl -X POST "https://api4.thetvdb.com/v4/login" \
  -H "Content-Type: application/json" \
  -d '{"apikey":"YOUR_API_KEY"}'
```

If you get `401 Unauthorized`, your API key is invalid or expired.

### Connection Refused
- Ensure the service is running: `curl http://localhost:22000/health`
- Check bind address in configuration
- Verify port 22000 is not already in use: `lsof -i :22000`

### Empty Search Results
- Verify search query is correct
- Check TVDB website to confirm the media exists
- Try different spelling or include year
- Check logs for TVDB API errors

### Anime Not Detected
- Verify the series has "Anime" genre on TVDB
- Use extended endpoint to see genres: `/series/:id/extended`
- Some anime may not be tagged correctly on TVDB

### Rate Limiting
TVDB API has rate limits. If you hit them:
- Implement request caching (not currently done)
- Reduce request frequency
- Contact TVDB for higher rate limits

## Integration with Scout

### Webserver Integration
The webserver calls tvdb_proxy via HTTP:

```go
// Search for media
response := tvdbClient.Get(fmt.Sprintf(
    "%s/series?mediaType=%s&mediaName=%s",
    tvdbProxyHost, mediaType, query))

// Get extended info (anime check)
response := tvdbClient.Get(fmt.Sprintf(
    "%s/series/%d/extended",
    tvdbProxyHost, seriesID))
```

### Service Discovery
In Kubernetes:
- Service name: `tvdb-proxy-service`
- Port: 22000
- DNS: `http://tvdb-proxy-service:22000`

In local development:
- URL: `http://localhost:22000`

## Dependencies

### Go Packages
- `github.com/gin-gonic/gin` - HTTP framework
- `github.com/BurntSushi/toml` - TOML parsing (for local config)
- Standard library packages (net/http, encoding/json, etc.)

### External APIs
- TVDB API v4 (https://api4.thetvdb.com/v4)

## Future Improvements

Potential enhancements if complexity grows:

1. **Caching Layer**
   - Cache TVDB responses to reduce API calls
   - Implement Redis or in-memory cache
   - Respect TVDB rate limits better

2. **Metrics & Monitoring**
   - Track API response times
   - Monitor error rates
   - Alert on authentication failures

3. **Refactor to Layered Architecture**
   - If logic grows, move to cmd/internal structure
   - Separate handlers, services, clients
   - Add proper interface-based design

4. **Enhanced Error Handling**
   - Retry logic for transient failures
   - Circuit breaker for TVDB API
   - Better error messages to clients

5. **Additional TVDB Features**
   - Actor information
   - Poster/banner images
   - Related series recommendations

## Contributing

When making changes:
1. Keep it simple - avoid over-engineering
2. Test with real TVDB API calls
3. Update this README with new endpoints or config
4. Ensure backward compatibility with webserver

## License

MIT
