package watch

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

// PollerWatchInterface реализует watch.Interface для shared poller
type PollerWatchInterface struct {
	client *WatchClient
	poller *SharedPoller
}

// ResultChan возвращает канал с событиями
func (w *PollerWatchInterface) ResultChan() <-chan watch.Event {
	// Создаем новый канал, который будет возвращать Unstructured
	unstructuredChan := make(chan watch.Event)

	go func() {
		defer close(unstructuredChan)
		for event := range w.client.eventChan {
			// Конвертируем в Unstructured
			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(event.Object)
			if err != nil {
				klog.Errorf("failed to convert object to unstructured: %v", err)
				continue
			}

			// Создаем новое событие с Unstructured объектом
			unstructuredEvent := watch.Event{
				Type:   event.Type,
				Object: &unstructured.Unstructured{Object: unstructuredObj},
			}
			unstructuredChan <- unstructuredEvent
		}
	}()

	return unstructuredChan
}

// Stop останавливает watch
func (w *PollerWatchInterface) Stop() {
	close(w.client.done)
	w.poller.RemoveClient(w.client.id)
}
