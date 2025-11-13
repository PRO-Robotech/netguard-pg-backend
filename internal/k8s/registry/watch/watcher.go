package watch

import (
	"context"
	"fmt"
	"time"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

// CacheWatcher реализует watch.Interface используя WatchCache
// Это K8s-совместимая реализация watch с поддержкой:
// - resourceVersion-based watching
// - limit и continue для пагинации
// - sendInitialEvents
// - bookmark events
type CacheWatcher struct {
	// cache - watch cache для получения событий
	cache *WatchCache

	// resultChan - канал для отправки событий клиенту
	resultChan chan watch.Event

	// ctx - контекст для отмены watch
	ctx    context.Context
	cancel context.CancelFunc

	// options - параметры watch
	options *WatchOptions

	// done - канал для уведомления о завершении
	done chan struct{}
}

// WatchOptions содержит параметры для watch
type WatchOptions struct {
	// ResourceVersion - начальная resourceVersion для watch
	// "0" или "" - начать с текущего состояния
	// "N" - начать с событий после resourceVersion N
	ResourceVersion int64

	// ResourceVersionMatch определяет семантику resourceVersion
	// "" (empty) - legacy behavior (зависит от RV)
	// "NotOlderThan" - начать с RV >= указанной
	// "Exact" - начать с точно указанной RV (fail если слишком старая)
	ResourceVersionMatch string

	// ListOptions - параметры фильтрации и пагинации
	ListOptions *metainternalversion.ListOptions

	// SendInitialEvents - если true, сначала отправить текущее состояние
	SendInitialEvents bool

	// AllowWatchBookmarks - если true, отправлять bookmark события
	AllowWatchBookmarks bool

	// TimeoutSeconds - максимальное время жизни watch (0 = без таймаута)
	TimeoutSeconds int64
}

// NewCacheWatcher создает новый watcher
func NewCacheWatcher(
	ctx context.Context,
	cache *WatchCache,
	options *WatchOptions,
) (*CacheWatcher, error) {
	if options == nil {
		options = &WatchOptions{
			ResourceVersion:     0,
			SendInitialEvents:   true,
			AllowWatchBookmarks: false,
			TimeoutSeconds:      0,
		}
	}

	// Apply timeout if specified
	watchCtx := ctx
	var cancel context.CancelFunc

	if options.TimeoutSeconds > 0 {
		watchCtx, cancel = context.WithTimeout(ctx, time.Duration(options.TimeoutSeconds)*time.Second)
		klog.V(5).InfoS("Watch created with timeout",
			"resourceType", cache.resourceType,
			"timeoutSeconds", options.TimeoutSeconds)
	} else {
		watchCtx, cancel = context.WithCancel(ctx)
	}

	// Validate resourceVersionMatch
	if err := validateResourceVersionMatch(options); err != nil {
		cancel()
		return nil, err
	}

	watcher := &CacheWatcher{
		cache:      cache,
		resultChan: make(chan watch.Event, 100),
		ctx:        watchCtx,
		cancel:     cancel,
		options:    options,
		done:       make(chan struct{}),
	}

	// Запускаем горутину для обработки watch
	go watcher.run()

	return watcher, nil
}

// validateResourceVersionMatch проверяет корректность resourceVersionMatch
func validateResourceVersionMatch(options *WatchOptions) error {
	if options.ResourceVersionMatch == "" {
		return nil // legacy mode
	}

	// Допустимые значения: NotOlderThan, Exact
	switch options.ResourceVersionMatch {
	case "NotOlderThan", "Exact":
		return nil
	default:
		return fmt.Errorf("invalid resourceVersionMatch: %s (must be NotOlderThan or Exact)",
			options.ResourceVersionMatch)
	}
}

// ResultChan возвращает канал для получения событий
func (cw *CacheWatcher) ResultChan() <-chan watch.Event {
	return cw.resultChan
}

// Stop останавливает watcher
func (cw *CacheWatcher) Stop() {
	cw.cancel()
}

// run - основная горутина обработки watch
func (cw *CacheWatcher) run() {
	defer close(cw.done)
	defer close(cw.resultChan)

	klog.V(4).InfoS("Starting cache watcher",
		"resourceType", cw.cache.resourceType,
		"resourceVersion", cw.options.ResourceVersion,
		"sendInitialEvents", cw.options.SendInitialEvents)

	// Шаг 1: Отправить initial events если требуется
	if cw.options.SendInitialEvents || cw.options.ResourceVersion == 0 {
		if err := cw.sendInitialEvents(); err != nil {
			klog.ErrorS(err, "Failed to send initial events")
			cw.sendErrorEvent(err)
			return
		}
	}

	// Шаг 2: Начать streaming новых событий
	if err := cw.streamEvents(); err != nil {
		klog.ErrorS(err, "Failed to stream events")
		cw.sendErrorEvent(err)
		return
	}
}

// sendInitialEvents отправляет текущее состояние как начальные события
func (cw *CacheWatcher) sendInitialEvents() error {
	// Определяем startRV на основе resourceVersionMatch
	startRV := int64(0)

	switch cw.options.ResourceVersionMatch {
	case "NotOlderThan":
		// Начинаем с указанной RV или позже
		startRV = cw.options.ResourceVersion

	case "Exact":
		// Начинаем с точно указанной RV
		startRV = cw.options.ResourceVersion

		// Проверяем что RV доступна в cache
		latestRV := cw.cache.GetLatestResourceVersion()
		if cw.options.ResourceVersion > 0 && cw.options.ResourceVersion < latestRV-int64(cw.cache.maxEvents) {
			return fmt.Errorf("resourceVersion %d is too old (cache only has events from ~%d)",
				cw.options.ResourceVersion, latestRV-int64(cw.cache.maxEvents))
		}

	default:
		// Legacy mode: RV=0 означает current state
		startRV = 0
	}

	events, continueToken, err := cw.cache.GetEventsSince(
		cw.ctx,
		startRV,
		cw.options.ListOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to get initial events: %w", err)
	}

	// Отправляем события
	for _, event := range events {
		select {
		case cw.resultChan <- watch.Event{
			Type:   event.Type,
			Object: event.Object,
		}:
		case <-cw.ctx.Done():
			return cw.ctx.Err()
		}
	}

	klog.V(5).InfoS("Sent initial events",
		"resourceType", cw.cache.resourceType,
		"eventCount", len(events),
		"continueToken", continueToken,
		"startRV", startRV,
		"resourceVersionMatch", cw.options.ResourceVersionMatch)

	return nil
}

// streamEvents стримит новые события по мере их появления
func (cw *CacheWatcher) streamEvents() error {
	// Получаем текущую resourceVersion как точку отсчета
	currentRV := cw.cache.GetLatestResourceVersion()

	// Тикер для периодической проверки новых событий
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Тикер для bookmark events
	var bookmarkTicker *time.Ticker
	if cw.options.AllowWatchBookmarks {
		bookmarkTicker = time.NewTicker(30 * time.Second)
		defer bookmarkTicker.Stop()
	}

	for {
		select {
		case <-cw.ctx.Done():
			return cw.ctx.Err()

		case <-ticker.C:
			// Проверяем новые события
			events, _, err := cw.cache.GetEventsSince(
				cw.ctx,
				currentRV,
				cw.options.ListOptions,
			)
			if err != nil {
				klog.ErrorS(err, "Failed to get events since",
					"currentRV", currentRV)
				continue
			}

			// Отправляем новые события
			for _, event := range events {
				select {
				case cw.resultChan <- watch.Event{
					Type:   event.Type,
					Object: event.Object,
				}:
					// Обновляем currentRV
					if event.ResourceVersion > currentRV {
						currentRV = event.ResourceVersion
					}
				case <-cw.ctx.Done():
					return cw.ctx.Err()
				}
			}

		case <-func() <-chan time.Time {
			if bookmarkTicker != nil {
				return bookmarkTicker.C
			}
			return make(chan time.Time) // never fires
		}():
			// Отправляем bookmark event
			if err := cw.sendBookmark(); err != nil {
				klog.ErrorS(err, "Failed to send bookmark")
			}
		}
	}
}

// sendBookmark отправляет bookmark событие
func (cw *CacheWatcher) sendBookmark() error {
	bookmark, err := cw.cache.CreateBookmark()
	if err != nil {
		return fmt.Errorf("failed to create bookmark: %w", err)
	}

	// Создаем bookmark object
	// В K8s bookmark - это специальный тип события
	bookmarkObj := &BookmarkObject{
		ResourceVersion: bookmark.ResourceVersion,
	}

	select {
	case cw.resultChan <- watch.Event{
		Type:   watch.Bookmark,
		Object: bookmarkObj,
	}:
		klog.V(6).InfoS("Sent bookmark event",
			"resourceType", cw.cache.resourceType,
			"resourceVersion", bookmark.ResourceVersion)
		return nil
	case <-cw.ctx.Done():
		return cw.ctx.Err()
	}
}

// sendErrorEvent отправляет событие ошибки
func (cw *CacheWatcher) sendErrorEvent(err error) {
	select {
	case cw.resultChan <- watch.Event{
		Type:   watch.Error,
		Object: nil, // TODO: создать proper error object
	}:
	case <-cw.ctx.Done():
	}
}

// BookmarkObject представляет bookmark событие
type BookmarkObject struct {
	ResourceVersion int64
}

// DeepCopyObject реализует runtime.Object
func (b *BookmarkObject) DeepCopyObject() runtime.Object {
	return &BookmarkObject{
		ResourceVersion: b.ResourceVersion,
	}
}

// GetObjectKind реализует runtime.Object
func (b *BookmarkObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}
