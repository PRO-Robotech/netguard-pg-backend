package worker

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/monitor"
)

// TestOutboxWorker_ImplementsConnectionListener проверяет что OutboxWorker реализует интерфейс
func TestOutboxWorker_ImplementsConnectionListener(t *testing.T) {
	var _ monitor.ConnectionListener = (*OutboxWorker)(nil)
}

// TestOutboxWorker_OnConnectionEvent_Connected проверяет обработку события подключения
func TestOutboxWorker_OnConnectionEvent_Connected(t *testing.T) {
	logger := zap.NewNop()
	worker := &OutboxWorker{
		logger:   logger,
		isPaused: true, // initially paused
	}

	event := monitor.ConnectionEvent{
		Type:       monitor.EventConnected,
		Timestamp:  timestamppb.Now(),
		OccurredAt: time.Now(),
	}

	worker.OnConnectionEvent(event)

	// Check that worker resumed
	worker.pausedMu.RLock()
	paused := worker.isPaused
	worker.pausedMu.RUnlock()

	if paused {
		t.Error("expected worker to resume after EventConnected, but still paused")
	}
}

// TestOutboxWorker_OnConnectionEvent_Disconnected проверяет обработку события отключения
func TestOutboxWorker_OnConnectionEvent_Disconnected(t *testing.T) {
	logger := zap.NewNop()
	worker := &OutboxWorker{
		logger:   logger,
		isPaused: false, // initially running
	}

	event := monitor.ConnectionEvent{
		Type:       monitor.EventDisconnected,
		Error:      nil,
		OccurredAt: time.Now(),
	}

	worker.OnConnectionEvent(event)

	// Check that worker paused
	worker.pausedMu.RLock()
	paused := worker.isPaused
	worker.pausedMu.RUnlock()

	if !paused {
		t.Error("expected worker to pause after EventDisconnected, but still running")
	}
}

// TestOutboxWorker_OnConnectionEvent_Timestamp проверяет что timestamp события игнорируются
func TestOutboxWorker_OnConnectionEvent_Timestamp(t *testing.T) {
	logger := zap.NewNop()
	worker := &OutboxWorker{
		logger:   logger,
		isPaused: false, // initially running
	}

	event := monitor.ConnectionEvent{
		Type:       monitor.EventTimestamp,
		Timestamp:  timestamppb.Now(),
		OccurredAt: time.Now(),
	}

	worker.OnConnectionEvent(event)

	// Check that nothing changed
	worker.pausedMu.RLock()
	paused := worker.isPaused
	worker.pausedMu.RUnlock()

	if paused {
		t.Error("expected worker to remain running after EventTimestamp")
	}
}

// TestOutboxWorker_OnConnectionEvent_ThreadSafety проверяет thread-safety обработки событий
func TestOutboxWorker_OnConnectionEvent_ThreadSafety(t *testing.T) {
	logger := zap.NewNop()
	worker := &OutboxWorker{
		logger:   logger,
		isPaused: false,
	}

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent connected events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			worker.OnConnectionEvent(monitor.ConnectionEvent{
				Type:       monitor.EventConnected,
				OccurredAt: time.Now(),
			})
		}
	}()

	// Concurrent disconnected events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			worker.OnConnectionEvent(monitor.ConnectionEvent{
				Type:       monitor.EventDisconnected,
				OccurredAt: time.Now(),
			})
		}
	}()

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			worker.pausedMu.RLock()
			_ = worker.isPaused
			worker.pausedMu.RUnlock()
		}
	}()

	wg.Wait()
	// If we get here without panic - thread-safety is OK
}

// TestOutboxWorker_GetListenerName проверяет возврат имени listener
func TestOutboxWorker_GetListenerName(t *testing.T) {
	worker := &OutboxWorker{}
	name := worker.GetListenerName()

	expected := "OutboxWorker"
	if name != expected {
		t.Errorf("expected listener name %q, got %q", expected, name)
	}
}
