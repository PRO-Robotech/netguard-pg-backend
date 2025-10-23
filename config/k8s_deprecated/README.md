# Netguard Kubernetes Aggregated API Server

Этот каталог содержит манифесты Kubernetes для развертывания Netguard Aggregated API Server.

## Архитектура

```
kubectl apply/get/watch → Aggregated API → Admission Controllers → gRPC → netguard-pg-backend
                                ↓                                              ↓
                           Validation/Mutation                        Хранение данных
```

## Компоненты

- **API Server** - основной сервер, реализующий Kubernetes Aggregated API
- **Admission Webhooks** - валидация и мутация ресурсов
- **Backend Client** - интеграция с netguard-pg-backend через gRPC

## Требования

- Kubernetes 1.19+
- netguard-pg-backend развернут и доступен
- cert-manager (для автоматического управления TLS сертификатами) или ручное управление сертификатами

## Быстрое развертывание

### 1. Создание namespace

```bash
kubectl apply -f namespace.yaml
```

### 2. Настройка TLS сертификатов

#### Вариант A: С cert-manager (рекомендуется)

```bash
# Установить cert-manager если не установлен
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Создать сертификат
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: netguard-apiserver-certs
  namespace: netguard-system
spec:
  secretName: netguard-apiserver-certs
  dnsNames:
  - netguard-apiserver.netguard-system.svc
  - netguard-apiserver.netguard-system.svc.cluster.local
  - netguard-apiserver-webhook.netguard-system.svc
  - netguard-apiserver-webhook.netguard-system.svc.cluster.local
  issuer:
    name: selfsigned-issuer
    kind: ClusterIssuer
EOF
```

#### Вариант B: Самоподписанные сертификаты

```bash
# Создать самоподписанный сертификат
openssl req -x509 -newkey rsa:2048 -keyout tls.key -out tls.crt -days 365 -nodes \
  -subj "/CN=netguard-apiserver.netguard-system.svc"

# Создать Secret
kubectl create secret tls netguard-apiserver-certs \
  --cert=tls.crt --key=tls.key -n netguard-system

# Получить CA bundle для webhook конфигурации
CA_BUNDLE=$(kubectl get secret netguard-apiserver-certs -n netguard-system -o jsonpath='{.data.tls\.crt}')
```

### 3. Развертывание всех компонентов

```bash
# Применить все манифесты
kubectl apply -k .

# Или по отдельности
kubectl apply -f rbac.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f apiservice.yaml
kubectl apply -f validating-webhook.yaml
kubectl apply -f mutating-webhook.yaml
```

### 4. Обновление CA bundle в webhooks

```bash
# Получить CA bundle
CA_BUNDLE=$(kubectl get secret netguard-apiserver-certs -n netguard-system -o jsonpath='{.data.tls\.crt}')

# Обновить validating webhook
kubectl patch validatingadmissionwebhook netguard-validator \
  --type='json' -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'$CA_BUNDLE'}]"

# Обновить mutating webhook
kubectl patch mutatingadmissionwebhook netguard-mutator \
  --type='json' -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'$CA_BUNDLE'}]"

# Обновить APIService
kubectl patch apiservice v1beta1.netguard.sgroups.io \
  --type='json' -p="[{'op': 'replace', 'path': '/spec/caBundle', 'value':'$CA_BUNDLE'}]"
```

## Проверка развертывания

### 1. Проверить статус подов

```bash
kubectl get pods -n netguard-system
kubectl logs -f deployment/netguard-apiserver -n netguard-system
```

### 2. Проверить APIService

```bash
kubectl get apiservice v1beta1.netguard.sgroups.io
kubectl describe apiservice v1beta1.netguard.sgroups.io
```

### 3. Проверить доступность API

```bash
# Проверить доступность ресурсов
kubectl api-resources --api-group=netguard.sgroups.io

# Проверить версии API
kubectl api-versions | grep netguard

# Создать тестовый ресурс
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service
  namespace: default
spec:
  description: "Test service"
  ingressPorts:
  - protocol: TCP
    port: "80"
    description: "HTTP port"
EOF

# Проверить созданный ресурс
kubectl get services.netguard.sgroups.io
kubectl describe service.netguard.sgroups.io test-service
```

### 4. Проверить subresources

```bash
# Проверить status subresource
kubectl get service.netguard.sgroups.io test-service -o yaml | grep -A 10 status:

# Проверить addressGroups subresource
kubectl get --raw "/apis/netguard.sgroups.io/v1beta1/namespaces/default/services/test-service/addressGroups"

# Проверить sync subresource
kubectl get --raw "/apis/netguard.sgroups.io/v1beta1/namespaces/default/services/test-service/sync"
```

### 5. Проверить admission webhooks

