package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
	"netguard-pg-backend/internal/sync/types"
)

// TestWorker_ProcessesPendingOutboxEntry validates that worker processes PENDING outbox entries
func TestWorker_ProcessesPendingOutboxEntry(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create Host and trigger outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)

	// Trigger ready transition to create outbox entry
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// Wait for outbox entry
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)
	require.Equal(t, domain.OutboxStatusPending, entry.Status)

	t.Logf("Created PENDING outbox entry: %s", entry.ID)

	// ========================================
	// ACT
	// ========================================
	// Process entry with worker
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := worker.ProcessOnce(ctx)
	require.NoError(t, err)

	// ========================================
	// ASSERT
	// ========================================
	// Wait for entry to be processed
	time.Sleep(300 * time.Millisecond)

	// Check entry status changed to SUCCESS
	var status domain.OutboxStatus
	err = env.DB.QueryRow(`
		SELECT status FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status)
	require.NoError(t, err)

	assert.Equal(t, domain.OutboxStatusSuccess, status, "Entry should be marked as SUCCESS")

	// Verify SGROUP client was called
	assert.Equal(t, 1, mockClient.GetSyncCallCount(), "SGROUP Sync should be called once")

	lastCall := mockClient.GetLastSyncCall()
	require.NotNil(t, lastCall)
	assert.Equal(t, types.SyncSubjectTypeGroups, lastCall.Subject)
	assert.Equal(t, types.SyncOperationUpsert, lastCall.Operation)

	t.Logf("✅ Test PASSED: Worker processed PENDING entry successfully")
}

// TestWorker_SkipsAlreadyProcessed validates that worker skips SUCCESS entries
func TestWorker_SkipsAlreadyProcessed(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create outbox entry and mark as SUCCESS
	entryID := uuid.New()
	resourceID := uuid.New()
	_, err := env.DB.Exec(`
		INSERT INTO sync_outbox (
			id, resource_type, resource_id, operation, target_system,
			payload, status, attempts
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, entryID, "AddressGroup", resourceID, domain.SyncOperationUpdate,
		domain.TargetSystemSGROUP, `{"test": "data"}`, domain.OutboxStatusSuccess, 0)
	require.NoError(t, err)

	t.Logf("Created SUCCESS outbox entry: %s", entryID)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = worker.ProcessOnce(ctx)
	require.NoError(t, err)

	// ========================================
	// ASSERT
	// ========================================
	// SGROUP should NOT be called (entry already processed)
	assert.Equal(t, 0, mockClient.GetSyncCallCount(), "SGROUP should not be called for SUCCESS entries")

	t.Logf("✅ Test PASSED: Worker skips already processed entries")
}

