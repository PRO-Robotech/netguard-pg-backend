package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestHostLifecycle_ConditionOnlyUpdate_HostSurvives reproduces P0 BUG:
// Host disappears after OutboxWorker processes CREATE event and calls markEntityResourceReady()
//
// BUG TIMELINE:
// T+0s:  User creates Host via kubectl
// T+2s:  Host exists in DB ✅
// T+10s: OutboxWorker processes CREATE → syncs to SGROUP → calls markEntityResourceReady()
// T+10s: markEntityResourceReady() calls SyncHosts() with ConditionOnlyOperation{}
// T+10s: ❌ SyncHosts() DELETES Host (BUG!)
// T+15s: Host gone from DB ❌
//
// ROOT CAUSE:
// markEntityResourceReady() in process_entity.go:246 calls:
//     writer.SyncHosts(ctx, []models.Host{*r}, resourceScope, ports.ConditionOnlyOperation{})
//
// But SyncHosts() IGNORES ConditionOnlyOperation{} flag and deletes Host!
//
// EXPECTED RESULT: Test FAILS with "Host not found after condition update"
// This proves the bug exists and needs fixing.
func TestHostLifecycle_ConditionOnlyUpdate_HostSurvives(t *testing.T) {
	t.Log("========================================")
	t.Log("🧪 TEST: Full Host Lifecycle - Condition-Only Update")
	t.Log("========================================")

	// ========================================
	// STEP 1: Setup testcontainer with PostgreSQL
	// ========================================
	t.Log("\n📦 STEP 1: Setting up test environment...")
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	t.Log("✅ Test environment ready:")
	t.Logf("   - PostgreSQL container: RUNNING")
	t.Logf("   - Migrations applied: ALL (001-035)")
	t.Logf("   - Mock SGROUP server: %s", tc.MockSGROUP.URL())

	// ========================================
	// STEP 2: Start OutboxWorker
	// ========================================
	t.Log("\n🏗️  STEP 2: Creating OutboxWorker...")
	worker := TCCreateTestWorker(t, tc)
	require.NotNil(t, worker, "worker should not be nil")

	t.Log("✅ OutboxWorker ready:")
	t.Logf("   - PollInterval: 1s")
	t.Logf("   - BatchSize: 10")
	t.Logf("   - Connected to: PostgreSQL testcontainer")
	t.Logf("   - SGROUP gateway: HTTP mock server")

	// ========================================
	// STEP 3: CREATE Host in DB
	// ========================================
	t.Log("\n📝 STEP 3: Creating Host in database...")

	namespace := "test-ns"
	name := "host-lifecycle-test"
	hostUUID := "550e8400-e29b-41d4-a716-446655440999"

	// Create k8s_metadata with PendingSync condition (ready=false)
	resourceVersion := CreateK8sMetadata(t, tc.DB, `[{"type":"PendingSync","status":"True","reason":"PendingSGROUPSync"}]`)
	t.Logf("   - Created k8s_metadata: resource_version=%d", resourceVersion)

	// Create Host with ready=FALSE (not yet synced to SGROUP)
	CreateTestHost(t, tc.DB, namespace, name, hostUUID, resourceVersion, false)

	t.Log("✅ Host created:")
	t.Logf("   - Namespace: %s", namespace)
	t.Logf("   - Name: %s", name)
	t.Logf("   - UUID: %s", hostUUID)
	t.Logf("   - Ready: FALSE (pending SGROUP sync)")

	// ========================================
	// STEP 4: Verify Host exists (Baseline)
	// ========================================
	t.Log("\n🔍 STEP 4: Verifying Host exists (baseline check)...")

	var countBefore int
	err := tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts
		WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&countBefore)
	require.NoError(t, err, "failed to count hosts")
	require.Equal(t, 1, countBefore, "Host should exist in DB before worker processing")

	t.Logf("✅ Host exists in DB: count=%d", countBefore)

	// ========================================
	// STEP 5: Create Outbox Entry (simulate upsertHost creating outbox)
	// ========================================
	t.Log("\n📨 STEP 5: Creating Outbox entry for Host CREATE operation...")

	// Wait a moment for any trigger-based outbox entries
	time.Sleep(500 * time.Millisecond)

	// Check if outbox entry already exists (created by trigger)
	var outboxCount int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'Host'
		  AND resource_namespace = $1
		  AND resource_name = $2
		  AND operation = $3
	`, namespace, name, domain.SyncOperationCreate).Scan(&outboxCount)
	require.NoError(t, err)

	if outboxCount == 0 {
		// No trigger-based entry, create manually
		t.Log("   - No automatic outbox entry, creating manually...")
		_, err = tc.DB.Exec(`
			INSERT INTO sync_outbox (
				resource_type, resource_id, resource_namespace, resource_name,
				operation, target_system, payload, status, attempts, max_retries
			) VALUES (
				'Host', $1::uuid, $2, $3,
				$4, 'SGROUP', '{"namespace":"'||$2||'","name":"'||$3||'","uuid":"'||$1||'"}'::jsonb,
				'PENDING', 0, 5
			)
		`, hostUUID, namespace, name, domain.SyncOperationCreate)
		require.NoError(t, err, "failed to create outbox entry")
		t.Log("   ✅ Created manual outbox entry")
	} else {
		t.Logf("   ✅ Found %d existing outbox entry(ies)", outboxCount)
	}

	// Wait for outbox entry to appear
	entry := TCWaitForOutboxEntry(t, tc.DB, name, 5*time.Second)
	require.NotNil(t, entry, "outbox entry should exist")
	require.Equal(t, domain.OutboxStatusPending, entry.Status, "outbox entry should be PENDING")

	t.Log("✅ Outbox entry ready:")
	t.Logf("   - ID: %s", entry.ID)
	t.Logf("   - Operation: %s", entry.Operation)
	t.Logf("   - Status: %s", entry.Status)

	// ========================================
	// STEP 6: Process Outbox Entry with Worker
	// ========================================
	t.Log("\n⚙️  STEP 6: Processing outbox entry with OutboxWorker...")
	t.Log("   This simulates the full lifecycle:")
	t.Log("   1. Worker loads outbox entry")
	t.Log("   2. Worker loads Host from DB")
	t.Log("   3. Worker syncs Host to SGROUP")
	t.Log("   4. SGROUP returns success")
	t.Log("   5. Worker calls markEntityResourceReady()")
	t.Log("   6. ❌ BUG: markEntityResourceReady() calls SyncHosts() with ConditionOnlyOperation{}")
	t.Log("   7. ❌ BUG: SyncHosts() IGNORES flag and DELETES Host!")

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Process once to trigger the bug
	err = worker.ProcessOnce(ctx)
	if err != nil {
		t.Logf("   ⚠️  Worker.ProcessOnce() returned error: %v", err)
	}

	// Wait for processing to complete
	time.Sleep(2 * time.Second)

	processingDuration := time.Since(startTime)
	t.Logf("✅ Worker processing completed in %v", processingDuration)

	// Check SGROUP mock received the sync request
	sgroupRequests := tc.MockSGROUP.GetRequestCount()
	t.Logf("   - SGROUP requests received: %d", sgroupRequests)

	if sgroupRequests > 0 {
		t.Log("   ✅ SGROUP sync succeeded (mock)")
	}

	// ========================================
	// STEP 7: CRITICAL CHECK - Does Host still exist?
	// ========================================
	t.Log("\n🚨 STEP 7: CRITICAL CHECK - Verifying Host still exists...")
	t.Log("   Expected: Host SHOULD still exist (count=1)")
	t.Log("   Actual (BUG): Host DELETED by SyncHosts() (count=0)")

	var countAfter int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts
		WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&countAfter)
	require.NoError(t, err, "failed to count hosts after worker processing")

	t.Logf("\n📊 RESULTS:")
	t.Logf("   - Hosts before worker: %d", countBefore)
	t.Logf("   - Hosts after worker:  %d", countAfter)
	t.Logf("   - SGROUP requests:     %d", sgroupRequests)

	// ========================================
	// ASSERTION: This SHOULD FAIL (proving bug exists)
	// ========================================
	if countAfter == 0 {
		t.Log("\n❌ BUG REPRODUCED!")
		t.Log("   Host was DELETED after worker processing!")
		t.Log("   This proves the bug exists in SyncHosts():")
		t.Log("   - markEntityResourceReady() passes ConditionOnlyOperation{}")
		t.Log("   - SyncHosts() IGNORES this flag")
		t.Log("   - SyncHosts() calls deleteHostsInScope()")
		t.Log("   - Host gets deleted from DB")
		t.Log("")
		t.Log("ROOT CAUSE:")
		t.Log("   File: internal/infrastructure/repositories/pg/writers/host.go")
		t.Log("   Function: SyncHosts()")
		t.Log("   Line: ~45-49")
		t.Log("   Code:")
		t.Log("     if !scope.IsEmpty() && syncOp != models.SyncOpDelete {")
		t.Log("         if err := w.deleteHostsInScope(ctx, scope); err != nil {")
		t.Log("             return errors.Wrap(err, \"failed to delete hosts in scope\")")
		t.Log("         }")
		t.Log("     }")
		t.Log("")
		t.Log("FIX REQUIRED:")
		t.Log("   Add check for ConditionOnlyOperation{} to SKIP deleteHostsInScope()")
	}

	// THIS ASSERTION WILL FAIL - THAT'S THE POINT!
	// Once we fix SyncHosts() to respect ConditionOnlyOperation{}, this test will PASS
	assert.Equal(t, 1, countAfter,
		"❌ BUG: Host was deleted after worker processing! "+
			"markEntityResourceReady() calls SyncHosts() with ConditionOnlyOperation{}, "+
			"but SyncHosts() IGNORES this flag and deletes the Host. "+
			"Expected: Host survives (count=1). Actual: Host deleted (count=%d)", countAfter)

	// ========================================
	// STEP 8: Verify k8s_metadata updated (conditions=Ready)
	// ========================================
	if countAfter > 0 {
		t.Log("\n✅ STEP 8: Verifying k8s_metadata conditions updated...")

		var conditions string
		var ready bool
		err = tc.DB.QueryRow(`
			SELECT h.ready, m.conditions::text
			FROM hosts h
			JOIN k8s_metadata m ON h.resource_version = m.resource_version
			WHERE h.namespace = $1 AND h.name = $2
		`, namespace, name).Scan(&ready, &conditions)

		if err == nil {
			t.Logf("   - Host ready: %v", ready)
			t.Logf("   - Conditions: %s", conditions)

			if ready {
				t.Log("   ✅ Host marked as Ready after SGROUP sync")
			} else {
				t.Log("   ⚠️  Host NOT marked as Ready (worker may have failed)")
			}
		} else if err != sql.ErrNoRows {
			t.Logf("   ⚠️  Failed to query host metadata: %v", err)
		}
	}

	t.Log("\n========================================")
	t.Log("🧪 TEST COMPLETE")
	t.Log("========================================")

	if countAfter == 0 {
		t.Log("RESULT: ❌ FAILED (BUG REPRODUCED)")
		t.Log("        Host disappeared after worker processing")
		t.Log("        This is the P0 bug we need to fix!")
	} else {
		t.Log("RESULT: ✅ PASSED (BUG FIXED)")
		t.Log("        Host survived condition-only update")
		t.Log("        SyncHosts() correctly respects ConditionOnlyOperation{}")
	}
}

// TestHostLifecycle_ConditionOnlyUpdate_MinimalReproduction is a minimal version
// that focuses purely on the bug without all the logging
func TestHostLifecycle_ConditionOnlyUpdate_MinimalReproduction(t *testing.T) {
	// ARRANGE
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	namespace := "test-ns"
	name := "host-minimal"
	hostUUID := "550e8400-e29b-41d4-a716-446655440888"

	// Create Host
	resourceVersion := CreateK8sMetadata(t, tc.DB, `[]`)
	CreateTestHost(t, tc.DB, namespace, name, hostUUID, resourceVersion, false)

	// Create Outbox entry
	_, err := tc.DB.Exec(`
		INSERT INTO sync_outbox (
			resource_type, resource_id, resource_namespace, resource_name,
			operation, target_system, payload, status, attempts, max_retries
		) VALUES (
			'Host', $1::uuid, $2, $3,
			$4, 'SGROUP', '{"namespace":"'||$2||'","name":"'||$3||'","uuid":"'||$1||'"}'::jsonb,
			'PENDING', 0, 5
		)
	`, hostUUID, namespace, name, domain.SyncOperationCreate)
	require.NoError(t, err)

	// Verify Host exists
	var countBefore int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&countBefore)
	require.NoError(t, err)
	require.Equal(t, 1, countBefore, "Host should exist before worker")

	// ACT: Process with worker
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = worker.ProcessOnce(ctx)
	time.Sleep(1 * time.Second)

	// ASSERT: Host should still exist (but BUG causes deletion)
	var countAfter int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&countAfter)
	require.NoError(t, err)

	// THIS WILL FAIL - proving the bug
	assert.Equal(t, 1, countAfter,
		"BUG: Host deleted after worker processing! "+
			"SyncHosts() ignores ConditionOnlyOperation{} flag. "+
			"Before: %d, After: %d",
		countBefore, countAfter)
}

// TestHostLifecycle_MultipleHosts_AllSurvive tests that multiple hosts
// don't get deleted when worker processes them
func TestHostLifecycle_MultipleHosts_AllSurvive(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	worker := TCCreateTestWorker(t, tc)

	namespace := "test-ns"
	hostCount := 3

	// Create multiple hosts
	for i := 0; i < hostCount; i++ {
		name := "host-" + string(rune('a'+i))
		hostUUID := "550e8400-e29b-41d4-a716-44665544000" + string(rune('1'+i))

		resourceVersion := CreateK8sMetadata(t, tc.DB, `[]`)
		CreateTestHost(t, tc.DB, namespace, name, hostUUID, resourceVersion, false)

		// Create outbox entry
		_, err := tc.DB.Exec(`
			INSERT INTO sync_outbox (
				resource_type, resource_id, resource_namespace, resource_name,
				operation, target_system, payload, status, attempts, max_retries
			) VALUES (
				'Host', $1::uuid, $2, $3,
				$4, 'SGROUP', '{"namespace":"'||$2||'","name":"'||$3||'","uuid":"'||$1||'"}'::jsonb,
				'PENDING', 0, 5
			)
		`, hostUUID, namespace, name, domain.SyncOperationCreate)
		require.NoError(t, err)
	}

	// Count hosts before
	var countBefore int
	err := tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts WHERE namespace = $1
	`, namespace).Scan(&countBefore)
	require.NoError(t, err)
	require.Equal(t, hostCount, countBefore, "All hosts should exist before worker")

	t.Logf("Created %d hosts, processing with worker...", hostCount)

	// Process with worker
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for i := 0; i < hostCount; i++ {
		_ = worker.ProcessOnce(ctx)
		time.Sleep(500 * time.Millisecond)
	}

	// Count hosts after
	var countAfter int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts WHERE namespace = $1
	`, namespace).Scan(&countAfter)
	require.NoError(t, err)

	t.Logf("Hosts before: %d, Hosts after: %d", countBefore, countAfter)

	// THIS WILL FAIL - all hosts get deleted due to bug
	assert.Equal(t, hostCount, countAfter,
		"BUG: %d hosts deleted after worker processing! Expected all %d to survive.",
		hostCount-countAfter, hostCount)
}
