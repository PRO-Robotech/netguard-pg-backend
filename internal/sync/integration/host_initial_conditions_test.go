package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"
)

// TestHost_InitialConditions_ReadyFalseBeforeSync validates that a newly created Host
// has Ready=False and PendingSync=True BEFORE OutboxWorker processes it.
//
// This test reproduces P0 BUG:
// - Expected: After kubectl create → Ready=False, PendingSync=True
// - Actual (BUG): After kubectl create → Ready=True (immediately!)
//
// The bug occurs because forcePendingSyncCondition() in writers/host.go
// sets Ready=True instead of Ready=False when creating initial conditions.
//
// TEST APPROACH:
// 1. Create Host through Writer (simulates kubectl create)
// 2. Read Host conditions IMMEDIATELY (before OutboxWorker)
// 3. Assert Ready=False and PendingSync=True
//
// CRITICAL: This test runs WITHOUT OutboxWorker, so we test ONLY the initial
// condition setting logic in the Writer.
func TestHost_InitialConditions_ReadyFalseBeforeSync(t *testing.T) {
	t.Log("========================================")
	t.Log("TEST: Host Initial Conditions - Ready=False Before SGROUP Sync")
	t.Log("========================================")

	// ========================================
	// STEP 1: Setup testcontainer with PostgreSQL
	// ========================================
	t.Log("\nSTEP 1: Setting up test environment...")
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	t.Log("✅ Test environment ready:")
	t.Logf("   - PostgreSQL container: RUNNING")
	t.Logf("   - Migrations applied: ALL (001-035)")

	ctx := context.Background()

	// ========================================
	// STEP 2: Create pgxpool for Writer
	// ========================================
	t.Log("\nSTEP 2: Creating pgxpool connection...")

	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	t.Log("✅ pgxpool created")

	// ========================================
	// STEP 3: CREATE Host via Writer (simulates kubectl create)
	// ========================================
	t.Log("\nSTEP 3: Creating Host through Writer (simulates kubectl create)...")

	namespace := "test-ns"
	name := "host-initial-conditions"
	hostUUID := uuid.New().String()

	// Create Host model
	host := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: namespace,
				Name:      name,
			},
		},
		UUID: hostUUID,
		IpList: []models.IPItem{
			{IP: "10.0.1.100"},
		},
	}

	// Initialize Meta (required by Writer)
	host.Meta.TouchOnCreate()

	t.Logf("   - Host: %s/%s", namespace, name)
	t.Logf("   - UUID: %s", hostUUID)
	t.Logf("   - Initial conditions: %v", host.Meta.Conditions)

	// ========================================
	// STEP 4: Write Host to DB using Writer
	// ========================================
	t.Log("\nSTEP 4: Writing Host to database...")

	// Begin transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")
	defer tx.Rollback(ctx)

	// Create Writer
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Sync Host (this calls forcePendingSyncCondition internally!)
	err = writer.SyncHosts(ctx, []models.Host{host}, ports.EmptyScope{}, nil)
	require.NoError(t, err, "SyncHosts should succeed")

	// Commit transaction
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	t.Log("✅ Host written to database")

	// Wait a moment for any triggers to complete
	time.Sleep(200 * time.Millisecond)

	// ========================================
	// STEP 5: Read Host from DB and check conditions
	// ========================================
	t.Log("\nSTEP 5: Reading Host from database and checking conditions...")

	// Query k8s_metadata for conditions
	var conditions string
	var ready bool
	err = tc.DB.QueryRow(`
		SELECT h.ready, m.conditions::text
		FROM hosts h
		JOIN k8s_metadata m ON h.resource_version = m.resource_version
		WHERE h.namespace = $1 AND h.name = $2
	`, namespace, name).Scan(&ready, &conditions)
	require.NoError(t, err, "failed to query host conditions")

	t.Logf("\nRESULTS:")
	t.Logf("   - DB ready column: %v", ready)
	t.Logf("   - Conditions JSON: %s", conditions)

	// Parse conditions JSON
	var conditionList []metav1.Condition
	err = tc.DB.QueryRow(`
		SELECT jsonb_agg(elem)
		FROM k8s_metadata,
		LATERAL jsonb_array_elements(conditions) AS elem
		WHERE resource_version = (
			SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2
		)
	`, namespace, name).Scan(&conditionList)

	if err == nil {
		t.Log("\nParsed Conditions:")
		for _, cond := range conditionList {
			t.Logf("   - Type: %s, Status: %s, Reason: %s, Message: %s",
				cond.Type, cond.Status, cond.Reason, cond.Message)
		}
	}

	// ========================================
	// STEP 6: CRITICAL ASSERTIONS
	// ========================================
	t.Log("\nSTEP 6: CRITICAL ASSERTIONS...")

	t.Run("DB ready column should be FALSE", func(t *testing.T) {
		assert.False(t, ready,
			"❌ BUG DETECTED: hosts.ready=TRUE immediately after creation! "+
				"Expected: ready=FALSE (pending SGROUP sync). "+
				"This means forcePendingSyncCondition() is setting ready=TRUE incorrectly.")
	})

	// Check Ready condition
	var readyCondStatus string
	var readyCondReason string
	err = tc.DB.QueryRow(`
		SELECT
			elem->>'status' as status,
			elem->>'reason' as reason
		FROM k8s_metadata,
		LATERAL jsonb_array_elements(conditions) AS elem
		WHERE resource_version = (
			SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2
		) AND elem->>'type' = 'Ready'
	`, namespace, name).Scan(&readyCondStatus, &readyCondReason)

	if err == nil {
		t.Run("Ready condition should be False", func(t *testing.T) {
			assert.Equal(t, string(metav1.ConditionFalse), readyCondStatus,
				"❌ BUG: Ready condition is %s, expected False! "+
					"The resource should NOT be Ready before SGROUP sync.", readyCondStatus)
		})

		t.Run("Ready condition reason should indicate pending sync", func(t *testing.T) {
			// Accept either "PendingSGROUPSync" or "Pending" or "NotReady"
			validReasons := []string{"PendingSGROUPSync", "Pending", "NotReady", "SyncPending"}
			assert.Contains(t, validReasons, readyCondReason,
				"Ready condition reason should indicate pending sync, got: %s", readyCondReason)
		})
	} else {
		t.Logf("⚠️  Ready condition not found in conditions (this is also a bug!)")
	}

	// Check PendingSync condition
	var pendingSyncStatus string
	err = tc.DB.QueryRow(`
		SELECT elem->>'status'
		FROM k8s_metadata,
		LATERAL jsonb_array_elements(conditions) AS elem
		WHERE resource_version = (
			SELECT resource_version FROM hosts WHERE namespace = $1 AND name = $2
		) AND elem->>'type' = 'PendingSync'
	`, namespace, name).Scan(&pendingSyncStatus)

	if err == nil {
		t.Run("PendingSync condition should be True", func(t *testing.T) {
			assert.Equal(t, string(metav1.ConditionTrue), pendingSyncStatus,
				"PendingSync should be True before OutboxWorker processes the resource")
		})
	} else {
		t.Logf("⚠️  PendingSync condition not found (may not be set initially)")
	}

	// ========================================
	// STEP 7: Verify Outbox Entry Created
	// ========================================
	t.Log("\nSTEP 7: Verifying outbox entry was created by trigger...")

	var outboxCount int
	err = tc.DB.QueryRow(`
		SELECT COUNT(*)
		FROM sync_outbox
		WHERE resource_type = 'Host'
		  AND resource_namespace = $1
		  AND resource_name = $2
	`, namespace, name).Scan(&outboxCount)
	require.NoError(t, err)

	t.Logf("   - Outbox entries: %d", outboxCount)

	if outboxCount > 0 {
		t.Log("   ✅ Outbox trigger fired successfully")
	} else {
		t.Log("   ⚠️  No outbox entry (trigger may have failed)")
	}

	t.Log("\n========================================")
	t.Log("TEST COMPLETE")
	t.Log("========================================")

	// Summary
	if !ready && readyCondStatus == string(metav1.ConditionFalse) {
		t.Log("RESULT: ✅ PASSED - Conditions set correctly!")
		t.Log("        Ready=False before SGROUP sync (as expected)")
	} else {
		t.Log("RESULT: ❌ FAILED - BUG CONFIRMED!")
		t.Log("        Ready=True immediately after creation")
		t.Log("        This is the P0 bug we need to fix!")
		t.Log("")
		t.Log("ROOT CAUSE:")
		t.Log("   File: internal/infrastructure/repositories/pg/writers/host.go")
		t.Log("   Function: forcePendingSyncCondition()")
		t.Log("   Issue: Sets Ready=True instead of Ready=False")
		t.Log("")
		t.Log("FIX REQUIRED:")
		t.Log("   Update forcePendingSyncCondition() to set:")
		t.Log("   - Ready: status=False, reason=PendingSGROUPSync")
		t.Log("   - PendingSync: status=True, reason=PendingSGROUPSync")
	}
}

