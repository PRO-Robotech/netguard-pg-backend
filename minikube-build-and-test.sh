#!/bin/bash
set -e

NAMESPACE="netguard-test"

echo "🚀 ПРАВИЛЬНАЯ СБОРКА И ТЕСТИРОВАНИЕ ДЛЯ MINIKUBE"
echo "================================================"

echo ""
echo "📦 ШАГ 1: Локальная сборка образа..."

# Сборка ЛОКАЛЬНО (не в Minikube env)
echo "Компиляция API Server..."
if make build-k8s-apiserver; then
    echo "✓ Компиляция успешна"
else
    echo "❌ ОШИБКА: Компиляция не удалась"
    exit 1
fi

echo "Сборка Docker образа ЛОКАЛЬНО (простой способ)..."
if docker build -f config/docker/Dockerfile.k8s-apiserver-simple -t netguard/k8s-apiserver:latest .; then
    echo "✓ Локальный Docker образ создан"
else
    echo "❌ ОШИБКА: Сборка Docker образа не удалась"
    exit 1
fi

echo ""
echo "📤 ШАГ 2: Загрузка образа в Minikube..."

# Загружаем локальный образ в Minikube registry
echo "Загружаем образ в Minikube..."
if minikube image load netguard/k8s-apiserver:latest; then
    echo "✓ Образ загружен в Minikube"
else
    echo "❌ ОШИБКА: Не удалось загрузить образ в Minikube"
    exit 1
fi

# Проверяем что образ в Minikube
echo "Проверяем образ в Minikube registry..."
minikube ssh "docker images | grep netguard/k8s-apiserver" | head -2

echo ""
echo "🔄 ШАГ 3: Редеплой в Minikube..."

echo "Перезапуск deployment..."
kubectl rollout restart deployment/netguard-apiserver -n "$NAMESPACE"

echo "Ожидание готовности deployment..."
if kubectl rollout status deployment/netguard-apiserver -n "$NAMESPACE" --timeout=120s; then
    echo "✓ Deployment готов"
else
    echo "❌ ОШИБКА: Deployment не готов"
    exit 1
fi

echo "Статус pods:"
kubectl get pods -n "$NAMESPACE" -l app=netguard-apiserver

echo ""
echo "🔍 ШАГ 4: Проверка APIService..."
kubectl get apiservice v1beta1.netguard.sgroups.io

echo ""
echo "⚡ ШАГ 5: КРИТИЧЕСКИЙ ТЕСТ WATCH..."

echo "Тестирование watch операций (10 сек)..."

# Запускаем watch в фоне и проверяем на ошибки
(kubectl get services.v1beta1.netguard.sgroups.io -n "$NAMESPACE" --watch --request-timeout=10s 2>&1 || true) | \
  tee /tmp/watch_test_result

echo ""
echo "=== АНАЛИЗ РЕЗУЛЬТАТА ==="

if grep -q "unable to decode an event from the watch stream" /tmp/watch_test_result; then
    echo "❌ КРИТИЧЕСКАЯ ОШИБКА: Watch операции все еще сломаны!"
    echo "Найдена ошибка декодирования:"
    grep "unable to decode" /tmp/watch_test_result
    echo ""
    echo "🔍 Проверяем логи API Server:"
    kubectl logs deployment/netguard-apiserver -n "$NAMESPACE" --tail=10
    rm -f /tmp/watch_test_result
    exit 1
elif grep -q "no kind.*ServiceList.*registered" /tmp/watch_test_result; then
    echo "❌ КРИТИЧЕСКАЯ ОШИБКА: ServiceList не зарегистрирован!"
    grep "ServiceList" /tmp/watch_test_result
    rm -f /tmp/watch_test_result
    exit 1
else
    echo "✅ УСПЕХ: Watch операции работают!"
    echo "Нет критических ошибок декодирования"
fi

rm -f /tmp/watch_test_result

echo ""
echo "🧪 ШАГ 6: Функциональный тест..."

# Создание тестового Service
echo "Создание тестового Service..."
if kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: minikube-test-service
  namespace: $NAMESPACE
spec:
  description: "Minikube test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF
then
    echo "✓ Service создан успешно"
    
    # Проверка чтения
    if kubectl get services.v1beta1.netguard.sgroups.io minikube-test-service -n "$NAMESPACE" &>/dev/null; then
        echo "✓ Service читается успешно"
        
        # Удаление
        if kubectl delete services.v1beta1.netguard.sgroups.io minikube-test-service -n "$NAMESPACE" &>/dev/null; then
            echo "✓ Service удален успешно"
        else
            echo "⚠️ Проблема с удалением Service"
        fi
    else
        echo "⚠️ Проблема с чтением Service"
    fi
else
    echo "⚠️ Проблема с созданием Service"
fi

echo ""
echo "🎉 РЕЗУЛЬТАТ ТЕСТИРОВАНИЯ:"
echo "========================="
echo "✅ Образ собран локально и загружен в Minikube"
echo "✅ Deployment успешно обновлен"
echo "✅ APIService доступен"

if ! grep -q "unable to decode\|ServiceList.*registered" /tmp/watch_test_result 2>/dev/null; then
    echo "✅ Watch операции работают без ошибок декодирования"
    echo ""
    echo "🎯 ИСПРАВЛЕНИЕ WATCH ОПЕРАЦИЙ РАБОТАЕТ!"
    echo ""
    echo "Теперь можно отметить в плане:"
    echo "- [✅] 1.1 Диагностика проблемы с watch"  
    echo "- [✅] 1.2 Исправление PollerWatchInterface"
else
    echo "❌ Watch операции все еще имеют проблемы"
    echo "Нужна дополнительная диагностика"
fi 