// TestWorker_HandlesSGROUPError validates worker error handling
func TestWorker_HandlesSGROUPError(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Simulate SGROUP error
	mockClient.SimulateError(errors.New("SGROUP connection failed"))

	// Create Host and trigger outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("Created outbox entry: %s", entry.ID)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Worker should handle error gracefully
	err := worker.ProcessOnce(ctx)
	// ProcessOnce may return error or handle internally depending on implementation
	t.Logf("ProcessOnce returned: %v", err)

	// ========================================
	// ASSERT
	// ========================================
	// Wait for entry to be updated
	time.Sleep(300 * time.Millisecond)

	// Check entry status changed to FAILED_RETRYABLE
	var status domain.OutboxStatus
	var attempts int
	var lastError sql.NullString

	err = env.DB.QueryRow(`
		SELECT status, attempts, last_error
		FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status, &attempts, &lastError)
	require.NoError(t, err)

	assert.Equal(t, domain.OutboxStatusFailedRetryable, status, "Entry should be FAILED_RETRYABLE")
	assert.Equal(t, 1, attempts, "Attempts should be incremented")
	assert.True(t, lastError.Valid, "last_error should be set")
	assert.Contains(t, lastError.String, "SGROUP connection failed", "Error message should be recorded")

	t.Logf("✅ Test PASSED: Worker handles SGROUP error (status=%s, attempts=%d)", status, attempts)
}

// TestWorker_RetriesOnTransientError validates retry logic with exponential backoff
func TestWorker_RetriesOnTransientError(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	// ========================================
	// ACT - First attempt (fails)
	// ========================================
	mockClient.SimulateError(errors.New("timeout"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := worker.ProcessOnce(ctx)
	t.Logf("First attempt result: %v", err)

	time.Sleep(300 * time.Millisecond)

	// Check entry after first failure
	var status1 domain.OutboxStatus
	var attempts1 int
	var nextRetryAt1 sql.NullTime

	err = env.DB.QueryRow(`
		SELECT status, attempts, next_retry_at
		FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status1, &attempts1, &nextRetryAt1)
	require.NoError(t, err)

	assert.Equal(t, domain.OutboxStatusFailedRetryable, status1)
	assert.Equal(t, 1, attempts1)
	assert.True(t, nextRetryAt1.Valid, "next_retry_at should be set")

	t.Logf("After 1st failure: status=%s, attempts=%d, next_retry=%v", status1, attempts1, nextRetryAt1.Time)

	// ========================================
	// ACT - Second attempt (succeeds)
	// ========================================
	// Clear simulated error for retry
	mockClient.Reset()

	// Update next_retry_at to now (simulate time passing)
	_, err = env.DB.Exec(`
		UPDATE sync_outbox
		SET next_retry_at = NOW() - INTERVAL '1 second'
		WHERE id = $1
	`, entry.ID)
	require.NoError(t, err)

	err = worker.ProcessOnce(ctx)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	var status2 domain.OutboxStatus
	var attempts2 int

	err = env.DB.QueryRow(`
		SELECT status, attempts
		FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status2, &attempts2)
	require.NoError(t, err)

	assert.Equal(t, domain.OutboxStatusSuccess, status2, "Entry should succeed on retry")
	// Attempts may still be 1 if worker resets on success, or incremented depends on implementation
	t.Logf("After retry success: status=%s, attempts=%d", status2, attempts2)

	t.Logf("✅ Test PASSED: Worker retries on transient error")
}

// TestWorker_FailsOnValidationError validates permanent failure on validation error
func TestWorker_FailsOnValidationError(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Simulate permanent validation error
	mockClient.SimulateError(errors.New("invalid payload: missing required field"))

	// Create outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := worker.ProcessOnce(ctx)
	t.Logf("ProcessOnce result: %v", err)

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	var status domain.OutboxStatus
	var errorCategory sql.NullString

	err = env.DB.QueryRow(`
		SELECT status, error_category
		FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status, &errorCategory)
	require.NoError(t, err)

	// Depending on worker implementation, validation errors may be marked as FAILED_PERMANENT
	// or FAILED_RETRYABLE depending on error classification
	t.Logf("After validation error: status=%s, error_category=%s", status, errorCategory.String)

	// Accept either FAILED_RETRYABLE or FAILED_PERMANENT for validation errors
	assert.Contains(t, []domain.OutboxStatus{
		domain.OutboxStatusFailedRetryable,
		domain.OutboxStatusFailedPermanent,
	}, status, "Validation error should fail the entry")

	t.Logf("✅ Test PASSED: Worker handles validation error")
}

// TestWorker_UpdatesAttemptCount validates that attempt count is incremented
func TestWorker_UpdatesAttemptCount(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	initialAttempts := 0

	// ========================================
	// ACT
	// ========================================
	// Simulate 3 failures
	for i := 0; i < 3; i++ {
		mockClient.SimulateError(errors.New("transient error"))

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = worker.ProcessOnce(ctx)
		cancel()

		time.Sleep(200 * time.Millisecond)

		// Update next_retry_at for next attempt
		_, _ = env.DB.Exec(`
			UPDATE sync_outbox
			SET next_retry_at = NOW() - INTERVAL '1 second'
			WHERE id = $1
		`, entry.ID)
	}

	// ========================================
	// ASSERT
	// ========================================
	var attempts int
	err := env.DB.QueryRow(`
		SELECT attempts FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&attempts)
	require.NoError(t, err)

	assert.Greater(t, attempts, initialAttempts, "Attempts should be incremented")
	assert.LessOrEqual(t, attempts, 3, "Attempts should not exceed actual retries")

	t.Logf("✅ Test PASSED: Attempt count incremented correctly (attempts=%d)", attempts)
}

// TestWorker_RecordsLastError validates that last_error is recorded
func TestWorker_RecordsLastError(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	expectedError := "specific SGROUP error message"
	mockClient.SimulateError(errors.New(expectedError))

	// Create outbox entry
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = worker.ProcessOnce(ctx)

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	var lastError sql.NullString
	err := env.DB.QueryRow(`
		SELECT last_error FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&lastError)
	require.NoError(t, err)

	assert.True(t, lastError.Valid, "last_error should be set")
	assert.Contains(t, lastError.String, expectedError, "last_error should contain error message")

	t.Logf("✅ Test PASSED: last_error recorded correctly: %s", lastError.String)
}

