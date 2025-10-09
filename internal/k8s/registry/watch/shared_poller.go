package watch

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"netguard-pg-backend/internal/k8s/client"
)

// SharedPoller - один поллер для всех Watch клиентов одного типа ресурса
type SharedPoller struct {
	backend      client.BackendClient
	resourceType string
	pollInterval time.Duration

	mu           sync.RWMutex
	clients      map[string]*WatchClient   // clientID -> WatchClient
	lastSnapshot map[string]runtime.Object // resourceKey -> resource

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// WatchClient представляет одного клиента Watch
type WatchClient struct {
	id        string
	eventChan chan watch.Event
	filter    *metainternalversion.ListOptions // Фильтр для этого клиента
	done      chan struct{}
}

// Converter интерфейс для конвертации ресурсов
type Converter interface {
	ConvertToK8s(resource interface{}) runtime.Object
	ListResources(ctx context.Context, backend client.BackendClient) ([]interface{}, error)
	GetResourceKey(resource interface{}) string
}

// NewSharedPoller создает новый SharedPoller
func NewSharedPoller(backend client.BackendClient, resourceType string, converter Converter) *SharedPoller {
	ctx, cancel := context.WithCancel(context.Background())

	poller := &SharedPoller{
		backend:      backend,
		resourceType: resourceType,
		pollInterval: 5 * time.Second,
		clients:      make(map[string]*WatchClient),
		lastSnapshot: make(map[string]runtime.Object),
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}

	go poller.pollLoop(converter)
	return poller
}

// AddClient добавляет нового клиента
func (p *SharedPoller) AddClient(options *metainternalversion.ListOptions) (*WatchClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clientID := uuid.New().String()
	client := &WatchClient{
		id:        clientID,
		eventChan: make(chan watch.Event, 100),
		filter:    options,
		done:      make(chan struct{}),
	}

	p.clients[clientID] = client

	// Отправить текущий snapshot новому клиенту
	go p.sendInitialSnapshot(client)

	return client, nil
}

// RemoveClient удаляет клиента
func (p *SharedPoller) RemoveClient(clientID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[clientID]; exists {
		close(client.eventChan)
		delete(p.clients, clientID)
	}

	// Остановить поллер если нет клиентов
	if len(p.clients) == 0 {
		p.cancel()
	}
}

// pollLoop основной цикл поллинга
func (p *SharedPoller) pollLoop(converter Converter) {
	defer close(p.done)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	backoffConfig := backoff.NewExponentialBackOff()
	backoffConfig.MaxElapsedTime = 0 // Retry indefinitely

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			err := p.checkForChanges(converter)
			if err != nil {
				// Exponential backoff при ошибках
				delay := backoffConfig.NextBackOff()
				time.Sleep(delay)
			} else {
				backoffConfig.Reset()
			}
		}
	}
}

// checkForChanges проверяет изменения в backend
func (p *SharedPoller) checkForChanges(converter Converter) error {
	// Получить все ресурсы из backend
	resources, err := converter.ListResources(p.ctx, p.backend)
	if err != nil {
		return fmt.Errorf("failed to list %s: %w", p.resourceType, err)
	}

	newSnapshot := make(map[string]runtime.Object)
	for _, resource := range resources {
		key := converter.GetResourceKey(resource)
		k8sResource := converter.ConvertToK8s(resource)
		newSnapshot[key] = k8sResource
	}

	// Сравнить с предыдущим snapshot и генерировать события
	p.generateEvents(newSnapshot)

	p.mu.Lock()
	p.lastSnapshot = newSnapshot
	p.mu.Unlock()

	return nil
}

// generateEvents генерирует события на основе изменений
func (p *SharedPoller) generateEvents(newSnapshot map[string]runtime.Object) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// ADDED и MODIFIED события
	for key, newRes := range newSnapshot {
		if oldRes, exists := p.lastSnapshot[key]; exists {
			// 🔒 CRITICAL FIX: Skip MODIFIED events for objects being deleted
			accessor, err := meta.Accessor(newRes)
			if err == nil {
				deletionTimestamp := accessor.GetDeletionTimestamp()
				if deletionTimestamp != nil && !deletionTimestamp.IsZero() {
					// Object is being deleted - don't send MODIFIED events
					// This prevents triggering sync loops during deletion
					continue
				}
			}

			// Ресурс существовал - проверить изменения
			if !reflect.DeepEqual(oldRes, newRes) {
				event := watch.Event{
					Type:   watch.Modified,
					Object: newRes,
				}
				p.broadcastEvent(event)
			}
		} else {
			// Новый ресурс
			event := watch.Event{
				Type:   watch.Added,
				Object: newRes,
			}
			p.broadcastEvent(event)
		}
	}

	// DELETED события
	for key, oldRes := range p.lastSnapshot {
		if _, exists := newSnapshot[key]; !exists {
			event := watch.Event{
				Type:   watch.Deleted,
				Object: oldRes,
			}
			p.broadcastEvent(event)
		}
	}
}

// broadcastEvent отправляет событие всем подходящим клиентам
func (p *SharedPoller) broadcastEvent(event watch.Event) {
	for _, client := range p.clients {
		// Применить фильтр клиента (namespace, label selector)
		if p.matchesFilter(event.Object, client.filter) {
			select {
			case client.eventChan <- event:
			case <-client.done:
				// Клиент закрыт, пропустить
			default:
				// Канал клиента переполнен, пропустить
			}
		}
	}
}

// matchesFilter проверяет соответствие объекта фильтру клиента
func (p *SharedPoller) matchesFilter(obj runtime.Object, filter *metainternalversion.ListOptions) bool {
	if filter == nil {
		return true
	}

	// TODO: Реализовать фильтрацию по namespace и label selector
	// Пока что возвращаем true для всех объектов
	return true
}

// sendInitialSnapshot отправляет текущий snapshot новому клиенту
func (p *SharedPoller) sendInitialSnapshot(client *WatchClient) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, resource := range p.lastSnapshot {
		if p.matchesFilter(resource, client.filter) {
			event := watch.Event{
				Type:   watch.Added,
				Object: resource,
			}

			select {
			case client.eventChan <- event:
			case <-client.done:
				return
			default:
				// Канал переполнен, пропустить
				return
			}
		}
	}
}

// Shutdown останавливает поллер
func (p *SharedPoller) Shutdown() {
	p.cancel()
	<-p.done
}
