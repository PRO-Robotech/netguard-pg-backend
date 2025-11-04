#!/bin/bash
# Telepresence intercept script for backend service
# This script sets up local development environment by intercepting backend traffic

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-incloud-sgroups}"
DEPLOYMENT="netguard-backend"
SERVICE="netguard-backend"
LOCAL_PORT="${LOCAL_PORT:-9090}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Telepresence Backend Intercept${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check if telepresence is installed
if ! command -v telepresence &> /dev/null; then
    echo -e "${RED}❌ Telepresence is not installed${NC}"
    echo "Install it from: https://www.telepresence.io/docs/latest/install/"
    exit 1
fi

# Check if minikube is running
if ! kubectl get nodes &> /dev/null; then
    echo -e "${RED}❌ Cannot connect to Kubernetes cluster${NC}"
    echo "Make sure minikube is running: minikube start -p incloud"
    exit 1
fi

# Check if backend deployment exists
if ! kubectl get deployment ${DEPLOYMENT} -n ${NAMESPACE} &> /dev/null; then
    echo -e "${RED}❌ Backend deployment not found in namespace ${NAMESPACE}${NC}"
    echo "Deploy backend first: ./deploy-backend-only.sh"
    exit 1
fi

echo -e "${YELLOW}📡 Connecting to Telepresence...${NC}"
telepresence connect

echo ""
echo -e "${YELLOW}🔌 Setting up port-forward for PostgreSQL...${NC}"
echo -e "${GREEN}PostgreSQL will be available at localhost:5432${NC}"

# Start PostgreSQL port-forward in background
kubectl port-forward svc/postgresql 5432:5432 -n ${NAMESPACE} > /tmp/pgsql-port-forward.log 2>&1 &
PG_PID=$!
echo "PostgreSQL port-forward PID: $PG_PID"

# Wait a bit for port-forward to establish
sleep 2

echo ""
echo -e "${YELLOW}🎯 Intercepting backend traffic...${NC}"
echo -e "  Deployment: ${DEPLOYMENT}"
echo -e "  Namespace: ${NAMESPACE}"
echo -e "  Local port: ${LOCAL_PORT}"
echo ""

# Create intercept
telepresence intercept ${DEPLOYMENT} \
    --namespace ${NAMESPACE} \
    --port ${LOCAL_PORT}:9090 \
    --env-file /tmp/backend-env.txt

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ✅ Intercept Active!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Environment setup:${NC}"
echo -e "  • Backend traffic intercepted"
echo -e "  • PostgreSQL: localhost:5432"
echo -e "  • SGROUPS: sgroups-server.incloud-sgroups.svc:9006 (via Telepresence DNS)"
echo ""
echo -e "${BLUE}Environment variables saved to:${NC} /tmp/backend-env.txt"
echo ""
echo -e "${YELLOW}To start backend locally:${NC}"
echo "  export \$(cat /tmp/backend-env.txt | xargs)"
echo "  export DATABASE_CONNECTION_URL=\"postgres://netguard:netguard@localhost:5432/netguard?sslmode=disable\""
echo "  dlv debug ./cmd/server/main.go --headless --listen=:2345 --api-version=2 -- --config=config/config.yaml"
echo ""
echo -e "${YELLOW}Or run without debugger:${NC}"
echo "  export \$(cat /tmp/backend-env.txt | xargs)"
echo "  export DATABASE_CONNECTION_URL=\"postgres://netguard:netguard@localhost:5432/netguard?sslmode=disable\""
echo "  go run ./cmd/server/main.go --config=config/config.yaml --pg-uri=\$DATABASE_CONNECTION_URL"
echo ""
echo -e "${RED}To stop intercept:${NC}"
echo "  telepresence leave ${DEPLOYMENT}-${NAMESPACE}"
echo "  kill $PG_PID  # Stop PostgreSQL port-forward"
echo ""
echo -e "${BLUE}Press Ctrl+C to stop intercept${NC}"

# Wait for user to stop
trap "echo ''; echo -e '${YELLOW}Cleaning up...${NC}'; telepresence leave ${DEPLOYMENT}-${NAMESPACE}; kill $PG_PID 2>/dev/null; echo -e '${GREEN}✅ Intercept stopped${NC}'" INT TERM

# Keep script running
while true; do
    sleep 1
done
