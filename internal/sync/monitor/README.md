# SGroupConnectionMonitor

Event-driven монитор подключения к SGROUP через SyncStatuses stream.

## Проблема

Backend зависал на 30 секунд при старте если SGROUP недоступен из-за синхронного Health() check.

## Решение

SGroupConnectionMonitor использует SyncStatuses stream как **единственный индикатор** доступности SGROUP:
- Stream открыт → SGROUP доступен
- Stream закрыт → SGROUP недоступен
- Timestamp получен → SGROUP жив

## Архитектура

```
┌──────────────────────────────────┐
│  SGroupConnectionMonitor         │
│  (владеет SyncStatuses stream)   │
└────────────┬─────────────────────┘
             │
             │ broadcastEvent()
             │
    ┌────────┼────────┬────────┐
    ▼        ▼        ▼        ▼
┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐
│ L1  │  │ L2  │  │ L3  │  │ Ln  │
└─────┘  └─────┘  └─────┘  └─────┘
ConnectionListener интерфейс
```

## Использование

### 1. Базовое использование

```go
import "netguard-pg-backend/internal/sync/monitor"

// Создать монитор
config := monitor.DefaultConfig()
mon := monitor.NewSGroupConnectionMonitor(sgroupClient, config, logger)

// Запустить мониторинг (non-blocking!)
mon.Start()

// Дождаться подключения (опционально)
if err := mon.WaitForConnection(30 * time.Second); err != nil {
    log.Fatal("SGROUP не доступен:", err)
}

// Проверить состояние
if mon.IsConnected() {
    log.Info("SGROUP доступен")
}

// При завершении
mon.Stop()
```

### 2. Подписка на события

```go
type MyListener struct {
    name string
}

func (l *MyListener) GetListenerName() string {
    return l.name
}

func (l *MyListener) OnConnectionEvent(event monitor.ConnectionEvent) {
    switch event.Type {
    case monitor.EventConnected:
        log.Info("SGROUP подключен")
        
    case monitor.EventDisconnected:
        log.Error("SGROUP отключен", event.Error)
        
    case monitor.EventTimestamp:
        log.Debug("Timestamp:", event.Timestamp.AsTime())
    }
}

// Подписаться
listener := &MyListener{name: "my-listener"}
mon.Subscribe(listener)

// Отписаться
mon.Unsubscribe("my-listener")
```

### 3. Получение статистики

```go
stats := mon.Stats()
fmt.Printf("Connected: %v\n", stats.IsConnected)
fmt.Printf("Last Connected: %v\n", stats.LastConnected)
fmt.Printf("Listeners: %d\n", stats.ListenerCount)
```

## Конфигурация

```go
config := monitor.ConnectionMonitorConfig{
    ReconnectInterval: 5 * time.Second,   // начальная задержка
    MaxReconnectDelay: 60 * time.Second,  // максимальная задержка
    BackoffMultiplier: 2.0,               // множитель backoff
}
```

**Exponential backoff:**
- 1-я попытка: 5s
- 2-я попытка: 10s
- 3-я попытка: 20s
- 4-я попытка: 40s
- 5+ попытки: 60s (макс)

## Особенности реализации

### Thread-Safety
- Все публичные методы thread-safe
- State защищен `sync.RWMutex`
- Listeners защищены отдельным `listenersMu`

### Non-Blocking
- `Start()` возвращается сразу
- `broadcastEvent()` рассылает в goroutines
- Listeners не блокируют друг друга

### Graceful Shutdown
- `Stop()` вызывает `cancel()`
- Ждет завершения через `wg.Wait()`
- Гарантирует чистую остановку

### Бесконечный Reconnect
- Нет `MaxRetries` - reconnect бесконечно
- Exponential backoff до `MaxReconnectDelay`
- При успехе - сброс delay

## События

| Тип              | Когда              | Поля                |
|------------------|--------------------|---------------------|
| EventConnected   | Stream открыт      | OccurredAt          |
| EventDisconnected| Stream закрыт      | Error, OccurredAt   |
| EventTimestamp   | Получен timestamp  | Timestamp, OccurredAt |

## Интеграция в main.go

```go
// Создать монитор ДО SyncManager
monitor := monitor.NewSGroupConnectionMonitor(
    sgroupClient,
    monitor.DefaultConfig(),
    logger,
)

// Запустить мониторинг
monitor.Start()
defer monitor.Stop()

// Дождаться подключения (optional, non-blocking)
go func() {
    if err := monitor.WaitForConnection(30 * time.Second); err != nil {
        logger.Warn("SGROUP недоступен при старте", zap.Error(err))
    } else {
        logger.Info("SGROUP доступен")
    }
}()

// Создать SyncManager (не ждет SGROUP!)
syncManager := setupSyncManager(ctx, cfg, db, sgroupClient, logger)
```

## Тесты

Тесты будут реализованы в отдельной задаче.

## Статус

✅ events.go реализован
✅ connection_monitor.go реализован
✅ Код компилируется без ошибок
✅ Следование Go best practices
✅ Package documentation
