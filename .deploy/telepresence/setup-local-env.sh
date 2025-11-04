#!/bin/bash
# Setup script for local development environment
# This prepares your machine for Telepresence-based development

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  Local Development Environment Setup${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Check Telepresence installation
echo -e "${YELLOW}📋 Checking prerequisites...${NC}"
if command -v telepresence &> /dev/null; then
    VERSION=$(telepresence version | grep "Client" | awk '{print $2}')
    echo -e "${GREEN}✅ Telepresence installed: ${VERSION}${NC}"
else
    echo -e "${RED}❌ Telepresence not found${NC}"
    echo ""
    echo "Install Telepresence:"
    echo "  macOS: brew install telepresence"
    echo "  Linux: See https://www.telepresence.io/docs/latest/install/"
    echo ""
    exit 1
fi

# Check kubectl
if command -v kubectl &> /dev/null; then
    echo -e "${GREEN}✅ kubectl installed${NC}"
else
    echo -e "${RED}❌ kubectl not found${NC}"
    exit 1
fi

# Check minikube
if command -v minikube &> /dev/null; then
    echo -e "${GREEN}✅ minikube installed${NC}"
else
    echo -e "${RED}❌ minikube not found${NC}"
    exit 1
fi

# Check Delve
if command -v dlv &> /dev/null; then
    DLV_VERSION=$(dlv version | grep "Version:" | awk '{print $2}')
    echo -e "${GREEN}✅ Delve installed: ${DLV_VERSION}${NC}"
else
    echo -e "${YELLOW}⚠️  Delve not found${NC}"
    echo ""
    echo "Install Delve for debugging:"
    echo "  go install github.com/go-delve/delve/cmd/dlv@latest"
    echo ""
fi

# Check goose
if command -v goose &> /dev/null; then
    echo -e "${GREEN}✅ goose installed${NC}"
else
    echo -e "${YELLOW}⚠️  goose not found (optional for manual migrations)${NC}"
    echo "  Install: go install github.com/pressly/goose/v3/cmd/goose@latest"
fi

echo ""
echo -e "${YELLOW}🔍 Checking cluster connectivity...${NC}"

# Check minikube status
if minikube status -p incloud &> /dev/null; then
    echo -e "${GREEN}✅ Minikube (incloud) is running${NC}"
else
    echo -e "${RED}❌ Minikube (incloud) is not running${NC}"
    echo "  Start it: minikube start -p incloud"
    exit 1
fi

# Check namespace
if kubectl get namespace incloud-sgroups &> /dev/null; then
    echo -e "${GREEN}✅ Namespace incloud-sgroups exists${NC}"
else
    echo -e "${RED}❌ Namespace incloud-sgroups not found${NC}"
    exit 1
fi

# Check deployments
echo ""
echo -e "${YELLOW}📦 Checking deployments...${NC}"
for deployment in netguard-backend netguard-apiserver netguard-webhook; do
    if kubectl get deployment $deployment -n incloud-sgroups &> /dev/null; then
        STATUS=$(kubectl get deployment $deployment -n incloud-sgroups -o jsonpath='{.status.conditions[?(@.type=="Available")].status}')
        if [ "$STATUS" == "True" ]; then
            echo -e "${GREEN}✅ $deployment is running${NC}"
        else
            echo -e "${YELLOW}⚠️  $deployment exists but not ready${NC}"
        fi
    else
        echo -e "${RED}❌ $deployment not found${NC}"
    fi
done

# Check PostgreSQL
if kubectl get statefulset postgresql -n incloud-sgroups &> /dev/null; then
    echo -e "${GREEN}✅ PostgreSQL is deployed${NC}"
else
    echo -e "${YELLOW}⚠️  PostgreSQL not found (may be deployed differently)${NC}"
fi

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ✅ Setup Check Complete${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "  1. Choose a service to debug:"
echo "     • Backend:   .deploy/telepresence/intercept-backend.sh"
echo "     • APIServer: .deploy/telepresence/intercept-apiserver.sh"
echo "     • Webhook:   .deploy/telepresence/intercept-webhook.sh"
echo ""
echo "  2. Or use Skaffold for automated workflow:"
echo "     • cd .deploy/skaffold"
echo "     • skaffold dev -p debug-backend"
echo ""
echo -e "${BLUE}Documentation:${NC}"
echo "  See .claude/knowledge/debug/ for detailed guides"
echo ""
