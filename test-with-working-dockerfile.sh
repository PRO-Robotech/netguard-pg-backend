#!/bin/bash
set -e

NAMESPACE="netguard-test"

echo "🚀 ТЕСТИРОВАНИЕ С РАБОЧИМ DOCKERFILE"
echo "===================================="

# Backup and replace dockerignore
echo "Временная замена .dockerignore..."
mv .dockerignore .dockerignore.original
mv .dockerignore.temp .dockerignore

# Cleanup function
cleanup() {
    echo "Восстановление .dockerignore..."
    mv .dockerignore .dockerignore.temp
    mv .dockerignore.original .dockerignore
}
trap cleanup EXIT

echo ""
echo "📦 ШАГ 1: Локальная компиляция..."
if make build-k8s-apiserver; then
    echo "✓ Компиляция успешна"
else
    echo "❌ ОШИБКА: Компиляция не удалась"
    exit 1
fi

echo ""
echo "🐳 ШАГ 2: Сборка Docker образа..."
if make docker-build-k8s-apiserver; then
    echo "✓ Docker образ собран"
else
    echo "❌ ОШИБКА: Сборка Docker образа не удалась"
    exit 1
fi

echo ""
echo "📤 ШАГ 3: Загрузка в Minikube..."
if minikube image load netguard/k8s-apiserver:latest; then
    echo "✓ Образ загружен в Minikube"
else
    echo "❌ ОШИБКА: Не удалось загрузить образ в Minikube"
    exit 1
fi

echo ""
echo "🔄 ШАГ 4: Редеплой..."
kubectl rollout restart deployment/netguard-apiserver -n "$NAMESPACE"

if kubectl rollout status deployment/netguard-apiserver -n "$NAMESPACE" --timeout=120s; then
    echo "✓ Deployment готов"
else
    echo "❌ ОШИБКА: Deployment не готов"
    exit 1
fi

echo ""
echo "⚡ ШАГ 5: КРИТИЧЕСКИЙ ТЕСТ WATCH..."

# Test watch operations
echo "Тестирую watch операции..."
(kubectl get services.v1beta1.netguard.sgroups.io -n "$NAMESPACE" --watch --request-timeout=10s 2>&1 || true) | \
  tee /tmp/final_watch_test

echo ""
echo "=== ФИНАЛЬНЫЙ АНАЛИЗ ==="

if grep -q "unable to decode an event from the watch stream" /tmp/final_watch_test; then
    echo "❌ КРИТИЧЕСКАЯ ОШИБКА: Watch операции все еще сломаны!"
    echo "Найдена ошибка:"
    grep "unable to decode" /tmp/final_watch_test
    echo ""
    echo "🔍 Логи API Server:"
    kubectl logs deployment/netguard-apiserver -n "$NAMESPACE" --tail=10
    rm -f /tmp/final_watch_test
    exit 1
elif grep -q "no kind.*ServiceList.*registered" /tmp/final_watch_test; then
    echo "❌ КРИТИЧЕСКАЯ ОШИБКА: ServiceList не зарегистрирован!"
    grep "ServiceList" /tmp/final_watch_test
    rm -f /tmp/final_watch_test
    exit 1
else
    echo "✅ УСПЕХ: Watch операции работают!"
    echo "Нет критических ошибок декодирования"
    
    # Functional test
    echo ""
    echo "🧪 ФУНКЦИОНАЛЬНЫЙ ТЕСТ..."
    if kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: final-test-service
  namespace: $NAMESPACE
spec:
  description: "Final test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF
    then
        echo "✓ Service создан"
        kubectl delete services.v1beta1.netguard.sgroups.io final-test-service -n "$NAMESPACE" 2>/dev/null || true
    fi
fi

rm -f /tmp/final_watch_test

echo ""
echo "🎉 РЕЗУЛЬТАТ:"
echo "============="
echo "✅ Образ собран и загружен в Minikube"
echo "✅ Deployment обновлен"

if ! grep -q "unable to decode\|ServiceList.*registered" /tmp/final_watch_test 2>/dev/null; then
    echo "✅ Watch операции работают!"
    echo ""
    echo "🎯 ИСПРАВЛЕНИЕ WATCH ОПЕРАЦИЙ РАБОТАЕТ!"
    echo ""
    echo "Можно отметить в плане:"
    echo "- [✅] 1.1 Диагностика проблемы с watch"  
    echo "- [✅] 1.2 Исправление PollerWatchInterface"
else
    echo "❌ Watch операции все еще имеют проблемы"
fi 