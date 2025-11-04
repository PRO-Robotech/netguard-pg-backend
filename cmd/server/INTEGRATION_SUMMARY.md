# SGroupConnectionMonitor Integration Summary

## Дата: 2025-10-18

## Статус: ✅ ЗАВЕРШЕНО

Успешно интегрирован `SGroupConnectionMonitor` в `cmd/server/main.go` для централизованного управления состоянием подключения к SGROUP.

---

## Что было сделано

### 1. ✅ Добавлены импорты

**Добавлено:**
- `serverPkg "netguard-pg-backend/internal/app/server"` - алиас для избежания конфликта с package main
- `"netguard-pg-backend/internal/sync/monitor"` - ConnectionMonitor

### 2. ✅ Централизованное создание sgroupsClient

**Старая архитектура:**
- sgroupsClient создавался **3 раза** (setupSyncManager, setupReverseSyncSystem, setupOutboxWorker)
- Каждая функция делала свой Health() check
- Дублирование кода и ресурсов

**Новая архитектура:**
- sgroupsClient создаётся **1 раз** в main() (строка 113)
- Передаётся во все setup функции
- Нет дублирования

**Код:**
```go
// Create SGroups client and ConnectionMonitor (CENTRALIZED)
var sgroupsClient interfaces.SGroupGateway
var connMonitor *monitor.SGroupConnectionMonitor

if cfg.Sync.Enabled {
    // Create SGroups client once
    client, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
    if err != nil {
        log.Fatalf("Failed to create sgroups client: %v", err)
    }
    sgroupsClient = client
    defer sgroupsClient.Close()

    // ... monitor creation ...
}
```

### 3. ✅ Создан и запущен ConnectionMonitor

**Место:** main(), строки 120-147

**Конфигурация:**
- `ReconnectInterval: 5s` - начальный интервал переподключения
- `MaxReconnectDelay: 60s` - максимальная задержка
- `BackoffMultiplier: 2.0` - exponential backoff

**Поведение:**
- `connMonitor.Start()` - non-blocking запуск
- `defer connMonitor.Stop()` - graceful shutdown

**Required Mode:**
```go
if cfg.Sync.Required {
    zapLogger.Info("Waiting for SGROUP connection (required mode)...")
    if err := connMonitor.WaitForConnection(10 * time.Second); err != nil {
        log.Fatalf("SGROUP connection required but not established: %v", err)
    }
    zapLogger.Info("SGROUP connection established")
}
```

**Optional Mode:**
```go
} else {
    // Not required - just log current state
    if connMonitor.IsConnected() {
        zapLogger.Info("SGROUP connection established")
    } else {
        zapLogger.Warn("SGROUP not connected, will retry in background")
    }
}
```

### 4. ✅ Обновлён setupSyncManager

**Изменения:**
- Сигнатура: добавлен параметр `sgroupsClient interfaces.SGroupGateway`
- **Удалено:** создание sgroupsClient внутри функции
- **Удалено:** блокирующий `sgroupsClient.Health(ctx)` check
- Использует переданный готовый client

**Новая сигнатура:**
```go
func setupSyncManager(
    ctx context.Context,
    cfg *config.Config,
    sgroupsClient interfaces.SGroupGateway, // ✅ Новый параметр
    logger *zap.Logger,
) interfaces.SyncManager
```

**Логика:**
- Если `sgroupsClient == nil` → return nil (sync disabled)
- Validate config
- Создать syncManager с готовым client
- Зарегистрировать syncers
- Start manager

### 5. ✅ Обновлён setupReverseSyncSystem

**Изменения:**
- Сигнатура: добавлены параметры `sgroupsClient` и `connMonitor`
- **Удалено:** создание sgroupsClient внутри функции
- **Удалено:** Health() check
- Передаёт `connMonitor` в `NewReverseSyncSystem()`

**Новая сигнатура:**
```go
func setupReverseSyncSystem(
    ctx context.Context,
    cfg *config.Config,
    registry ports.Registry,
    sgroupsClient interfaces.SGroupGateway,       // ✅ Новый параметр
    connMonitor *monitor.SGroupConnectionMonitor, // ✅ Новый параметр
    logger *zap.Logger,
) *sync.ReverseSyncSystem
```