```bash
# Проверить статус webhooks
kubectl get validatingadmissionwebhooks netguard-validator
kubectl get mutatingadmissionwebhooks netguard-mutator

# Проверить работу валидации (должна отклонить невалидный ресурс)
cat <<EOF | kubectl apply -f -
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: invalid-service
  namespace: default
spec:
  description: ""  # Пустое описание должно быть заполнено мутацией
  ingressPorts:
  - protocol: TCP
    port: "invalid-port"  # Невалидный порт должен быть отклонен валидацией
EOF
```

## Конфигурация

### Переменные окружения

Основные переменные окружения в Deployment:

- `APISERVER_TLS_ENABLED` - включить TLS (true/false)
- `APISERVER_CERT_FILE` - путь к TLS сертификату
- `APISERVER_KEY_FILE` - путь к TLS ключу
- `BACKEND_ENDPOINT` - адрес netguard-pg-backend
- `LOG_LEVEL` - уровень логирования (debug/info/warn/error)
- `LOG_FORMAT` - формат логов (json/text)

### ConfigMap

Детальная конфигурация в `configmap.yaml`:

- `apiserver.yaml` - production конфигурация
- `apiserver-dev.yaml` - development конфигурация

## Мониторинг

### Health Checks

API Server предоставляет следующие endpoints:

- `/healthz` - liveness probe
- `/readyz` - readiness probe
- `/metrics` - Prometheus метрики (если включены)

### Логирование

Логи в JSON формате содержат:

- Trace ID для корреляции запросов
- Информацию о CRUD операциях
- Ошибки валидации и backend взаимодействия
- Метрики производительности

## Устранение неисправностей

### Общие проблемы

1. **APIService недоступен**
   ```bash
   kubectl describe apiservice v1beta1.netguard.sgroups.io
   kubectl logs -f deployment/netguard-apiserver -n netguard-system
   ```

2. **TLS ошибки**
   ```bash
   # Проверить сертификаты
   kubectl get secret netguard-apiserver-certs -n netguard-system -o yaml
   
   # Проверить CA bundle в APIService
   kubectl get apiservice v1beta1.netguard.sgroups.io -o yaml | grep caBundle
   ```

3. **Backend недоступен**
   ```bash
   # Проверить доступность backend
   kubectl get endpoints netguard-backend -n netguard-system
   kubectl logs -f deployment/netguard-apiserver -n netguard-system | grep backend
   ```

4. **Admission webhooks не работают**
   ```bash
   # Проверить конфигурацию webhooks
   kubectl describe validatingadmissionwebhook netguard-validator
   kubectl describe mutatingadmissionwebhook netguard-mutator
   
   # Проверить логи webhook вызовов
   kubectl logs -f deployment/netguard-apiserver -n netguard-system | grep webhook
   ```

### Отладка

Включить debug логирование:

```bash
kubectl set env deployment/netguard-apiserver LOG_LEVEL=debug -n netguard-system
```

Проверить детальные логи:

```bash
kubectl logs -f deployment/netguard-apiserver -n netguard-system --tail=100
```

## Безопасность

### RBAC

API Server имеет минимальные права:

- Чтение APIService и admission webhooks
- Чтение собственных ресурсов netguard.sgroups.io
- Создание events для логирования
- Чтение конфигурации в своем namespace

### Network Policies

Рекомендуется настроить Network Policies для ограничения трафика:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: netguard-apiserver
  namespace: netguard-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: netguard-apiserver
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector: {}  # Разрешить от всех namespace (для API вызовов)
    ports:
    - protocol: TCP
      port: 8443
  egress:
  - to:
    - podSelector:
        matchLabels:
          app.kubernetes.io/name: netguard-backend
    ports:
    - protocol: TCP
      port: 8080
  - to: {}  # DNS и другие системные вызовы
    ports:
    - protocol: UDP
      port: 53
```

### Pod Security

Deployment использует restrictive security context:

- `runAsNonRoot: true`
- `readOnlyRootFilesystem: true`
- `allowPrivilegeEscalation: false`
- `capabilities.drop: [ALL]`

## Обновление

### Rolling Update

```bash
# Обновить образ
kubectl set image deployment/netguard-apiserver apiserver=netguard/k8s-apiserver:v1.1.0 -n netguard-system

# Проверить статус обновления
kubectl rollout status deployment/netguard-apiserver -n netguard-system

# Откатить при необходимости
kubectl rollout undo deployment/netguard-apiserver -n netguard-system
```

### Обновление конфигурации

```bash
# Обновить ConfigMap
kubectl apply -f configmap.yaml

# Перезапустить поды для применения новой конфигурации
kubectl rollout restart deployment/netguard-apiserver -n netguard-system
```

## Масштабирование

```bash
# Увеличить количество реплик
kubectl scale deployment netguard-apiserver --replicas=3 -n netguard-system

# Или через HPA
kubectl autoscale deployment netguard-apiserver --cpu-percent=70 --min=2 --max=5 -n netguard-system
``` 