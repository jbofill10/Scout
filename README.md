# Scout

Over-engineered microservices architecture for automated media torrenting with a web interface.

## ⚠️ Security Notice

**Before deploying Scout, you must configure secrets properly.** This repository does not include actual credentials.

1. Copy template files in `k8s/secrets/*.template.yaml` and fill in your values
2. Never commit files containing real credentials
3. See [SECURITY.md](SECURITY.md) for detailed setup instructions

## 🏗️ Architecture

Scout is built as four microservices that work together:

- **webserver** (Go): Central coordinator, REST API, episode scheduling
- **tvdb_proxy** (Go): TVDB API proxy for media search and metadata
- **torrenter** (Go): Download processor with qBittorrent and Plex integration
- **ui** (React/TypeScript): Web interface for search and management

## 🚀 Quick Start with Kubernetes (Recommended)

### Prerequisites
- [Minikube](https://minikube.sigs.k8s.io/docs/start/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- Docker

### Deploy to Minikube
```bash
# Start Minikube
minikube start --cpus=4 --memory=8192

# Deploy all services
./deploy-minikube.sh

# Add to /etc/hosts
echo "$(minikube ip) scout.local" | sudo tee -a /etc/hosts

# Access the UI
open http://scout.local
```

### Development Workflow
```bash
# Rebuild a service after changes
./dev.sh rebuild webserver

# View logs
./dev.sh logs torrenter

# Check status
./dev.sh status

# See all commands
./dev.sh help
```

See [k8s/README.md](k8s/README.md) for detailed documentation.

## 🐳 Running Locally (Without K8s)

Each service can run independently for development:

### 1. Start tvdb_proxy
```bash
cd tvdb_proxy
go run *.go
# Listens on localhost:22000
```

### 2. Start torrenter
```bash
cd torrenter
go run *.go
# Listens on localhost:22001
```

### 3. Start webserver
```bash
cd webserver
go run *.go
# Listens on 192.168.0.111:22920
```

### 4. Start UI
```bash
cd ui
npm install
npm run dev
# Listens on localhost:5173
```

## 📋 Configuration

### ⚠️ Required Setup

**You must configure secrets and configmaps before deployment.** See [SECURITY.md](SECURITY.md) for complete instructions.

Quick setup:
```bash
# Copy configmap templates (for environment-specific settings)
cp k8s/configmaps/torrenter-configmap.template.yaml k8s/configmaps/torrenter-configmap.yaml
cp k8s/configmaps/tvdb-proxy-configmap.template.yaml k8s/configmaps/tvdb-proxy-configmap.yaml

# Copy secret templates (for credentials)
cp k8s/secrets/postgres-secret.template.yaml k8s/secrets/postgres-secret.yaml
cp k8s/secrets/torrenter-secrets.template.yaml k8s/secrets/torrenter-secrets.yaml
cp k8s/secrets/tvdb-proxy-secrets.template.yaml k8s/secrets/tvdb-proxy-secrets.yaml

# Edit configmaps and replace <YOUR_LAN_IP> with your actual IP
# Edit secrets and replace placeholders with base64-encoded values
# See SECURITY.md for detailed instructions
```

### Local Development Configuration

Each service has its own TOML config file:

- `tvdb_proxy/api.toml`: TVDB API credentials (not in repo - create your own)
- `torrenter/config.toml`: qBittorrent, Plex, Prowlarr settings (not in repo - create your own)
- No config needed for webserver and ui

### Kubernetes Configuration
In Kubernetes, configs are managed via:
- **ConfigMaps**: Environment-specific settings (IPs, ports, library IDs) - create from templates
- **Secrets**: Sensitive credentials (API keys, passwords) - create from templates

Both actual configmaps and secrets are gitignored. Only templates are committed to the repository.

## 🔧 Tech Stack

**Backend:**
- Go 1.21+
- Gin web framework
- SQLite (scheduler and media databases)
- TOML configuration

**Frontend:**
- React 19
- TypeScript
- Material-UI
- Vite

**Infrastructure:**
- Kubernetes (Minikube for local)
- Docker multi-stage builds
- Persistent volumes for databases

**Integrations:**
- TVDB API v4 (media metadata)
- qBittorrent (torrent downloads)
- Plex Media Server (library management)
- Prowlarr (indexer management)

## 📁 Project Structure

```
Scout/
├── webserver/          # Central coordinator service
├── tvdb_proxy/         # TVDB API proxy
├── torrenter/          # Download processor
├── ui/                 # React frontend
├── shared/             # Shared Go types
│   └── media/          # TVDB data structures
├── k8s/                # Kubernetes manifests
│   ├── configmaps/     # Service configurations
│   ├── secrets/        # Sensitive data (gitignored, use templates)
│   ├── deployments/    # Pod definitions
│   ├── services/       # Service discovery
│   ├── pvcs/           # Persistent storage
│   └── ingress/        # External access
├── deploy-minikube.sh  # Automated deployment
└── dev.sh              # Development helpers
```

## 🎯 Features

- **Media Search**: Search TVDB for TV shows and movies
- **Automatic Downloads**: Scheduled downloads for new episodes
- **Anime Support**: Detects and handles anime differently
- **Plex Integration**: Automatic library syncing
- **Episode Scheduling**: Downloads episodes as they air
- **Multi-season Support**: Batch download entire seasons

## 🔐 Security

**READ THIS BEFORE DEPLOYMENT:** [SECURITY.md](SECURITY.md)

Important security practices:
- All secrets must be configured from templates before deployment
- Never commit actual credentials (`.gitignore` protects you)
- Rotate credentials if you suspect exposure
- Use environment-specific secrets for dev/staging/prod
- Consider Sealed Secrets or external secret managers for production

The repository includes:
- `.template.yaml` files with placeholders for all required secrets
- Comprehensive security documentation in SECURITY.md
- `.gitignore` rules to prevent credential leaks

## 📚 Documentation

- [Kubernetes Setup Guide](k8s/README.md)
- [Development Workflow](k8s/README.md#-development-workflow)
- [Configuration Strategy](k8s/README.md#-configuration-strategy)
- [Troubleshooting](k8s/README.md#-troubleshooting)

## 🤝 Contributing

1. Make changes to a service
2. Test locally: `go run *.go` or `npm run dev`
3. Test in K8s: `./dev.sh rebuild <service>`
4. Check logs: `./dev.sh logs <service>`

## 📝 License

MIT
