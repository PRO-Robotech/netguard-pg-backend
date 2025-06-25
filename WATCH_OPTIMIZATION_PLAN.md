# План оптимизации Watch functionality: gRPC Streaming

## Обзор

Текущая реализация использует **Shared Poller** подход - один поллер на тип ресурса с polling каждые 5 секунд. Это эффективное решение, но имеет ограничения:

- **Задержка событий:** до 5 секунд
- **Polling overhead:** постоянные запросы к backend даже без изменений
- **Ресурсоемкость:** 8 поллеров работают постоянно

**gRPC Streaming** - оптимальное решение для real-time событий с минимальной задержкой и нагрузкой.

---

## Архитектура gRPC Streaming

### Текущая архитектура (Shared Poller)
```
kubectl get -w → API Server → Shared Poller → Backend List API (каждые 5 сек)
                     ↓
                Event Multiplexing → Multiple Clients
```

### Целевая архитектура (gRPC Streaming)
```
kubectl get -w → API Server → Stream Manager → Backend Stream API (real-time)
                     ↓
                Event Multiplexing → Multiple Clients
```

**Преимущества:**
- **Real-time события:** < 100ms задержка
- **Минимальная нагрузка:** события только при изменениях
- **Эффективность:** один stream на тип ресурса
- **Обратная совместимость:** тот же API для клиентов

---

## Этап 1: Расширение Protobuf API

### 1.1 Добавление streaming методов
- [ ] **Задача:** Расширить protobuf определения для streaming
- [ ] **Зачем:** Backend должен поддерживать real-time streams

**Файл:** `protos/api/sgroups/sgroups.proto`

```protobuf
service NetguardService {
    // Существующие методы...
    rpc GetService(GetServiceReq) returns (GetServiceResp);
    rpc ListServices(ListServicesReq) returns (ListServicesResp);
    
    // Новые streaming методы
    rpc WatchServices(WatchServicesReq) returns (stream WatchEvent);
    rpc WatchAddressGroups(WatchAddressGroupsReq) returns (stream WatchEvent);
    rpc WatchAddressGroupBindings(WatchAddressGroupBindingsReq) returns (stream WatchEvent);
    rpc WatchAddressGroupPortMappings(WatchAddressGroupPortMappingsReq) returns (stream WatchEvent);
    rpc WatchRuleS2S(WatchRuleS2SReq) returns (stream WatchEvent);
    rpc WatchServiceAliases(WatchServiceAliasesReq) returns (stream WatchEvent);
    rpc WatchAddressGroupBindingPolicies(WatchAddressGroupBindingPoliciesReq) returns (stream WatchEvent);
    rpc WatchIEAgAgRules(WatchIEAgAgRulesReq) returns (stream WatchEvent);
}

// Общий тип события для всех ресурсов
message WatchEvent {
    enum EventType {
        ADDED = 0;
        MODIFIED = 1;
        DELETED = 2;
        ERROR = 3;
    }
    
    EventType type = 1;
    google.protobuf.Any resource = 2;  // Содержит конкретный тип ресурса
    string resource_version = 3;
    string error_message = 4;  // Для ERROR событий
}

// Запросы для Watch методов
message WatchServicesReq {
    string namespace = 1;  // Пустое = все namespaces
    string label_selector = 2;
    string resource_version = 3;  // Начать с этой версии
}

message WatchAddressGroupsReq {
    string namespace = 1;
    string label_selector = 2;
    string resource_version = 3;
}

// ... аналогично для всех остальных типов ресурсов
```

### 1.2 Генерация Go кода
- [ ] **Задача:** Обновить protobuf генерацию
- [ ] **Зачем:** Получить Go интерфейсы для streaming

```bash
cd protos
make generate
```

---

## Этап 2: Backend Implementation

### 2.1 Event Store в Backend
- [ ] **Задача:** Реализовать хранение событий в backend
- [ ] **Зачем:** Источник событий для streaming

**Файл:** `internal/infrastructure/event_store.go`

