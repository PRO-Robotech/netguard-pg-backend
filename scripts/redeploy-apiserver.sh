#!/bin/bash

set -e

NAMESPACE="netguard-test"
DEPLOYMENT_NAME="netguard-apiserver"
IMAGE_NAME="netguard/k8s-apiserver:latest"
MINIKUBE_PROFILE="incloud"

echo "🚀 Starting Netguard API Server redeploy..."
echo "============================================"

# 1. Удаляем существующий deployment
echo "1️⃣ Deleting existing deployment..."
kubectl delete deployment $DEPLOYMENT_NAME -n $NAMESPACE --ignore-not-found=true

# 2. Удаляем старый образ из minikube
echo "2️⃣ Removing old image from minikube..."
minikube -p $MINIKUBE_PROFILE ssh -- docker rmi $IMAGE_NAME --force 2>/dev/null || echo "Image not found or already removed"

# 3. Собираем новый образ
echo "3️⃣ Building new image..."
make docker-build-k8s-apiserver

# 4. Проверяем что образ собрался
echo "📋 Checking build result..."
docker images | grep "netguard/k8s-apiserver" || (echo "❌ Build failed!" && exit 1)

# 5. Загружаем образ в minikube
echo "4️⃣ Loading image to minikube..."
minikube -p $MINIKUBE_PROFILE image load $IMAGE_NAME

# 6. Создаем deployment
echo "5️⃣ Creating deployment..."
kubectl apply -f config/k8s/deployment.yaml

# 7. Ждем готовности пода
echo "⏳ Waiting for pod to be ready..."
kubectl wait --for=condition=Ready pod -l app=$DEPLOYMENT_NAME -n $NAMESPACE --timeout=60s || echo "⚠️ Pod may still be starting..."

# 8. Показываем статус
echo "📊 Deployment status:"
kubectl get pods -n $NAMESPACE -l app=$DEPLOYMENT_NAME

echo ""
echo "✅ Redeploy completed!"
echo "🔍 To check logs: kubectl logs -n $NAMESPACE deployment/$DEPLOYMENT_NAME"
echo "🔍 To follow logs: kubectl logs -f -n $NAMESPACE deployment/$DEPLOYMENT_NAME" 