#!/bin/bash

set -euo pipefail

NAMESPACE="netguard-system"
DEPLOYMENT_NAME="netguard-backend"
IMAGE_NAME="netguard/pg-backend:latest"
MINIKUBE_PROFILE="incloud"

echo "🚀 Starting Netguard Backend redeploy..."
echo "============================================"

# 1. Delete existing deployment (if any)
echo "1️⃣ Deleting existing deployment..."
kubectl delete deployment ${DEPLOYMENT_NAME} -n ${NAMESPACE} --ignore-not-found=true

# 2. Remove old image from the embedded docker inside Minikube
echo "2️⃣ Removing old image from minikube..."
minikube -p ${MINIKUBE_PROFILE} ssh -- docker rmi ${IMAGE_NAME} --force 2>/dev/null || echo "Image not found or already removed"

# 3. Build fresh docker image
echo "3️⃣ Building new image..."
make docker-build-pg-backend

# 4. Verify build succeeded
echo "📋 Checking build result..."
docker images | grep "netguard/pg-backend" || { echo "❌ Build failed!"; exit 1; }

# 5. Load image into Minikube docker
echo "4️⃣ Loading image to minikube..."
minikube -p ${MINIKUBE_PROFILE} image load ${IMAGE_NAME}

# 6. Apply deployment manifest
echo "5️⃣ Creating deployment..."
kubectl apply -f config/k8s/backend-deployment.yaml

# 7. Wait until backend pod becomes Ready
echo "⏳ Waiting for pod to be ready..."
kubectl wait --for=condition=Ready pod -l app=${DEPLOYMENT_NAME} -n ${NAMESPACE} --timeout=60s || echo "⚠️ Pod may still be starting..."

# 8. Show resulting status
echo "📊 Deployment status:"
kubectl get pods -n ${NAMESPACE} -l app=${DEPLOYMENT_NAME}

echo ""
echo "✅ Backend redeploy completed!"
echo "🔍 To check logs: kubectl logs -n ${NAMESPACE} deployment/${DEPLOYMENT_NAME}"
echo "🔍 To follow logs: kubectl logs -f -n ${NAMESPACE} deployment/${DEPLOYMENT_NAME}" 