```go
type EventStore interface {
    // Подписка на события для типа ресурса
    Subscribe(ctx context.Context, resourceType string, options SubscribeOptions) (<-chan Event, error)
    
    // Публикация события (вызывается из repository при изменениях)
    Publish(event Event) error
    
    // Получение событий с определенной версии
    GetEventsSince(resourceType string, resourceVersion string) ([]Event, error)
}

type Event struct {
    Type           EventType
    ResourceType   string
    ResourceID     models.ResourceIdentifier
    Resource       interface{}
    ResourceVersion string
    Timestamp      time.Time
}

type SubscribeOptions struct {
    Namespace     string
    LabelSelector string
    ResourceVersion string
}

// In-memory реализация для начала
type InMemoryEventStore struct {
    mu         sync.RWMutex
    events     []Event
    subscribers map[string][]chan Event  // resourceType -> channels
    resourceVersionCounter int64
}
```

### 2.2 Интеграция с Repository
- [ ] **Задача:** Генерировать события при изменениях в repository
- [ ] **Зачем:** Event Store должен знать об изменениях

**Обновление:** `internal/infrastructure/repository/service_repository.go`

```go
type ServiceRepository struct {
    db         *sql.DB
    eventStore EventStore  // Новая зависимость
}

func (r *ServiceRepository) Create(ctx context.Context, service *models.Service) error {
    // Существующая логика создания
    err := r.createInDB(ctx, service)
    if err != nil {
        return err
    }
    
    // Публикация события
    event := Event{
        Type:         EventTypeAdded,
        ResourceType: "services",
        ResourceID:   service.ResourceIdentifier,
        Resource:     service,
        ResourceVersion: r.generateResourceVersion(),
        Timestamp:    time.Now(),
    }
    r.eventStore.Publish(event)
    
    return nil
}

func (r *ServiceRepository) Update(ctx context.Context, service *models.Service) error {
    // Существующая логика обновления
    err := r.updateInDB(ctx, service)
    if err != nil {
        return err
    }
    
    // Публикация события
    event := Event{
        Type:         EventTypeModified,
        ResourceType: "services",
        ResourceID:   service.ResourceIdentifier,
        Resource:     service,
        ResourceVersion: r.generateResourceVersion(),
        Timestamp:    time.Now(),
    }
    r.eventStore.Publish(event)
    
    return nil
}

// Аналогично для Delete и всех остальных repository
```

### 2.3 gRPC Streaming Handlers
- [ ] **Задача:** Реализовать streaming методы в gRPC сервере
- [ ] **Зачем:** API для получения real-time событий

**Файл:** `internal/grpc/watch_handlers.go`

```go
func (s *NetguardServer) WatchServices(req *pb.WatchServicesReq, stream pb.NetguardService_WatchServicesServer) error {
    ctx := stream.Context()
    
    // Подписка на события
    eventChan, err := s.eventStore.Subscribe(ctx, "services", SubscribeOptions{
        Namespace:       req.Namespace,
        LabelSelector:   req.LabelSelector,
        ResourceVersion: req.ResourceVersion,
    })
    if err != nil {
        return fmt.Errorf("failed to subscribe to service events: %w", err)
    }
    
    // Отправка initial snapshot если нужно
    if req.ResourceVersion == "" || req.ResourceVersion == "0" {
        if err := s.sendInitialSnapshot(stream, "services", req); err != nil {
            return fmt.Errorf("failed to send initial snapshot: %w", err)
        }
    }
    
    // Streaming событий
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event, ok := <-eventChan:
            if !ok {
                return nil  // Channel closed
            }
            
            // Конвертация в protobuf
            pbEvent, err := s.convertEventToProtobuf(event)
            if err != nil {
                log.Printf("Failed to convert event to protobuf: %v", err)
                continue
            }
            
            // Отправка события
            if err := stream.Send(pbEvent); err != nil {
                return fmt.Errorf("failed to send event: %w", err)
            }
        }
    }
}

func (s *NetguardServer) sendInitialSnapshot(stream pb.NetguardService_WatchServicesServer, resourceType string, req *pb.WatchServicesReq) error {
    // Получить все существующие ресурсы
    services, err := s.serviceApp.ListServices(stream.Context(), ports.Scope{})
    if err != nil {
        return err
    }
    
    // Отправить как ADDED события
    for _, service := range services {
        event := &pb.WatchEvent{
            Type: pb.WatchEvent_ADDED,
            Resource: &google_protobuf.Any{
                TypeUrl: "type.googleapis.com/sgroups.Service",
                Value:   mustMarshal(service),
            },
            ResourceVersion: s.getCurrentResourceVersion(),
        }
        
        if err := stream.Send(event); err != nil {
            return err
        }
    }
    
    return nil
}

// Аналогично для всех остальных WatchXXX методов
```