// TestHost_InitialConditions_MinimalReproduction is a minimal version
// that focuses purely on the bug without verbose logging
func TestHost_InitialConditions_MinimalReproduction(t *testing.T) {
	// ARRANGE
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	namespace := "test-ns"
	name := "host-minimal-cond-test"
	hostUUID := uuid.New().String()

	// Create Host
	host := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: namespace,
				Name:      name,
			},
		},
		UUID: hostUUID,
		IpList: []models.IPItem{
			{IP: "10.0.2.100"},
		},
	}
	host.Meta.TouchOnCreate()

	// Write to DB
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	err = writer.SyncHosts(ctx, []models.Host{host}, ports.EmptyScope{}, nil)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// ACT: Read ready status from DB
	var ready bool
	err = tc.DB.QueryRow(`
		SELECT ready FROM hosts WHERE namespace = $1 AND name = $2
	`, namespace, name).Scan(&ready)
	require.NoError(t, err)

	// ASSERT: ready should be FALSE before SGROUP sync
	assert.False(t, ready,
		"BUG: hosts.ready=TRUE immediately after creation! "+
			"Expected: FALSE (pending SGROUP sync). "+
			"Check forcePendingSyncCondition() in writers/host.go")
}

// TestHost_InitialConditions_MultipleResources tests that ALL resources
// (Host, Network, AddressGroup) have correct initial conditions
func TestHost_InitialConditions_MultipleResources(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()
	pool, _ := pgxpool.New(ctx, tc.ConnectionString)
	defer pool.Close()

	namespace := "test-ns"

	t.Run("Host initial conditions", func(t *testing.T) {
		host := models.Host{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      "test-host",
				},
			},
			UUID: uuid.New().String(),
		}
		host.Meta.TouchOnCreate()

		tx, _ := pool.Begin(ctx)
		defer tx.Rollback(ctx)
		registry := NewMockRegistry()
		writer := writers.NewWriter(registry, tx, ctx)

		_ = writer.SyncHosts(ctx, []models.Host{host}, ports.EmptyScope{}, nil)
		_ = tx.Commit(ctx)
		time.Sleep(100 * time.Millisecond)

		var ready bool
		tc.DB.QueryRow(`SELECT ready FROM hosts WHERE namespace = $1 AND name = $2`,
			namespace, "test-host").Scan(&ready)

		assert.False(t, ready, "Host should have ready=FALSE initially")
	})

	t.Run("Network initial conditions", func(t *testing.T) {
		network := models.Network{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      "test-network",
				},
			},
			CIDR: "10.0.0.0/16",
		}
		network.Meta.TouchOnCreate()

		tx, _ := pool.Begin(ctx)
		defer tx.Rollback(ctx)
		registry := NewMockRegistry()
		writer := writers.NewWriter(registry, tx, ctx)

		_ = writer.SyncNetworks(ctx, []models.Network{network}, ports.EmptyScope{}, nil)
		_ = tx.Commit(ctx)
		time.Sleep(100 * time.Millisecond)

		var ready bool
		tc.DB.QueryRow(`SELECT ready FROM networks WHERE namespace = $1 AND name = $2`,
			namespace, "test-network").Scan(&ready)

		assert.False(t, ready, "Network should have ready=FALSE initially")
	})

	t.Run("AddressGroup initial conditions", func(t *testing.T) {
		ag := models.AddressGroup{
			SelfRef: models.SelfRef{
				ResourceIdentifier: models.ResourceIdentifier{
					Namespace: namespace,
					Name:      "test-ag",
				},
			},
			DefaultAction: models.ActionDrop,
		}
		ag.Meta.TouchOnCreate()

		tx, _ := pool.Begin(ctx)
		defer tx.Rollback(ctx)
		registry := NewMockRegistry()
		writer := writers.NewWriter(registry, tx, ctx)

		_ = writer.SyncAddressGroups(ctx, []models.AddressGroup{ag}, ports.EmptyScope{}, nil)
		_ = tx.Commit(ctx)
		time.Sleep(100 * time.Millisecond)

		var ready bool
		tc.DB.QueryRow(`SELECT ready FROM address_groups WHERE namespace = $1 AND name = $2`,
			namespace, "test-ag").Scan(&ready)

		assert.False(t, ready, "AddressGroup should have ready=FALSE initially")
	})
}
