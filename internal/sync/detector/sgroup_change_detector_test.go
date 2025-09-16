package detector

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"netguard-pg-backend/internal/sync/interfaces"
	"netguard-pg-backend/internal/sync/types"
)

// MockSGroupGateway implements interfaces.SGroupGateway for testing
type MockSGroupGateway struct {
	mock.Mock
}

// Ensure MockSGroupGateway implements interfaces.SGroupGateway
var _ interfaces.SGroupGateway = (*MockSGroupGateway)(nil)

func (m *MockSGroupGateway) GetStatuses(ctx context.Context) (chan *timestamppb.Timestamp, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(chan *timestamppb.Timestamp), args.Error(1)
}

func (m *MockSGroupGateway) Sync(ctx context.Context, req *types.SyncRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockSGroupGateway) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockSGroupGateway) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockChangeHandler for testing
type MockChangeHandler struct {
	mu           sync.Mutex
	events       []ChangeEvent
	shouldError  bool
	errorMessage string
}

func NewMockChangeHandler() *MockChangeHandler {
	return &MockChangeHandler{
		events: make([]ChangeEvent, 0),
	}
}

func (m *MockChangeHandler) OnChange(ctx context.Context, event ChangeEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, event)

	if m.shouldError {
		return fmt.Errorf("%s", m.errorMessage)
	}
	return nil
}

func (m *MockChangeHandler) SetShouldError(shouldError bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldError = shouldError
	m.errorMessage = message
}

func (m *MockChangeHandler) GetEvents() []ChangeEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy
	events := make([]ChangeEvent, len(m.events))
	copy(events, m.events)
	return events
}

func (m *MockChangeHandler) GetEventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *MockChangeHandler) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = make([]ChangeEvent, 0)
}

func TestNewSGROUPChangeDetector(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{
		ReconnectInterval: 1 * time.Second,
		MaxRetries:        3,
		ChangeEventSource: "test-sgroup",
	}

	detector := NewSGROUPChangeDetector(mockClient, config)
	require.NotNil(t, detector)

	sgDetector, ok := detector.(*SGROUPChangeDetector)
	require.True(t, ok)

	assert.Equal(t, mockClient, sgDetector.client)
	assert.Equal(t, config.ReconnectInterval, sgDetector.config.ReconnectInterval)
	assert.Equal(t, config.MaxRetries, sgDetector.config.MaxRetries)
	assert.Equal(t, config.ChangeEventSource, sgDetector.config.ChangeEventSource)
	assert.False(t, sgDetector.IsStarted())
	assert.Equal(t, 0, sgDetector.GetHandlerCount())
}

func TestNewSGROUPChangeDetectorDefaults(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{} // Empty config to test defaults

	detector := NewSGROUPChangeDetector(mockClient, config)
	sgDetector := detector.(*SGROUPChangeDetector)

	assert.Equal(t, 5*time.Second, sgDetector.config.ReconnectInterval)
	assert.Equal(t, "sgroup", sgDetector.config.ChangeEventSource)
}

