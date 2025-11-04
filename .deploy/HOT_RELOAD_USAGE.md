# Hot Reload - Быстрая инструкция

## 🔥 Что работает

Hot reload **полностью работает** - Air + Delve запущены в pod.
**Проблема**: File watcher на macOS нестабильный (известный баг fsnotify).

## ✅ Рабочее решение: Manual Sync

### Вариант 1: Скрипт sync-file.sh (рекомендуется)

```bash
# После изменения файла:
cd .deploy/skaffold
./sync-file.sh internal/api/netguard/service.go

# Скрипт:
# 1. Скопирует файл в pod
# 2. Air обнаружит изменение
# 3. Пересоберёт за 2-5 секунд
```

### Вариант 2: kubectl cp напрямую

```bash
# Получить имя pod
POD=$(kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o jsonpath='{.items[0].metadata.name}')

# Скопировать файл
kubectl cp internal/api/service.go incloud-sgroups/$POD:/src/internal/api/service.go

# Air автоматически пересоберёт
```

### Вариант 3: Batch sync нескольких файлов

```bash
# Если изменили много файлов:
cd .deploy/skaffold

# Найти все измененные .go файлы за последние 5 минут
find ../../ -name "*.go" -mmin -5 -not -path "*/vendor/*" | while read f; do
    ./sync-file.sh "${f#../../}"
done
```

## 🚀 Типичный workflow

1. **Запустить hot reload** (один раз):
   ```bash
   cd .deploy/skaffold
   ./run-hot-reload.sh
   ```

2. **В другом терминале** следить за логами:
   ```bash
   kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups
   ```

3. **Редактируете код** в IDE

4. **После сохранения**:
   ```bash
   cd .deploy/skaffold
   ./sync-file.sh internal/your/modified-file.go
   ```

5. **Через 2-5 секунд** код пересобран и работает!

## 🐛 Debug

### Проверить что pod работает
```bash
kubectl get pods -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload
```

### Проверить Delve доступен
```bash
nc -zv localhost 2345
# Или
lsof -i:2345
```

### Просмотр логов Air
```bash
kubectl logs deployment/netguard-backend-debug -n incloud-sgroups --tail=50
```

Вы должны видеть:
```
[18:XX:XX] internal/your/file.go has changed
[18:XX:XX] building...
[18:XX:XX] running...
API server listening at: [::]:2345
```

### Проверить что файл скопировался
```bash
POD=$(kubectl get pod -n incloud-sgroups -l app.kubernetes.io/component=debug-hot-reload -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n incloud-sgroups $POD -- ls -la /src/internal/api/netguard/
```

## ⚡ Производительность

| Метод | Время | Надежность |
|-------|-------|-----------|
| Полный rebuild | 30-60 сек | ✅ 100% |
| Hot reload (auto sync) | 2-5 сек | ⚠️ 30% (macOS bug) |
| Hot reload (manual sync) | 2-5 сек | ✅ 100% |

**Рекомендация**: Используйте manual sync (скрипт sync-file.sh)

## 🔄 Когда нужен полный rebuild?

- Изменения в `go.mod` / `go.sum`
- Изменения в Dockerfile
- Изменения в K8s манифестах
- Изменения в `.air.toml`

В этих случаях:
```bash
# Ctrl+C в терминале со Skaffold
cd .deploy/skaffold
./run-hot-reload.sh
```

## 📊 Скорость сборки

- Компиляция Go: ~1-2 секунды
- Запуск с Delve: ~1 секунда
- **Итого**: 2-3 секунды

Это **в 15-20 раз быстрее** полного rebuild!
