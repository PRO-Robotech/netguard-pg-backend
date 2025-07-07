#!/bin/bash

set -e

echo "🔧 ТЕСТИРУЕМ КОМПИЛЯЦИЮ С MOCK BACKEND CLIENT"

echo "1️⃣ Очищаем кеш..."
go clean -cache

echo "2️⃣ Vendor обновляем..."
go mod vendor

echo "3️⃣ Компилируем apiserver..."
make build-k8s-apiserver

echo "4️⃣ Собираем Docker образ..."
docker build --no-cache -f Dockerfile.apiserver -t netguard/k8s-apiserver:latest .

echo "✅ ВСЕ СОБРАЛОСЬ! MOCK BACKEND РАБОТАЕТ!"

echo "5️⃣ Загружаем в minikube..."
minikube image load netguard/k8s-apiserver:latest

echo "🎉 ГОТОВО! МОЖНО ТЕСТИРОВАТЬ WATCH!" 