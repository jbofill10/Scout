#!/bin/bash

# Scout Development Helper Script for Minikube
# Quick commands for iterating on services during development

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

show_help() {
    echo "Scout Minikube Development Helper"
    echo ""
    echo "Usage: ./dev.sh [command] [service]"
    echo ""
    echo "Commands:"
    echo "  rebuild <service>  - Rebuild and redeploy a specific service"
    echo "  logs <service>     - Tail logs for a service"
    echo "  shell <service>    - Open a shell in the service pod"
    echo "  restart <service>  - Restart a service"
    echo "  status             - Show status of all pods"
    echo "  clean              - Delete all Scout resources"
    echo "  port-forward       - Set up port forwarding for direct service access"
    echo ""
    echo "Services: webserver, tvdb-proxy, torrenter, ui"
    echo ""
    echo "Examples:"
    echo "  ./dev.sh rebuild webserver"
    echo "  ./dev.sh logs torrenter"
    echo "  ./dev.sh status"
}

rebuild_service() {
    local service=$1
    echo -e "${YELLOW}Rebuilding $service...${NC}"

    # Set docker env for minikube
    eval $(minikube docker-env)

    case $service in
        webserver)
            docker build -f ./webserver/Dockerfile -t scout-webserver:latest .
            kubectl rollout restart deployment/webserver
            ;;
        tvdb)
            docker build -f ./tvdb_proxy/Dockerfile -t scout-tvdb-proxy:latest .
            kubectl rollout restart deployment/tvdb-proxy
            ;;
        torrenter)
            docker build -f torrenter/Dockerfile -t scout-torrenter:latest .
            kubectl rollout restart deployment/torrenter
            ;;
        ui)
            docker build -f ui/Dockerfile -t scout-ui:latest ui/
            kubectl rollout restart deployment/ui
            ;;
        *)
            echo -e "${RED}Unknown service: $service${NC}"
            exit 1
            ;;
    esac

    echo -e "${GREEN}✅ $service rebuilt and restarted${NC}"
    echo "Waiting for rollout..."
    kubectl rollout status deployment/$service
}

show_logs() {
    local service=$1
    echo -e "${BLUE}Tailing logs for $service...${NC}"
    kubectl logs -f deployment/$service
}

shell_service() {
    local service=$1
    echo -e "${BLUE}Opening shell in $service...${NC}"
    kubectl exec -it deployment/$service -- /bin/sh
}

restart_service() {
    local service=$1
    echo -e "${YELLOW}Restarting $service...${NC}"
    kubectl rollout restart deployment/$service
    kubectl rollout status deployment/$service
    echo -e "${GREEN}✅ $service restarted${NC}"
}

show_status() {
    echo -e "${BLUE}Scout Services Status:${NC}"
    echo ""
    kubectl get pods -l app=webserver -o wide
    kubectl get pods -l app=tvdb-proxy -o wide
    kubectl get pods -l app=torrenter -o wide
    kubectl get pods -l app=ui -o wide
    echo ""
    echo -e "${BLUE}Services:${NC}"
    kubectl get svc | grep -E 'NAME|scout|webserver|tvdb-proxy|torrenter|ui'
}

clean_all() {
    echo -e "${YELLOW}Cleaning all Scout resources...${NC}"
    kubectl delete -f k8s/deployments/ --ignore-not-found=true
    kubectl delete -f k8s/services/ --ignore-not-found=true
    kubectl delete -f k8s/ingress/ --ignore-not-found=true
    kubectl delete -f k8s/pvcs/ --ignore-not-found=true
    kubectl delete -f k8s/secrets/ --ignore-not-found=true
    kubectl delete -f k8s/configmaps/ --ignore-not-found=true
    echo -e "${GREEN}✅ All resources cleaned${NC}"
}

# Main command routing
case ${1:-help} in
    rebuild)
        rebuild_service $2
        ;;
    logs)
        show_logs $2
        ;;
    shell)
        shell_service $2
        ;;
    restart)
        restart_service $2
        ;;
    status)
        show_status
        ;;
    clean)
        clean_all
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo "Unknown command: $1"
        show_help
        exit 1
        ;;
esac