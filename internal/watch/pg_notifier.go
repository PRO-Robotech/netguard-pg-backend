package watch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/watch/converters"
)

// PGNotifier слушает PostgreSQL NOTIFY события и обновляет watch cache
// Использует PostgreSQL LISTEN/NOTIFY для real-time уведомлений об изменениях
type PGNotifier struct {
	mu sync.RWMutex

	// db - PostgreSQL соединение для LISTEN
	db *sql.DB

	// listener - PostgreSQL listener
	listener *pq.Listener

	// caches - мапа watch кэшей по типу ресурса
	caches map[string]*WatchCache

	// converters - мапа конвертеров для каждого типа ресурса
	converters map[string]converters.ResourceConverter

	// ctx - контекст для остановки
	ctx    context.Context
	cancel context.CancelFunc

	// done - канал для уведомления о завершении
	done chan struct{}

	// channel - имя PostgreSQL канала для NOTIFY
	channel string
}

// PGNotification представляет NOTIFY сообщение из PostgreSQL
type PGNotification struct {
	// Operation - тип операции (INSERT, UPDATE, DELETE)
	Operation string `json:"operation"`

	// ResourceType - тип ресурса (hosts, services, address_groups, etc)
	ResourceType string `json:"resource_type"`

	// Namespace - namespace ресурса
	Namespace string `json:"namespace"`

	// Name - имя ресурса
	Name string `json:"name"`

	// ResourceVersion - resourceVersion после операции
	ResourceVersion int64 `json:"resource_version"`

	// Data - полные данные объекта (для INSERT/UPDATE)
	Data map[string]interface{} `json:"data,omitempty"`
}

// NewPGNotifier создает новый PostgreSQL notifier
func NewPGNotifier(
	ctx context.Context,
	db *sql.DB,
	connString string,
	channel string,
) (*PGNotifier, error) {
	notifierCtx, cancel := context.WithCancel(ctx)

	// Создаем PostgreSQL listener
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			klog.ErrorS(err, "PostgreSQL listener problem",
				"event", ev)
		}
	}

	minReconnectInterval := 10 * time.Second
	maxReconnectInterval := 1 * time.Minute

	if connString == "" {
		return nil, fmt.Errorf("connection string is required for PG notifier")
	}

	listener := pq.NewListener(
		connString,
		minReconnectInterval,
		maxReconnectInterval,
		reportProblem,
	)

	notifier := &PGNotifier{
		db:         db,
		listener:   listener,
		caches:     make(map[string]*WatchCache),
		converters: make(map[string]converters.ResourceConverter),
		ctx:        notifierCtx,
		cancel:     cancel,
		done:       make(chan struct{}),
		channel:    channel,
	}

	// Начинаем слушать
	if err := listener.Listen(channel); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start listening on channel %s: %w", channel, err)
	}

	klog.InfoS("PostgreSQL notifier started",
		"channel", channel)

	// Запускаем горутину для обработки уведомлений
	go notifier.run()

	return notifier, nil
}

// RegisterCache регистрирует watch cache для типа ресурса
func (pgn *PGNotifier) RegisterCache(cache *WatchCache, converter converters.ResourceConverter) {
	pgn.mu.Lock()
	defer pgn.mu.Unlock()

	resourceType := converter.ResourceType()
	pgn.caches[resourceType] = cache
	pgn.converters[resourceType] = converter

	klog.V(4).InfoS("Registered watch cache",
		"resourceType", resourceType)
}

// UnregisterCache отменяет регистрацию watch cache
func (pgn *PGNotifier) UnregisterCache(resourceType string) {
	pgn.mu.Lock()
	defer pgn.mu.Unlock()

	delete(pgn.caches, resourceType)
	delete(pgn.converters, resourceType)

	klog.V(4).InfoS("Unregistered watch cache",
		"resourceType", resourceType)
}

// run - основная горутина для обработки NOTIFY событий
func (pgn *PGNotifier) run() {
	defer close(pgn.done)

	klog.InfoS("PostgreSQL notifier started processing",
		"channel", pgn.channel)

	for {
		select {
		case <-pgn.ctx.Done():
			klog.InfoS("PostgreSQL notifier stopped")
			return

		case notification := <-pgn.listener.Notify:
			if notification == nil {
				// Channel closed, reconnect will be handled by listener
				continue
			}

			if err := pgn.handleNotification(notification); err != nil {
				klog.ErrorS(err, "Failed to handle notification",
					"payload", notification.Extra)
			}

		case <-time.After(90 * time.Second):
			// Ping для поддержания соединения
			if err := pgn.listener.Ping(); err != nil {
				klog.ErrorS(err, "Failed to ping PostgreSQL listener")
			}
		}
	}
}