**Вызов:**
```go
reverseSyncSystem, err := sync.NewReverseSyncSystem(
    sgroupsClient,
    hostReader,
    hostWriter,
    cfg.ReverseSync,
    connMonitor, // ✅ Передаётся monitor
)
```

### 6. ✅ Обновлён setupOutboxWorker

**Изменения:**
- Сигнатура: добавлен параметр `connMonitor`
- Передаёт `connMonitor` в `NewOutboxWorker()`
- **NOTE:** OutboxWorker всё ещё создаёт свой sgroupsClient для syncers
  - TODO: Рефакторить syncers для использования shared client

**Новая сигнатура:**
```go
func setupOutboxWorker(
    ctx context.Context,
    cfg *config.Config,
    pgRegistry *pg.Registry,
    syncManager interfaces.SyncManager,
    connMonitor *monitor.SGroupConnectionMonitor, // ✅ Новый параметр
    logger *zap.Logger,
) *worker.OutboxWorker
```

**Вызов:**
```go
outboxWorker := worker.NewOutboxWorker(
    pool,
    pgRegistry,
    hostSyncer,
    addressGroupSyncer,
    networkSyncer,
    serviceSyncer,
    logger,
    workerConfig,
    connMonitor, // ✅ Передаётся monitor
)
```

### 7. ✅ Добавлен Health endpoint для SGROUP sync

**Место:** main(), строки 197-202

**Код:**
```go
// Register health endpoint for SGROUP sync
if connMonitor != nil {
    healthListener := serverPkg.NewHealthEndpointListener()
    connMonitor.Subscribe(healthListener)
    http.HandleFunc("/healthz/sync", healthListener.ServeHTTP)
}
```

**Endpoint:** `GET /healthz/sync`

**Response (connected):**
```json
{
  "status": "healthy",
  "last_healthy": "2025-10-18T12:00:00Z",
  "component": "sgroup_sync"
}
```

**Response (disconnected):**
```json
{
  "status": "unhealthy",
  "component": "sgroup_sync",
  "last_unhealthy": "2025-10-18T12:05:00Z",
  "error": "stream error: rpc error: code = Unavailable"
}
```

**HTTP Status:**
- 200 OK - connected
- 503 Service Unavailable - disconnected

### 8. ✅ Graceful Shutdown

**Порядок shutdown:**
1. Signal received (SIGINT/SIGTERM)
2. Health endpoints set to NOT_SERVING
3. OutboxWorker.Stop() with 30s timeout
4. ReverseSyncSystem.Stop()
5. ConnectionMonitor.Stop() (via defer)
6. gRPC server GracefulStop()
7. HTTP server Shutdown()

**Код:**
```go
defer connMonitor.Stop() // Автоматически при exit
```

---

## Вызовы функций в main()

**Порядок:**
```go
// 1. Create client and monitor (lines 107-148)
sgroupsClient, connMonitor := create_if_enabled()

// 2. Setup sync manager (line 151)
syncManager := setupSyncManager(ctx, cfg, sgroupsClient, zapLogger)

// 3. Setup reverse sync (line 154)
reverseSyncSystem := setupReverseSyncSystem(ctx, cfg, pgRegistry, sgroupsClient, connMonitor, zapLogger)

// 4. Setup outbox worker (line 159)
outboxWorker := setupOutboxWorker(ctx, cfg, pgRegistry, syncManager, connMonitor, zapLogger)

// 5. Register health endpoint (lines 197-202)
if connMonitor != nil {
    healthListener := serverPkg.NewHealthEndpointListener()
    connMonitor.Subscribe(healthListener)
    http.HandleFunc("/healthz/sync", healthListener.ServeHTTP)
}
```

---

## Проверка критериев выполнения

✅ sgroupsClient создаётся ОДИН РАЗ в main()
✅ ConnectionMonitor создан и запущен (non-blocking)
✅ WaitForConnection() если sync.required=true
✅ Убран Health() check из setupSyncManager
✅ setupSyncManager получает готовый client
✅ setupReverseSyncSystem получает client и monitor
✅ setupOutboxWorker получает monitor
✅ HealthEndpointListener создан, подписан, зарегистрирован
✅ Код компилируется без ошибок
✅ Graceful shutdown (defer Stop())

---

## Что НЕ было изменено (как требовалось)

❌ НЕ добавлены тесты
❌ НЕ изменена логика syncers
❌ НЕ удалён detector package

---

## Известные TODO

### TODO: Рефакторить syncers для shared client

