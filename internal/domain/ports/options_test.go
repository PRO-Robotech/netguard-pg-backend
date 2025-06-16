package ports

import (
	"testing"

	"netguard-pg-backend/internal/domain/models"
)

func TestWithSyncOp(t *testing.T) {
	tests := []struct {
		name     string
		op       models.SyncOp
		expected models.SyncOp
	}{
		{"NoOp", models.SyncOpNoOp, models.SyncOpNoOp},
		{"FullSync", models.SyncOpFullSync, models.SyncOpFullSync},
		{"Upsert", models.SyncOpUpsert, models.SyncOpUpsert},
		{"Delete", models.SyncOpDelete, models.SyncOpDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			option := WithSyncOp(tt.op)
			syncOption, ok := option.(SyncOption)
			if !ok {
				t.Fatalf("WithSyncOp(%v) did not return a SyncOption", tt.op)
			}
			if syncOption.Operation != tt.expected {
				t.Errorf("WithSyncOp(%v) = %v, want %v", tt.op, syncOption.Operation, tt.expected)
			}
		})
	}
}
