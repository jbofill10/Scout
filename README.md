# Scout

Over-engineered microservices architecture for automated media torrenting with a web interface.

## ⚠️ Security Notice

**Before deploying Scout, you must configure environment variables properly.** This repository does not include actual credentials.

1. Copy `.env.template` to `.env` and fill in your secrets
2. Never commit your `.env` file (protected by `.gitignore`)
3. See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed setup instructions

## 🏗️ Architecture

Scout is built as four microservices that work together:

- **webserver** (Go): Central coordinator, REST API, episode scheduling
- **tvdb_proxy** (Go): TVDB API proxy for media search and metadata
- **torrenter** (Go): Download processor with qBittorrent and Plex integration
- **ui** (React/TypeScript): Web interface for search and management

## 🚀 Quick Start with Docker Compose (Recommended)

### Prerequisites
- Docker (version 20.10+)
- Docker Compose (version 2.0+)
- At least 4GB RAM available for Docker

### Deploy Scout
```bash
# Copy template files and configure
cp .env.template .env
cp docker-compose.yml.template docker-compose.yml
cp nginx/nginx.conf.template nginx/nginx.conf
nano .env  # Fill in your API keys and credentials

# Deploy all services
./compose-deploy.sh
```

Access Scout at `http://localhost:30030` or `http://<YOUR_LAN_IP>:30030`

### Development Workflow
```bash
# Rebuild a service after changes
./compose-deploy.sh rebuild webserver

# View logs
./compose-deploy.sh logs torrenter

# Check status
./compose-deploy.sh status

# See all commands
./compose-deploy.sh --help
```

See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed documentation.

## 🐳 Running Services Locally (Without Docker)

Each service can run independently for development:

### 1. Start tvdb_proxy
```bash
cd tvdb_proxy
go run main.go
# Listens on localhost:22000
```

### 2. Start torrenter
```bash
cd torrenter
go run cmd/torrenter/main.go
# Listens on localhost:22001
```

### 3. Start webserver
```bash
cd webserver
go run cmd/webserver/main.go
# Listens on 0.0.0.0:22920 (configurable via BIND_ADDRESS env var)
```

### 4. Start UI
```bash
cd ui
npm install
npm run dev
# Listens on localhost:5173
```

**Configuration Note:** Services will look for configuration in this order:
1. Environment variables from `.env` file (highest priority)
2. Local TOML files (tvdb_proxy only, for local development without Docker)
3. Default values coded in services

## 📋 Configuration

All configuration is managed via the `.env` file. See [DEPLOYMENT.md](DEPLOYMENT.md) for complete instructions.

Quick setup:
```bash
# Copy template
cp .env.template .env

# Edit with your credentials
nano .env  # or use your preferred editor
```

Required secrets:
- **TVDB** API credentials (api-key, token) - Get from https://thetvdb.com/api-information
- **Prowlarr** API key - Find in Prowlarr Settings → General
- **qBittorrent** credentials (username, password)
- **Plex** token and library sections - Get token from https://support.plex.tv/articles/204059436
- **Database** password (optional, has default: scoutpass)

### Local Development (Without Docker)

For running services directly without containers, you can use TOML config files:
- `tvdb_proxy/api.toml`: TVDB API credentials (optional, can use env vars)
- `torrenter/config.toml`: Service settings (optional, can use env vars)

Or export environment variables from your `.env` file:
```bash
export $(cat .env | xargs)
```

## 🔧 Tech Stack

**Backend:**
- Go 1.23+
- Gin web framework
- PostgreSQL (shared database for all services)
- TOML configuration

**Frontend:**
- React 19
- TypeScript
- Material-UI
- Vite

**Infrastructure:**
- Docker Compose
- Docker multi-stage builds
- Named volumes (PostgreSQL) and bind mounts (/data)
- Nginx reverse proxy

**Integrations:**
- TVDB API v4 (media metadata)
- qBittorrent (torrent downloads)
- Plex Media Server (library management)
- Prowlarr (indexer management)

## 📁 Project Structure

```
Scout/
├── webserver/                    # Central coordinator service (Go)
│   ├── cmd/webserver/            # Entry point
│   │   └── main.go
│   └── internal/                 # Private packages
│       ├── handlers/             # HTTP handlers
│       ├── interactors/          # Business logic
│       ├── clients/              # Service clients
│       ├── repository/           # Database layer
│       ├── scheduler/            # Episode scheduler
│       └── config/               # Configuration
├── torrenter/                    # Download processor (Go)
│   ├── cmd/torrenter/            # Entry point
│   │   └── main.go
│   └── internal/                 # Private packages
│       ├── handlers/             # HTTP handlers
│       ├── interactors/          # Business logic
│       ├── service/              # Domain services (torrent, plex, media processor)
│       ├── repository/           # Database layer
│       ├── models/               # Data structures
│       └── config/               # Configuration
├── tvdb_proxy/                   # TVDB API proxy (Go)
│   └── main.go                   # Single file service
├── ui/                           # React frontend
│   ├── src/
│   │   ├── components/           # React components
│   │   ├── App.tsx
│   │   └── main.tsx
│   └── package.json
├── shared/                       # Shared Go modules
│   ├── media/                    # TVDB data structures
│   ├── status/                   # Download status types
│   └── telemetry/                # OpenTelemetry tracing utilities
├── sql/                          # Database setup and migration scripts
├── nginx/                        # Nginx reverse proxy configuration
├── docker-compose.yml            # Docker Compose orchestration
├── .env.template                 # Environment variables template
└── compose-deploy.sh             # Deployment and management script
```

## 🎯 Features

- **Media Search**: Search TVDB for TV shows and movies
- **Automatic Downloads**: Scheduled downloads for new episodes
- **Anime Support**: Detects and handles anime differently
- **Plex Integration**: Automatic library syncing
- **Episode Scheduling**: Downloads episodes as they air
- **Multi-season Support**: Batch download entire seasons

## 🔐 Security

**READ THIS BEFORE DEPLOYMENT:** Configure your `.env` file properly

Important security practices:
- Configure `.env` file from `.env.template` before deployment
- Never commit your `.env` file (`.gitignore` protects you)
- Rotate credentials if you suspect exposure
- Use strong, unique passwords for all services
- For production, consider external secret managers (HashiCorp Vault, AWS Secrets Manager, etc.)

The repository includes:
- `.env.template` file with all required variables documented
- Comprehensive deployment documentation in DEPLOYMENT.md
- `.gitignore` rules to prevent credential leaks

## 📚 Documentation

- [Deployment Guide](DEPLOYMENT.md) - Docker Compose setup, LAN access, troubleshooting
- [Architecture Overview](CLAUDE.md) - Microservices architecture, data flow, development commands
- [Observability](OBSERVABILITY.md) - OpenTelemetry tracing and structured logging
- Service READMEs: [webserver](webserver/README.md), [torrenter](torrenter/README.md), [tvdb_proxy](tvdb_proxy/README.md), [ui](ui/README.md)

## 🤝 Contributing

1. Make changes to a service
2. Test locally:
   - Go services: `go run cmd/<service>/main.go` (or `go run main.go` for tvdb_proxy)
   - UI: `npm run dev`
3. Test with Docker Compose: `./compose-deploy.sh rebuild <service>`
4. Check logs: `./compose-deploy.sh logs <service>`
5. Run tests: `go test ./...` (in service directory)
6. Run linter: `golangci-lint run` (in service directory)

## 📝 License

MIT
