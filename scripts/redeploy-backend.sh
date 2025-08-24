#!/bin/bash

set -euo pipefail

# Parse mode parameter
MODE="${1:-postgresql}"
if [[ "$MODE" != "memory" && "$MODE" != "postgresql" ]]; then
    echo "❌ Invalid mode: $MODE"
    echo "Usage: $0 [memory|postgresql]"
    echo "  postgresql  - Deploy with PostgreSQL backend (default)"
    echo "  memory      - Deploy with in-memory backend"
    exit 1
fi

NAMESPACE="netguard-system"
DEPLOYMENT_NAME="netguard-backend"
IMAGE_NAME="netguard/pg-backend:latest"
MINIKUBE_PROFILE="incloud"
OVERLAY_PATH="config/k8s/overlays/$MODE"

echo "🚀 Starting Netguard Backend redeploy in $MODE mode..."
echo "============================================"
echo "📋 Mode: $MODE"
echo "📁 Overlay: $OVERLAY_PATH"

# 1. Delete existing deployment based on mode
echo "1️⃣ Deleting existing deployment..."
if [[ "$MODE" == "postgresql" ]]; then
    # Delete both memory and postgresql to switch cleanly
    kubectl delete -k config/k8s/overlays/memory --ignore-not-found=true
    kubectl delete -k config/k8s/overlays/postgresql --ignore-not-found=true
    echo "⏳ Waiting for clean shutdown..."
    sleep 10
else
    # Delete both to switch cleanly to memory
    kubectl delete -k config/k8s/overlays/postgresql --ignore-not-found=true
    kubectl delete -k config/k8s/overlays/memory --ignore-not-found=true
    echo "⏳ Waiting for clean shutdown..."
    sleep 5
fi

# 2. Remove old images from the embedded docker inside Minikube
echo "2️⃣ Removing old images from minikube..."
minikube -p ${MINIKUBE_PROFILE} ssh -- docker rmi ${IMAGE_NAME} --force 2>/dev/null || echo "Backend image not found or already removed"

# 2a. Remove old goose image if PostgreSQL mode
if [[ "$MODE" == "postgresql" ]]; then
    echo "2️⃣🐘 Removing old goose image from minikube..."
    minikube -p ${MINIKUBE_PROFILE} ssh -- docker rmi netguard/goose:latest --force 2>/dev/null || echo "Goose image not found or already removed"
fi

# 3. Build fresh docker image
echo "3️⃣ Building new image..."
make docker-build-pg-backend

# 3a. Build goose migration image if PostgreSQL mode
if [[ "$MODE" == "postgresql" ]]; then
    echo "3️⃣🐘 Building goose migration image with latest migrations..."
    docker build -f Dockerfile.goose -t netguard/goose:latest .
    echo "📋 Checking goose build result..."
    docker images | grep "netguard/goose" || { echo "❌ Goose build failed!"; exit 1; }
fi

# 4. Verify build succeeded
echo "📋 Checking build result..."
docker images | grep "netguard/pg-backend" || { echo "❌ Build failed!"; exit 1; }

# 5. Load image into Minikube docker
echo "4️⃣ Loading image to minikube..."
minikube -p ${MINIKUBE_PROFILE} image load ${IMAGE_NAME}

# 5a. Load goose image into Minikube if PostgreSQL mode
if [[ "$MODE" == "postgresql" ]]; then
    echo "4️⃣🐘 Loading goose image to minikube..."
    minikube -p ${MINIKUBE_PROFILE} image load netguard/goose:latest
fi

# 6. Apply deployment manifest using overlay
echo "5️⃣ Creating deployment using $MODE overlay..."
kubectl apply -k ${OVERLAY_PATH}

# 7. Wait until backend pod becomes Ready
echo "⏳ Waiting for pod to be ready..."
if [[ "$MODE" == "postgresql" ]]; then
    echo "🐘 PostgreSQL mode - waiting longer for database startup + migrations..."
    kubectl wait --for=condition=Ready pod -l app=postgresql -n ${NAMESPACE} --timeout=120s || echo "⚠️ PostgreSQL may still be starting..."
    kubectl wait --for=condition=Ready pod -l app=${DEPLOYMENT_NAME} -n ${NAMESPACE} --timeout=180s || echo "⚠️ Backend may still be starting..."
else
    echo "🧠 Memory mode - quick startup expected..."
    kubectl wait --for=condition=Ready pod -l app=${DEPLOYMENT_NAME} -n ${NAMESPACE} --timeout=60s || echo "⚠️ Backend may still be starting..."
fi

# 8. Show resulting status
echo "📊 Deployment status:"
if [[ "$MODE" == "postgresql" ]]; then
    echo "🐘 PostgreSQL components:"
    kubectl get pods,svc,statefulset -n ${NAMESPACE} -l app=postgresql
    echo ""
fi
echo "🚀 Backend components:"
kubectl get pods,svc,deployment -n ${NAMESPACE} -l app=${DEPLOYMENT_NAME}

echo ""
echo "✅ Backend redeploy completed in $MODE mode!"
echo "🔍 To check backend logs: kubectl logs -n ${NAMESPACE} deployment/${DEPLOYMENT_NAME}"
echo "🔍 To follow backend logs: kubectl logs -f -n ${NAMESPACE} deployment/${DEPLOYMENT_NAME}"
if [[ "$MODE" == "postgresql" ]]; then
    echo "🐘 To check PostgreSQL logs: kubectl logs -n ${NAMESPACE} statefulset/postgresql"
    echo "📊 To check deployment status: make status-deployment"
fi 