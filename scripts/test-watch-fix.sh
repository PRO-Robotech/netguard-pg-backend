#!/bin/bash
NAMESPACE="netguard-test"
RESOURCE_NAME="test-watch-service"

echo "🧪 Тестирование исправления watch операций..."

# 1. Запуск watch в фоне
timeout 30s kubectl get services.v1beta1.netguard.sgroups.io -n "$NAMESPACE" --watch > /tmp/watch_output 2>&1 &
WATCH_PID=$!

sleep 3

# 2. CREATE событие
echo "Creating service..."
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: $RESOURCE_NAME
  namespace: $NAMESPACE
spec:
  description: "Watch test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF

sleep 5

# 3. MODIFY событие
echo "Updating service..."
kubectl patch services.v1beta1.netguard.sgroups.io "$RESOURCE_NAME" -n "$NAMESPACE" \
  --type=merge -p '{"spec":{"description":"Updated by watch test"}}'

sleep 5

# 4. DELETE событие
echo "Deleting service..."
kubectl delete services.v1beta1.netguard.sgroups.io "$RESOURCE_NAME" -n "$NAMESPACE"

sleep 3

# 5. Остановить watch
kill $WATCH_PID 2>/dev/null || true
wait $WATCH_PID 2>/dev/null || true

# 6. Проверить результаты
echo "=== Watch Output ==="
cat /tmp/watch_output

echo ""
echo "=== Проверка результатов ==="
if grep -q "unable to decode" /tmp/watch_output; then
    echo "❌ FAILED: Найдены ошибки декодирования"
    exit 1
elif grep -q "ADDED.*$RESOURCE_NAME" /tmp/watch_output && grep -q "DELETED.*$RESOURCE_NAME" /tmp/watch_output; then
    echo "✅ SUCCESS: Watch события корректно обработаны"
    exit 0
else
    echo "⚠️ PARTIAL: Watch работает, но не все события обнаружены"
    exit 1
fi 