package types

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostSyncRequest(t *testing.T) {
	hostUUIDs := []string{"uuid1", "uuid2", "uuid3"}
	namespace := "test-namespace"

	req := NewHostSyncRequest(namespace, hostUUIDs)

	assert.Equal(t, namespace, req.Namespace)
	assert.Equal(t, hostUUIDs, req.HostUUIDs)
	assert.False(t, req.ForceSync)
	assert.Equal(t, 50, req.BatchSize) // Default batch size
}

func TestHostSyncRequestSerialization(t *testing.T) {
	req := &HostSyncRequest{
		HostUUIDs: []string{"uuid1", "uuid2"},
		Namespace: "test",
		ForceSync: true,
		BatchSize: 25,
	}

	// Test JSON marshaling
	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var unmarshaled HostSyncRequest
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, req.HostUUIDs, unmarshaled.HostUUIDs)
	assert.Equal(t, req.Namespace, unmarshaled.Namespace)
	assert.Equal(t, req.ForceSync, unmarshaled.ForceSync)
	assert.Equal(t, req.BatchSize, unmarshaled.BatchSize)
}

func TestNewHostSyncResult(t *testing.T) {
	result := NewHostSyncResult()

	assert.NotNil(t, result.SyncedHostUUIDs)
	assert.NotNil(t, result.FailedUUIDs)
	assert.NotNil(t, result.Errors)
	assert.NotNil(t, result.Details)
	assert.Len(t, result.SyncedHostUUIDs, 0)
	assert.Len(t, result.FailedUUIDs, 0)
	assert.Len(t, result.Errors, 0)
	assert.Equal(t, 0, result.TotalRequested)
	assert.Equal(t, 0, result.TotalSynced)
	assert.Equal(t, 0, result.TotalFailed)
}

func TestHostSyncResultAddSyncedHost(t *testing.T) {
	result := NewHostSyncResult()

	result.AddSyncedHost("uuid1")
	result.AddSyncedHost("uuid2")

	assert.Equal(t, 2, result.TotalSynced)
	assert.Len(t, result.SyncedHostUUIDs, 2)
	assert.Contains(t, result.SyncedHostUUIDs, "uuid1")
	assert.Contains(t, result.SyncedHostUUIDs, "uuid2")
}

func TestHostSyncResultAddFailedHost(t *testing.T) {
	result := NewHostSyncResult()

	result.AddFailedHost("uuid1", "error 1")
	result.AddFailedHost("uuid2", "error 2")
	result.AddFailedHost("uuid3", "") // No error message but still failed

	assert.Equal(t, 3, result.TotalFailed)
	assert.Len(t, result.FailedUUIDs, 3)
	assert.Contains(t, result.FailedUUIDs, "uuid1")
	assert.Contains(t, result.FailedUUIDs, "uuid2")
	assert.Contains(t, result.FailedUUIDs, "uuid3")

	assert.Equal(t, "error 1", result.GetError("uuid1"))
	assert.Equal(t, "error 2", result.GetError("uuid2"))
	assert.Equal(t, "", result.GetError("uuid3"))
	assert.Equal(t, "", result.GetError("non-existent"))
}

func TestHostSyncResultHasErrors(t *testing.T) {
	result := NewHostSyncResult()

	// Initially no errors
	assert.False(t, result.HasErrors())

	// Add failed host
	result.AddFailedHost("uuid1", "test error")
	assert.True(t, result.HasErrors())
}

func TestHostSyncResultSetTotalRequested(t *testing.T) {
	result := NewHostSyncResult()

	result.SetTotalRequested(10)
	assert.Equal(t, 10, result.TotalRequested)
}

