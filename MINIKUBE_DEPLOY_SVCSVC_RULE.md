# Успешный деплой SvcSvcRule в Minikube - Пошаговое руководство

## Дата: 2025-10-21

## Проблема
Образ `:local-dev` в Minikube не обновлялся автоматически при использовании `deploy-local.sh` и `minikube image load`, потому что:
1. Minikube кеширует образы по тегу
2. Deployment не триггерит pod recreation если тег не изменился
3. `minikube image load` не заменяет существующий образ с тем же тегом

## Успешное решение (Delete + LocalDev Strategy)

### Шаг 1: Сборка нового образа
```bash
docker build -f Dockerfile.backend -t netguard/pg-backend:local-dev .
```

**Верификация:**
```bash
docker images netguard/pg-backend:local-dev --format "{{.ID}}" | head -1
# Output: 3200f8249f2b (новый hash)
```

### Шаг 2: Scale down deployment
Это освобождает образ от использования в pod и позволяет его удалить.

```bash
kubectl scale deployment/netguard-backend --replicas=0 -n incloud-sgroups
sleep 5
```

**Верификация:**
```bash
kubectl get pods -n incloud-sgroups -l app.kubernetes.io/name=backend
# Output: No resources found (pods terminated)
```

### Шаг 3: Удаление старого образа из Minikube (force)
```bash
minikube -p incloud ssh "docker rmi -f netguard/pg-backend:local-dev"
```

**Output:**
```
Untagged: netguard/pg-backend:local-dev
Deleted: sha256:ed54b739b31d... (старый hash)
```

**Верификация:**
```bash
minikube -p incloud ssh "docker images netguard/pg-backend:local-dev"
# Output: (empty - образ удалён)
```

### Шаг 4: Загрузка нового образа в Minikube
```bash
minikube -p incloud image load netguard/pg-backend:local-dev
```

**Верификация:**
```bash
minikube -p incloud ssh "docker images netguard/pg-backend:local-dev --format '{{.ID}}'"
# Output: 1007f4014a57 (соответствует локальному hash)
```

### Шаг 5: Scale up deployment
```bash
kubectl scale deployment/netguard-backend --replicas=1 -n incloud-sgroups
sleep 10
kubectl rollout status deployment/netguard-backend -n incloud-sgroups --timeout=120s
```

**Output:**
```
deployment.apps/netguard-backend scaled
Waiting for deployment "netguard-backend" rollout to finish: 0 of 1 updated replicas are available...
deployment "netguard-backend" successfully rolled out
```

### Шаг 6: Верификация образа в pod
```bash
kubectl get pod -n incloud-sgroups -l app.kubernetes.io/name=backend \
  -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="backend")].imageID}'
```

**Expected output:**
```
docker://sha256:1007f4014a57... (соответствует новому образу)
```

### Шаг 7: Проверка логов
```bash
kubectl logs -n incloud-sgroups -l app.kubernetes.io/name=backend --tail=50
```

**Должны увидеть:**
- Новые ошибки (если есть) отражают новый код
- Timestamp соответствует времени пересоздания pod

## Альтернативные подходы (НЕ РАБОТАЛИ)

### ❌ НЕ РАБОТАЕТ: `deploy-local.sh backend`
**Проблема:** Скрипт загружает образ, но не удаляет старый, поэтому Minikube продолжает использовать кешированный образ.

### ❌ НЕ РАБОТАЕТ: `kubectl rollout restart`
**Проблема:** Просто перезапускает pod с тем же образом из кеша.

### ❌ НЕ РАБОТАЕТ: `kubectl delete pod`
**Проблема:** Kubernetes создаёт новый pod, но pull'ит образ из кеша Minikube (старый).

### ❌ НЕ РАБОТАЕТ: `minikube image load` (без предварительного удаления)
**Проблема:** Не заменяет существующий образ с тем же тегом.

## Ключевые принципы

1. **Всегда scale down перед удалением образа** - иначе `docker rmi -f` не удалит образ полностью
2. **Всегда проверять hash образа** - сравнивать локальный, Minikube и pod
3. **Использовать force delete** - `docker rmi -f` гарантирует удаление
4. **Верифицировать после каждого шага** - не предполагать успех

## Верификация 3-х уровней

### Уровень 1: Локальный Docker
```bash
docker images netguard/pg-backend:local-dev --format "{{.ID}}"
```

### Уровень 2: Minikube Docker
```bash
minikube -p incloud ssh "docker images netguard/pg-backend:local-dev --format '{{.ID}}'"
```

### Уровень 3: Pod
```bash
kubectl get pod -n incloud-sgroups -l app.kubernetes.io/name=backend \
  -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="backend")].imageID}'
```

**Все 3 hash должны совпадать** (первые 12 символов от полного sha256).

## Команды для копирования (полный workflow)

```bash
# 1. Сборка
docker build -f Dockerfile.backend -t netguard/pg-backend:local-dev .

# 2. Scale down
kubectl scale deployment/netguard-backend --replicas=0 -n incloud-sgroups && sleep 5

# 3. Удаление старого образа
minikube -p incloud ssh "docker rmi -f netguard/pg-backend:local-dev"

# 4. Загрузка нового
minikube -p incloud image load netguard/pg-backend:local-dev

# 5. Верификация в Minikube
minikube -p incloud ssh "docker images netguard/pg-backend:local-dev --format '{{.ID}}'"

# 6. Scale up
kubectl scale deployment/netguard-backend --replicas=1 -n incloud-sgroups && sleep 10

# 7. Ждём готовности
kubectl rollout status deployment/netguard-backend -n incloud-sgroups --timeout=120s

# 8. Верификация pod image
kubectl get pod -n incloud-sgroups -l app.kubernetes.io/name=backend \
  -o jsonpath='{.items[0].status.containerStatuses[?(@.name=="backend")].imageID}'

# 9. Проверка логов
kubectl logs -n incloud-sgroups -l app.kubernetes.io/name=backend --tail=50
```

## Время выполнения

- **Сборка образа:** ~20-30 сек
- **Scale down:** 5-10 сек
- **Удаление образа:** 2-3 сек
- **Загрузка образа:** 15-20 сек
- **Scale up + rollout:** 20-30 сек
- **Общее время:** ~70-100 сек

## Изменённые файлы для SvcSvcRule support

### Файлы с кодом (9 мест):
1. `internal/sync/syncers/svcsvc_rule_syncer.go` - NEW (syncer)
2. `internal/domain/models/svcsvc-rule.go` - ToSGroupsProto()
3. `internal/domain/registry/types.go` - TypeSvcSvcRule
4. `cmd/server/main.go` - 2 места (setupSyncManager, setupOutboxWorker)
5. `internal/sync/worker/outbox_worker.go` - struct field + constructor
6. `internal/sync/worker/process_entity.go` - 7 мест
7. `internal/sync/worker/dependency_checker.go` - extractSvcSvcRuleDependencies

### Миграции:
- `migrations/031_add_svcsvc_rule_unique_namespace_name.sql` - UNIQUE constraint

## Выводы

Delete + LocalDev Strategy - **ЕДИНСТВЕННЫЙ надёжный способ** обновить образ в Minikube при использовании тега `:local-dev`.

Это документировано для будущего использования.
