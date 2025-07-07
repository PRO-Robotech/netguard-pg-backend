package watch

import (
	"k8s.io/apimachinery/pkg/watch"
)

// PollerWatchInterface реализует watch.Interface для shared poller
type PollerWatchInterface struct {
	client *WatchClient
	poller *SharedPoller
}

// NewPollerWatchInterface создает новый PollerWatchInterface
func NewPollerWatchInterface(client *WatchClient, poller *SharedPoller) *PollerWatchInterface {
	return &PollerWatchInterface{
		client: client,
		poller: poller,
	}
}

// ResultChan возвращает канал с событиями БЕЗ конвертации в Unstructured
func (w *PollerWatchInterface) ResultChan() <-chan watch.Event {
	// ИСПРАВЛЕНИЕ: возвращаем прямо канал с типизированными объектами
	// НЕ конвертируем в Unstructured - это нарушает декодирование List типов!
	return w.client.eventChan
}

// Stop останавливает watch
func (w *PollerWatchInterface) Stop() {
	close(w.client.done)
	w.poller.RemoveClient(w.client.id)
}
