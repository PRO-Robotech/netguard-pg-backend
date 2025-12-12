package domain

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxEntry_TableName(t *testing.T) {
	entry := &OutboxEntry{}
	assert.Equal(t, "sync_outbox", entry.TableName())
}

func TestOutboxEntry_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   *OutboxEntry
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid entry",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{"test": true}`),
				Status:            OutboxStatusPending,
				MaxRetries:        5,
			},
			wantErr: false,
		},
		{
			name: "missing resource_type",
			entry: &OutboxEntry{
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "resource_type is required",
		},
		{
			name: "nil resource_id",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.Nil,
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "resource_id is required",
		},
		{
			name: "missing resource_namespace",
			entry: &OutboxEntry{
				ResourceType: "Host",
				ResourceID:   uuid.New(),
				ResourceName: "test-host",
				Operation:    SyncOperationCreate,
				TargetSystem: TargetSystemSGROUP,
				Payload:      json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "resource_namespace is required",
		},
		{
			name: "missing resource_name",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "resource_name is required",
		},
		{
			name: "missing operation",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "operation is required",
		},
		{
			name: "invalid operation",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         "INVALID",
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "invalid operation",
		},
		{
			name: "missing target_system",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "target_system is required",
		},
		{
			name: "invalid target_system",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      "INVALID",
				Payload:           json.RawMessage(`{}`),
			},
			wantErr: true,
			errMsg:  "invalid target_system",
		},
		{
			name: "empty payload",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage{},
			},
			wantErr: true,
			errMsg:  "payload is required",
		},
		{
			name: "negative max_retries",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
				MaxRetries:        -1,
			},
			wantErr: true,
			errMsg:  "max_retries must be non-negative",
		},
		{
			name: "invalid status",
			entry: &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
				Status:            "INVALID_STATUS",
				MaxRetries:        5,
			},
			wantErr: true,
			errMsg:  "invalid status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOutboxEntry_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		status     OutboxStatus
		attempts   int
		maxRetries int
		want       bool
	}{
		{"pending is retryable", OutboxStatusPending, 0, 5, true},
		{"failed_retryable is retryable", OutboxStatusFailedRetryable, 2, 5, true},
		{"max attempts reached", OutboxStatusFailedRetryable, 5, 5, false},
		{"failed_permanent not retryable", OutboxStatusFailedPermanent, 1, 5, false},
		{"success not retryable", OutboxStatusSuccess, 0, 5, false},
		{"processing is retryable", OutboxStatusProcessing, 1, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &OutboxEntry{
				Status:     tt.status,
				Attempts:   tt.attempts,
				MaxRetries: tt.maxRetries,
			}
			assert.Equal(t, tt.want, entry.IsRetryable())
		})
	}
}

func TestOutboxEntry_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from OutboxStatus
		to   OutboxStatus
		want bool
	}{
		// From PENDING
		{OutboxStatusPending, OutboxStatusProcessing, true},
		{OutboxStatusPending, OutboxStatusSuccess, true},
		{OutboxStatusPending, OutboxStatusFailedRetryable, false},
		{OutboxStatusPending, OutboxStatusFailedPermanent, false},

		// From PROCESSING
		{OutboxStatusProcessing, OutboxStatusSuccess, true},
		{OutboxStatusProcessing, OutboxStatusFailedRetryable, true},
		{OutboxStatusProcessing, OutboxStatusFailedPermanent, true},
		{OutboxStatusProcessing, OutboxStatusPending, false},

		// From FAILED_RETRYABLE
		{OutboxStatusFailedRetryable, OutboxStatusProcessing, true},
		{OutboxStatusFailedRetryable, OutboxStatusFailedPermanent, true},
		{OutboxStatusFailedRetryable, OutboxStatusSuccess, false},
		{OutboxStatusFailedRetryable, OutboxStatusPending, false},

		// From SUCCESS (terminal)
		{OutboxStatusSuccess, OutboxStatusProcessing, false},
		{OutboxStatusSuccess, OutboxStatusPending, false},
		{OutboxStatusSuccess, OutboxStatusFailedRetryable, false},
		{OutboxStatusSuccess, OutboxStatusFailedPermanent, false},

		// From FAILED_PERMANENT (terminal)
		{OutboxStatusFailedPermanent, OutboxStatusProcessing, false},
		{OutboxStatusFailedPermanent, OutboxStatusPending, false},
		{OutboxStatusFailedPermanent, OutboxStatusSuccess, false},
		{OutboxStatusFailedPermanent, OutboxStatusFailedRetryable, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s->%s", tt.from, tt.to), func(t *testing.T) {
			entry := &OutboxEntry{Status: tt.from}
			assert.Equal(t, tt.want, entry.CanTransitionTo(tt.to))
		})
	}
}