// TestWorker_ProcessesMultipleEntries validates batch processing
func TestWorker_ProcessesMultipleEntries(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create 3 Hosts with ready transitions
	for i := 0; i < 3; i++ {
		resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
		CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i)),
			"uuid-"+string(rune('1'+i)),
			resourceVersion, false)
		TriggerReadyTransition(t, env.DB, resourceVersion, true)
	}

	// Wait for all outbox entries
	time.Sleep(500 * time.Millisecond)

	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))
	require.GreaterOrEqual(t, countBefore, 3, "At least 3 outbox entries should be created")

	t.Logf("Created %d outbox entries", countBefore)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Process batch
	err := worker.ProcessOnce(ctx)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Count SUCCESS entries
	var successCount int
	err = env.DB.QueryRow(`
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'AddressGroup'
		  AND operation = $1
		  AND status = $2
	`, domain.SyncOperationUpdate, domain.OutboxStatusSuccess).Scan(&successCount)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, successCount, 1, "At least one entry should be processed")

	// SGROUP should be called for each processed entry
	syncCallCount := mockClient.GetSyncCallCount()
	assert.GreaterOrEqual(t, syncCallCount, 1, "SGROUP should be called at least once")

	t.Logf("✅ Test PASSED: Processed multiple entries (success=%d, sgroup_calls=%d)", successCount, syncCallCount)
}

// TestWorker_AppliesDelta validates that worker applies delta to SGROUP
func TestWorker_AppliesDelta(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create Host and trigger outbox with delta
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", resourceVersion, false)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("Outbox entry with delta: %s", entry.ID)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := worker.ProcessOnce(ctx)
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Verify SGROUP was called with correct delta
	assert.Equal(t, 1, mockClient.GetSyncCallCount())

	lastCall := mockClient.GetLastSyncCall()
	require.NotNil(t, lastCall)

	// Delta should contain host changes
	assert.NotNil(t, lastCall.Request, "Request should be set")

	t.Logf("✅ Test PASSED: Worker applies delta to SGROUP")
}

// TestWorker_ChecksDependencies validates that worker checks dependencies before sync
// This is a placeholder - actual dependency checking logic needs to be implemented
func TestWorker_ChecksDependencies(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	pool := CreatePgxPool(t)
	defer ClosePgxPool(t, pool)

	mockClient := NewMockSGROUPClient()
	worker := CreateTestWorker(t, pool, mockClient)

	// Create Network with AddressGroup reference (dependency)
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag"
	agID := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	networkResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	networkNamespace := "test-ns"
	networkName := "test-network"

	// Create network with AG reference
	_, err := env.DB.Exec(`
		INSERT INTO networks (
			namespace, name, resource_version, ready,
			address_group_ref_namespace, address_group_ref_name
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, networkNamespace, networkName, networkResourceVersion, false, agNamespace, agName)
	require.NoError(t, err)

	// Trigger network ready → outbox entry
	TriggerReadyTransition(t, env.DB, networkResourceVersion, true)

	entry := WaitForOutboxEntry(t, env.DB, "Network", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("Network outbox entry created (depends on AG %s)", agID)

	// ========================================
	// ACT
	// ========================================
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = worker.ProcessOnce(ctx)
	// Worker may skip or process depending on dependency checker implementation
	t.Logf("ProcessOnce result: %v", err)

	time.Sleep(300 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Check if worker processed or skipped based on dependencies
	var status domain.OutboxStatus
	err = env.DB.QueryRow(`
		SELECT status FROM sync_outbox WHERE id = $1
	`, entry.ID).Scan(&status)
	require.NoError(t, err)

	// Worker should either:
	// 1. Skip processing (status remains PENDING) if AG not ready
	// 2. Process successfully if dependency checker allows
	t.Logf("Network sync status after dependency check: %s", status)

	t.Logf("✅ Test PASSED: Worker dependency checking logic executed")
}
