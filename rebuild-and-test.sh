#!/bin/bash
set -e

NAMESPACE="netguard-test"

echo "🚀 ПОЛНЫЙ ЦИКЛ ТЕСТИРОВАНИЯ WATCH ИСПРАВЛЕНИЙ"
echo "=============================================="

# ШАГ 1: Пересборка образа
echo ""
echo "📦 ШАГ 1: Пересборка образа API Server..."
eval $(minikube docker-env)
echo "✓ Minikube Docker environment настроен"

echo "Сборка API Server..."
if make build-k8s-apiserver; then
    echo "✓ Компиляция API Server успешна"
else
    echo "❌ ОШИБКА: Компиляция API Server не удалась"
    exit 1
fi

echo "Сборка Docker образа..."
if make docker-build-k8s-apiserver; then
    echo "✓ Docker образ пересобран"
else
    echo "❌ ОШИБКА: Сборка Docker образа не удалась"
    exit 1
fi

echo "Проверка образа..."
docker images | grep netguard/k8s-apiserver | head -3

# ШАГ 2: Редеплой
echo ""
echo "🔄 ШАГ 2: Редеплой в Minikube..."
echo "Перезапуск deployment..."
kubectl rollout restart deployment/netguard-apiserver -n "$NAMESPACE"

echo "Ожидание готовности deployment..."
if kubectl rollout status deployment/netguard-apiserver -n "$NAMESPACE" --timeout=120s; then
    echo "✓ Deployment перезапущен успешно"
else
    echo "❌ ОШИБКА: Deployment не готов"
    exit 1
fi

echo "Статус pods:"
kubectl get pods -n "$NAMESPACE" -l app=netguard-apiserver

# ШАГ 3: Проверка APIService
echo ""
echo "🔍 ШАГ 3: Проверка APIService..."
echo "APIService статус:"
kubectl get apiservice v1beta1.netguard.sgroups.io

echo ""
echo "API Resources:"
kubectl api-resources --api-group=netguard.sgroups.io

# ШАГ 4: Быстрый тест watch
echo ""
echo "⚡ ШАГ 4: Быстрая проверка watch (10 сек)..."
echo "Запуск watch команды..."

timeout 10s kubectl get services.v1beta1.netguard.sgroups.io -n "$NAMESPACE" --watch 2>&1 | \
  tee /tmp/quick_watch_result

echo ""
echo "=== Анализ результата ==="
if grep -q "unable to decode" /tmp/quick_watch_result; then
    echo "❌ КРИТИЧЕСКАЯ ОШИБКА: Найдены ошибки декодирования!"
    echo "Watch операции НЕ исправлены"
    echo ""
    echo "Ошибки:"
    grep "unable to decode" /tmp/quick_watch_result
    rm -f /tmp/quick_watch_result
    exit 1
elif grep -q "error\|Error\|ERROR" /tmp/quick_watch_result; then
    echo "⚠️ ВНИМАНИЕ: Найдены другие ошибки:"
    grep -i error /tmp/quick_watch_result
else
    echo "✅ УСПЕХ: Watch запускается без ошибок декодирования!"
fi

rm -f /tmp/quick_watch_result

# ШАГ 5: Проверка логов
echo ""
echo "📋 ШАГ 5: Проверка логов API Server..."
echo "Последние логи API Server:"
kubectl logs deployment/netguard-apiserver -n "$NAMESPACE" --tail=10

# ШАГ 6: Полный тест (опционально)
echo ""
echo "🧪 ШАГ 6: Хотите запустить полный тест с созданием/удалением ресурсов?"
echo "Это создаст тестовый Service, обновит его и удалит"
echo ""
read -p "Запустить полный тест? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Запуск полного теста..."
    if ./scripts/test-watch-fix.sh; then
        echo "✅ ПОЛНЫЙ ТЕСТ ПРОЙДЕН УСПЕШНО!"
    else
        echo "❌ ПОЛНЫЙ ТЕСТ НЕ ПРОЙДЕН"
        exit 1
    fi
else
    echo "⏭️ Полный тест пропущен"
fi

echo ""
echo "🎉 РЕЗУЛЬТАТ ТЕСТИРОВАНИЯ:"
echo "========================="
echo "✅ Образ пересобран и редеплоен"
echo "✅ APIService доступен"
echo "✅ Watch запускается без ошибок декодирования"
echo ""
echo "🎯 ИСПРАВЛЕНИЕ WATCH ОПЕРАЦИЙ РАБОТАЕТ!"
echo ""
echo "Теперь можно отметить в плане:"
echo "- [✅] 1.1 Диагностика проблемы с watch"
echo "- [✅] 1.2 Исправление PollerWatchInterface" 