---

## Этап 3: Aggregated API Integration

### 3.1 gRPC Streaming Client
- [ ] **Задача:** Расширить BackendClient для streaming
- [ ] **Зачем:** API Server должен уметь получать streams от backend

**Обновление:** `internal/k8s/client/backend.go`

```go
type BackendClient interface {
    // Существующие методы...
    
    // Новые streaming методы
    WatchServices(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchAddressGroups(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchAddressGroupBindings(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchAddressGroupPortMappings(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchRuleS2S(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchServiceAliases(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchAddressGroupBindingPolicies(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
    WatchIEAgAgRules(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error)
}

type WatchOptions struct {
    Namespace       string
    LabelSelector   string
    ResourceVersion string
}

type WatchEvent struct {
    Type     WatchEventType
    Resource interface{}
    ResourceVersion string
    Error    error
}

func (c *GRPCBackendClient) WatchServices(ctx context.Context, options WatchOptions) (<-chan WatchEvent, error) {
    req := &pb.WatchServicesReq{
        Namespace:       options.Namespace,
        LabelSelector:   options.LabelSelector,
        ResourceVersion: options.ResourceVersion,
    }
    
    stream, err := c.client.WatchServices(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("failed to start watch stream: %w", err)
    }
    
    eventChan := make(chan WatchEvent, 100)
    
    go func() {
        defer close(eventChan)
        
        for {
            pbEvent, err := stream.Recv()
            if err != nil {
                if err == io.EOF {
                    return  // Stream closed normally
                }
                eventChan <- WatchEvent{Error: err}
                return
            }
            
            // Конвертация из protobuf
            event, err := c.convertEventFromProtobuf(pbEvent)
            if err != nil {
                log.Printf("Failed to convert event from protobuf: %v", err)
                continue
            }
            
            select {
            case eventChan <- event:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return eventChan, nil
}
```

### 3.2 Stream Manager
- [ ] **Задача:** Заменить Shared Poller на Stream Manager
- [ ] **Зачем:** Управление gRPC streams и мультиплексирование событий

**Файл:** `internal/k8s/registry/watch/stream_manager.go`