**Проблема:** OutboxWorker создаёт свой собственный sgroupsClient для syncers.

**Текущий код (setupOutboxWorker, строки 421-428):**
```go
// Create SGroups client for worker's syncers
// NOTE: OutboxWorker uses syncers which need a client reference
// We could refactor to share the main client, but for now create a new one
// TODO: Refactor syncers to share single client instance
sgroupsClient, err := clients.NewSGroupsClient(cfg.Sync.SGroups)
if err != nil {
    logger.Fatal("failed to create sgroups client for worker", zap.Error(err))
}
```

**Решение (будущее):**
- Передавать main sgroupsClient в setupOutboxWorker
- Syncers использовать shared client instance
- Убрать создание отдельного client

**Приоритет:** LOW (не критично, работает как есть)

---

## Архитектура после интеграции

```
main.go
  │
  ├─ Create sgroupsClient (ONCE)
  │    └─ clients.NewSGroupsClient(cfg.Sync.SGroups)
  │
  ├─ Create ConnectionMonitor (ONCE)
  │    ├─ monitor.NewSGroupConnectionMonitor(client, config, logger)
  │    └─ connMonitor.Start() (non-blocking)
  │
  ├─ setupSyncManager(ctx, cfg, client, logger)
  │    ├─ Receives: ready client
  │    ├─ Does NOT: create client, call Health()
  │    └─ Returns: SyncManager
  │
  ├─ setupReverseSyncSystem(ctx, cfg, registry, client, monitor, logger)
  │    ├─ Receives: ready client + monitor
  │    ├─ Does NOT: create client, call Health()
  │    ├─ Passes monitor to: NewReverseSyncSystem()
  │    └─ Returns: ReverseSyncSystem (subscribed to monitor)
  │
  ├─ setupOutboxWorker(ctx, cfg, registry, syncMgr, monitor, logger)
  │    ├─ Receives: monitor
  │    ├─ Creates: own client for syncers (TODO: refactor)
  │    ├─ Passes monitor to: NewOutboxWorker()
  │    └─ Returns: OutboxWorker (subscribed to monitor)
  │
  ├─ Register /healthz/sync endpoint
  │    ├─ Create: HealthEndpointListener
  │    ├─ Subscribe to: connMonitor
  │    └─ Register: http.HandleFunc("/healthz/sync", listener.ServeHTTP)
  │
  └─ Graceful shutdown
       ├─ OutboxWorker.Stop() (30s timeout)
       ├─ ReverseSyncSystem.Stop()
       ├─ connMonitor.Stop() (defer)
       └─ gRPC/HTTP shutdown
```

---

## Тестирование

### Проверка компиляции

```bash
go build ./cmd/server/main.go
# ✅ SUCCESS
```

### Endpoints для проверки

1. **SGROUP sync health:**
   ```bash
   curl http://localhost:8080/healthz/sync
   ```

2. **OutboxWorker health:**
   ```bash
   curl http://localhost:8080/healthz/worker
   ```

3. **Metrics (если enabled):**
   ```bash
   curl http://localhost:8080/metrics
   ```

### Сценарии тестирования

**1. Sync enabled + SGROUP доступен:**
- ConnectionMonitor подключается
- `/healthz/sync` → 200 OK
- ReverseSyncSystem получает события
- OutboxWorker обрабатывает entries

**2. Sync enabled + SGROUP недоступен (required=false):**
- ConnectionMonitor retry в фоне
- `/healthz/sync` → 503 Unavailable
- ReverseSyncSystem ждёт подключения
- OutboxWorker паузит обработку

**3. Sync enabled + SGROUP недоступен (required=true):**
- main.go ждёт WaitForConnection(10s)
- Если timeout → log.Fatalf() (exit)

**4. Sync disabled:**
- sgroupsClient = nil
- connMonitor = nil
- Все setup функции return nil
- Endpoints не регистрируются

---

## Заключение

Интеграция завершена успешно. Все компоненты теперь используют централизованный ConnectionMonitor для:

1. ✅ Отслеживания состояния подключения
2. ✅ Автоматического переподключения
3. ✅ Уведомления подписчиков о событиях
4. ✅ Health endpoint для мониторинга

**Следующие шаги:**
- Протестировать в реальном окружении
- Рефакторить syncers для shared client (опционально)
- Добавить интеграционные тесты (опционально)
