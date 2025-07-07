# 📋 NETGUARD v1beta1 AGGREGATION LAYER - РУКОВОДСТВО ПО РАЗВЕРТЫВАНИЮ

## 🎯 ОБЗОР
Данное руководство описывает процесс развертывания Netguard v1beta1 Aggregation Layer в Kubernetes кластере после внесения изменений в backend код.

## 📋 ПРЕДВАРИТЕЛЬНЫЕ ТРЕБОВАНИЯ

### Инструменты
- `kubectl` 1.20+
- `Docker` 20.10+
- `jq` 1.6+
- `make` 4.0+

### Кластер
- Kubernetes 1.20+ с поддержкой API Aggregation Layer
- RBAC включен
- Поддержка TLS/SSL

## 🚀 ПРОЦЕСС РАЗВЕРТЫВАНИЯ

### Шаг 1: Подготовка кода и сборка

```bash
# 1.1 Переход в директорию проекта
cd /path/to/netguard-pg-backend

# 1.2 Компиляция API Server с последними изменениями
make build-k8s-apiserver

# 1.3 Сборка Docker образа
make docker-build-k8s-apiserver

# 1.4 Проверка образа
docker images | grep netguard/k8s-apiserver:latest
```

### Шаг 2: Подготовка namespace и TLS сертификатов

```bash
# 2.1 Создание namespace
kubectl create namespace netguard-test

# 2.2 Генерация TLS сертификатов
NAMESPACE=netguard-test ./scripts/generate-certs.sh

# 2.3 Создание secret с сертификатами
kubectl create secret tls netguard-apiserver-certs \
  --cert=certs/tls.crt \
  --key=certs/tls.key \
  -n netguard-test
```

### Шаг 3: Исправление конфигурации namespace

```bash
# 3.1 Массовое обновление namespace в конфигурационных файлах
find config/k8s -name "*.yaml" -exec sed -i '' 's/namespace: netguard-system/namespace: netguard-test/g' {} \;

# 3.2 Обновление kustomization.yaml
sed -i '' 's/namespace: netguard-system/namespace: netguard-test/g' config/k8s/kustomization.yaml
```

### Шаг 4: Конфигурация сервисов и endpoint'ов

```bash
# 4.1 Проверка selector'ов в сервисах (должны использовать app: netguard-apiserver)
# В config/k8s/deployment.yaml:
selector:
  app: netguard-apiserver  # НЕ app.kubernetes.io/name

# 4.2 Проверка порта в APIService (должен быть 443, НЕ 8443)
# В config/k8s/apiservice.yaml:
service:
  name: netguard-apiserver
  namespace: netguard-test
  port: 443  # НЕ 8443
```

### Шаг 5: Развертывание через Kustomize

```bash
# 5.1 Применение всех ресурсов
kubectl apply -k config/k8s/

# 5.2 Ожидание готовности deployment'ов
kubectl rollout status deployment/netguard-apiserver -n netguard-test --timeout=120s
kubectl rollout status deployment/netguard-backend -n netguard-test --timeout=120s
```

### Шаг 6: Проверка статуса APIService

```bash
# 6.1 Проверка регистрации APIService
kubectl get apiservice v1beta1.netguard.sgroups.io

# 6.2 Проверка доступности (должно быть True)
kubectl get apiservice v1beta1.netguard.sgroups.io -o jsonpath='{.status.conditions[?(@.type=="Available")].status}'

# 6.3 Проверка endpoints
kubectl get endpoints netguard-apiserver -n netguard-test
```

## 🔄 ОБНОВЛЕНИЕ ПОСЛЕ ИЗМЕНЕНИЙ В КОДЕ

### При изменениях в API Server

```bash
# 1. Пересборка образа
make build-k8s-apiserver
make docker-build-k8s-apiserver

# 2. Рестарт deployment
kubectl rollout restart deployment/netguard-apiserver -n netguard-test

# 3. Ожидание завершения
kubectl rollout status deployment/netguard-apiserver -n netguard-test --timeout=120s

# 4. Проверка новых ресурсов
kubectl api-resources --api-group=netguard.sgroups.io
```

### При изменениях в Backend

