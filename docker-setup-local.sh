#!/bin/bash
# ==============================================================================
# Script Name: docker-setup-local.sh (Root Workspace Version)
# Description: Manages starting, stopping, and building containers locally.
# ==============================================================================

set -euo pipefail

# Disable path conversion in Git Bash / MSYS on Windows
export MSYS_NO_PATHCONV=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# Container & Service Names
SERVICE_NAME="rutils-app"
CONTAINER_NAME="rutils-local"

usage() {
    echo "Usage: $0 {start|stop|restart|build|status|logs}"
    exit 1
}

start_local() {
    echo "=================================================="
    echo "🚀 Starting Local Containers via Docker Compose..."
    echo "=================================================="
    
    cd "$SCRIPT_DIR"
    docker compose -f ./docker-compose.yml up -d

    echo "=================================================="
    echo "🎉 Local Docker Setup Started Successfully!"
    echo "🌐 Frontend Interface : http://localhost:3000"
    echo "🌐 Backend API & Docs : http://localhost:8080/swagger/"
    echo "📁 Local Storage DB   : ./data"
    echo "=================================================="
}

build_local() {
    local target_service="${2:-}"
    echo "=================================================="
    echo "🏗️  Building Local Docker Images..."
    echo "=================================================="
    cd "$SCRIPT_DIR"
    if [ -n "$target_service" ]; then
        echo "Building target service: $target_service..."
        docker compose -f ./docker-compose.yml build "$target_service"
    else
        echo "Building all modified services..."
        docker compose -f ./docker-compose.yml build
    fi
    echo "✅ Rebuild completed."
}

stop_local() {
    echo "🧹 Stopping and removing local Docker containers..."
    cd "$SCRIPT_DIR"
    docker compose -f ./docker-compose.yml down
    echo "✅ Containers stopped and removed."
}

status_local() {
    echo "=================================================="
    echo "📊 Checking Local Container Status"
    echo "=================================================="
    cd "$SCRIPT_DIR"
    docker compose -f ./docker-compose.yml ps -a
    
    echo ""
    echo "📊 Application Logs (last 10 lines):"
    docker compose -f ./docker-compose.yml logs --tail 10 "$SERVICE_NAME" 2>/dev/null || echo "No logs available."
}

logs_local() {
    cd "$SCRIPT_DIR"
    docker compose -f ./docker-compose.yml logs -f "$SERVICE_NAME"
}

if [ $# -lt 1 ]; then
    usage
fi

case "$1" in
    start)
        start_local "$@"
        ;;
    stop)
        stop_local "$@"
        ;;
    build)
        build_local "$@"
        ;;
    restart)
        stop_local "$@"
        sleep 1
        start_local "$@"
        ;;
    status)
        status_local "$@"
        ;;
    logs)
        logs_local "$@"
        ;;
    *)
        usage
        ;;
esac
