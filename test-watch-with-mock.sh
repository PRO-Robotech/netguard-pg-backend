#!/bin/bash

set -e

echo "🚀 ПОЛНЫЙ ТЕСТ WATCH ОПЕРАЦИЙ С MOCK BACKEND"

echo "1️⃣ Собираем и загружаем новый образ..."
./test-mock-compilation.sh

echo "2️⃣ Пересоздаем deployment с новым образом..."
kubectl delete deployment netguard-apiserver -n netguard-test --ignore-not-found=true
kubectl apply -f config/k8s/deployment.yaml

echo "3️⃣ Ждем готовности пода..."
kubectl wait --for=condition=ready pod -l app=netguard-apiserver -n netguard-test --timeout=120s

echo "4️⃣ Проверяем логи apiserver..."
kubectl logs -n netguard-test -l app=netguard-apiserver --tail=10

echo "5️⃣ ТЕСТИРУЕМ API RESOURCES..."
kubectl api-resources --api-group=netguard.sgroups.io -o wide

echo "6️⃣ ТЕСТИРУЕМ LIST ADDRESSGROUPS..."
kubectl get addressgroups.netguard.sgroups.io -n netguard-test -o wide

echo "7️⃣ ТЕСТИРУЕМ WATCH ADDRESSGROUPS..."
echo "   Запускаем watch на 10 секунд..."

timeout 10s kubectl get addressgroups.netguard.sgroups.io -n netguard-test --watch --output-watch-events || {
    echo "⚠️  Watch завершился (ожидаемо через 10 сек)"
}

echo "8️⃣ ПРОВЕРЯЕМ ЛОГИ НА ОШИБКИ..."
echo "   Ищем ошибки 'unable to decode' или 'AddressGroupList not registered':"
kubectl logs -n netguard-test -l app=netguard-apiserver --tail=20 | grep -i "unable to decode\|AddressGroupList" || {
    echo "✅ НЕТ ОШИБОК ДЕКОДИРОВАНИЯ!"
}

echo ""
echo "🎯 РЕЗУЛЬТАТ ТЕСТИРОВАНИЯ:"
echo "   - Если видишь mock AddressGroups в списке - ✅ MOCK РАБОТАЕТ"
echo "   - Если нет ошибок 'unable to decode' - ✅ WATCH ИСПРАВЛЕН"
echo "   - Если watch показывает события - ✅ AGGREGATED API РАБОТАЕТ"

echo ""
echo "📋 ДЛЯ ДОПОЛНИТЕЛЬНОГО ТЕСТИРОВАНИЯ:"
echo "   kubectl get addressgroups.netguard.sgroups.io -n netguard-test --watch" 