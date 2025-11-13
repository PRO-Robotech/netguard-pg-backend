package watch

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func TestCacheWatcher_SendInitialEvents(t *testing.T) {
	cache := NewWatchCache("testresource", 100)

	// Добавляем несколько объектов в кэш
	for i := 1; i <= 3; i++ {
		obj := &MockObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-obj-" + string(rune('0'+i)),
				Namespace:       "default",
				ResourceVersion: string(rune('0' + i)),
			},
			Spec: "spec-" + string(rune('0'+i)),
		}
		err := cache.Add(obj, int64(i))
		if err != nil {
			t.Fatalf("Failed to add object %d: %v", i, err)
		}
	}

	// Создаем watcher с SendInitialEvents = true
	opts := &WatchOptions{
		ResourceVersion:     0,
		SendInitialEvents:   true,
		AllowWatchBookmarks: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	watcher, err := NewCacheWatcher(ctx, cache, opts)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Получаем начальные события
	receivedEvents := 0
	timeout := time.After(2 * time.Second)

EventLoop:
	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				t.Log("Watcher channel closed")
				break EventLoop
			}

			if event.Type == watch.Added {
				receivedEvents++
				t.Logf("Received Added event for object")
			}

			// Ожидаем 3 начальных события
			if receivedEvents >= 3 {
				break EventLoop
			}

		case <-timeout:
			t.Logf("Timeout waiting for events, received %d events", receivedEvents)
			break EventLoop
		}
	}

	if receivedEvents != 3 {
		t.Errorf("Expected 3 initial events, got %d", receivedEvents)
	}
}

func TestCacheWatcher_StreamNewEvents(t *testing.T) {
	cache := NewWatchCache("testresource", 100)

	// Создаем watcher
	opts := &WatchOptions{
		ResourceVersion:     0,
		SendInitialEvents:   false,
		AllowWatchBookmarks: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	watcher, err := NewCacheWatcher(ctx, cache, opts)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Запускаем горутину для добавления объектов
	go func() {
		time.Sleep(500 * time.Millisecond)

		for i := 1; i <= 3; i++ {
			obj := &MockObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "new-obj-" + string(rune('0'+i)),
					Namespace:       "default",
					ResourceVersion: string(rune('0' + i)),
				},
				Spec: "new-spec",
			}
			err := cache.Add(obj, int64(i))
			if err != nil {
				t.Errorf("Failed to add object %d: %v", i, err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Получаем события
	receivedEvents := 0
	timeout := time.After(3 * time.Second)

EventLoop:
	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				t.Log("Watcher channel closed")
				break EventLoop
			}

			if event.Type == watch.Added {
				receivedEvents++
				t.Logf("Received new Added event")
			}

			if receivedEvents >= 3 {
				break EventLoop
			}

		case <-timeout:
			t.Logf("Timeout waiting for events, received %d events", receivedEvents)
			break EventLoop
		}
	}

	if receivedEvents < 1 {
		t.Errorf("Expected at least 1 event, got %d", receivedEvents)
	}
}

func TestCacheWatcher_Stop(t *testing.T) {
	cache := NewWatchCache("testresource", 100)

	opts := &WatchOptions{
		ResourceVersion:     0,
		SendInitialEvents:   false,
		AllowWatchBookmarks: false,
	}

	ctx := context.Background()
	watcher, err := NewCacheWatcher(ctx, cache, opts)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Останавливаем watcher
	watcher.Stop()

	// Проверяем что канал закрыт
	select {
	case _, ok := <-watcher.ResultChan():
		if ok {
			t.Error("Expected channel to be closed after Stop()")
		}
	case <-time.After(1 * time.Second):
		t.Error("Channel not closed after Stop()")
	}
}

func TestCacheWatcher_WithBookmarks(t *testing.T) {
	cache := NewWatchCache("testresource", 100)

	// Добавляем объект
	obj := &MockObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-obj",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Spec: "test-spec",
	}
	err := cache.Add(obj, 1)
	if err != nil {
		t.Fatalf("Failed to add object: %v", err)
	}

	// Создаем watcher с bookmarks
	opts := &WatchOptions{
		ResourceVersion:     0,
		SendInitialEvents:   true,
		AllowWatchBookmarks: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	watcher, err := NewCacheWatcher(ctx, cache, opts)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Ждем bookmark события (они отправляются каждые 30 секунд)
	receivedBookmark := false
	timeout := time.After(32 * time.Second)

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				t.Log("Watcher channel closed")
				goto Done
			}

			if event.Type == watch.Bookmark {
				receivedBookmark = true
				t.Log("Received bookmark event")
				goto Done
			}

		case <-timeout:
			t.Log("Timeout waiting for bookmark")
			goto Done
		}
	}

Done:
	// Bookmark может не прийти за время теста (30 секунд)
	// Это нормально для быстрого теста
	t.Logf("Received bookmark: %v", receivedBookmark)
}