func TestOutboxEntry_MarkProcessing(t *testing.T) {
	entry := &OutboxEntry{Status: OutboxStatusPending}
	before := time.Now()

	entry.MarkProcessing()

	assert.Equal(t, OutboxStatusProcessing, entry.Status)
	assert.True(t, entry.UpdatedAt.After(before) || entry.UpdatedAt.Equal(before))
}

func TestOutboxEntry_MarkSuccess(t *testing.T) {
	entry := &OutboxEntry{
		Status:    OutboxStatusProcessing,
		LastError: stringPtr("some error"),
	}
	before := time.Now()

	entry.MarkSuccess()

	assert.Equal(t, OutboxStatusSuccess, entry.Status)
	assert.Nil(t, entry.LastError)
	assert.Nil(t, entry.ErrorCategory)
	assert.NotNil(t, entry.ProcessedAt)
	assert.True(t, entry.UpdatedAt.After(before) || entry.UpdatedAt.Equal(before))
	assert.True(t, entry.ProcessedAt.After(before) || entry.ProcessedAt.Equal(before))
}

func TestOutboxEntry_MarkFailed(t *testing.T) {
	t.Run("retryable failure", func(t *testing.T) {
		entry := &OutboxEntry{
			Status:     OutboxStatusProcessing,
			Attempts:   1,
			MaxRetries: 5,
		}

		entry.MarkFailed(fmt.Errorf("network timeout"), "NETWORK", false)

		assert.Equal(t, OutboxStatusFailedRetryable, entry.Status)
		assert.Equal(t, 2, entry.Attempts)
		assert.Equal(t, "network timeout", *entry.LastError)
		assert.Equal(t, "NETWORK", *entry.ErrorCategory)
		assert.NotNil(t, entry.NextRetryAt)
	})

	t.Run("permanent failure", func(t *testing.T) {
		entry := &OutboxEntry{
			Status:     OutboxStatusProcessing,
			Attempts:   1,
			MaxRetries: 5,
		}

		entry.MarkFailed(fmt.Errorf("validation error"), "VALIDATION", true)

		assert.Equal(t, OutboxStatusFailedPermanent, entry.Status)
		assert.Equal(t, 2, entry.Attempts)
		assert.Nil(t, entry.NextRetryAt)
	})

	t.Run("max retries reached", func(t *testing.T) {
		entry := &OutboxEntry{
			Status:     OutboxStatusProcessing,
			Attempts:   4,
			MaxRetries: 5,
		}

		entry.MarkFailed(fmt.Errorf("network error"), "NETWORK", false)

		assert.Equal(t, OutboxStatusFailedPermanent, entry.Status)
		assert.Equal(t, 5, entry.Attempts)
	})

	t.Run("exactly at max retries", func(t *testing.T) {
		entry := &OutboxEntry{
			Status:     OutboxStatusProcessing,
			Attempts:   5,
			MaxRetries: 5,
		}

		entry.MarkFailed(fmt.Errorf("another error"), "NETWORK", false)

		assert.Equal(t, OutboxStatusFailedPermanent, entry.Status)
		assert.Equal(t, 6, entry.Attempts)
		assert.Nil(t, entry.NextRetryAt)
	})
}

func TestOutboxEntry_CalculateNextRetry(t *testing.T) {
	tests := []struct {
		attempts   int
		minSeconds float64
		maxSeconds float64
	}{
		{1, 1.9, 3},    // 2^1 = 2s (allow small timing variance)
		{2, 3.9, 5},    // 2^2 = 4s
		{3, 7.9, 9},    // 2^3 = 8s
		{4, 15.9, 17},  // 2^4 = 16s
		{5, 31.9, 33},  // 2^5 = 32s
		{10, 299, 301}, // Capped at 300s
		{15, 299, 301}, // Also capped at 300s
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempts), func(t *testing.T) {
			entry := &OutboxEntry{Attempts: tt.attempts}
			nextRetry := entry.CalculateNextRetry()

			diff := nextRetry.Sub(time.Now()).Seconds()
			assert.GreaterOrEqual(t, diff, tt.minSeconds)
			assert.LessOrEqual(t, diff, tt.maxSeconds)
		})
	}
}

