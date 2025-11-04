package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestBUG002_RetryStateNotPersisted detects CRITICAL bug: retry state not saved to database
//
// BUG DESCRIPTION:
// Worker processes entries and fails, but does NOT update:
// - attempts (stays 0)
// - last_error (stays NULL)
// - next_retry_at (stays NULL)
//
// CONSEQUENCE:
// - Infinite retries (no max attempts check)
// - No exponential backoff (next_retry_at always NULL)
// - Lost state on worker restart
//
// EXPECTED BEHAVIOR:
// After SGROUP failure, outbox entry should have:
// - attempts = 1 (incremented)
// - last_error = "connection refused" (persisted)
// - next_retry_at = NOW() + backoff (calculated)
//
// See: product-workspace/stories/resilient-sync/bugs/BUG-002-retry-persistence/
func TestBUG002_RetryStateNotPersisted(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	// Clean slate
	TCCleanOutboxTable(t, tc.DB)

	// STEP 1: Create AddressGroup
	t.Log("📝 Creating AddressGroup...")
	agID := TCCreateTestAddressGroup(t, tc.DB, "default", "test-retry-ag")
	require.NotEmpty(t, agID)

	// STEP 2: Manually insert outbox entry (since direct INSERT doesn't trigger spec triggers)
	// NOTE: In production, Repository writers create outbox entries via triggers
	// For this test, we simulate that by manually inserting
	t.Log("📝 Manually creating outbox entry (simulating Repository writer)...")
	var entryID string
	err := tc.DB.QueryRow(`
		INSERT INTO sync_outbox (
			resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system, payload, status, attempts, max_retries
		) VALUES (
			'AddressGroup', gen_random_uuid(), 'default', 'test-retry-ag',
			'CREATE', 'SGROUP', '{}'::jsonb, 'PENDING', 0, 5
		) RETURNING id
	`).Scan(&entryID)
	require.NoError(t, err)
	t.Logf("  ✅ Created outbox entry: %s", entryID)

	// STEP 3: Verify outbox entry exists with initial state
	t.Log("🔍 Verifying initial outbox entry state...")
	entry := TCWaitForOutboxEntry(t, tc.DB, "test-retry-ag", 2*time.Second)
	require.NotNil(t, entry, "outbox entry should exist")

	assert.Equal(t, "AddressGroup", entry.ResourceType)
	assert.Equal(t, "default", entry.ResourceNamespace)
	assert.Equal(t, "test-retry-ag", entry.ResourceName)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)
	assert.Equal(t, 0, entry.Attempts, "Initial attempts should be 0")
	assert.Nil(t, entry.LastError, "Initial last_error should be NULL")

	initialEntryID := entry.ID
	t.Logf("  ✅ Initial entry ID: %s, attempts=%d", initialEntryID, entry.Attempts)

	// STEP 3: NOTE about Worker requirement
	t.Log("ℹ️  This test requires OutboxWorker to be running")
	t.Log("ℹ️  In production: Worker processes entries and updates retry state")
	t.Log("ℹ️  BUG-002: Worker does NOT persist retry state to database")
	t.Log("")
	t.Log("🔬 TEST STRATEGY:")
	t.Log("   1. Create outbox entry (DONE)")
	t.Log("   2. Deploy code WITH OutboxWorker")
	t.Log("   3. Simulate SGROUP failure")
	t.Log("   4. Verify retry state persists")
	t.Log("")
	t.Log("📋 MANUAL VALIDATION STEPS:")
	t.Log("   1. Deploy backend with OutboxWorker enabled")
	t.Log("   2. Stop SGROUP service")
	t.Log("   3. Create AddressGroup in K8s")
	t.Log("   4. Query: SELECT attempts, last_error, next_retry_at FROM sync_outbox")
	t.Log("   5. If attempts=0, last_error=NULL → BUG-002 EXISTS")
	t.Log("")
	t.Skip("⏭️  Skipping automated test - requires running OutboxWorker")
}

// TestBUG002_RetryAfterWorkerRestart detects retry state loss on worker restart
//
// This test verifies that retry state persists across worker restarts.
// If BUG-002 exists, worker will restart from attempts=0 (losing history).
func TestBUG002_RetryAfterWorkerRestart(t *testing.T) {
	t.Skip("TODO: Requires worker lifecycle management - implement after BUG-002 fixed")

	// SCENARIO:
	// 1. Worker fails to process entry (attempts=1, next_retry_at set)
	// 2. Worker restarts (simulated)
	// 3. Worker should NOT reset attempts to 0
	// 4. Worker should respect next_retry_at (not retry immediately)
	//
	// EXPECTED: After restart, attempts preserved, respects backoff
	// BUG-002: After restart, attempts=0, retries immediately
}

// TestBUG002_ExponentialBackoffWorks verifies exponential backoff calculation
//
// This test checks if retry intervals increase exponentially:
// - Attempt 1: retry after 10s
// - Attempt 2: retry after 30s (3x)
// - Attempt 3: retry after 90s (3x)
func TestBUG002_ExponentialBackoffWorks(t *testing.T) {
	t.Skip("TODO: Requires time manipulation - implement after BUG-002 fixed")

	// SCENARIO:
	// 1. Create outbox entry
	// 2. Force 3 consecutive failures
	// 3. Check next_retry_at progression:
	//    - After attempt 1: next_retry ≈ now + 10s
	//    - After attempt 2: next_retry ≈ now + 30s
	//    - After attempt 3: next_retry ≈ now + 90s
	//
	// EXPECTED: Exponential backoff 10s → 30s → 90s
	// BUG-002: next_retry_at stays NULL, no backoff
}

// Note: TCWaitForOutboxEntry is defined in helpers_testcontainer.go