func TestHostSyncResultDetails(t *testing.T) {
	result := NewHostSyncResult()

	// Set details
	result.SetDetail("source", "sgroup")
	result.SetDetail("batch_count", 3)
	result.SetDetail("duration_ms", 150)

	assert.Equal(t, "sgroup", result.GetDetail("source"))
	assert.Equal(t, 3, result.GetDetail("batch_count"))
	assert.Equal(t, 150, result.GetDetail("duration_ms"))
	assert.Nil(t, result.GetDetail("non-existent"))

	// Test nil details
	emptyResult := &HostSyncResult{}
	assert.Nil(t, emptyResult.GetDetail("test"))

	emptyResult.SetDetail("test", "value")
	assert.Equal(t, "value", emptyResult.GetDetail("test"))
}

func TestHostSyncResultIsEmpty(t *testing.T) {
	result := NewHostSyncResult()

	// Initially empty
	assert.True(t, result.IsEmpty())

	// Add synced host
	result.AddSyncedHost("uuid1")
	assert.False(t, result.IsEmpty())

	// Create another result with only failed hosts
	result2 := NewHostSyncResult()
	result2.AddFailedHost("uuid1", "error")
	assert.False(t, result2.IsEmpty())
}

func TestHostSyncResultSuccessRate(t *testing.T) {
	result := NewHostSyncResult()

	// No requests - should be 100%
	assert.Equal(t, 100.0, result.SuccessRate())

	// Set total requested
	result.SetTotalRequested(10)

	// No synced yet - should be 0%
	assert.Equal(t, 0.0, result.SuccessRate())

	// Add some synced hosts
	result.AddSyncedHost("uuid1")
	result.AddSyncedHost("uuid2")
	result.AddSyncedHost("uuid3") // 3 synced out of 10
	assert.Equal(t, 30.0, result.SuccessRate())

	// Add some failed hosts
	result.AddFailedHost("uuid4", "error")
	result.AddFailedHost("uuid5", "error")      // 3 synced, 2 failed out of 10
	assert.Equal(t, 30.0, result.SuccessRate()) // Still 30% because success rate is synced/requested

	// 100% success
	result2 := NewHostSyncResult()
	result2.SetTotalRequested(5)
	for i := 0; i < 5; i++ {
		result2.AddSyncedHost(fmt.Sprintf("uuid%d", i))
	}
	assert.Equal(t, 100.0, result2.SuccessRate())
}

func TestHostIPSetUpdate(t *testing.T) {
	update := HostIPSetUpdate{
		HostUUID:  "uuid123",
		HostID:    "host-id-123",
		Namespace: "default",
		Name:      "test-host",
		IPSet:     []string{"192.168.1.10", "10.0.0.5"},
		SGName:    "test-sg",
	}

	// Test JSON serialization
	data, err := json.Marshal(update)
	require.NoError(t, err)

	var unmarshaled HostIPSetUpdate
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, update.HostUUID, unmarshaled.HostUUID)
	assert.Equal(t, update.HostID, unmarshaled.HostID)
	assert.Equal(t, update.Namespace, unmarshaled.Namespace)
	assert.Equal(t, update.Name, unmarshaled.Name)
	assert.Equal(t, update.IPSet, unmarshaled.IPSet)
	assert.Equal(t, update.SGName, unmarshaled.SGName)
}

func TestHostSyncResultComplexScenario(t *testing.T) {
	result := NewHostSyncResult()
	result.SetTotalRequested(100)

	// Add successful syncs
	for i := 0; i < 80; i++ {
		result.AddSyncedHost(fmt.Sprintf("uuid-%d", i))
	}

	// Add failed syncs
	for i := 80; i < 95; i++ {
		result.AddFailedHost(fmt.Sprintf("uuid-%d", i), "sync error")
	}

	// Test final state
	assert.Equal(t, 100, result.TotalRequested)
	assert.Equal(t, 80, result.TotalSynced)
	assert.Equal(t, 15, result.TotalFailed)
	assert.Equal(t, 80.0, result.SuccessRate())
	assert.True(t, result.HasErrors())
	assert.False(t, result.IsEmpty())

	// Test specific errors
	assert.Equal(t, "sync error", result.GetError("uuid-85"))
	assert.Equal(t, "", result.GetError("uuid-5")) // Successful sync, no error
}