```go
// StreamManager управляет gRPC streams для всех типов ресурсов
type StreamManager struct {
    backend BackendClient
    
    mu      sync.RWMutex
    streams map[string]*ResourceStream  // resourceType -> ResourceStream
}

type ResourceStream struct {
    resourceType string
    backend      BackendClient
    
    mu           sync.RWMutex
    clients      map[string]*StreamClient
    eventChan    <-chan WatchEvent
    cancel       context.CancelFunc
    done         chan struct{}
}

type StreamClient struct {
    id        string
    eventChan chan watch.Event
    filter    *metav1.ListOptions
    done      chan struct{}
}

func NewStreamManager(backend BackendClient) *StreamManager {
    return &StreamManager{
        backend: backend,
        streams: make(map[string]*ResourceStream),
    }
}

func (sm *StreamManager) GetStream(resourceType string) *ResourceStream {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    if stream, exists := sm.streams[resourceType]; exists {
        return stream
    }
    
    // Создать новый stream
    stream := NewResourceStream(resourceType, sm.backend)
    sm.streams[resourceType] = stream
    
    return stream
}

func NewResourceStream(resourceType string, backend BackendClient) *ResourceStream {
    ctx, cancel := context.WithCancel(context.Background())
    
    stream := &ResourceStream{
        resourceType: resourceType,
        backend:      backend,
        clients:      make(map[string]*StreamClient),
        cancel:       cancel,
        done:         make(chan struct{}),
    }
    
    go stream.streamLoop(ctx)
    return stream
}

func (rs *ResourceStream) streamLoop(ctx context.Context) {
    defer close(rs.done)
    
    // Запуск gRPC stream
    var eventChan <-chan WatchEvent
    var err error
    
    switch rs.resourceType {
    case "services":
        eventChan, err = rs.backend.WatchServices(ctx, WatchOptions{})
    case "addressgroups":
        eventChan, err = rs.backend.WatchAddressGroups(ctx, WatchOptions{})
    // ... остальные типы ресурсов
    default:
        log.Printf("Unsupported resource type: %s", rs.resourceType)
        return
    }
    
    if err != nil {
        log.Printf("Failed to start stream for %s: %v", rs.resourceType, err)
        return
    }
    
    rs.eventChan = eventChan
    
    // Обработка событий
    for {
        select {
        case <-ctx.Done():
            return
        case event, ok := <-eventChan:
            if !ok {
                log.Printf("Stream closed for %s", rs.resourceType)
                return
            }
            
            if event.Error != nil {
                log.Printf("Stream error for %s: %v", rs.resourceType, event.Error)
                // Можно реализовать reconnect логику здесь
                continue
            }
            
            // Конвертация в Kubernetes event
            k8sEvent := watch.Event{
                Type:   rs.convertEventType(event.Type),
                Object: rs.convertResource(event.Resource),
            }
            
            rs.broadcastEvent(k8sEvent)
        }
    }
}

func (rs *ResourceStream) AddClient(options *metav1.ListOptions) (*StreamClient, error) {
    rs.mu.Lock()
    defer rs.mu.Unlock()
    
    clientID := uuid.New().String()
    client := &StreamClient{
        id:        clientID,
        eventChan: make(chan watch.Event, 100),
        filter:    options,
        done:      make(chan struct{}),
    }
    
    rs.clients[clientID] = client
    return client, nil
}

func (rs *ResourceStream) broadcastEvent(event watch.Event) {
    rs.mu.RLock()
    defer rs.mu.RUnlock()
    
    for _, client := range rs.clients {
        if rs.matchesFilter(event.Object, client.filter) {
            select {
            case client.eventChan <- event:
            case <-client.done:
                // Клиент закрыт
            default:
                // Channel переполнен
                log.Printf("Client %s event channel full, dropping event", client.id)
            }
        }
    }
}
```

### 3.3 Обновление Storage для Streaming
- [ ] **Задача:** Заменить Shared Poller на Stream Manager в Storage
- [ ] **Зачем:** Использовать real-time события вместо polling

**Обновление:** `internal/k8s/registry/service/storage.go`

```go
func (s *ServiceStorage) Watch(ctx context.Context, options *metav1.ListOptions) (watch.Interface, error) {
    streamManager := GetStreamManager(s.backendClient)
    stream := streamManager.GetStream("services")
    
    client, err := stream.AddClient(options)
    if err != nil {
        return nil, fmt.Errorf("failed to add stream client: %w", err)
    }
    
    return &StreamWatchInterface{
        client: client,
        stream: stream,
    }, nil
}
```

---

## Этап 4: Миграция и Backward Compatibility

### 4.1 Feature Flag
- [ ] **Задача:** Возможность переключения между Polling и Streaming
- [ ] **Зачем:** Безопасная миграция и откат при проблемах

**Файл:** `internal/k8s/config/watch_config.go`

