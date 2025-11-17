package watch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

// WatchCache хранит события изменений ресурсов для поддержки watch API
// Это аналог Watch Cache из Kubernetes, обеспечивающий:
// - Быстрый доступ к событиям по resourceVersion
// - Поддержку historical watches (watch from specific RV)
// - Пагинацию событий с limit/continue
// - Bookmark events для long-running watches
type WatchCache struct {
	mu sync.RWMutex

	// resourceType - тип ресурса (Host, Service, AddressGroup, etc)
	resourceType string

	// events - кольцевой буфер событий watch
	// Хранит последние N событий для поддержки historical watches
	events []CachedEvent

	// eventsByRV - индекс для быстрого поиска события по resourceVersion
	eventsByRV map[int64]int // resourceVersion -> index in events

	// currentObjects - текущее состояние всех объектов
	// key = namespace/name, value = объект с актуальным состоянием
	currentObjects map[string]*CachedObject

	// maxEvents - максимальное количество событий в кэше
	maxEvents int

	// nextEventID - следующий ID события (монотонно возрастающий)
	nextEventID int64

	// latestResourceVersion - последняя известная resourceVersion
	latestResourceVersion int64

	// bookmarkInterval - интервал отправки bookmark events
	bookmarkInterval time.Duration
}

// CachedEvent представляет одно событие изменения ресурса
type CachedEvent struct {
	// EventID - уникальный монотонно возрастающий ID события
	EventID int64

	// Type - тип события (Added, Modified, Deleted)
	Type watch.EventType

	// Object - объект ресурса в момент события
	Object runtime.Object

	// ResourceVersion - resourceVersion объекта на момент события
	ResourceVersion int64

	// Timestamp - время события
	Timestamp time.Time

	// IsBookmark - является ли это bookmark событием
	IsBookmark bool
}

// CachedObject представляет текущее состояние объекта в кэше
type CachedObject struct {
	// Object - текущий объект
	Object runtime.Object

	// ResourceVersion - текущая resourceVersion
	ResourceVersion int64

	// Key - ключ объекта (namespace/name)
	Key string

	// LastEventID - ID последнего события для этого объекта
	LastEventID int64
}

// NewWatchCache создает новый watch cache
func NewWatchCache(resourceType string, maxEvents int) *WatchCache {
	if maxEvents <= 0 {
		maxEvents = 10000 // default
	}

	return &WatchCache{
		resourceType:          resourceType,
		events:                make([]CachedEvent, 0, maxEvents),
		eventsByRV:            make(map[int64]int),
		currentObjects:        make(map[string]*CachedObject),
		maxEvents:             maxEvents,
		bookmarkInterval:      30 * time.Second,
		latestResourceVersion: 0,
	}
}

// Add добавляет событие в кэш
func (wc *WatchCache) Add(obj runtime.Object, resourceVersion int64) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	key, err := getObjectKey(obj)
	if err != nil {
		return fmt.Errorf("failed to get object key: %w", err)
	}

	eventType := watch.Added
	if _, exists := wc.currentObjects[key]; exists {
		eventType = watch.Modified
	}

	return wc.addEventLocked(eventType, obj, resourceVersion, key)
}

// Update обновляет объект в кэше
func (wc *WatchCache) Update(obj runtime.Object, resourceVersion int64) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	key, err := getObjectKey(obj)
	if err != nil {
		return fmt.Errorf("failed to get object key: %w", err)
	}

	return wc.addEventLocked(watch.Modified, obj, resourceVersion, key)
}

// Delete удаляет объект из кэша
func (wc *WatchCache) Delete(obj runtime.Object, resourceVersion int64) error {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	key, err := getObjectKey(obj)
	if err != nil {
		return fmt.Errorf("failed to get object key: %w", err)
	}

	err = wc.addEventLocked(watch.Deleted, obj, resourceVersion, key)
	if err != nil {
		return err
	}

	// Удаляем объект из currentObjects
	delete(wc.currentObjects, key)
	return nil
}

