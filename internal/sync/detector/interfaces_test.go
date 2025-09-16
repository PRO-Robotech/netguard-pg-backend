package detector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockChangeDetector implements ChangeDetector interface for testing
type MockChangeDetector struct {
	handlers map[string]ChangeHandler
	started  bool
	nextID   int
}

func NewMockChangeDetector() *MockChangeDetector {
	return &MockChangeDetector{
		handlers: make(map[string]ChangeHandler),
	}
}

func (m *MockChangeDetector) Start(ctx context.Context) error {
	m.started = true
	return nil
}

func (m *MockChangeDetector) Stop() error {
	m.started = false
	return nil
}

func (m *MockChangeDetector) Subscribe(handler ChangeHandler) error {
	m.nextID++
	id := fmt.Sprintf("handler-%d", m.nextID)
	m.handlers[id] = handler
	return nil
}

func (m *MockChangeDetector) Unsubscribe(handler ChangeHandler) error {
	// For testing purposes, just remove any handler (this is simplified)
	for id, h := range m.handlers {
		// Use a simple heuristic - remove first handler found
		// In real implementation, we'd need proper handler identification
		if h != nil {
			delete(m.handlers, id)
			break
		}
	}
	return nil
}

// SimulateChange simulates a change event for testing
func (m *MockChangeDetector) SimulateChange(ctx context.Context, event ChangeEvent) error {
	for _, handler := range m.handlers {
		if err := handler.OnChange(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockChangeDetector) IsStarted() bool {
	return m.started
}

func (m *MockChangeDetector) HandlerCount() int {
	return len(m.handlers)
}

// TestChangeDetectorInterface verifies that MockChangeDetector implements ChangeDetector
func TestChangeDetectorInterface(t *testing.T) {
	var detector ChangeDetector = NewMockChangeDetector()

	ctx := context.Background()

	// Test Start
	err := detector.Start(ctx)
	assert.NoError(t, err)

	// Test Subscribe
	handler := ChangeHandlerFunc(func(ctx context.Context, event ChangeEvent) error {
		return nil
	})
	err = detector.Subscribe(handler)
	assert.NoError(t, err)

	// Test Unsubscribe
	err = detector.Unsubscribe(handler)
	assert.NoError(t, err)

	// Test Stop
	err = detector.Stop()
	assert.NoError(t, err)
}

func TestChangeEvent(t *testing.T) {
	now := time.Now()
	event := ChangeEvent{
		Timestamp: now,
		Source:    "test-source",
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	assert.Equal(t, now, event.Timestamp)
	assert.Equal(t, "test-source", event.Source)
	assert.Equal(t, "value1", event.Metadata["key1"])
	assert.Equal(t, 123, event.Metadata["key2"])
}

func TestChangeHandlerFunc(t *testing.T) {
	called := false
	var receivedEvent ChangeEvent

	handler := ChangeHandlerFunc(func(ctx context.Context, event ChangeEvent) error {
		called = true
		receivedEvent = event
		return nil
	})

	event := ChangeEvent{
		Timestamp: time.Now(),
		Source:    "test",
	}

	ctx := context.Background()
	err := handler.OnChange(ctx, event)

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, event.Source, receivedEvent.Source)
}

func TestMockChangeDetectorFunctionality(t *testing.T) {
	detector := NewMockChangeDetector()
	ctx := context.Background()

	// Initially not started
	assert.False(t, detector.IsStarted())
	assert.Equal(t, 0, detector.HandlerCount())

	// Start detector
	err := detector.Start(ctx)
	require.NoError(t, err)
	assert.True(t, detector.IsStarted())

	// Subscribe handlers
	var events1, events2 []ChangeEvent
	handler1 := ChangeHandlerFunc(func(ctx context.Context, event ChangeEvent) error {
		events1 = append(events1, event)
		return nil
	})
	handler2 := ChangeHandlerFunc(func(ctx context.Context, event ChangeEvent) error {
		events2 = append(events2, event)
		return nil
	})

	err = detector.Subscribe(handler1)
	require.NoError(t, err)
	err = detector.Subscribe(handler2)
	require.NoError(t, err)
	assert.Equal(t, 2, detector.HandlerCount())

	// Simulate change
	event := ChangeEvent{
		Timestamp: time.Now(),
		Source:    "sgroup",
		Metadata:  map[string]interface{}{"test": true},
	}

	err = detector.SimulateChange(ctx, event)
	require.NoError(t, err)

	// Both handlers should receive the event
	assert.Len(t, events1, 1)
	assert.Len(t, events2, 1)
	assert.Equal(t, "sgroup", events1[0].Source)
	assert.Equal(t, "sgroup", events2[0].Source)

	// Unsubscribe one handler
	err = detector.Unsubscribe(handler1)
	require.NoError(t, err)
	assert.Equal(t, 1, detector.HandlerCount())

	// Simulate another change
	event2 := ChangeEvent{
		Timestamp: time.Now(),
		Source:    "sgroup",
		Metadata:  map[string]interface{}{"test": false},
	}

	err = detector.SimulateChange(ctx, event2)
	require.NoError(t, err)

	// Only handler2 should receive the second event
	assert.Len(t, events1, 1) // Still 1
	assert.Len(t, events2, 2) // Now 2

	// Stop detector
	err = detector.Stop()
	require.NoError(t, err)
	assert.False(t, detector.IsStarted())
}
