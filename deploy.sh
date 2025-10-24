#!/bin/bash

# Scout Kubernetes Deployment Script for Minikube
# Consolidated deployment with pre-flight checks and mount setup

set -euo pipefail

# Colors for better output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
SKIP_CHECKS=false
SETUP_MOUNTS=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-checks)
            SKIP_CHECKS=true
            shift
            ;;
        --setup-mounts)
            SETUP_MOUNTS=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-checks    Skip pre-flight validation checks"
            echo "  --setup-mounts   Set up minikube mount processes in background"
            echo "  -h, --help       Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                        # Full deployment with checks"
            echo "  $0 --skip-checks          # Quick redeploy"
            echo "  $0 --setup-mounts         # Deploy and setup mounts"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use -h or --help for usage information"
            exit 1
            ;;
    esac
done

# ============================================================================
# PRE-FLIGHT CHECKS
# ============================================================================

if [ "$SKIP_CHECKS" = false ]; then
    echo -e "${BLUE}🔍 Running pre-flight checks...${NC}"
    echo ""

    ERRORS=0
    WARNINGS=0

    # Check if Minikube is installed
    echo -n "Checking Minikube installation... "
    if command -v minikube &> /dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo -e "${RED}  Error: Minikube not found. Install from: https://minikube.sigs.k8s.io/docs/start/${NC}"
        ((ERRORS++))
    fi

    # Check if kubectl is installed
    echo -n "Checking kubectl installation... "
    if command -v kubectl &> /dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo -e "${RED}  Error: kubectl not found. Install from: https://kubernetes.io/docs/tasks/tools/${NC}"
        ((ERRORS++))
    fi

    # Check if Docker is installed
    echo -n "Checking Docker installation... "
    if command -v docker &> /dev/null; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo -e "${RED}  Error: Docker not found.${NC}"
        ((ERRORS++))
    fi

    # Check ConfigMap files
    echo ""
    echo -e "${BLUE}Kubernetes ConfigMaps:${NC}"

    echo -n "  k8s/configmaps/tvdb-proxy-configmap.yaml... "
    if [ -f "k8s/configmaps/tvdb-proxy-configmap.yaml" ]; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ((ERRORS++))
    fi

    echo -n "  k8s/configmaps/torrenter-configmap.yaml... "
    if [ -f "k8s/configmaps/torrenter-configmap.yaml" ]; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ((ERRORS++))
    fi

    # Check Secret files
    echo ""
    echo -e "${BLUE}Kubernetes Secrets:${NC}"

    echo -n "  k8s/secrets/tvdb-proxy-secrets.yaml... "
    if [ -f "k8s/secrets/tvdb-proxy-secrets.yaml" ]; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ((ERRORS++))
    fi

    echo -n "  k8s/secrets/torrenter-secrets.yaml... "
    if [ -f "k8s/secrets/torrenter-secrets.yaml" ]; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        ((ERRORS++))
    fi

    # Check Dockerfiles
    echo ""
    echo -e "${BLUE}Dockerfiles:${NC}"

    for service in webserver tvdb_proxy torrenter ui; do
        echo -n "  $service/Dockerfile... "
        if [ -f "$service/Dockerfile" ]; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            ((ERRORS++))
        fi
    done

    # Summary
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

    if [ $ERRORS -gt 0 ]; then
        echo -e "${RED}❌ $ERRORS error(s) found. Fix errors before deploying.${NC}"
        if [ $WARNINGS -gt 0 ]; then
            echo -e "${YELLOW}⚠ $WARNINGS warning(s) also found.${NC}"
        fi
        exit 1
    elif [ $WARNINGS -gt 0 ]; then
        echo -e "${YELLOW}⚠ $WARNINGS warning(s) found. Deployment may work but check warnings above.${NC}"
    else
        echo -e "${GREEN}✅ All checks passed!${NC}"
    fi
    echo ""
fi

# ============================================================================
# MINIKUBE STATUS CHECK
# ============================================================================

echo -e "${BLUE}🚀 Starting Scout deployment to Minikube...${NC}"
echo ""

echo -e "${BLUE}Checking Minikube status...${NC}"
if ! minikube status &>/dev/null; then
    echo -e "${RED}❌ Minikube is not running.${NC}"
    echo -e "${YELLOW}Start Minikube with:${NC}"
    echo "  minikube start"
    echo ""
    echo -e "${YELLOW}For persistent mounts (optional):${NC}"
    echo "  minikube start --mount --mount-string=\"/data/Media:/data/Media\" --mount-string=\"/data/Downloads:/data/Downloads\""
    echo ""
    exit 1
fi

echo -e "${GREEN}✅ Minikube is running${NC}"

# ============================================================================
# SETUP MINIKUBE MOUNTS (OPTIONAL)
# ============================================================================

