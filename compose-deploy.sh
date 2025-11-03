#!/bin/bash
set -e

# Scout Docker-Compose Deployment Script
# Usage:
#   ./compose-deploy.sh          - Full deployment (build + start)
#   ./compose-deploy.sh rebuild  - Rebuild all services and restart
#   ./compose-deploy.sh rebuild <service> - Rebuild specific service

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Pre-flight checks
preflight_checks() {
    log_info "Running pre-flight checks..."

    # Check if docker is installed
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    # Check if docker compose is installed
    if ! command -v docker compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi

    # Check if Docker daemon is running
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running. Please start Docker first."
        exit 1
    fi

    # Check if template files have been copied
    if [ ! -f ".env" ]; then
        log_error ".env file not found!"
        log_info "Please copy .env.template to .env and fill in your secrets:"
        log_info "  cp .env.template .env"
        log_info "  nano .env  # or use your preferred editor"
        exit 1
    fi

    if [ ! -f "docker-compose.yml" ]; then
        log_error "docker-compose.yml file not found!"
        log_info "Please copy docker-compose.yml.template to docker-compose.yml:"
        log_info "  cp docker-compose.yml.template docker-compose.yml"
        log_info "  # Optionally customize for your environment"
        exit 1
    fi

    if [ ! -f "nginx/nginx.conf" ]; then
        log_error "nginx/nginx.conf file not found!"
        log_info "Please copy nginx/nginx.conf.template to nginx/nginx.conf:"
        log_info "  cp nginx/nginx.conf.template nginx/nginx.conf"
        log_info "  # Optionally customize for your environment"
        exit 1
    fi

    # Verify required directories exist
    if [ ! -d "/data/Downloads" ]; then
        log_warn "/data/Downloads directory does not exist. Torrenter will fail to access downloads."
        log_warn "Please create it with: sudo mkdir -p /data/Downloads"
    fi

    if [ ! -d "/data/Media" ]; then
        log_warn "/data/Media directory does not exist. Torrenter will fail to access media."
        log_warn "Please create it with: sudo mkdir -p /data/Media"
    fi

    # Verify required files exist
    local required_files=(
        "docker-compose.yml"
        "nginx/nginx.conf"
        "sql/setup-postgres.sql"
        "webserver/Dockerfile"
        "tvdb_proxy/Dockerfile"
        "torrenter/Dockerfile"
        "ui/Dockerfile"
    )

    for file in "${required_files[@]}"; do
        if [ ! -f "$file" ]; then
            log_error "Required file not found: $file"
            exit 1
        fi
    done

    log_success "Pre-flight checks passed!"
}

# Build images
build_images() {
    local service=$1

    if [ -z "$service" ]; then
        log_info "Building all Docker images..."
        docker compose build --no-cache
    else
        log_info "Building $service image..."
        docker compose build --no-cache "$service"
    fi

    log_success "Build completed!"
}

# Start services
start_services() {
    log_info "Starting Scout services..."
    docker compose up -d

    log_info "Waiting for services to be healthy..."
    sleep 5

    # Wait for postgres to be healthy
    log_info "Waiting for PostgreSQL to be ready..."
    timeout=60
    elapsed=0
    while [ $elapsed -lt $timeout ]; do
        if docker compose exec -T postgres pg_isready -U scoutuser -d scoutdb &> /dev/null; then
            log_success "PostgreSQL is ready!"
            break
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    if [ $elapsed -ge $timeout ]; then
        log_warn "PostgreSQL health check timed out, but continuing..."
    fi

    log_success "Scout services are starting up!"
}

# Show status
show_status() {
    log_info "Service status:"
    docker compose ps

    echo ""
    log_info "Access Scout at:"
    log_success "  UI:      http://localhost:30030"
    log_success "  UI (LAN): http://$(hostname -I | awk '{print $1}'):30030"
    log_success "  API:     http://localhost:22920"
    echo ""
    log_info "View logs with:"
    log_info "  docker compose logs -f [service_name]"
    log_info ""
    log_info "Available services: postgres, tvdb-proxy, torrenter, webserver, ui, nginx"
}

# Stop services
stop_services() {
    log_info "Stopping Scout services..."
    docker compose down
    log_success "Services stopped!"
}

# Restart specific service
restart_service() {
    local service=$1

    if [ -z "$service" ]; then
        log_error "Please specify a service to restart"
        log_info "Available services: postgres, tvdb-proxy, torrenter, webserver, ui, nginx"
        exit 1
    fi

    log_info "Restarting $service..."
    docker compose restart "$service"
    log_success "$service restarted!"
}

# Rebuild and restart
rebuild() {
    local service=$1

    preflight_checks
    build_images "$service"

    if [ -z "$service" ]; then
        log_info "Restarting all services..."
        docker compose up -d
    else
        log_info "Restarting $service..."
        docker compose up -d "$service"
    fi

    log_success "Rebuild and restart completed!"
    show_status
}

# Main script logic
main() {
    local command=$1
    local service=$2

    case "$command" in
        rebuild)
            rebuild "$service"
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_service "$service"
            ;;
        status)
            show_status
            ;;
        logs)
            if [ -z "$service" ]; then
                docker compose logs -f
            else
                docker compose logs -f "$service"
            fi
            ;;
        *)
            # Default: full deployment
            preflight_checks
            build_images
            start_services
            show_status
            ;;
    esac
}

# Show usage if --help or -h is passed
if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    echo "Scout Docker-Compose Deployment Script"
    echo ""
    echo "Usage:"
    echo "  ./compose-deploy.sh                  - Full deployment (build + start)"
    echo "  ./compose-deploy.sh rebuild          - Rebuild all services and restart"
    echo "  ./compose-deploy.sh rebuild <service> - Rebuild specific service"
    echo "  ./compose-deploy.sh stop             - Stop all services"
    echo "  ./compose-deploy.sh restart <service> - Restart specific service"
    echo "  ./compose-deploy.sh status           - Show service status"
    echo "  ./compose-deploy.sh logs [service]   - View logs (all or specific service)"
    echo ""
    echo "Services: postgres, tvdb-proxy, torrenter, webserver, ui, nginx"
    echo ""
    echo "Examples:"
    echo "  ./compose-deploy.sh                  # Deploy everything"
    echo "  ./compose-deploy.sh rebuild torrenter # Rebuild just torrenter"
    echo "  ./compose-deploy.sh logs webserver   # View webserver logs"
    exit 0
fi

# Run main script
main "$@"
