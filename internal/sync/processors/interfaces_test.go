package processors

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/sync/detector"
)

// MockEntityProcessor implements EntityProcessor interface for testing
type MockEntityProcessor struct {
	entityType  string
	processFunc func(ctx context.Context, event detector.ChangeEvent) error
	callCount   int
	lastEvent   *detector.ChangeEvent
}

func NewMockEntityProcessor(entityType string) *MockEntityProcessor {
	return &MockEntityProcessor{
		entityType: entityType,
		processFunc: func(ctx context.Context, event detector.ChangeEvent) error {
			return nil
		},
	}
}

func (m *MockEntityProcessor) GetEntityType() string {
	return m.entityType
}

func (m *MockEntityProcessor) ProcessChanges(ctx context.Context, event detector.ChangeEvent) error {
	m.callCount++
	m.lastEvent = &event
	return m.processFunc(ctx, event)
}

func (m *MockEntityProcessor) SetProcessFunc(f func(ctx context.Context, event detector.ChangeEvent) error) {
	m.processFunc = f
}

func (m *MockEntityProcessor) GetCallCount() int {
	return m.callCount
}

func (m *MockEntityProcessor) GetLastEvent() *detector.ChangeEvent {
	return m.lastEvent
}

func TestEntityProcessorInterface(t *testing.T) {
	processor := NewMockEntityProcessor("test-entity")
	var entityProcessor EntityProcessor = processor

	// Test GetEntityType
	entityType := entityProcessor.GetEntityType()
	assert.Equal(t, "test-entity", entityType)

	// Test ProcessChanges
	ctx := context.Background()
	event := detector.ChangeEvent{
		Timestamp: time.Now(),
		Source:    "test-source",
		Metadata:  map[string]interface{}{"key": "value"},
	}

	err := entityProcessor.ProcessChanges(ctx, event)
	require.NoError(t, err)

	// Check that mock received the call
	assert.Equal(t, 1, processor.GetCallCount())
	assert.Equal(t, event.Source, processor.GetLastEvent().Source)
}

func TestEntityProcessorFunc(t *testing.T) {
	called := false
	var receivedEvent detector.ChangeEvent

	processor := NewEntityProcessorFunc("test-func", func(ctx context.Context, event detector.ChangeEvent) error {
		called = true
		receivedEvent = event
		return nil
	})

	assert.Equal(t, "test-func", processor.GetEntityType())

	ctx := context.Background()
	event := detector.ChangeEvent{
		Timestamp: time.Now(),
		Source:    "test",
	}

	err := processor.ProcessChanges(ctx, event)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, event.Source, receivedEvent.Source)
}

func TestEntityProcessorFuncWithError(t *testing.T) {
	expectedError := errors.New("test error")
	processor := NewEntityProcessorFunc("test-error", func(ctx context.Context, event detector.ChangeEvent) error {
		return expectedError
	})

	ctx := context.Background()
	event := detector.ChangeEvent{
		Timestamp: time.Now(),
		Source:    "test",
	}

	err := processor.ProcessChanges(ctx, event)
	assert.Equal(t, expectedError, err)
}

func TestProcessResult(t *testing.T) {
	result := &ProcessResult{}

	// Initially empty
	assert.Equal(t, 0, result.ProcessedCount)
	assert.Equal(t, 0, result.ErrorCount)
	assert.False(t, result.HasErrors())
	assert.Len(t, result.Errors, 0)

	// Add processed items
	result.AddProcessed(5)
	assert.Equal(t, 5, result.ProcessedCount)

	result.AddProcessed(3)
	assert.Equal(t, 8, result.ProcessedCount)

	// Add errors
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	result.AddError(err1)
	assert.Equal(t, 1, result.ErrorCount)
	assert.True(t, result.HasErrors())
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, err1, result.Errors[0])

	result.AddError(err2)
	assert.Equal(t, 2, result.ErrorCount)
	assert.Len(t, result.Errors, 2)

	// Adding nil error should not change anything
	result.AddError(nil)
	assert.Equal(t, 2, result.ErrorCount)
	assert.Len(t, result.Errors, 2)
}

func TestProcessResultDetails(t *testing.T) {
	result := &ProcessResult{}

	// Initially no details
	assert.Nil(t, result.Details)
	assert.Nil(t, result.GetDetail("key"))

	// Set detail
	result.SetDetail("key1", "value1")
	assert.NotNil(t, result.Details)
	assert.Equal(t, "value1", result.GetDetail("key1"))

	// Set more details
	result.SetDetail("key2", 42)
	result.SetDetail("key3", []string{"a", "b", "c"})

	assert.Equal(t, "value1", result.GetDetail("key1"))
	assert.Equal(t, 42, result.GetDetail("key2"))
	assert.Equal(t, []string{"a", "b", "c"}, result.GetDetail("key3"))

	// Get non-existent detail
	assert.Nil(t, result.GetDetail("non-existent"))

	// Check details map
	assert.Len(t, result.Details, 3)
}

func TestMockEntityProcessorFunctionality(t *testing.T) {
	processor := NewMockEntityProcessor("hosts")

	// Test initial state
	assert.Equal(t, "hosts", processor.GetEntityType())
	assert.Equal(t, 0, processor.GetCallCount())
	assert.Nil(t, processor.GetLastEvent())

	// Test successful processing
	ctx := context.Background()
	event := detector.ChangeEvent{
		Timestamp: time.Now(),
		Source:    "sgroup",
		Metadata:  map[string]interface{}{"test": true},
	}

	err := processor.ProcessChanges(ctx, event)
	require.NoError(t, err)
	assert.Equal(t, 1, processor.GetCallCount())
	assert.Equal(t, "sgroup", processor.GetLastEvent().Source)

	// Test error processing
	expectedError := errors.New("processing error")
	processor.SetProcessFunc(func(ctx context.Context, event detector.ChangeEvent) error {
		return expectedError
	})

	event2 := detector.ChangeEvent{
		Timestamp: time.Now(),
		Source:    "sgroup2",
	}

	err = processor.ProcessChanges(ctx, event2)
	assert.Equal(t, expectedError, err)
	assert.Equal(t, 2, processor.GetCallCount())
	assert.Equal(t, "sgroup2", processor.GetLastEvent().Source)
}