```go
import (
    "time"
    "github.com/ilyakaznacheev/cleanenv"
)

type WatchConfig struct {
    Mode                 WatchMode     `yaml:"mode" env:"WATCH_MODE" env-default:"polling" env-description:"Watch mode (polling, streaming, auto)"`
    StreamingEnabled     bool          `yaml:"streaming_enabled" env:"WATCH_STREAMING_ENABLED" env-default:"false" env-description:"Enable streaming if supported"`
    PollingInterval      time.Duration `yaml:"polling_interval" env:"WATCH_POLLING_INTERVAL" env-default:"5s" env-description:"Polling interval"`
    StreamReconnectDelay time.Duration `yaml:"stream_reconnect_delay" env:"WATCH_STREAM_RECONNECT_DELAY" env-default:"1s" env-description:"Stream reconnect delay"`
}

type WatchMode string

const (
    WatchModePolling   WatchMode = "polling"
    WatchModeStreaming WatchMode = "streaming"
    WatchModeAuto      WatchMode = "auto"  // Попробовать streaming, fallback на polling
)

func (wc *WatchConfig) ShouldUseStreaming() bool {
    switch wc.Mode {
    case WatchModeStreaming:
        return true
    case WatchModePolling:
        return false
    case WatchModeAuto:
        return wc.StreamingEnabled  // Определяется автоматически при подключении
    default:
        return false
    }
}

// Validate проверяет корректность конфигурации
func (w *WatchConfig) Validate() error {
    switch w.Mode {
    case WatchModePolling, WatchModeStreaming, WatchModeAuto:
        // OK
    default:
        return fmt.Errorf("invalid watch mode: %s", w.Mode)
    }
    
    if w.PollingInterval <= 0 {
        return fmt.Errorf("polling_interval must be positive")
    }
    
    if w.StreamReconnectDelay <= 0 {
        return fmt.Errorf("stream_reconnect_delay must be positive")
    }
    
    return nil
}

// LoadWatchConfig загружает конфигурацию Watch с помощью cleanenv
func LoadWatchConfig(configPath string) (WatchConfig, error) {
    var config WatchConfig
    
    if configPath != "" {
        err := cleanenv.ReadConfig(configPath, &config)
        if err != nil {
            return config, fmt.Errorf("failed to read watch config from %s: %w", configPath, err)
        }
    } else {
        err := cleanenv.ReadEnv(&config)
        if err != nil {
            return config, fmt.Errorf("failed to read watch config from environment: %w", err)
        }
    }
    
    return config, nil
}
```

### 4.2 Hybrid Watch Manager
- [ ] **Задача:** Менеджер который может использовать оба подхода
- [ ] **Зачем:** Graceful fallback при недоступности streaming

**Файл:** `internal/k8s/registry/watch/hybrid_manager.go`

```go
type HybridWatchManager struct {
    config        WatchConfig
    backend       BackendClient
    streamManager *StreamManager
    pollerManager *PollerManager
    
    mu            sync.RWMutex
    capabilities  BackendCapabilities
}

type BackendCapabilities struct {
    SupportsStreaming bool
    StreamingVersion  string
}

func (hwm *HybridWatchManager) Watch(resourceType string, options *metav1.ListOptions) (watch.Interface, error) {
    if hwm.config.ShouldUseStreaming() && hwm.capabilities.SupportsStreaming {
        // Попробовать streaming
        watcher, err := hwm.streamManager.Watch(resourceType, options)
        if err == nil {
            return watcher, nil
        }
        
        log.Printf("Streaming failed for %s, falling back to polling: %v", resourceType, err)
    }
    
    // Fallback на polling
    return hwm.pollerManager.Watch(resourceType, options)
}

func (hwm *HybridWatchManager) DetectCapabilities(ctx context.Context) error {
    // Попробовать создать stream для проверки поддержки
    testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    _, err := hwm.backend.WatchServices(testCtx, WatchOptions{ResourceVersion: "0"})
    if err != nil {
        if isUnimplementedError(err) {
            hwm.capabilities.SupportsStreaming = false
            log.Printf("Backend does not support streaming, using polling mode")
        } else {
            log.Printf("Failed to test streaming capabilities: %v", err)
        }
        return nil
    }
    
    hwm.capabilities.SupportsStreaming = true
    log.Printf("Backend supports streaming, enabling streaming mode")
    return nil
}
```

---

## Этап 5: Тестирование и Мониторинг

