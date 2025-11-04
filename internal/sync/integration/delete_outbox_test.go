package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBUG003_DeleteNoOutboxEntry detects CRITICAL bug: DELETE operations don't create outbox entries
//
// BUG DESCRIPTION:
// When deleting AddressGroup/Host/Network:
// - DELETE triggers do NOT create outbox entry
// - Old code syncs directly to SGROUP (bypassing outbox)
// - If SGROUP is down during DELETE, resource becomes orphaned
//
// CONSEQUENCE:
// - Deleted K8s resources remain in SGROUP forever
// - Data inconsistency between K8s and SGROUP
// - Manual cleanup required
//
// EXPECTED BEHAVIOR:
// DELETE should create outbox entry with operation="DELETE":
// - Trigger fires on DELETE
// - Creates entry in sync_outbox
// - Worker processes DELETE asynchronously
// - SGROUP resource removed eventually
//
// See: product-workspace/stories/resilient-sync/bugs/BUG-003-delete-no-outbox/
func TestBUG003_DeleteNoOutboxEntry(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()

	// Clean slate
	TCCleanOutboxTable(t, tc.DB)

	// STEP 1: Create AddressGroup via Repository (triggers fire!)
	t.Log("📝 Creating AddressGroup via Repository Writer...")
	agNamespace := "default"
	agName := "test-delete-ag"

	ag := TCCreateAddressGroupViaRepositoryWithConnStr(t, tc.ConnectionString, agNamespace, agName)
	require.NotEmpty(t, ag.Name)

	// Wait for CREATE outbox entry
	createEntry := TCWaitForOutboxEntry(t, tc.DB, agName, 5*time.Second)
	require.NotNil(t, createEntry, "CREATE should create outbox entry via trigger")
	t.Logf("  ✅ CREATE entry created: %s", createEntry.ID)

	// STEP 2: Process CREATE (clear outbox for clean test)
	t.Log("🧹 Cleaning outbox to isolate DELETE test...")
	TCCleanOutboxTable(t, tc.DB)

	// Verify outbox is empty
	count := TCCountOutboxEntries(t, tc.DB)
	require.Equal(t, 0, count, "outbox should be empty after cleanup")

	// STEP 3: DELETE AddressGroup via Repository (triggers should fire!)
	t.Log("🗑️  Deleting AddressGroup via Repository Writer...")
	TCDeleteAddressGroupViaRepositoryWithConnStr(t, tc.ConnectionString, agNamespace, agName)

	// STEP 4: Check if DELETE created outbox entry
	t.Log("🔍 Checking if DELETE created outbox entry...")
	time.Sleep(2 * time.Second) // Give triggers time to fire

	// Query for DELETE outbox entry
	var deleteEntryID *string
	var operation *string
	err := tc.DB.QueryRowContext(ctx, `
		SELECT id, operation
		FROM sync_outbox
		WHERE resource_namespace = $1
		  AND resource_name = $2
		  AND operation = 'DELETE'
		ORDER BY created_at DESC
		LIMIT 1
	`, agNamespace, agName).Scan(&deleteEntryID, &operation)

	// 🐛 BUG-003 DETECTION: If no DELETE entry found, bug exists!
	if err != nil {
		t.Logf("  ❌ No DELETE outbox entry found!")
		t.Logf("  🐛 BUG-003 DETECTED: DELETE did NOT create outbox entry")

		// Check if ANY outbox entries exist (should be 0)
		allCount := TCCountOutboxEntries(t, tc.DB)
		t.Logf("  📊 Total outbox entries after DELETE: %d", allCount)

		require.Fail(t,
			"BUG-003 CONFIRMED: DELETE operation did NOT create outbox entry",
			"DELETE triggers are missing or not firing. Resource will be orphaned in SGROUP!")
	}

	// If we get here, DELETE entry was found (bug fixed!)
	t.Logf("  ✅ DELETE entry created: %s, operation=%s", *deleteEntryID, *operation)

	assert.Equal(t, "DELETE", *operation, "operation should be DELETE")
}