if [ "$SETUP_MOUNTS" = true ]; then
    echo -e "${YELLOW}📁 Setting up Minikube mounts in background...${NC}"

    # Kill any existing mount processes
    pkill -f "minikube mount" || true

    # Start mount processes in background
    minikube mount /data/Media:/data/Media --uid=1000 --gid=1000 &
    minikube mount /data/Downloads:/data/Downloads --uid=1000 --gid=1000 &

    echo -e "${GREEN}✓ Mount processes started${NC}"
    echo -e "${YELLOW}Note: These processes will run in the background. Kill them with: pkill -f 'minikube mount'${NC}"
    echo ""
    sleep 2
fi

# ============================================================================
# ENABLE INGRESS
# ============================================================================

echo -e "${YELLOW}📦 Ensuring Minikube ingress addon is enabled...${NC}"
minikube addons enable ingress || true

# ============================================================================
# DOCKER GROUP CHECK
# ============================================================================

echo -e "${YELLOW}🐳 Preparing to build images inside Minikube's Docker daemon...${NC}"
if ! groups "$USER" | grep -qw docker; then
    echo -e "${YELLOW}Note: Your user is not in the 'docker' group. You can either:
    1) Add yourself to the docker group: sudo usermod -aG docker $USER && newgrp docker
    2) Or run the script with a user that already has Docker access.${NC}"
fi

# ============================================================================
# BUILD DOCKER IMAGES
# ============================================================================

echo -e "${BLUE}Switching Docker to use Minikube's Docker daemon...${NC}"
eval "$(minikube docker-env)"

echo -e "${YELLOW}🏗️  Building Docker images inside Minikube's daemon...${NC}"

build_image() {
    local name="$1"; shift
    local dockerfile="$1"; shift || true
    echo -e "${BLUE}Building ${name}...${NC}"
    if [ -n "${dockerfile:-}" ]; then
        docker build -f "${dockerfile}" -t "${name}:latest" .
    else
        docker build -t "${name}:latest" "$2"
    fi
}

build_image "scout-webserver" "webserver/Dockerfile"
build_image "scout-tvdb-proxy" "tvdb_proxy/Dockerfile"
build_image "scout-torrenter" "torrenter/Dockerfile"
echo -e "${BLUE}Building UI...${NC}"
docker build -t scout-ui:latest ./ui

echo -e "${GREEN}✅ All Docker images built successfully!${NC}"

# ============================================================================
# APPLY KUBERNETES MANIFESTS
# ============================================================================

echo -e "${YELLOW}⚙️  Applying Kubernetes manifests...${NC}"

apply_if_exists() {
    local path="$1"
    if [ -d "$path" ] || [ -f "$path" ]; then
        echo "  - Applying ${path}"
        kubectl apply -f "$path"
    else
        echo "  - Skipping ${path} (not present)"
    fi
}

apply_if_exists k8s/configmaps/
apply_if_exists k8s/secrets/
apply_if_exists k8s/pvcs/
apply_if_exists k8s/services/
apply_if_exists k8s/services/qbittorrent-external.yaml
apply_if_exists k8s/services/plex-external.yaml
apply_if_exists k8s/deployments/
apply_if_exists k8s/ingress/

# Quick check: warn if qbittorrent endpoints are missing
if ! kubectl get endpoints qbittorrent-service -o yaml >/dev/null 2>&1; then
    echo -e "${YELLOW}Warning: qbittorrent-service endpoints not found. If you use an external qBittorrent, ensure k8s/services/qbittorrent-external.yaml has the correct host IP.${NC}"
fi

# ============================================================================
# WAIT FOR DEPLOYMENTS
# ============================================================================

echo -e "${YELLOW}⏳ Waiting for deployments to be ready...${NC}"
for d in webserver tvdb-proxy torrenter ui; do
    if kubectl get deployment "$d" &>/dev/null; then
        kubectl wait --for=condition=available --timeout=60s deployment/"$d" || echo -e "${RED}Warning: ${d} not ready${NC}"
    fi
done

# ============================================================================
# SUCCESS MESSAGE
# ============================================================================

echo ""
echo -e "${GREEN}✅ Deployment complete!${NC}"
echo ""
echo -e "${BLUE}🌐 Access your application:${NC}"
echo "  UI:  http://scout.local"
echo "  API: http://scout.local/api"
echo ""
echo -e "${YELLOW}📝 To access from your host, add to /etc/hosts:${NC}"
echo "  $(minikube ip) scout.local"
echo ""
echo -e "${BLUE}🔍 Useful commands:${NC}"
echo "  View pods:        kubectl get pods"
echo "  View services:    kubectl get svc"
echo "  View logs:        kubectl logs -f deployment/<service-name>"
echo "  Restart service:  kubectl rollout restart deployment/<service-name>"
echo "  Development:      ./dev.sh help"
echo ""
if [ "$SETUP_MOUNTS" = true ]; then
    echo -e "${BLUE}📁 Mount info:${NC}"
    echo "  Active mounts: /data/Media and /data/Downloads"
    echo "  Stop mounts:   pkill -f 'minikube mount'"
    echo ""
fi
echo -e "${GREEN}🎉 Happy developing!${NC}"