func TestSGROUPChangeDetectorSubscribe(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	handler1 := NewMockChangeHandler()
	handler2 := NewMockChangeHandler()

	// Test subscription
	err := detector.Subscribe(handler1)
	require.NoError(t, err)
	assert.Equal(t, 1, detector.GetHandlerCount())

	err = detector.Subscribe(handler2)
	require.NoError(t, err)
	assert.Equal(t, 2, detector.GetHandlerCount())

	// Test nil handler
	err = detector.Subscribe(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler cannot be nil")
}

func TestSGROUPChangeDetectorUnsubscribe(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	handler1 := NewMockChangeHandler()
	handler2 := NewMockChangeHandler()

	// Subscribe handlers
	err := detector.Subscribe(handler1)
	require.NoError(t, err)
	err = detector.Subscribe(handler2)
	require.NoError(t, err)
	assert.Equal(t, 2, detector.GetHandlerCount())

	// Unsubscribe
	err = detector.Unsubscribe(handler1)
	require.NoError(t, err)
	assert.Equal(t, 1, detector.GetHandlerCount())

	// Test nil handler
	err = detector.Unsubscribe(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler cannot be nil")
}

func TestSGROUPChangeDetectorStartStop(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{
		ReconnectInterval: 100 * time.Millisecond, // Fast for testing
	}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	// Mock the GetStatuses call to return a channel that closes immediately
	timestampChan := make(chan *timestamppb.Timestamp)
	close(timestampChan) // Close immediately to simulate stream end

	mockClient.On("GetStatuses", mock.Anything).Return(timestampChan, nil)

	ctx := context.Background()

	// Test start
	assert.False(t, detector.IsStarted())
	err := detector.Start(ctx)
	require.NoError(t, err)
	assert.True(t, detector.IsStarted())

	// Test double start
	err = detector.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Wait a bit for the goroutine to process
	time.Sleep(200 * time.Millisecond)

	// Test stop
	err = detector.Stop()
	require.NoError(t, err)
	assert.False(t, detector.IsStarted())

	// Test double stop (should not error)
	err = detector.Stop()
	require.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestSGROUPChangeDetectorEventProcessing(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{
		ChangeEventSource: "test-sgroup",
	}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	// Create a channel for timestamps
	timestampChan := make(chan *timestamppb.Timestamp, 10)

	// Set up mock to return our channel
	mockClient.On("GetStatuses", mock.Anything).Return(timestampChan, nil)

	// Create and subscribe handlers
	handler1 := NewMockChangeHandler()
	handler2 := NewMockChangeHandler()

	err := detector.Subscribe(handler1)
	require.NoError(t, err)
	err = detector.Subscribe(handler2)
	require.NoError(t, err)

	ctx := context.Background()

	// Start detector
	err = detector.Start(ctx)
	require.NoError(t, err)

	// Give the detector a moment to start
	time.Sleep(50 * time.Millisecond)

	// Send timestamps
	now := time.Now()
	timestamp1 := timestamppb.New(now)
	timestamp2 := timestamppb.New(now.Add(1 * time.Second))

	timestampChan <- timestamp1
	timestampChan <- timestamp2

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check that handlers received events
	events1 := handler1.GetEvents()
	events2 := handler2.GetEvents()

	assert.Len(t, events1, 2)
	assert.Len(t, events2, 2)

	// Check first event
	assert.Equal(t, "test-sgroup", events1[0].Source)
	assert.True(t, events1[0].Timestamp.Equal(now))

	// Check second event
	assert.Equal(t, "test-sgroup", events1[1].Source)
	assert.True(t, events1[1].Timestamp.Equal(now.Add(1*time.Second)))

	// Close timestamp channel and stop detector
	close(timestampChan)
	err = detector.Stop()
	require.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestSGROUPChangeDetectorDuplicateTimestamps(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	timestampChan := make(chan *timestamppb.Timestamp, 10)
	mockClient.On("GetStatuses", mock.Anything).Return(timestampChan, nil)

	handler := NewMockChangeHandler()
	err := detector.Subscribe(handler)
	require.NoError(t, err)

	ctx := context.Background()
	err = detector.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Send same timestamp twice
	now := time.Now()
	timestamp := timestamppb.New(now)

	timestampChan <- timestamp
	timestampChan <- timestamp // Duplicate

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Should only receive one event (first timestamp)
	events := handler.GetEvents()
	assert.Len(t, events, 1)

	// Now send a newer timestamp
	newerTimestamp := timestamppb.New(now.Add(1 * time.Second))
	timestampChan <- newerTimestamp

	time.Sleep(100 * time.Millisecond)

	// Should now have two events
	events = handler.GetEvents()
	assert.Len(t, events, 2)

	close(timestampChan)
	err = detector.Stop()
	require.NoError(t, err)

	mockClient.AssertExpectations(t)
}

func TestSGROUPChangeDetectorHandlerError(t *testing.T) {
	mockClient := &MockSGroupGateway{}
	config := SGROUPDetectorConfig{}
	detector := NewSGROUPChangeDetector(mockClient, config).(*SGROUPChangeDetector)

	timestampChan := make(chan *timestamppb.Timestamp, 10)
	mockClient.On("GetStatuses", mock.Anything).Return(timestampChan, nil)

	// Create handlers - one that errors, one that doesn't
	goodHandler := NewMockChangeHandler()
	errorHandler := NewMockChangeHandler()
	errorHandler.SetShouldError(true, "test error")

	err := detector.Subscribe(goodHandler)
	require.NoError(t, err)
	err = detector.Subscribe(errorHandler)
	require.NoError(t, err)

	ctx := context.Background()
	err = detector.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// Send timestamp
	timestamp := timestamppb.New(time.Now())
	timestampChan <- timestamp

	time.Sleep(100 * time.Millisecond)

	// Both handlers should have received the event
	assert.Equal(t, 1, goodHandler.GetEventCount())
	assert.Equal(t, 1, errorHandler.GetEventCount())

	close(timestampChan)
	err = detector.Stop()
	require.NoError(t, err)

	mockClient.AssertExpectations(t)
}
