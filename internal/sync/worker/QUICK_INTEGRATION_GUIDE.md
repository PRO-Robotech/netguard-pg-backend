# Quick Integration Guide - OutboxWorker + ConnectionMonitor

## Для быстрой интеграции в main.go

### Шаг 1: Создать ConnectionMonitor после SGroupGateway

```go
// После создания sgroupClient
sgroupClient, err := sgroup.NewGateway(cfg.Sync.SGroups, logger)
if err != nil {
    logger.Fatal("failed to create SGROUP client", zap.Error(err))
}

// ДОБАВИТЬ: Создать connection monitor
connectionMonitor := monitor.NewSGroupConnectionMonitor(
    sgroupClient,
    monitor.ConnectionMonitorConfig{
        ReconnectInterval: 5 * time.Second,
        MaxReconnectDelay: 60 * time.Second,
        BackoffMultiplier: 2.0,
    },
    logger,
)

// Запустить monitor
connectionMonitor.Start()
logger.Info("SGroupConnectionMonitor started")
```

### Шаг 2: Передать в OutboxWorker

```go
// Найти создание OutboxWorker (примерно строка 384)
outboxWorker := worker.NewOutboxWorker(
    pool,
    registry,
    hostSyncer,
    addressGroupSyncer,
    networkSyncer,
    serviceSyncer,
    logger,
    workerConfig,
    connectionMonitor, // ДОБАВИТЬ эту строку
)
```

### Шаг 3: Остановить monitor в shutdown

```go
// В shutdown handler (перед sgroupClient.Close())
logger.Info("Stopping connection monitor...")
connectionMonitor.Stop()
logger.Info("Connection monitor stopped")
```

---

## Для integration тестов

### `internal/sync/integration/test_helpers.go:114`

```go
// Добавить helper
func createTestConnectionMonitor(t *testing.T, logger *zap.Logger) *monitor.SGroupConnectionMonitor {
    mockGateway := &mockSGroupGateway{} // используйте существующий mock
    config := monitor.DefaultConfig()
    return monitor.NewSGroupConnectionMonitor(mockGateway, config, logger)
}

// Обновить createTestWorker
func createTestWorker(...) *worker.OutboxWorker {
    monitor := createTestConnectionMonitor(t, logger)

    return worker.NewOutboxWorker(
        pool,
        registry,
        hostSyncer,
        addressGroupSyncer,
        networkSyncer,
        serviceSyncer,
        logger,
        workerConfig,
        monitor, // ДОБАВИТЬ
    )
}
```

### `internal/sync/integration/worker_helpers.go:242`

Аналогично test_helpers.go - добавить monitor parameter.

---

## Для e2e тестов

### `test/e2e/helpers/test_env.go:83`

```go
// В setupOutboxWorker
monitor := createTestConnectionMonitor(t, env.logger)

env.outboxWorker = worker.NewOutboxWorker(
    env.pool,
    env.registry,
    env.hostSyncer,
    env.addressGroupSyncer,
    env.networkSyncer,
    env.serviceSyncer,
    env.logger,
    workerConfig,
    monitor, // ДОБАВИТЬ
)
```

---

## Проверка после интеграции

### 1. Компиляция
```bash
go build ./...
```

### 2. Тесты
```bash
go test ./internal/sync/worker -v
go test ./internal/sync/integration -v
go test ./test/e2e -v
```

### 3. Логи при запуске

**Ожидаемые логи:**
```
INFO SGroupConnectionMonitor started
INFO OutboxWorker subscribed to connection events
INFO Connection monitor started
INFO Monitor loop started
INFO SyncStatuses stream established  # когда SGROUP доступен
INFO OutboxWorker starting
```

### 4. Логи при disconnect/reconnect

**Disconnect:**
```
WARN SyncStatuses stream closed by server
WARN OutboxWorker: SGROUP disconnected, pausing processing
DEBUG Skipping batch processing - SGROUP disconnected
```

**Reconnect:**
```
INFO SyncStatuses stream established
INFO OutboxWorker: SGROUP connected, resuming processing
INFO processing batch  # обработка возобновлена
```

---

## Rollback plan

Если что-то пошло не так:

```bash
# Откатить изменения
git checkout HEAD -- cmd/server/main.go
git checkout HEAD -- internal/sync/integration/
git checkout HEAD -- test/e2e/helpers/
```

Старый код без monitor будет падать на компиляции с:
```
not enough arguments in call to worker.NewOutboxWorker
```

---

**Last updated:** 2025-10-18