### 5.1 Unit тесты для Streaming
- [ ] **Задача:** Тестировать streaming компоненты
- [ ] **Зачем:** Гарантия корректности работы

**Файлы:**
- [ ] `internal/grpc/watch_handlers_test.go`
- [ ] `internal/k8s/registry/watch/stream_manager_test.go`
- [ ] `internal/infrastructure/event_store_test.go`

### 5.2 Integration тесты
- [ ] **Задача:** Тестировать end-to-end streaming
- [ ] **Зачем:** Проверка работы всего пайплайна

**Тесты:**
- [ ] Создание/изменение/удаление ресурса генерирует события
- [ ] События доходят до Kubernetes API клиентов
- [ ] Фильтрация по namespace и label selector работает
- [ ] Reconnect при обрыве соединения
- [ ] Fallback на polling при недоступности streaming

### 5.3 Performance тесты
- [ ] **Задача:** Сравнить производительность с polling
- [ ] **Зачем:** Подтвердить преимущества streaming

**Метрики:**
- [ ] Latency событий (polling vs streaming)
- [ ] CPU/Memory usage
- [ ] Network traffic
- [ ] Количество одновременных streams

### 5.4 Мониторинг
- [ ] **Задача:** Добавить метрики для streaming
- [ ] **Зачем:** Observability в production

**Метрики:**
- [ ] `watch_streams_active_total{resource_type}` - активные streams
- [ ] `watch_events_total{resource_type, event_type}` - количество событий
- [ ] `watch_stream_reconnects_total{resource_type}` - reconnects
- [ ] `watch_event_latency_seconds{resource_type}` - задержка событий

---

## Этап 6: Deployment и Rollout

### 6.1 Backend Deployment
- [ ] **Задача:** Обновить backend с поддержкой streaming
- [ ] **Зачем:** Источник streaming событий

**Последовательность:**
1. Deploy backend с новыми streaming методами
2. Verify streaming endpoints работают
3. Enable streaming в API Server конфигурации

### 6.2 API Server Deployment
- [ ] **Задача:** Обновить API Server с hybrid watch manager
- [ ] **Зачем:** Постепенный переход на streaming

**Конфигурация:**
```yaml
watch:
  mode: "auto"  # Автоматическое определение возможностей backend
  streaming_enabled: true
  polling_interval: "5s"
  stream_reconnect_delay: "1s"
```

### 6.3 Gradual Rollout
- [ ] **Задача:** Постепенное включение streaming
- [ ] **Зачем:** Минимизация рисков

**Этапы:**
1. **Week 1:** Deploy с `mode: "polling"` (без изменений)
2. **Week 2:** Switch на `mode: "auto"` (streaming с fallback)
3. **Week 3:** Monitor и optimize
4. **Week 4:** Switch на `mode: "streaming"` (pure streaming)

---

## Время выполнения

| Этап | Время (часы) | Сложность |
|------|-------------|-----------|
| 1: Protobuf API | 4-6 | Средняя |
| 2: Backend Implementation | 12-16 | Высокая |
| 3: API Integration | 8-12 | Высокая |
| 4: Migration & Compatibility | 6-8 | Средняя |
| 5: Testing & Monitoring | 8-12 | Высокая |
| 6: Deployment | 4-6 | Средняя |

**Общее время:** 42-60 часов (5-8 рабочих дней)

---

## Критерии готовности

✅ **Готово к production, когда:**
- [ ] Backend поддерживает все streaming методы
- [ ] Event Store работает корректно
- [ ] Hybrid Watch Manager с fallback на polling
- [ ] Latency событий < 100ms
- [ ] Reconnect при обрыве streams работает
- [ ] Performance тесты показывают улучшения
- [ ] Мониторинг и алерты настроены
- [ ] Gradual rollout план выполнен

## Ожидаемые улучшения

- **Latency:** с 2.5s (среднее) до < 100ms
- **Backend load:** снижение на 60-80% (нет постоянного polling)
- **Resource usage:** снижение CPU на 30-50%
- [ ] User experience: практически мгновенные обновления в `kubectl get -w` 