func TestOutboxEntry_IsDueForRetry(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Second)
	future := now.Add(10 * time.Second)

	tests := []struct {
		name        string
		status      OutboxStatus
		nextRetryAt *time.Time
		want        bool
	}{
		{"not failed_retryable", OutboxStatusPending, nil, false},
		{"processing not due", OutboxStatusProcessing, nil, false},
		{"success not due", OutboxStatusSuccess, nil, false},
		{"failed_permanent not due", OutboxStatusFailedPermanent, nil, false},
		{"failed_retryable no next_retry_at", OutboxStatusFailedRetryable, nil, true},
		{"failed_retryable next_retry in past", OutboxStatusFailedRetryable, &past, true},
		{"failed_retryable next_retry in future", OutboxStatusFailedRetryable, &future, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &OutboxEntry{
				Status:      tt.status,
				NextRetryAt: tt.nextRetryAt,
			}
			assert.Equal(t, tt.want, entry.IsDueForRetry())
		})
	}
}

func TestOutboxEntry_JSONSerialization(t *testing.T) {
	original := &OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    SyncOperationCreate,
		TargetSystem: TargetSystemSGROUP,
		Payload:      json.RawMessage(`{"name":"test"}`),
		Status:       OutboxStatusPending,
		Attempts:     0,
		MaxRetries:   5,
		CreatedAt:    time.Now().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().Truncate(time.Millisecond),
	}

	// Marshal
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var decoded OutboxEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Compare
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.ResourceType, decoded.ResourceType)
	assert.Equal(t, original.Operation, decoded.Operation)
	assert.Equal(t, original.TargetSystem, decoded.TargetSystem)
	assert.Equal(t, original.Status, decoded.Status)
}

func TestOutboxEntry_JSONSerializationWithOptionalFields(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	errorMsg := "test error"
	errorCat := "NETWORK"

	original := &OutboxEntry{
		ID:               uuid.New(),
		ResourceType:     "Network",
		ResourceID:       uuid.New(),
		Operation:        SyncOperationUpdate,
		TargetSystem:     TargetSystemInternal,
		Payload:          json.RawMessage(`{"cidr":"10.0.0.0/24"}`),
		Delta:            json.RawMessage(`{"status":"updated"}`),
		AffectsResources: json.RawMessage(`[{"type":"Host","id":"123"}]`),
		Status:           OutboxStatusFailedRetryable,
		Attempts:         2,
		MaxRetries:       5,
		NextRetryAt:      &now,
		LastError:        &errorMsg,
		ErrorCategory:    &errorCat,
		CreatedAt:        now,
		UpdatedAt:        now,
		ProcessedAt:      &now,
	}

	// Marshal
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var decoded OutboxEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Compare all fields
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.ResourceType, decoded.ResourceType)
	assert.Equal(t, original.ResourceID, decoded.ResourceID)
	assert.Equal(t, original.Operation, decoded.Operation)
	assert.Equal(t, original.TargetSystem, decoded.TargetSystem)
	assert.JSONEq(t, string(original.Payload), string(decoded.Payload))
	assert.JSONEq(t, string(original.Delta), string(decoded.Delta))
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.Attempts, decoded.Attempts)
	assert.Equal(t, *original.LastError, *decoded.LastError)
	assert.Equal(t, *original.ErrorCategory, *decoded.ErrorCategory)
}

