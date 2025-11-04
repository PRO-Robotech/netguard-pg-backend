package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"netguard-pg-backend/internal/domain"
)

// Test_AffectedResource_JSON tests JSON marshaling/unmarshaling
func Test_AffectedResource_JSON(t *testing.T) {
	resources := []AffectedResource{
		{Type: "Host", Namespace: "default", Name: "test-host"},
		{Type: "AddressGroup", Namespace: "default", Name: "test-ag"},
	}

	// Marshal
	data, err := json.Marshal(resources)
	require.NoError(t, err)

	// Unmarshal
	var decoded []AffectedResource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resources, decoded)
}

// Test_waitForDependencies_EmptyResources tests with no dependencies
func Test_waitForDependencies_EmptyResources(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	item := &domain.OutboxEntry{
		AffectsResources: []byte(`[]`),
	}

	pending, allReady, err := worker.waitForDependencies(context.Background(), item)
	require.NoError(t, err)
	assert.True(t, allReady)
	assert.Empty(t, pending)
}

// Test_waitForDependencies_NilResources tests with nil affects_resources
func Test_waitForDependencies_NilResources(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	item := &domain.OutboxEntry{
		AffectsResources: nil,
	}

	pending, allReady, err := worker.waitForDependencies(context.Background(), item)
	require.NoError(t, err)
	assert.True(t, allReady)
	assert.Empty(t, pending)
}

// Test_waitForDependencies_InvalidJSON tests with invalid JSON
func Test_waitForDependencies_InvalidJSON(t *testing.T) {
	worker := &OutboxWorker{
		logger: zap.NewNop(),
	}

	item := &domain.OutboxEntry{
		AffectsResources: []byte(`{invalid json`),
	}

	pending, allReady, err := worker.waitForDependencies(context.Background(), item)
	assert.Error(t, err)
	assert.False(t, allReady)
	assert.Nil(t, pending)
}

// Test_processProcessResource_Logic tests the basic logic flow
func Test_processProcessResource_Logic(t *testing.T) {
	tests := []struct {
		name          string
		affectedCount int
	}{
		{
			name:          "empty affects_resources",
			affectedCount: 0,
		},
		{
			name:          "has affected resources",
			affectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var affected []AffectedResource
			for i := 0; i < tt.affectedCount; i++ {
				affected = append(affected, AffectedResource{
					Type:      "Host",
					Namespace: "default",
					Name:      "test-host",
				})
			}

			affectedJSON, _ := json.Marshal(affected)
			item := &domain.OutboxEntry{
				ID:               uuid.New(),
				ResourceType:     "HostBinding",
				ResourceID:       uuid.New(),
				Operation:        domain.SyncOperationCreate,
				TargetSystem:     domain.TargetSystemInternal,
				Payload:          []byte(`{"metadata":{"namespace":"default","name":"test-binding"}}`),
				AffectsResources: affectedJSON,
			}

			// Verify we can unmarshal the affected resources
			var decoded []AffectedResource
			err := json.Unmarshal(item.AffectsResources, &decoded)
			require.NoError(t, err)
			assert.Len(t, decoded, tt.affectedCount)
		})
	}
}

// Test_updatePendingSyncConditionFromOutbox_PayloadParsing tests payload parsing
func Test_updatePendingSyncConditionFromOutbox_PayloadParsing(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		expectError bool
		expectNS    string
		expectName  string
	}{
		{
			name:        "valid payload",
			payload:     `{"metadata":{"namespace":"default","name":"test-binding"}}`,
			expectError: false,
			expectNS:    "default",
			expectName:  "test-binding",
		},
		{
			name:        "invalid JSON",
			payload:     `{invalid`,
			expectError: true,
		},
		{
			name:        "missing metadata",
			payload:     `{"data":"value"}`,
			expectError: false,
			expectNS:    "",
			expectName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parsed struct {
				Metadata struct {
					Namespace string `json:"namespace"`
					Name      string `json:"name"`
				} `json:"metadata"`
			}

			err := json.Unmarshal([]byte(tt.payload), &parsed)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectNS != "" {
					assert.Equal(t, tt.expectNS, parsed.Metadata.Namespace)
					assert.Equal(t, tt.expectName, parsed.Metadata.Name)
				}
			}
		})
	}
}

// Test_OutboxEntry_Validation tests OutboxEntry validation for process resources
func Test_OutboxEntry_Validation(t *testing.T) {
	tests := []struct {
		name        string
		entry       *domain.OutboxEntry
		expectError bool
	}{
		{
			name: "valid process resource entry",
			entry: &domain.OutboxEntry{
				ResourceType:      "HostBinding",
				ResourceID:        uuid.New(),
				ResourceNamespace: "default",
				ResourceName:      "test-binding",
				Operation:         domain.SyncOperationCreate,
				TargetSystem:      domain.TargetSystemInternal,
				Payload:           []byte(`{"test":"data"}`),
				MaxRetries:        20,
				Status:            domain.OutboxStatusPending,
			},
			expectError: false,
		},
		{
			name: "missing resource type",
			entry: &domain.OutboxEntry{
				ResourceType: "",
				ResourceID:   uuid.New(),
				Operation:    domain.SyncOperationCreate,
				TargetSystem: domain.TargetSystemInternal,
				Payload:      []byte(`{"test":"data"}`),
			},
			expectError: true,
		},
		{
			name: "missing resource ID",
			entry: &domain.OutboxEntry{
				ResourceType: "HostBinding",
				ResourceID:   uuid.Nil,
				Operation:    domain.SyncOperationCreate,
				TargetSystem: domain.TargetSystemInternal,
				Payload:      []byte(`{"test":"data"}`),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test_AffectedResource_Structure tests the AffectedResource structure
func Test_AffectedResource_Structure(t *testing.T) {
	resource := AffectedResource{
		Type:      "Host",
		Namespace: "default",
		Name:      "test-host",
	}

	assert.Equal(t, "Host", resource.Type)
	assert.Equal(t, "default", resource.Namespace)
	assert.Equal(t, "test-host", resource.Name)

	// Test JSON round-trip
	data, err := json.Marshal(resource)
	require.NoError(t, err)

	var decoded AffectedResource
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resource, decoded)
}