```bash
# 1. Пересборка backend образа
make docker-build  # или другая команда для backend

# 2. Рестарт backend deployment  
kubectl rollout restart deployment/netguard-backend -n netguard-test

# 3. Проверка connectivity
kubectl exec -n netguard-test deployment/netguard-backend -- nc -zv localhost 9090
```

## 🧪 БАЗОВЫЕ ПРОВЕРКИ

### 1. Инфраструктурные проверки
```bash
# Проверка pods
kubectl get pods -n netguard-test

# Проверка services
kubectl get services -n netguard-test

# Проверка APIService
kubectl get apiservice v1beta1.netguard.sgroups.io
```

### 2. API проверки
```bash
# Список доступных ресурсов
kubectl api-resources --api-group=netguard.sgroups.io

# Прямой вызов API
kubectl get --raw /apis/netguard.sgroups.io/v1beta1

# Количество ресурсов
kubectl get --raw /apis/netguard.sgroups.io/v1beta1 | jq '.resources | length'
```

### 3. Функциональные проверки
```bash
# Создание тестового Service
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service
  namespace: netguard-test
spec:
  description: "Test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
EOF

# Проверка создания
kubectl get services.v1beta1.netguard.sgroups.io -n netguard-test

# Удаление тестового ресурса
kubectl delete services.v1beta1.netguard.sgroups.io test-service -n netguard-test
```

## 🛠 ИСПОЛЬЗОВАНИЕ АВТОМАТИЗИРОВАННЫХ СКРИПТОВ

### Полная переустановка
```bash
# Автоматическая переустановка с исправлением namespace
NAMESPACE=netguard-test ./scripts/clean-redeploy.sh run

# Или в неинтерактивном режиме
NAMESPACE=netguard-test ./scripts/clean-redeploy.sh run -y
```

### Тестирование
```bash
# Быстрые проверки
NAMESPACE=netguard-test ./scripts/test-complete.sh quick

# Полное тестирование
NAMESPACE=netguard-test ./scripts/test-complete.sh all

# Показать статус
NAMESPACE=netguard-test ./scripts/test-complete.sh status
```

## 🔍 ДИАГНОСТИКА ПРОБЛЕМ

### Проверка логов
```bash
# Логи API Server
kubectl logs -f deployment/netguard-apiserver -n netguard-test

# Логи Backend
kubectl logs -f deployment/netguard-backend -n netguard-test

# События в namespace
kubectl get events -n netguard-test --sort-by='.lastTimestamp'
```

### Проверка connectivity
```bash
# Проверка связи API Server -> Backend
kubectl exec -n netguard-test deployment/netguard-apiserver -- nc -zv netguard-backend 9090

# Проверка endpoints
kubectl describe endpoints netguard-apiserver -n netguard-test
```

## 📋 ЧЕКЛИСТ РАЗВЕРТЫВАНИЯ

- [ ] Код скомпилирован (`make build-k8s-apiserver`)
- [ ] Образ собран (`make docker-build-k8s-apiserver`)
- [ ] Namespace создан и настроен
- [ ] TLS сертификаты сгенерированы и применены
- [ ] Конфигурация namespace обновлена во всех файлах
- [ ] Селекторы сервисов исправлены (`app: netguard-apiserver`)
- [ ] Порт APIService настроен на 443
- [ ] Все ресурсы применены (`kubectl apply -k config/k8s/`)
- [ ] Deployment'ы готовы (Running)
- [ ] APIService доступен (Available: True)
- [ ] Endpoints созданы
- [ ] API ресурсы обнаруживаются
- [ ] Базовый CRUD для Service работает

## ⚠️ ИЗВЕСТНЫЕ ОГРАНИЧЕНИЯ

1. **Проблемы с некоторыми ресурсами**: AddressGroup, ServiceAlias не создаются (backend limitations)
2. **PATCH операции**: требуют доработки
3. **Namespace dependency**: требуется точное соответствие namespace в конфигурации
4. **Selector mismatch**: критично использовать правильные селекторы

## 📞 ПОДДЕРЖКА

При возникновении проблем:
1. Проверьте логи через `kubectl logs`
2. Используйте `./scripts/test-complete.sh` для диагностики
3. Проверьте events через `kubectl get events`
4. Убедитесь в корректности namespace конфигурации 