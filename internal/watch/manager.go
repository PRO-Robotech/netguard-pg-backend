package watch

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/watch/converters"
)

const (
	defaultCacheSize   = 10000
	defaultChannelName = "k8s_resource_changes"
)

// Manager orchestrates watch caches, converters, and the PostgreSQL notifier.
type Manager struct {
	notifier   *PGNotifier
	caches     map[string]*WatchCache
	converters map[string]converters.ResourceConverter
	cacheSize  int
}

func NewManager(
	ctx context.Context,
	db *sql.DB,
	facade *services.NetguardFacade,
) (*Manager, error) {
	return NewManagerWithConfig(ctx, db, facade, defaultCacheSize, defaultChannelName)
}

func NewManagerWithConfig(
	ctx context.Context,
	db *sql.DB,
	facade *services.NetguardFacade,
	cacheSize int,
	channel string,
) (*Manager, error) {
	if cacheSize <= 0 {
		cacheSize = defaultCacheSize
	}
	if channel == "" {
		channel = defaultChannelName
	}

	notifier, err := NewPGNotifier(ctx, db, channel)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		notifier:   notifier,
		caches:     make(map[string]*WatchCache),
		converters: make(map[string]converters.ResourceConverter),
		cacheSize:  cacheSize,
	}

	for _, conv := range converters.BuildAllConverters(facade) {
		resourceType := conv.ResourceType()
		cache := NewWatchCache(resourceType, cacheSize)
		manager.caches[resourceType] = cache
		manager.converters[resourceType] = conv
		manager.notifier.RegisterCache(cache, conv)
	}

	if err := manager.warmCaches(ctx); err != nil {
		return nil, err
	}

	return manager, nil
}

func (m *Manager) warmCaches(ctx context.Context) error {
	for resourceType, conv := range m.converters {
		cache := m.caches[resourceType]
		if cache == nil {
			continue
		}

		objects, err := conv.List(ctx)
		if err != nil {
			return fmt.Errorf("warm cache for %s: %w", resourceType, err)
		}
		for _, obj := range objects {
			rv, err := extractResourceVersion(obj)
			if err != nil {
				return fmt.Errorf("warm cache %s: %w", resourceType, err)
			}
			if err := cache.Add(obj, rv); err != nil {
				return fmt.Errorf("warm cache %s: %w", resourceType, err)
			}
		}
	}
	return nil
}

func (m *Manager) Cache(resourceType string) (*WatchCache, bool) {
	cache, ok := m.caches[resourceType]
	return cache, ok
}

func (m *Manager) NotifierStats() PGNotifierStats {
	return m.notifier.GetStats()
}

func (m *Manager) Stop() error {
	return m.notifier.Stop()
}

func extractResourceVersion(obj runtime.Object) (int64, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return 0, fmt.Errorf("resource does not expose ObjectMeta: %w", err)
	}
	rv := accessor.GetResourceVersion()
	if rv == "" {
		return 0, fmt.Errorf("object %s/%s missing resourceVersion", accessor.GetNamespace(), accessor.GetName())
	}
	parsed, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid resourceVersion %q: %w", rv, err)
	}
	return parsed, nil
}