func TestOutboxEntry_AllOperationTypes(t *testing.T) {
	operations := []SyncOperation{
		SyncOperationCreate,
		SyncOperationUpdate,
		SyncOperationDelete,
	}

	for _, op := range operations {
		t.Run(string(op), func(t *testing.T) {
			entry := &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         op,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
				Status:            OutboxStatusPending,
				MaxRetries:        5,
			}

			err := entry.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestOutboxEntry_AllTargetSystems(t *testing.T) {
	systems := []TargetSystem{
		TargetSystemSGROUP,
		TargetSystemInternal,
	}

	for _, sys := range systems {
		t.Run(string(sys), func(t *testing.T) {
			entry := &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      sys,
				Payload:           json.RawMessage(`{}`),
				Status:            OutboxStatusPending,
				MaxRetries:        5,
			}

			err := entry.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestOutboxEntry_AllStatuses(t *testing.T) {
	statuses := []OutboxStatus{
		OutboxStatusPending,
		OutboxStatusProcessing,
		OutboxStatusSuccess,
		OutboxStatusFailedRetryable,
		OutboxStatusFailedPermanent,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			entry := &OutboxEntry{
				ResourceType:      "Host",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-host",
				Operation:         SyncOperationCreate,
				TargetSystem:      TargetSystemSGROUP,
				Payload:           json.RawMessage(`{}`),
				Status:            status,
				MaxRetries:        5,
			}

			err := entry.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestOutboxEntry_CompleteLifecycle(t *testing.T) {
	// Create entry
	entry := &OutboxEntry{
		ID:                uuid.New(),
		ResourceType:      "Host",
		ResourceID:        uuid.New(),
		ResourceNamespace: "default",
		ResourceName:      "test-host",
		Operation:         SyncOperationCreate,
		TargetSystem:      TargetSystemSGROUP,
		Payload:           json.RawMessage(`{"name":"test-host"}`),
		Status:            OutboxStatusPending,
		Attempts:          0,
		MaxRetries:        3,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Validate initial state
	err := entry.Validate()
	assert.NoError(t, err)
	assert.True(t, entry.IsRetryable())
	assert.True(t, entry.CanTransitionTo(OutboxStatusProcessing))

	// Mark as processing
	entry.MarkProcessing()
	assert.Equal(t, OutboxStatusProcessing, entry.Status)
	assert.True(t, entry.CanTransitionTo(OutboxStatusSuccess))

	// Simulate first failure (attempt becomes 1)
	entry.MarkFailed(fmt.Errorf("temporary error"), "NETWORK", false)
	assert.Equal(t, OutboxStatusFailedRetryable, entry.Status)
	assert.Equal(t, 1, entry.Attempts)
	assert.True(t, entry.IsRetryable())
	assert.NotNil(t, entry.NextRetryAt)

	// Retry (attempt 1 -> processing)
	entry.MarkProcessing()
	assert.Equal(t, OutboxStatusProcessing, entry.Status)

	// Second failure (attempt becomes 2)
	entry.MarkFailed(fmt.Errorf("another error"), "TIMEOUT", false)
	assert.Equal(t, OutboxStatusFailedRetryable, entry.Status)
	assert.Equal(t, 2, entry.Attempts)
	assert.True(t, entry.IsRetryable())

	// Third retry (attempt 2 -> processing)
	entry.MarkProcessing()
	entry.MarkFailed(fmt.Errorf("third error"), "NETWORK", false)
	assert.Equal(t, OutboxStatusFailedPermanent, entry.Status) // 3 attempts = max_retries reached
	assert.Equal(t, 3, entry.Attempts)
	assert.False(t, entry.IsRetryable())
	assert.Nil(t, entry.NextRetryAt)
}

func TestOutboxEntry_SuccessfulLifecycle(t *testing.T) {
	// Create entry
	entry := &OutboxEntry{
		ID:                uuid.New(),
		ResourceType:      "Network",
		ResourceID:        uuid.New(),
		ResourceNamespace: "default",
		ResourceName:      "test-network",
		Operation:         SyncOperationUpdate,
		TargetSystem:      TargetSystemSGROUP,
		Payload:           json.RawMessage(`{"cidr":"10.0.0.0/24"}`),
		Status:            OutboxStatusPending,
		Attempts:          0,
		MaxRetries:        5,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	// Validate
	err := entry.Validate()
	assert.NoError(t, err)

	// Process successfully
	entry.MarkProcessing()
	assert.Equal(t, OutboxStatusProcessing, entry.Status)

	entry.MarkSuccess()
	assert.Equal(t, OutboxStatusSuccess, entry.Status)
	assert.NotNil(t, entry.ProcessedAt)
	assert.Nil(t, entry.LastError)
	assert.False(t, entry.IsRetryable())
	assert.False(t, entry.CanTransitionTo(OutboxStatusProcessing))
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
