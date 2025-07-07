#!/bin/bash
echo "🔄 Быстрая проверка watch операций..."
echo "Запускаю watch на 10 секунд..."

# Простая проверка что watch запускается без ошибок декодирования
timeout 10s kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch 2>&1 | \
  tee /tmp/quick_watch_test

echo ""
echo "=== Результат ==="
if grep -q "unable to decode" /tmp/quick_watch_test; then
    echo "❌ ОШИБКА: Найдены ошибки декодирования!"
    echo "Watch операции НЕ исправлены"
elif grep -q "services" /tmp/quick_watch_test || grep -q "ADDED\|MODIFIED\|DELETED" /tmp/quick_watch_test; then
    echo "✅ УСПЕХ: Watch операции работают!"
    echo "Нет ошибок декодирования"
else
    echo "⚠️ Частичный успех: Watch запущен, но нет событий для проверки"
    echo "Попробуйте создать/удалить Service в другом терминале"
fi

rm -f /tmp/quick_watch_test 