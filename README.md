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