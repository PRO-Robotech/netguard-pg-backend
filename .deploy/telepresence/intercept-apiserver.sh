#!/bin/bash
# Telepresence intercept script for apiserver service

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

NAMESPACE="${NAMESPACE:-incloud-sgroups}"
DEPLOYMENT="netguard-apiserver"
LOCAL_PORT="${LOCAL_PORT:-8443}"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Telepresence APIServer Intercept${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

if ! command -v telepresence &> /dev/null; then
    echo -e "${RED}❌ Telepresence is not installed${NC}"
    exit 1
fi

if ! kubectl get deployment ${DEPLOYMENT} -n ${NAMESPACE} &> /dev/null; then
    echo -e "${RED}❌ APIServer deployment not found${NC}"
    exit 1
fi

echo -e "${YELLOW}📡 Connecting to Telepresence...${NC}"
telepresence connect

echo ""
echo -e "${YELLOW}🎯 Intercepting apiserver traffic...${NC}"
telepresence intercept ${DEPLOYMENT} \
    --namespace ${NAMESPACE} \
    --port ${LOCAL_PORT}:8443 \
    --env-file /tmp/apiserver-env.txt

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ✅ Intercept Active!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Environment variables saved to:${NC} /tmp/apiserver-env.txt"
echo ""
echo -e "${YELLOW}To start apiserver locally:${NC}"
echo "  export \$(cat /tmp/apiserver-env.txt | xargs)"
echo "  export BACKEND_ENDPOINT=\"netguard-backend.incloud-sgroups.svc:9090\""
echo "  dlv debug ./cmd/k8s-apiserver/main.go --headless --listen=:2346 --api-version=2"
echo ""
echo -e "${RED}To stop intercept:${NC}"
echo "  telepresence leave ${DEPLOYMENT}-${NAMESPACE}"
echo ""

trap "echo ''; telepresence leave ${DEPLOYMENT}-${NAMESPACE}; echo -e '${GREEN}✅ Intercept stopped${NC}'" INT TERM

while true; do
    sleep 1
done
