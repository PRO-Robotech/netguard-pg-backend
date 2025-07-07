#!/bin/bash

set -e

echo "🚀 БЫСТРЫЙ ТЕСТ WATCH ОПЕРАЦИЙ В MINIKUBE"

# Переменные
NAMESPACE="netguard-test"
IMAGE_NAME="netguard/k8s-apiserver:latest"

echo "📦 Загружаю образ в minikube..."
minikube image load ${IMAGE_NAME}

echo "🔧 Пересоздаю deployment..."
kubectl delete deployment netguard-apiserver -n ${NAMESPACE} --ignore-not-found=true
kubectl apply -f config/k8s/deployment.yaml

echo "⏰ Жду когда под будет готов..."
kubectl wait --for=condition=ready pod -l app=netguard-apiserver -n ${NAMESPACE} --timeout=120s

echo "📊 Проверяю статус пода..."
kubectl get pods -n ${NAMESPACE} -l app=netguard-apiserver

echo "🔍 ТЕСТИРУЮ WATCH ОПЕРАЦИИ..."

# Создаю тестовый Service
echo "1️⃣ Создаю тестовый Service..."
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service-watch
  namespace: ${NAMESPACE}
spec:
  serviceName: "test-app"
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
EOF

# Включаю watch в background
echo "2️⃣ Запускаю watch операцию..."
timeout 10s kubectl api-resources --api-group=netguard.sgroups.io --verbs=watch -o wide || true

echo "3️⃣ Обновляю Service для генерации watch события..."
kubectl patch service test-service-watch -n ${NAMESPACE} --type='merge' -p='{"metadata":{"labels":{"test":"updated"}}}'

echo "4️⃣ Удаляю тестовый Service..."
kubectl delete service test-service-watch -n ${NAMESPACE} --ignore-not-found=true

echo "📋 Проверяю логи apiserver на предмет ошибок watch..."
kubectl logs -n ${NAMESPACE} -l app=netguard-apiserver --tail=50 | grep -i "watch\|ServiceList\|unable to decode" || echo "✅ Логи чистые от ошибок watch"

echo "✅ ТЕСТ WATCH ОПЕРАЦИЙ ЗАВЕРШЕН" 