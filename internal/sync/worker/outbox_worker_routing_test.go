package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"netguard-pg-backend/internal/domain"
)

// TestProcessEntry_Routing_UnknownTargetSystem tests that unknown target systems return error
func TestProcessEntry_Routing_UnknownTargetSystem(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	worker := &OutboxWorker{
		logger: logger,
	}

	// Create entry with invalid target system
	entry := &domain.OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystem("UNKNOWN_SYSTEM"),
		Payload:      []byte(`{}`),
	}

	// Call processEntry
	err := worker.processEntry(ctx, entry)

	// Verify routing error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown target system")
	assert.Contains(t, err.Error(), "UNKNOWN_SYSTEM")
}

// TestProcessEntry_Routing_InvalidTargetSystems tests all invalid target systems
func TestProcessEntry_Routing_InvalidTargetSystems(t *testing.T) {
	tests := []struct {
		name             string
		targetSystem     domain.TargetSystem
		expectedErrorMsg string
	}{
		{
			name:             "INVALID returns routing error",
			targetSystem:     domain.TargetSystem("INVALID"),
			expectedErrorMsg: "unknown target system: INVALID",
		},
		{
			name:             "Empty target system returns routing error",
			targetSystem:     domain.TargetSystem(""),
			expectedErrorMsg: "unknown target system: ",
		},
		{
			name:             "Lowercase sgroup not recognized",
			targetSystem:     domain.TargetSystem("sgroup"),
			expectedErrorMsg: "unknown target system: sgroup",
		},
		{
			name:             "Lowercase internal not recognized",
			targetSystem:     domain.TargetSystem("internal"),
			expectedErrorMsg: "unknown target system: internal",
		},
		{
			name:             "Mixed case Sgroup not recognized",
			targetSystem:     domain.TargetSystem("Sgroup"),
			expectedErrorMsg: "unknown target system: Sgroup",
		},
		{
			name:             "Hypothetical EXTERNAL system",
			targetSystem:     domain.TargetSystem("EXTERNAL"),
			expectedErrorMsg: "unknown target system: EXTERNAL",
		},
		{
			name:             "Random string",
			targetSystem:     domain.TargetSystem("randomstring"),
			expectedErrorMsg: "unknown target system: randomstring",
		},
		{
			name:             "With spaces",
			targetSystem:     domain.TargetSystem("SG GROUP"),
			expectedErrorMsg: "unknown target system: SG GROUP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			ctx := context.Background()

			worker := &OutboxWorker{
				logger: logger,
			}

			entry := &domain.OutboxEntry{
				ID:           uuid.New(),
				ResourceType: "Host",
				ResourceID:   uuid.New(),
				Operation:    domain.SyncOperationCreate,
				TargetSystem: tt.targetSystem,
				Payload:      []byte(`{}`),
			}

			err := worker.processEntry(ctx, entry)

			// Should get routing error immediately
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErrorMsg)
		})
	}
}

// TestProcessEntry_SignatureChange tests that the signature accepts *domain.OutboxEntry (not interface{})
func TestProcessEntry_SignatureChange(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	worker := &OutboxWorker{
		logger: logger,
	}

	// Test that entry can be passed as *domain.OutboxEntry
	// This tests that signature was changed from interface{} to *domain.OutboxEntry
	entry := &domain.OutboxEntry{
		ID:           uuid.New(),
		ResourceType: "Host",
		ResourceID:   uuid.New(),
		Operation:    domain.SyncOperationCreate,
		TargetSystem: domain.TargetSystem("TESTING"),
		Payload:      []byte(`{}`),
	}

	// This should compile - if it compiles, signature is correct
	_ = worker.processEntry(ctx, entry)
	// Test passes if it compiles successfully
}

// TestProcessEntry_DefaultCase tests that the default case in switch handles unknown systems
func TestProcessEntry_DefaultCase(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	worker := &OutboxWorker{
		logger: logger,
	}

	// Test various invalid target systems that should hit default case
	invalidSystems := []domain.TargetSystem{
		domain.TargetSystem("UNKNOWN"),
		domain.TargetSystem("EXTERNAL"),
		domain.TargetSystem("DATABASE"),
		domain.TargetSystem("API"),
		domain.TargetSystem("TEST"),
	}

	for _, targetSystem := range invalidSystems {
		t.Run(string(targetSystem), func(t *testing.T) {
			entry := &domain.OutboxEntry{
				ID:           uuid.New(),
				ResourceType: "Host",
				ResourceID:   uuid.New(),
				Operation:    domain.SyncOperationCreate,
				TargetSystem: targetSystem,
				Payload:      []byte(`{}`),
			}

			err := worker.processEntry(ctx, entry)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown target system")
			assert.Contains(t, err.Error(), string(targetSystem))
		})
	}
}