// addEventLocked добавляет событие в кэш (должен вызываться под lock)
func (wc *WatchCache) addEventLocked(eventType watch.EventType, obj runtime.Object, resourceVersion int64, key string) error {
	wc.nextEventID++

	event := CachedEvent{
		EventID:         wc.nextEventID,
		Type:            eventType,
		Object:          obj.DeepCopyObject(),
		ResourceVersion: resourceVersion,
		Timestamp:       time.Now(),
		IsBookmark:      false,
	}

	// Обновляем latestResourceVersion
	if resourceVersion > wc.latestResourceVersion {
		wc.latestResourceVersion = resourceVersion
	}

	// Добавляем событие в кольцевой буфер
	if len(wc.events) < wc.maxEvents {
		// Буфер еще не заполнен
		wc.events = append(wc.events, event)
		wc.eventsByRV[resourceVersion] = len(wc.events) - 1
	} else {
		// Буфер заполнен, используем кольцевую структуру
		oldestIdx := int(wc.nextEventID % int64(wc.maxEvents))

		// Удаляем старое событие из индекса
		if oldEvent := wc.events[oldestIdx]; oldEvent.EventID > 0 {
			delete(wc.eventsByRV, oldEvent.ResourceVersion)
		}

		// Добавляем новое событие
		wc.events[oldestIdx] = event
		wc.eventsByRV[resourceVersion] = oldestIdx
	}

	// Обновляем currentObjects (кроме Delete)
	if eventType != watch.Deleted {
		wc.currentObjects[key] = &CachedObject{
			Object:          obj.DeepCopyObject(),
			ResourceVersion: resourceVersion,
			Key:             key,
			LastEventID:     wc.nextEventID,
		}
	}

	recordCacheEvent(wc.resourceType, string(eventType))

	klog.V(6).InfoS("Added watch event to cache",
		"resourceType", wc.resourceType,
		"eventType", eventType,
		"eventID", event.EventID,
		"resourceVersion", resourceVersion,
		"key", key)

	return nil
}

// GetEventsSince возвращает события начиная с указанной resourceVersion
// Поддерживает пагинацию через limit и continueToken
func (wc *WatchCache) GetEventsSince(
	ctx context.Context,
	resourceVersion int64,
	listOpts *metainternalversion.ListOptions,
) ([]CachedEvent, string, error) {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	var result []CachedEvent
	var continueToken string

	// Если resourceVersion = 0, возвращаем все текущие объекты как Added события
	if resourceVersion == 0 {
		return wc.getCurrentObjectsAsEventsLocked(listOpts)
	}

	// Ищем события начиная с resourceVersion
	startIdx, found := wc.eventsByRV[resourceVersion]
	if !found {
		// resourceVersion слишком старая или не существует
		// Возвращаем все текущие объекты
		klog.V(4).InfoS("ResourceVersion not found in cache, returning current state",
			"resourceType", wc.resourceType,
			"requestedRV", resourceVersion,
			"latestRV", wc.latestResourceVersion)
		return wc.getCurrentObjectsAsEventsLocked(listOpts)
	}

	// Собираем события начиная с startIdx + 1
	limit := int64(0)
	if listOpts != nil && listOpts.Limit > 0 {
		limit = listOpts.Limit
	}

	// Определяем начальную позицию с учетом continueToken
	startEventID := wc.events[startIdx].EventID + 1
	if listOpts != nil && listOpts.Continue != "" {
		// Parse continueToken (формат: "eventID")
		var continueEventID int64
		if _, err := fmt.Sscanf(listOpts.Continue, "%d", &continueEventID); err == nil {
			startEventID = continueEventID
		}
	}

	// Собираем события
	for _, event := range wc.events {
		if event.EventID >= startEventID && event.ResourceVersion > resourceVersion {
			// Применяем фильтры
			if wc.matchesFilters(event.Object, listOpts) {
				result = append(result, event)

				// Проверяем limit
				if limit > 0 && int64(len(result)) >= limit {
					// Формируем continueToken
					if event.EventID < wc.nextEventID {
						continueToken = fmt.Sprintf("%d", event.EventID+1)
					}
					break
				}
			}
		}
	}

	return result, continueToken, nil
}

// getCurrentObjectsAsEventsLocked возвращает все текущие объекты как Added события
func (wc *WatchCache) getCurrentObjectsAsEventsLocked(listOpts *metainternalversion.ListOptions) ([]CachedEvent, string, error) {
	var result []CachedEvent
	var continueToken string

	limit := int64(0)
	if listOpts != nil && listOpts.Limit > 0 {
		limit = listOpts.Limit
	}

	// Конвертируем текущие объекты в Added события
	for _, cachedObj := range wc.currentObjects {
		if wc.matchesFilters(cachedObj.Object, listOpts) {
			result = append(result, CachedEvent{
				EventID:         cachedObj.LastEventID,
				Type:            watch.Added,
				Object:          cachedObj.Object.DeepCopyObject(),
				ResourceVersion: cachedObj.ResourceVersion,
				Timestamp:       time.Now(),
				IsBookmark:      false,
			})

			if limit > 0 && int64(len(result)) >= limit {
				// TODO: Implement proper pagination for current objects
				break
			}
		}
	}

	return result, continueToken, nil
}

