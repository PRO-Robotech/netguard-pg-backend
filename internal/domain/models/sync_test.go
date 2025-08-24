package models

import (
	"testing"
)

func TestProtoToSyncOp(t *testing.T) {
	tests := []struct {
		name     string
		protoOp  int32
		expected SyncOp
	}{
		{"NoOp", 0, SyncOpNoOp},
		{"FullSync", 1, SyncOpFullSync},
		{"Upsert", 2, SyncOpUpsert},
		{"Delete", 3, SyncOpDelete},
		{"Invalid", 4, SyncOpFullSync},   // По умолчанию должен быть FullSync
		{"Negative", -1, SyncOpFullSync}, // По умолчанию должен быть FullSync
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProtoToSyncOp(tt.protoOp)
			if result != tt.expected {
				t.Errorf("ProtoToSyncOp(%d) = %v, want %v", tt.protoOp, result, tt.expected)
			}
		})
	}
}

func TestSyncOpToProto(t *testing.T) {
	tests := []struct {
		name     string
		syncOp   SyncOp
		expected int32
	}{
		{"NoOp", SyncOpNoOp, 0},
		{"FullSync", SyncOpFullSync, 1},
		{"Upsert", SyncOpUpsert, 2},
		{"Delete", SyncOpDelete, 3},
		{"Invalid", SyncOp(4), 1},   // По умолчанию должен быть FullSync (1)
		{"Negative", SyncOp(-1), 1}, // По умолчанию должен быть FullSync (1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SyncOpToProto(tt.syncOp)
			if result != tt.expected {
				t.Errorf("SyncOpToProto(%v) = %d, want %d", tt.syncOp, result, tt.expected)
			}
		})
	}
}

func TestIsValidSyncOp(t *testing.T) {
	tests := []struct {
		name     string
		syncOp   SyncOp
		expected bool
	}{
		{"NoOp", SyncOpNoOp, true},
		{"FullSync", SyncOpFullSync, true},
		{"Upsert", SyncOpUpsert, true},
		{"Delete", SyncOpDelete, true},
		{"Invalid", SyncOp(4), false},
		{"Negative", SyncOp(-1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidSyncOp(tt.syncOp)
			if result != tt.expected {
				t.Errorf("IsValidSyncOp(%v) = %v, want %v", tt.syncOp, result, tt.expected)
			}
		})
	}
}

func TestDefaultSyncOp(t *testing.T) {
	result := DefaultSyncOp()
	if result != SyncOpFullSync {
		t.Errorf("DefaultSyncOp() = %v, want %v", result, SyncOpFullSync)
	}
}

func TestSyncOpString(t *testing.T) {
	tests := []struct {
		name     string
		syncOp   SyncOp
		expected string
	}{
		{"NoOp", SyncOpNoOp, "NoOp"},
		{"FullSync", SyncOpFullSync, "FullSync"},
		{"Upsert", SyncOpUpsert, "Upsert"},
		{"Delete", SyncOpDelete, "Delete"},
		{"Invalid", SyncOp(4), "Unknown"},
		{"Negative", SyncOp(-1), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.syncOp.String()
			if result != tt.expected {
				t.Errorf("SyncOp(%v).String() = %s, want %s", tt.syncOp, result, tt.expected)
			}
		})
	}
}
