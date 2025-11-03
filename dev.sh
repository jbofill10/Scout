#!/bin/bash
set -e

# Scout Development Helper Script
# Quick rebuild and restart for iterative development

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check if required configuration files exist
check_config() {
    if [ ! -f "docker-compose.yml" ]; then
        log_error "docker-compose.yml not found!"
        log_info "Please copy docker-compose.yml.template to docker-compose.yml:"
        log_info "  cp docker-compose.yml.template docker-compose.yml"
        exit 1
    fi
}

# Available services
SERVICES=("webserver" "tvdb-proxy" "torrenter" "ui" "nginx" "postgres")

# Show usage
usage() {
    echo "Scout Development Helper"
    echo ""
    echo "Usage:"
    echo "  ./dev.sh rebuild <service>     - Rebuild and restart a service"
    echo "  ./dev.sh logs <service>        - Follow logs for a service"
    echo "  ./dev.sh shell <service>       - Open shell in service container"
    echo "  ./dev.sh restart <service>     - Restart a service (no rebuild)"
    echo "  ./dev.sh ps                    - Show service status"
    echo "  ./dev.sh down                  - Stop all services"
    echo "  ./dev.sh up                    - Start all services"
    echo ""
    echo "Services: ${SERVICES[*]}"
    echo ""
    echo "Examples:"
    echo "  ./dev.sh rebuild webserver     # Quick rebuild after code changes"
    echo "  ./dev.sh logs torrenter        # Watch torrenter logs"
    echo "  ./dev.sh shell webserver       # Debug inside container"
    exit 0
}

# Validate service name
validate_service() {
    local service=$1
    if [[ ! " ${SERVICES[@]} " =~ " ${service} " ]]; then
        log_error "Invalid service: $service"
        log_info "Available services: ${SERVICES[*]}"
        exit 1
    fi
}

# Rebuild a service
rebuild() {
    check_config
    local service=$1

    if [ -z "$service" ]; then
        log_error "Please specify a service to rebuild"
        log_info "Usage: ./dev.sh rebuild <service>"
        log_info "Available: ${SERVICES[*]}"
        exit 1
    fi

    validate_service "$service"

    log_info "Rebuilding $service..."
    docker compose build "$service"

    log_info "Restarting $service..."
    docker compose up -d "$service"

    log_success "$service rebuilt and restarted!"
    log_info "View logs with: ./dev.sh logs $service"
}

# View logs
logs() {
    check_config
    local service=$1

    if [ -z "$service" ]; then
        log_info "Following logs for all services (Ctrl+C to exit)..."
        docker compose logs -f
    else
        validate_service "$service"
        log_info "Following logs for $service (Ctrl+C to exit)..."
        docker compose logs -f "$service"
    fi
}

# Open shell in container
shell() {
    check_config
    local service=$1

    if [ -z "$service" ]; then
        log_error "Please specify a service"
        log_info "Usage: ./dev.sh shell <service>"
        log_info "Available: ${SERVICES[*]}"
        exit 1
    fi

    validate_service "$service"

    log_info "Opening shell in $service container..."

    # Try sh first (alpine containers), fallback to bash
    if docker compose exec "$service" sh -c "exit" 2>/dev/null; then
        docker compose exec "$service" sh
    else
        docker compose exec "$service" bash
    fi
}

# Restart service (no rebuild)
restart() {
    check_config
    local service=$1

    if [ -z "$service" ]; then
        log_error "Please specify a service to restart"
        log_info "Usage: ./dev.sh restart <service>"
        log_info "Available: ${SERVICES[*]}"
        exit 1
    fi

    validate_service "$service"

    log_info "Restarting $service..."
    docker compose restart "$service"
    log_success "$service restarted!"
}

# Show status
status() {
    check_config
    log_info "Service status:"
    docker compose ps
}

# Stop all services
down() {
    check_config
    log_info "Stopping all services..."
    docker compose down
    log_success "All services stopped!"
}

# Start all services
up() {
    check_config
    log_info "Starting all services..."
    docker compose up -d
    log_success "All services started!"
    status
}

# Main command dispatcher
case "$1" in
    rebuild)
        rebuild "$2"
        ;;
    logs)
        logs "$2"
        ;;
    shell)
        shell "$2"
        ;;
    restart)
        restart "$2"
        ;;
    ps|status)
        status
        ;;
    down|stop)
        down
        ;;
    up|start)
        up
        ;;
    -h|--help|help|"")
        usage
        ;;
    *)
        log_error "Unknown command: $1"
        usage
        ;;
esac