// matchesFilters проверяет соответствие объекта фильтрам ListOptions
func (wc *WatchCache) matchesFilters(obj runtime.Object, listOpts *metainternalversion.ListOptions) bool {
	if listOpts == nil {
		return true
	}

	// Label selector
	if listOpts.LabelSelector != nil && !listOpts.LabelSelector.Empty() {
		objMeta, err := getObjectMeta(obj)
		if err != nil {
			return false
		}
		if !listOpts.LabelSelector.Matches(labels.Set(objMeta.Labels)) {
			return false
		}
	}

	// Field selector
	if listOpts.FieldSelector != nil && !listOpts.FieldSelector.Empty() {
		objMeta, err := getObjectMeta(obj)
		if err != nil {
			return false
		}

		// Поддерживаем базовые field selectors
		fieldsSet := fields.Set{
			"metadata.name":      objMeta.Name,
			"metadata.namespace": objMeta.Namespace,
		}

		if !listOpts.FieldSelector.Matches(fieldsSet) {
			return false
		}
	}

	return true
}

// GetLatestResourceVersion возвращает последнюю resourceVersion в кэше
func (wc *WatchCache) GetLatestResourceVersion() int64 {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	return wc.latestResourceVersion
}

// CreateBookmark создает bookmark событие с текущей resourceVersion
func (wc *WatchCache) CreateBookmark() (CachedEvent, error) {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	// Создаем пустой объект для bookmark
	// В K8s bookmark - это специальное событие без реального объекта
	return CachedEvent{
		EventID:         wc.nextEventID + 1,
		Type:            watch.Bookmark,
		Object:          nil,
		ResourceVersion: wc.latestResourceVersion,
		Timestamp:       time.Now(),
		IsBookmark:      true,
	}, nil
}

// GetCurrentObjects возвращает все текущие объекты в кэше
func (wc *WatchCache) GetCurrentObjects() []runtime.Object {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	result := make([]runtime.Object, 0, len(wc.currentObjects))
	for _, cachedObj := range wc.currentObjects {
		result = append(result, cachedObj.Object.DeepCopyObject())
	}
	return result
}

// GetStats возвращает статистику кэша
func (wc *WatchCache) GetStats() WatchCacheStats {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	return WatchCacheStats{
		ResourceType:          wc.resourceType,
		EventCount:            len(wc.events),
		CurrentObjectCount:    len(wc.currentObjects),
		LatestResourceVersion: wc.latestResourceVersion,
		MaxEvents:             wc.maxEvents,
	}
}

// WatchCacheStats содержит статистику watch cache
type WatchCacheStats struct {
	ResourceType          string
	EventCount            int
	CurrentObjectCount    int
	LatestResourceVersion int64
	MaxEvents             int
}

// Helper functions

// getObjectKey возвращает ключ объекта в формате "namespace/name"
func getObjectKey(obj runtime.Object) (string, error) {
	objMeta, err := getObjectMeta(obj)
	if err != nil {
		return "", err
	}

	if objMeta.Namespace != "" {
		return fmt.Sprintf("%s/%s", objMeta.Namespace, objMeta.Name), nil
	}
	return objMeta.Name, nil
}

// getObjectMeta извлекает метаданные из объекта
func getObjectMeta(obj runtime.Object) (*ObjectMeta, error) {
	// Используем meta.Accessor для извлечения метаданных
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to get accessor: %w", err)
	}

	return &ObjectMeta{
		Name:      accessor.GetName(),
		Namespace: accessor.GetNamespace(),
		Labels:    accessor.GetLabels(),
	}, nil
}

// ObjectMeta минимальная структура метаданных объекта
type ObjectMeta struct {
	Name      string
	Namespace string
	Labels    map[string]string
}

// GetCachedObject возвращает объект из текущего состояния кэша.
func (wc *WatchCache) GetCachedObject(namespace, name string) runtime.Object {
	key := name
	if namespace != "" {
		key = fmt.Sprintf("%s/%s", namespace, name)
	}

	wc.mu.RLock()
	defer wc.mu.RUnlock()

	if cached, ok := wc.currentObjects[key]; ok && cached != nil && cached.Object != nil {
		return cached.Object.DeepCopyObject()
	}
	return nil
}
