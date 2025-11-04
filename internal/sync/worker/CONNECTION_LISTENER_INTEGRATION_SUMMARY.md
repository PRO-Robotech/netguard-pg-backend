# OutboxWorker ConnectionListener Integration - Summary

## ✅ Что сделано

### 1. Создан SGroupConnectionMonitor (`internal/sync/monitor/connection_monitor.go`)

**Назначение:** Отслеживает состояние подключения к SGROUP через SyncStatuses stream и уведомляет подписчиков.

**Основные возможности:**
- Подключение к SyncStatuses stream
- Автоматический reconnect с exponential backoff
- Рассылка событий (Connected/Disconnected/Timestamp) всем подписчикам
- Thread-safe подписка/отписка
- Получение статистики подключения

**API:**
```go
func NewSGroupConnectionMonitor(
    client interfaces.SGroupGateway,
    config ConnectionMonitorConfig,
    logger *zap.Logger,
) *SGroupConnectionMonitor

func (m *SGroupConnectionMonitor) Start()
func (m *SGroupConnectionMonitor) Stop()
func (m *SGroupConnectionMonitor) Subscribe(listener ConnectionListener)
func (m *SGroupConnectionMonitor) Unsubscribe(listenerName string)
func (m *SGroupConnectionMonitor) IsConnected() bool
func (m *SGroupConnectionMonitor) WaitForConnection(timeout time.Duration) error
func (m *SGroupConnectionMonitor) Stats() ConnectionStats
```

### 2. OutboxWorker реализует ConnectionListener (`internal/sync/worker/outbox_worker.go`)

**Изменения в структуре:**
```go
type OutboxWorker struct {
    // ... existing fields ...

    // Connection monitoring (NEW)
    connectionMonitor *monitor.SGroupConnectionMonitor
    isPaused          bool
    pausedMu          sync.RWMutex // protects isPaused
}
```

**Новые методы:**
```go
// ConnectionListener interface implementation
func (w *OutboxWorker) OnConnectionEvent(event monitor.ConnectionEvent)
func (w *OutboxWorker) GetListenerName() string
```

**Изменения в NewOutboxWorker:**
- Добавлен обязательный параметр `connectionMonitor *monitor.SGroupConnectionMonitor`
- Валидация: `if connectionMonitor == nil { logger.Fatal(...) }`

**Изменения в Start():**
- Подписка на connection events: `w.connectionMonitor.Subscribe(w)`

**Изменения в processBatch():**
- Проверка паузы в начале метода
- Если `isPaused == true` → skip обработки, entries остаются в pending

**Логика обработки событий:**
- `EventConnected` → `isPaused = false`, resume processing
- `EventDisconnected` → `isPaused = true`, pause processing
- `EventTimestamp` → игнорируется

### 3. Unit тесты

**Файлы:**
- `internal/sync/worker/outbox_worker_connection_listener_test.go` - тесты ConnectionListener interface
- `internal/sync/worker/outbox_worker_test.go` - обновлены существующие тесты
- `internal/sync/worker/mock_sgroup_gateway_test.go` - mock для SGroupGateway

**Покрытие:**
- ✅ Interface implementation
- ✅ OnConnectionEvent - Connected
- ✅ OnConnectionEvent - Disconnected
- ✅ OnConnectionEvent - Timestamp (ignored)
- ✅ Thread-safety
- ✅ GetListenerName
- ✅ ProcessBatch when paused (skips FindPending)

**Все тесты проходят:**
```
ok  	netguard-pg-backend/internal/sync/worker	0.540s
```

---

## ❌ Что НЕ сделано (по требованию)

### Интеграция в другие компоненты

**Следующие файлы требуют обновления для полной интеграции:**

#### 1. `cmd/server/main.go`

**Нужно:**
1. Создать SGroupConnectionMonitor перед OutboxWorker
2. Передать monitor в NewOutboxWorker
3. Запустить monitor.Start() после создания
4. Остановить monitor.Stop() в shutdown

**Пример:**
```go
// После создания sgroupClient

// Создать connection monitor
connectionMonitor := monitor.NewSGroupConnectionMonitor(
    sgroupClient,
    monitor.DefaultConfig(), // или из config файла
    logger,
)

// Запустить monitor
connectionMonitor.Start()
defer connectionMonitor.Stop()

// Создать OutboxWorker с monitor
outboxWorker := worker.NewOutboxWorker(
    pool,
    registry,
    hostSyncer,
    addressGroupSyncer,
    networkSyncer,
    serviceSyncer,
    logger,
    workerConfig,
    connectionMonitor, // ДОБАВИТЬ
)
```

#### 2. `internal/sync/integration/test_helpers.go`

**Строка 114:** Добавить mock connectionMonitor в NewOutboxWorker

#### 3. `internal/sync/integration/worker_helpers.go`

**Строка 242:** Добавить mock connectionMonitor в NewOutboxWorker

#### 4. `test/e2e/helpers/test_env.go`

