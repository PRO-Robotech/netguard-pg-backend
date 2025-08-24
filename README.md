# Netguard PG Backend

Netguard PG Backend - это сервис, обеспечивающий хранение и управление ресурсами сетевой безопасности. Он реализует логику [sgroups-k8s-netguard](https://github.com/PRO-robotech/sgroups-k8s-netguard).

## Возможности

- Хранение ресурсов сетевой безопасности в базах данных PostgreSQL и в памяти
- Поддержка различных типов ресурсов:
  - Сервисы (Services)
  - Группы адресов (Address Groups)
  - Привязки групп адресов (Address Group Bindings)
  - Сопоставления портов групп адресов (Address Group Port Mappings)
  - Правила взаимодействия сервисов (RuleS2S)
- Синхронизация с внешними системами (SGROUP)

## Синхронизация с SGROUP

Netguard PG Backend поддерживает автоматическую синхронизацию с внешним сервисом SGROUP для обеспечения консистентности сетевых политик и правил безопасности.

### Поддерживаемые типы синхронизации

- **AddressGroup** → SGROUP Groups
- **Network** → SGROUP Networks  
- **IEAgAgRule** → SGROUP IEAgAgRules

### Конфигурация синхронизации

```yaml
# Настройки подключения к SGROUP
sgroup:
  endpoint: "sgroup-service:9090"
  tls:
    enabled: true
    cert_file: "/certs/client.crt"
    key_file: "/certs/client.key"
    ca_file: "/certs/ca.crt"
  timeout: "30s"

# Настройки синхронизации
sync:
  enabled: true
  debouncing:
    enabled: true
    window: "1s"
  batch:
    enabled: true
    max_size: 100
    timeout: "5s"
```

### Примеры использования

#### Создание AddressGroup с автоматической синхронизацией

```yaml
apiVersion: netguard.sgroups.io/v1beta1
kind: AddressGroup
metadata:
  name: web-servers
  namespace: production
spec:
  addresses:
    - "10.0.1.10"
    - "10.0.1.11"
    - "10.0.1.12"
```

После создания этого ресурса в Kubernetes, он автоматически синхронизируется с SGROUP как группа `production/web-servers`.

#### Мониторинг синхронизации

```bash
# Проверка статуса синхронизации
curl http://netguard-backend:8080/sync/status

# Проверка метрик
curl http://netguard-backend:8080/metrics | grep sync

# Проверка здоровья SGROUP соединения
curl http://netguard-backend:8080/health/sgroup
```

### Документация

Подробная документация по синхронизации доступна в следующих файлах:

- **[SGROUP_SYNC.md](docs/SGROUP_SYNC.md)** - Полное описание архитектуры синхронизации
- **[SYNC_SCENARIOS.md](docs/SYNC_SCENARIOS.md)** - Практические сценарии использования
- **[SYNC_OPERATIONS.md](docs/SYNC_OPERATIONS.md)** - Операционное руководство
- **[SYNC_DIAGRAMS.md](docs/SYNC_DIAGRAMS.md)** - Диаграммы архитектуры и процессов

## Архитектура

- **Domain**: Содержит основную бизнес-логику и модели
  - `models`: Доменные сущности
  - `ports`: Интерфейсы для репозиториев и сервисов
- **Application**: Содержит сервисы приложения, которые оркестрируют доменную логику
  - `services`: Реализации сервисов
- **Infrastructure**: Содержит реализации интерфейсов, определенных в доменном слое
  - `repositories`: Реализации репозиториев (PostgreSQL, in-memory)
- **Cmd**: Содержит точки входа в приложение
  - `server`: Основное серверное приложение

## Начало работы

### Предварительные требования

- Go 1.23 или выше
- PostgreSQL (опционально, для постоянного хранения)

### Установка

1. Клонировать репозиторий:
   ```
   git clone https://github.com/yourusername/netguard-pg-backend.git
   cd netguard-pg-backend
   ```

2. Собрать приложение:
   ```
   go build -o netguard-server ./cmd/server
   ```

### Использование

Запуск с базой данных в памяти:
```
./netguard-server --memory
```

Запуск с базой данных PostgreSQL:
```
./netguard-server --pg-uri "postgres://user:password@localhost:5432/netguard"
```

### Развертывание с Docker

Проект включает поддержку Docker для простого развертывания.

#### Использование Docker

1. Сборка Docker-образа:
   ```
   docker build -t netguard-pg-backend .
   ```

2. Запуск контейнера с базой данных в памяти:
   ```
   docker run -p 8080:8080 -p 9090:9090 netguard-pg-backend
   ```

3. Запуск контейнера с базой данных PostgreSQL:
   ```
   docker run -p 8080:8080 -p 9090:9090 netguard-pg-backend ./netguard-server --pg-uri="postgres://user:password@postgres-host:5432/netguard" --grpc-addr=:9090 --http-addr=:8080
   ```

#### Использование Docker Compose

1. Запуск сервиса с базой данных в памяти:
   ```
   docker-compose up
   ```

2. Для использования PostgreSQL раскомментируйте сервис PostgreSQL в docker-compose.yml и выполните:
   ```
   docker-compose up
   ```

3. Доступ к сервису:
   - Swagger UI: http://localhost:8080/swagger/
   - gRPC: localhost:9090

#### Тестирование настройки Docker

Предоставляется тестовый скрипт для проверки настройки Docker:

```
./test-docker.sh
```

Этот скрипт собирает Docker-образ, запускает контейнер, проверяет доступность сервиса, а затем останавливает контейнер.

## API

Netguard PG Backend предоставляет RESTful API, созданный с использованием gRPC с gRPC-Gateway. API документирован с использованием Swagger.

### Swagger UI

Swagger UI доступен по адресу: http://localhost:8080/swagger/

### Доступные конечные точки

- `GET /v1/services` - Получить список сервисов
- `GET /v1/address-groups` - Получить список групп адресов
- `GET /v1/address-group-bindings` - Получить список привязок групп адресов
- `GET /v1/address-group-port-mappings` - Получить список сопоставлений портов групп адресов
- `GET /v1/rule-s2s` - Получить список правил взаимодействия сервисов
- `GET /v1/sync/status` - Получить статус последней синхронизации
- `POST /v1/sync` - Синхронизировать данные

## Разработка

### Структура проекта

```
netguard-pg-backend/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   └── netguard/
│   │       └── service.go
│   ├── app/
│   │   └── server/
│   │       └── setup-server.go
│   ├── application/
│   │   └── services/
│   │       └── service.go
│   ├── domain/
│   │   ├── models/
│   │   │   └── resources.go
│   │   └── ports/
│   │       ├── repositories.go
│   │       └── scopes.go
│   ├── infrastructure/
│   │   └── repositories/
│   │       ├── db.go
│   │       ├── mem/
│   │       │   ├── db.go
│   │       │   └── registry.go
│   │       └── pg/
│   │           └── models.go
│   └── patterns/
│       └── subject.go
├── protos/
│   ├── api/
│   │   ├── common/
│   │   │   └── ip-transport.proto
│   │   └── netguard/
│   │       └── api.proto
│   ├── 3d-party/
│   │   └── google/
│   │       ├── api/
│   │       │   ├── annotations.proto
│   │       │   ├── http.proto
│   │       │   └── ...
│   │       └── rpc/
│   │           ├── code.proto
│   │           ├── status.proto
│   │           └── ...
│   └── Makefile
├── swagger-ui/
│   └── index.html
├── Dockerfile
├── docker-compose.yml
├── copy-swagger.sh
└── go.mod
```

# Netguard Aggregated API Server: План реализации и тестирования

Этот документ описывает шаги для сборки, тестирования и деплоя `netguard-k8s-apiserver` и его зависимостей.

## 1. Архитектура

Система состоит из двух основных компонентов:
- **`netguard-pg-backend`**: Backend сервер, который хранит данные в PostgreSQL (или в in-memory для тестирования).
- **`netguard-k8s-apiserver`**: Aggregated API Server, который предоставляет Kubernetes-совместимый API и общается с backend'ом по gRPC.

## 2. Сборка Docker образов

Для сборки образов используются два Dockerfile:
- `Dockerfile.backend` - для `netguard-pg-backend`
- `Dockerfile.apiserver` - для `netguard-k8s-apiserver`

Сборка образов производится командой:
```bash
# Собираем backend
docker build -f Dockerfile.backend -t netguard/pg-backend:latest .

# Собираем API server
docker build -f Dockerfile.apiserver -t netguard/k8s-apiserver:latest .
```

## 3. Локальное тестирование

Для локального тестирования нам нужно:
1. Сгенерировать TLS сертификаты
2. Запустить backend сервер
3. Запустить API server
4. Проверить API через `curl`

### 3.1. Генерация TLS сертификатов
```bash
chmod +x scripts/generate-certs.sh
./scripts/generate-certs.sh
```

### 3.2. Запуск backend сервера
```bash
go run ./cmd/server --memory --grpc-addr ":9090" --http-addr ":8080"
```

### 3.3. Запуск API server
Создайте файл `local-config.yaml` с конфигурацией для локального тестирования:
```yaml
bind_address: "127.0.0.1"
secure_port: 8443
insecure_port: 0

authn:
  type: "tls"
  tls:
    cert-file: "certs/tls.crt"
    key-file: "certs/tls.key"
    client:
      verify: "skip"

backend_client:
  endpoint: "localhost:9090"
```

Запустите API server:
```bash
go run ./cmd/k8s-apiserver --config local-config.yaml
```

### 3.4. Тестирование API
```bash
# Проверяем health check
curl -k https://localhost:8443/healthz

# Проверяем API discovery
curl -k https://localhost:8443/apis/netguard.sgroups.io/v1beta1 | jq
```

## 4. Деплой в Kubernetes

### 4.1. Загрузка образов в кластер
Для локальных кластеров (minikube, kind):
```bash
# Для minikube
minikube image load netguard/pg-backend:latest
minikube image load netguard/k8s-apiserver:latest

# Для kind
kind load docker-image netguard/pg-backend:latest
kind load docker-image netguard/k8s-apiserver:latest
```
Для удаленных кластеров нужно запушить образы в registry.

### 4.2. Создание Kubernetes ресурсов
Все манифесты находятся в директории `config/k8s/`.

1. **Создаем Secret с сертификатами**:
   ```bash
   kubectl create secret tls netguard-apiserver-certs --cert=certs/tls.crt --key=certs/tls.key
   ```
2. **Применяем манифесты**:
   ```bash
   kubectl apply -f config/k8s/
   ```

### 4.3. Проверка деплоймента
```bash
# Проверяем статус подов
kubectl get pods -l app=netguard-apiserver
kubectl get pods -l app=netguard-backend

# Проверяем доступность API
kubectl api-resources --api-group=netguard.sgroups.io

# Проверяем создание ресурса
kubectl apply -f - <<EOF
apiVersion: netguard.sgroups.io/v1beta1
kind: Service
metadata:
  name: test-service
spec:
  ports:
  - port: 80
    protocol: TCP
EOF

kubectl get services.v1beta1.netguard.sgroups.io
```