// handleNotification обрабатывает одно NOTIFY событие
func (pgn *PGNotifier) handleNotification(notification *pq.Notification) error {
	// Парсим JSON payload
	var pgNotif PGNotification
	if err := json.Unmarshal([]byte(notification.Extra), &pgNotif); err != nil {
		return fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	log.Printf("[WatchNotifier] payload op=%s type=%s ns=%s name=%s rv=%d",
		pgNotif.Operation, pgNotif.ResourceType, pgNotif.Namespace, pgNotif.Name, pgNotif.ResourceVersion)

	// Получаем соответствующий cache и converter
	pgn.mu.RLock()
	cache, cacheExists := pgn.caches[pgNotif.ResourceType]
	converter, converterExists := pgn.converters[pgNotif.ResourceType]
	pgn.mu.RUnlock()

	if !cacheExists || !converterExists {
		// Нет зарегистрированного cache для этого типа ресурса
		klog.V(6).InfoS("No cache registered for resource type",
			"resourceType", pgNotif.ResourceType)
		return nil
	}

	// Обрабатываем событие в зависимости от операции
	recordPGNotifyEvent(pgNotif.ResourceType, pgNotif.Operation)

	switch pgNotif.Operation {
	case "INSERT", "UPDATE":
		return pgn.handleInsertOrUpdate(cache, converter, &pgNotif)
	case "DELETE":
		return pgn.handleDelete(cache, converter, &pgNotif)
	default:
		return fmt.Errorf("unknown operation: %s", pgNotif.Operation)
	}
}

// handleInsertOrUpdate обрабатывает INSERT/UPDATE событие
func (pgn *PGNotifier) handleInsertOrUpdate(
	cache *WatchCache,
	converter converters.ResourceConverter,
	notification *PGNotification,
) error {
	id := models.NewResourceIdentifier(notification.Name, models.WithNamespace(notification.Namespace))

	obj, err := converter.Convert(pgn.ctx, id)
	if err != nil {
		recordPGNotifyError()
		return fmt.Errorf("failed to convert resource %s/%s: %w", notification.Namespace, notification.Name, err)
	}

	log.Printf("[WatchNotifier] converted %s/%s rv=%d type=%s",
		notification.Namespace, notification.Name, notification.ResourceVersion, notification.ResourceType)

	setObjectResourceVersion(obj, notification.ResourceVersion)

	if notification.Operation == "INSERT" {
		return cache.Add(obj, notification.ResourceVersion)
	}
	log.Printf("[WatchNotifier] forwarding UPDATE to cache %s/%s rv=%d", notification.Namespace, notification.Name, notification.ResourceVersion)
	return cache.Update(obj, notification.ResourceVersion)
}

// handleDelete обрабатывает DELETE событие
func (pgn *PGNotifier) handleDelete(
	cache *WatchCache,
	converter converters.ResourceConverter,
	notification *PGNotification,
) error {
	id := models.NewResourceIdentifier(notification.Name, models.WithNamespace(notification.Namespace))

	obj, err := converter.Convert(pgn.ctx, id)
	if err != nil {
		klog.V(4).InfoS("Failed to fetch deleted object, attempting cache lookup",
			"resourceType", notification.ResourceType,
			"namespace", notification.Namespace,
			"name", notification.Name,
			"error", err.Error())
		recordPGNotifyError()
		obj = cache.GetCachedObject(id.Namespace, id.Name)
		if obj == nil {
			return fmt.Errorf("cached object not found for deleted resource %s/%s", id.Namespace, id.Name)
		}
	}

	setObjectResourceVersion(obj, notification.ResourceVersion)

	return cache.Delete(obj, notification.ResourceVersion)
}

func setObjectResourceVersion(obj runtime.Object, rv int64) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	accessor.SetResourceVersion(fmt.Sprintf("%d", rv))
}

// Stop останавливает notifier
func (pgn *PGNotifier) Stop() error {
	pgn.cancel()

	// Останавливаем listener
	if err := pgn.listener.Close(); err != nil {
		klog.ErrorS(err, "Failed to close PostgreSQL listener")
	}

	// Ждем завершения
	<-pgn.done

	klog.InfoS("PostgreSQL notifier stopped")
	return nil
}

// IsHealthy проверяет здоровье notifier
func (pgn *PGNotifier) IsHealthy() bool {
	// Проверяем соединение с listener
	if err := pgn.listener.Ping(); err != nil {
		klog.ErrorS(err, "PostgreSQL listener ping failed")
		return false
	}
	return true
}

// GetStats возвращает статистику notifier
func (pgn *PGNotifier) GetStats() PGNotifierStats {
	pgn.mu.RLock()
	defer pgn.mu.RUnlock()

	stats := PGNotifierStats{
		Channel:          pgn.channel,
		RegisteredCaches: len(pgn.caches),
		CacheStats:       make(map[string]WatchCacheStats),
	}

	for resourceType, cache := range pgn.caches {
		stats.CacheStats[resourceType] = cache.GetStats()
	}

	return stats
}

// PGNotifierStats содержит статистику notifier
type PGNotifierStats struct {
	Channel          string
	RegisteredCaches int
	CacheStats       map[string]WatchCacheStats
}
