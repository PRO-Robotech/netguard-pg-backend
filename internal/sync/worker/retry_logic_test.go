package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"netguard-pg-backend/internal/domain"
)

// TestCalculateBackoff tests exponential backoff calculation
func TestCalculateBackoff(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{
		logger: logger,
		config: DefaultConfig(),
	}

	tests := []struct {
		name     string
		attempts int
		expected time.Duration
	}{
		{
			name:     "First attempt",
			attempts: 0,
			expected: 10 * time.Second,
		},
		{
			name:     "Second attempt",
			attempts: 1,
			expected: 30 * time.Second,
		},
		{
			name:     "Third attempt",
			attempts: 2,
			expected: 90 * time.Second,
		},
		{
			name:     "Fourth attempt",
			attempts: 3,
			expected: 270 * time.Second,
		},
		{
			name:     "Fifth attempt",
			attempts: 4,
			expected: 600 * time.Second,
		},
		{
			name:     "Beyond max (should cap at 600s)",
			attempts: 10,
			expected: 600 * time.Second,
		},
		{
			name:     "Way beyond max",
			attempts: 100,
			expected: 600 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := worker.calculateBackoff(tt.attempts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHandleSyncSuccess tests successful sync handling
func TestHandleSyncSuccess(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	deleteCalled := false
	mockRepo := &mockOutboxRepo{
		deleteFunc: func(ctx context.Context, id uuid.UUID) error {
			deleteCalled = true
			return nil
		},
	}

	worker := &OutboxWorker{
		logger:     logger,
		outboxRepo: mockRepo,
		config:     DefaultConfig(),
	}

	entry := &domain.OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      []byte(`{}`),
		Attempts:     1,
	}

	err := worker.handleSyncSuccess(ctx, entry)

	assert.NoError(t, err)
	assert.True(t, deleteCalled, "Delete should be called on success")
}

// TestHandleSyncSuccess_DeleteError tests delete failure handling
func TestHandleSyncSuccess_DeleteError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	mockRepo := &mockOutboxRepo{
		deleteFunc: func(ctx context.Context, id uuid.UUID) error {
			return errors.New("delete failed")
		},
	}

	worker := &OutboxWorker{
		logger:     logger,
		outboxRepo: mockRepo,
		config:     DefaultConfig(),
	}

	entry := &domain.OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystemSGROUP,
		Payload:      []byte(`{}`),
	}

	err := worker.handleSyncSuccess(ctx, entry)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete outbox entry")
}

// TestConvertToSyncOperation tests operation conversion
func TestConvertToSyncOperation(t *testing.T) {
	tests := []struct {
		name     string
		input    domain.SyncOperation
		expected string
	}{
		{
			name:     "CREATE to Upsert",
			input:    domain.SyncOperationCreate,
			expected: "Upsert",
		},
		{
			name:     "UPDATE to Update",
			input:    domain.SyncOperationUpdate,
			expected: "Update",
		},
		{
			name:     "DELETE to Delete",
			input:    domain.SyncOperationDelete,
			expected: "Delete",
		},
		{
			name:     "Unknown defaults to Upsert",
			input:    domain.SyncOperation("UNKNOWN"),
			expected: "Upsert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToSyncOperation(tt.input)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

// TestCategorizeError_Validation tests validation error categorization
func TestCategorizeError_Validation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{
		logger: logger,
	}

	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{
			name:     "Context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: ErrorCategoryTemporary,
		},
		{
			name:     "Context canceled",
			err:      context.Canceled,
			expected: ErrorCategoryTemporary,
		},
		{
			name:     "Generic error defaults to temporary",
			err:      errors.New("some error"),
			expected: ErrorCategoryTemporary,
		},
		{
			name:     "Nil error defaults to temporary",
			err:      nil,
			expected: ErrorCategoryTemporary,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := worker.categorizeError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetMaxAttempts tests max attempts for each category
func TestGetMaxAttempts(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
		expected int
	}{
		{
			name:     "Validation errors fail fast (3 attempts)",
			category: ErrorCategoryValidation,
			expected: 3,
		},
		{
			name:     "Temporary errors standard retry (20 attempts)",
			category: ErrorCategoryTemporary,
			expected: 20,
		},
		{
			name:     "Network errors long retry (100 attempts)",
			category: ErrorCategoryNetwork,
			expected: 100,
		},
		{
			name:     "Conflict errors moderate retry (10 attempts)",
			category: ErrorCategoryConflict,
			expected: 10,
		},
		{
			name:     "Unknown category defaults to 10",
			category: ErrorCategory("unknown"),
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMaxAttempts(tt.category)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBackoffSequence tests the complete backoff sequence
func TestBackoffSequence(t *testing.T) {
	logger := zaptest.NewLogger(t)
	worker := &OutboxWorker{
		logger: logger,
		config: DefaultConfig(),
	}

	expectedSequence := []time.Duration{
		10 * time.Second,  // Attempt 0
		30 * time.Second,  // Attempt 1
		90 * time.Second,  // Attempt 2
		270 * time.Second, // Attempt 3
		600 * time.Second, // Attempt 4 (max)
	}

	for i, expected := range expectedSequence {
		result := worker.calculateBackoff(i)
		assert.Equal(t, expected, result, "Attempt %d should have backoff %v", i, expected)
	}

	// All attempts beyond 4 should be capped at 600s
	for i := 5; i < 20; i++ {
		result := worker.calculateBackoff(i)
		assert.Equal(t, 600*time.Second, result, "Attempt %d should be capped at 600s", i)
	}
}
