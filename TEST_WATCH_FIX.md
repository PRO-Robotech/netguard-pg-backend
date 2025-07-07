# 🧪 ПЛАН ТЕСТИРОВАНИЯ WATCH ИСПРАВЛЕНИЙ

**Цель:** Проверить что исправление watch операций работает  
**Дата:** 29 декабря 2024

---

## 📋 ОБЯЗАТЕЛЬНЫЕ ШАГИ ТЕСТИРОВАНИЯ

### ШАГ 1: Пересборка образа API Server
```bash
cd netguard-pg-backend

# Подготовка Minikube environment
eval $(minikube docker-env)

# Пересборка с изменениями
make build-k8s-apiserver
make docker-build-k8s-apiserver

# Проверить что образ пересобран
docker images | grep netguard/k8s-apiserver
```

### ШАГ 2: Редеплой в Minikube
```bash
# Перезапуск deployment чтобы подхватить новый образ
kubectl rollout restart deployment/netguard-apiserver -n netguard-test

# Ждем готовности
kubectl rollout status deployment/netguard-apiserver -n netguard-test --timeout=120s

# Проверяем что pods перезапустились
kubectl get pods -n netguard-test -l app=netguard-apiserver
```

### ШАГ 3: Проверка APIService
```bash
# Проверить что APIService доступен
kubectl get apiservice v1beta1.netguard.sgroups.io

# Проверить API resources
kubectl api-resources --api-group=netguard.sgroups.io

# Проверить что Service discovery работает
kubectl get --raw /apis/netguard.sgroups.io/v1beta1 | jq '.resources[] | select(.name == "services")'
```

### ШАГ 4: Тестирование watch БЕЗ событий (проверка декодирования)
```bash
# Быстрый тест - должен запуститься БЕЗ ошибок декодирования
timeout 10s kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch

# КРИТЕРИЙ УСПЕХА: НЕ должно быть ошибки "unable to decode an event from the watch stream"
```

### ШАГ 5: Тестирование watch С событиями (полный тест)
```bash
# Запустить полный тест
./scripts/test-watch-fix.sh

# КРИТЕРИИ УСПЕХА:
# 1. Watch запускается без ошибок декодирования
# 2. ADDED события отображаются при создании ресурса
# 3. MODIFIED события отображаются при обновлении
# 4. DELETED события отображаются при удалении
```

### ШАГ 6: Проверка логов API Server
```bash
# Проверить логи на ошибки
kubectl logs deployment/netguard-apiserver -n netguard-test | tail -20

# НЕ должно быть ошибок типа:
# - "unable to decode"
# - "no kind 'ServiceList' is registered"
# - "failed to convert object to unstructured"
```

---

## 🎯 КРИТЕРИИ УСПЕШНОГО ТЕСТИРОВАНИЯ

### ✅ ТЕСТ ПРОЙДЕН если:
- [ ] Образ пересобран и редеплоен успешно
- [ ] APIService доступен
- [ ] `kubectl get services.v1beta1.netguard.sgroups.io --watch` запускается БЕЗ ошибок декодирования
- [ ] Watch события (ADDED, MODIFIED, DELETED) отображаются корректно
- [ ] Логи API Server не содержат ошибок watch

### ❌ ТЕСТ НЕ ПРОЙДЕН если:
- Ошибки декодирования все еще присутствуют
- Watch не показывает события
- API Server логи содержат ошибки

---

## 🚀 КОМАНДЫ ДЛЯ ВЫПОЛНЕНИЯ

**Полный цикл тестирования:**
```bash
# 1. Пересборка и редеплой
eval $(minikube docker-env)
make docker-build-k8s-apiserver
kubectl rollout restart deployment/netguard-apiserver -n netguard-test
kubectl rollout status deployment/netguard-apiserver -n netguard-test --timeout=120s

# 2. Быстрая проверка
timeout 10s kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test --watch

# 3. Полный тест
./scripts/test-watch-fix.sh
```

**ТОЛЬКО ПОСЛЕ УСПЕШНОГО ПРОХОЖДЕНИЯ ВСЕХ ТЕСТОВ можно ставить галочки готовности в плане!** 