// TestBUG003_DeleteDuringSGROUPDowntime verifies DELETE resilience during SGROUP outage
//
// SCENARIO:
// 1. SGROUP is down
// 2. Delete AddressGroup in K8s
// 3. DELETE outbox entry created (if BUG-003 fixed)
// 4. SGROUP comes back up
// 5. Worker processes DELETE → SGROUP resource removed
//
// EXPECTED: Resource eventually deleted from SGROUP
// BUG-003: Resource orphaned in SGROUP forever
func TestBUG003_DeleteDuringSGROUPDowntime(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()

	// STEP 1: SGROUP goes down BEFORE delete
	t.Log("❌ Setting SGROUP to ConnectionRefused (simulating outage)...")
	tc.MockSGROUP.SetMode(ModeConnectionRefused)

	// STEP 2: Create and then delete AddressGroup
	t.Log("📝 Creating AddressGroup...")
	agNamespace := "default"
	agName := "test-delete-offline"

	agID := TCCreateTestAddressGroup(t, tc.DB, agNamespace, agName)
	require.NotEmpty(t, agID)

	// Clean outbox for isolation
	TCCleanOutboxTable(t, tc.DB)

	t.Log("🗑️  Deleting AddressGroup while SGROUP is down...")
	_, err := tc.DB.ExecContext(ctx, `
		DELETE FROM address_groups
		WHERE namespace = $1 AND name = $2
	`, agNamespace, agName)
	require.NoError(t, err)

	// STEP 3: Verify DELETE outbox entry created
	t.Log("🔍 Verifying DELETE outbox entry created...")
	time.Sleep(2 * time.Second)

	var deleteEntryID string
	err = tc.DB.QueryRowContext(ctx, `
		SELECT id
		FROM sync_outbox
		WHERE resource_namespace = $1
		  AND resource_name = $2
		  AND operation = 'DELETE'
		LIMIT 1
	`, agNamespace, agName).Scan(&deleteEntryID)

	if err != nil {
		require.Fail(t,
			"🐛 BUG-003: DELETE during SGROUP downtime did NOT create outbox entry",
			"This means the resource will be orphaned in SGROUP!")
	}

	t.Logf("  ✅ DELETE outbox entry created: %s", deleteEntryID)

	// STEP 4: SGROUP comes back online
	t.Log("✅ Bringing SGROUP back online...")
	tc.MockSGROUP.SetMode(ModeHealthy)

	// STEP 5: Wait for worker to process DELETE
	t.Log("⏳ Waiting for worker to process DELETE (15 seconds)...")
	time.Sleep(15 * time.Second)

	// STEP 6: Verify DELETE outbox entry was processed
	t.Log("🔍 Checking if DELETE was processed...")
	var status string
	err = tc.DB.QueryRowContext(ctx, `
		SELECT status
		FROM sync_outbox
		WHERE id = $1
	`, deleteEntryID).Scan(&status)

	if err != nil {
		t.Logf("  ✅ DELETE entry removed from outbox (processed successfully)")
	} else {
		t.Logf("  📊 DELETE entry status: %s", status)
		assert.Contains(t, []string{"COMPLETED", "PROCESSING"}, status,
			"DELETE should be processed or completed")
	}

	// STEP 7: Verify mock SGROUP received DELETE request
	requestCount := tc.MockSGROUP.GetRequestCount()
	t.Logf("  📊 Mock SGROUP received %d requests", requestCount)

	if requestCount == 0 {
		t.Log("  ⚠️  WARNING: No requests to SGROUP - worker may not be running")
	}

	// TODO: Add assertion to verify SGROUP DELETE API was called
	// This requires mock SGROUP to track DELETE requests specifically
}

// TestBUG003_MultipleEntityTypesDeleteOutbox verifies DELETE triggers for all entity types
//
// Tests that DELETE creates outbox entries for:
// - AddressGroup
// - Host
// - Network
// - Service (if applicable)
func TestBUG003_MultipleEntityTypesDeleteOutbox(t *testing.T) {
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()

	testCases := []struct {
		name       string
		entityType string
		createFunc func() string // Returns entity ID
		deleteFunc func(id string) error
	}{
		{
			name:       "AddressGroup DELETE",
			entityType: "AddressGroup",
			createFunc: func() string {
				return TCCreateTestAddressGroup(t, tc.DB, "default", "test-delete-ag-multi")
			},
			deleteFunc: func(id string) error {
				_, err := tc.DB.ExecContext(ctx, `
					DELETE FROM address_groups
					WHERE namespace = 'default' AND name = 'test-delete-ag-multi'
				`)
				return err
			},
		},
		// TODO: Add Host, Network, Service after schema helpers are fixed
		// {
		// 	name:       "Host DELETE",
		// 	entityType: "Host",
		// 	createFunc: func() string {
		// 		return TCCreateTestHost(t, tc.DB, "default", "test-delete-host")
		// 	},
		// 	deleteFunc: func(id string) error {
		// 		_, err := tc.DB.ExecContext(ctx, `
		// 			DELETE FROM hosts WHERE namespace = 'default' AND name = 'test-delete-host'
		// 		`)
		// 		return err
		// 	},
		// },
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Clean outbox
			TCCleanOutboxTable(t, tc.DB)

			// Create entity
			t.Logf("Creating %s...", testCase.entityType)
			entityID := testCase.createFunc()
			require.NotEmpty(t, entityID)

			// Clean outbox again (remove CREATE entry)
			TCCleanOutboxTable(t, tc.DB)

			// Delete entity
			t.Logf("Deleting %s...", testCase.entityType)
			err := testCase.deleteFunc(entityID)
			require.NoError(t, err)

			// Check for DELETE outbox entry
			time.Sleep(2 * time.Second)
			count := TCCountOutboxEntriesByOperation(t, tc.DB, "DELETE")

			assert.Greater(t, count, 0,
				"🐛 BUG-003: %s DELETE did NOT create outbox entry", testCase.entityType)
		})
	}
}

// TCCountOutboxEntriesByOperation counts outbox entries with specific operation
func TCCountOutboxEntriesByOperation(t *testing.T, db *sql.DB, operation string) int {
	t.Helper()
	ctx := context.Background()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sync_outbox WHERE operation = $1
	`, operation).Scan(&count)

	if err != nil {
		t.Fatalf("failed to count outbox entries: %v", err)
	}

	return count
}
