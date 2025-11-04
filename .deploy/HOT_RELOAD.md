# Hot Reload Development Mode

Быстрая разработка с Air + Skaffold file sync для backend сервиса.

## Преимущества

- ⚡ **2-5 секунд** вместо 30-60 секунд на изменение кода
- 🔄 Pod **не перезапускается** - port-forwards сохраняются
- 🐛 Delve debugger **всегда доступен** на `:2345`
- 📁 File sync без rebuild Docker image

## Быстрый старт

### 1. Запустить hot reload режим

```bash
cd .deploy/skaffold
./run-hot-reload.sh
```

**Первый запуск**: ~30-60 секунд (сборка image)
**Последующие изменения**: ~2-5 секунд (только rebuild внутри pod)

### 2. Изменить код

```bash
# В другом терминале
vim internal/api/service.go
# Сохранить изменения
```

**Что происходит:**
1. Skaffold замечает изменение файла (~100ms)
2. Копирует файл в pod через file sync (~500ms)
3. Air внутри pod пересобирает binary (~2-3 сек)
4. Процесс автоматически перезапускается (~500ms)
5. ✅ Готово! Pod жив, port-forwards работают

### 3. Подключить debugger

```bash
# Backend Delve доступен на localhost:2345
# Настройте IDE (VSCode/GoLand) на этот порт
```

### 4. Просмотр логов Air

```bash
# В третьем терминале
kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups
```

## Доступные порты

После запуска доступны:
- **:2345** - Delve debugger (backend)
- **:9090** - gRPC API (backend)
- **:8080** - HTTP API (backend)
- **:5432** - PostgreSQL

## Когда нужен полный rebuild?

Hot reload НЕ работает для:
- Изменений в `go.mod` / `go.sum` (новые зависимости)
- Изменений в `Dockerfile`
- Изменений в k8s манифестах

В этих случаях:
```bash
# Остановить Skaffold (Ctrl+C)
# Перезапустить
./run-hot-reload.sh
```

## Сравнение режимов

### Старый способ (debug-all):
```
Изменили код → rebuild image (30-60 сек) → pod restart → port-forwards теряются
```

### Hot reload (debug-hot-reload):
```
Изменили код → file sync (100ms) → Air rebuild (2-3 сек) → restart процесса (500ms)
✅ Pod жив, port-forwards работают!
```

## Возврат к обычному debug режиму

```bash
cd .deploy/skaffold
./run-debug-quiet.sh debug-all
```

## Устранение проблем

### Air не видит изменения
```bash
# Проверить, что файл действительно скопирован
kubectl exec -it deployment/netguard-backend-debug -n incloud-sgroups -- ls -la /src/internal/api/

# Проверить логи Air
kubectl logs -f deployment/netguard-backend-debug -n incloud-sgroups
```

### Pod не запускается
```bash
# Проверить статус
kubectl get pods -n incloud-sgroups -l app.kubernetes.io/name=backend

# Посмотреть события
kubectl describe pod -n incloud-sgroups -l app.kubernetes.io/name=backend
```

### Ошибки компиляции
```bash
# Air покажет ошибки в логах pod
kubectl logs deployment/netguard-backend-debug -n incloud-sgroups --tail=50
```

## Расширение на другие сервисы

Если hot reload для backend работает хорошо, можно добавить для:
- APIServer
- Webhook

Просто создайте аналогичные профили в `skaffold.yaml` с соответствующими Dockerfile-ами.