**Строка 83:** Добавить mock connectionMonitor в NewOutboxWorker

**Для тестов можно использовать:**
```go
func createTestMonitor(t *testing.T, logger *zap.Logger) *monitor.SGroupConnectionMonitor {
    mockGateway := &mockSGroupGateway{} // или ваш mock
    config := monitor.DefaultConfig()
    return monitor.NewSGroupConnectionMonitor(mockGateway, config, logger)
}
```

---

## 📋 Критерии выполнения (DoD)

### ✅ Выполнено

- [x] OutboxWorker implements ConnectionListener interface
- [x] Добавлена зависимость от SGroupConnectionMonitor
- [x] isPaused флаг защищён RWMutex
- [x] processBatch() проверяет isPaused
- [x] OnConnectionEvent() корректно обрабатывает CONNECTED/DISCONNECTED
- [x] Subscribe() вызван в Start()
- [x] Код компилируется (worker пакет)
- [x] Сохранена существующая логика retry/error handling
- [x] Unit тесты написаны и проходят

### ❌ НЕ выполнено (намеренно)

- [ ] Интеграция в main.go (по требованию не делать)
- [ ] Обновление integration тестов (по требованию не делать)
- [ ] Обновление e2e тестов (по требованию не делать)
- [ ] Метрики prometheus для pause/resume (TODO в комментариях)

---

## 🔍 Как проверить работу

### 1. Unit тесты
```bash
go test -v ./internal/sync/worker -run "TestOutboxWorker"
```

### 2. Проверка pause/resume логики

**Тест сценарий:**
1. OutboxWorker запущен, monitor подписан
2. SGROUP disconnected → `isPaused = true`
3. ProcessBatch вызван → skip, entries остаются в pending
4. SGROUP connected → `isPaused = false`
5. ProcessBatch вызван → обработка возобновляется

**Проверено в тесте:**
```go
TestOutboxWorker_ProcessBatch_WhenPaused
TestOutboxWorker_OnConnectionEvent_Connected
TestOutboxWorker_OnConnectionEvent_Disconnected
```

### 3. Thread-safety

**Проверено в тесте:**
```go
TestOutboxWorker_OnConnectionEvent_ThreadSafety
```

Concurrent вызовы OnConnectionEvent (Connected/Disconnected) + concurrent reads `isPaused` - без race conditions.

---

## 📊 Метрики (TODO)

В коде помечены места для добавления метрик:

```go
// OnConnectionEvent
if wasPaused {
    w.logger.Info("OutboxWorker: SGROUP connected, resuming processing")
    // TODO: Add metric for pause duration
}

// OnConnectionEvent
w.logger.Warn("OutboxWorker: SGROUP disconnected, pausing processing", ...)
// TODO: Add metric for pause count
```

**Предложенные метрики:**
- `outbox_worker_paused` (gauge, 0/1)
- `outbox_worker_pause_count_total` (counter)
- `outbox_worker_pause_duration_seconds` (histogram)
- `outbox_worker_batches_skipped_total` (counter, reason="sgroup_disconnected")

---

## 🚀 Следующие шаги для полной интеграции

1. **Обновить cmd/server/main.go:**
   - Создать SGroupConnectionMonitor
   - Передать в NewOutboxWorker
   - Start/Stop lifecycle

2. **Обновить integration тесты:**
   - Добавить mock monitor в createTestWorker helpers

3. **Обновить e2e тесты:**
   - Добавить mock monitor в test environment setup

4. **Добавить конфигурацию monitor в config.yaml:**
   ```yaml
   connection_monitor:
     reconnect_interval: 5s
     max_reconnect_delay: 60s
     backoff_multiplier: 2.0
   ```

5. **Добавить prometheus метрики:**
   - Pause/resume events
   - Pause duration
   - Skipped batches count

6. **Документация:**
   - Обновить ARCHITECTURE.md
   - Добавить в README.md описание pause механизма

---

## 🔧 Troubleshooting

### Компиляция падает с "not enough arguments"

**Причина:** Старые вызовы NewOutboxWorker без connectionMonitor.

**Решение:** Добавить mock monitor:
```go
mockMonitor := monitor.NewSGroupConnectionMonitor(mockGateway, monitor.DefaultConfig(), logger)
worker := NewOutboxWorker(..., mockMonitor)
```

### Worker не возобновляет обработку после reconnect

**Причина:** Monitor не подписан или не запущен.

**Решение:**
1. Проверить что `connectionMonitor.Start()` вызван
2. Проверить что `connectionMonitor.Subscribe(worker)` вызван в worker.Start()
3. Проверить логи: "OutboxWorker subscribed to connection events"

### Entries накапливаются в pending

**Причина:** Worker постоянно в pause (SGROUP не доступен).

**Решение:**
1. Проверить connectivity к SGROUP
2. Проверить `connectionMonitor.IsConnected()` status
3. Проверить логи monitor: "SyncStatuses stream established"

---

**Generated:** 2025-10-18
**Status:** ✅ Core implementation complete, integration pending
