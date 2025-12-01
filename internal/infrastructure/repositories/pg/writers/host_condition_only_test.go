package writers

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/patterns"
)

// setupTestDB creates a test database connection pool
func setupTestDB(t *testing.T) *pgxpool.Pool {
	// Use DATABASE_URL from environment or default test database
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err, "Failed to connect to test database")

	// Verify connection
	err = pool.Ping(ctx)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}

	return pool
}

// cleanupHostTestDB truncates hosts table and related data
func cleanupHostTestDB(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()

	// Disable triggers temporarily to avoid outbox entries during cleanup
	_, err := pool.Exec(ctx, "SET session_replication_role = 'replica'")
	require.NoError(t, err)

	// Clean up in correct order (respecting foreign keys)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE sync_outbox CASCADE")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "TRUNCATE TABLE hosts CASCADE")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "TRUNCATE TABLE k8s_metadata CASCADE")
	require.NoError(t, err)

	// Re-enable triggers
	_, err = pool.Exec(ctx, "SET session_replication_role = 'origin'")
	require.NoError(t, err)
}

// mockSubject is a minimal Subject implementation for testing
type mockSubject struct{}

func (m *mockSubject) Subscribe(observer interface{}) error {
	return nil
}

func (m *mockSubject) Unsubscribe(observer interface{}) error {
	return nil
}

func (m *mockSubject) Notify(event interface{}) {}

// mockRegistry is a minimal registry implementation for testing
type mockRegistry struct {
	pool *pgxpool.Pool
}

func (m *mockRegistry) Subject() patterns.Subject {
	return &mockSubject{}
}

func (m *mockRegistry) Writer(ctx context.Context) (ports.Writer, error) {
	return nil, nil
}

func (m *mockRegistry) WriterForConditions(ctx context.Context) (ports.Writer, error) {
	return nil, nil
}

func (m *mockRegistry) WriterForDeletes(ctx context.Context) (ports.Writer, error) {
	return nil, nil
}

func (m *mockRegistry) Reader(ctx context.Context) (ports.Reader, error) {
	return nil, nil
}

func (m *mockRegistry) ReaderWithReadCommitted(ctx context.Context) (ports.Reader, error) {
	return nil, nil
}

func (m *mockRegistry) ReaderFromWriter(ctx context.Context, w ports.Writer) (ports.Reader, error) {
	return nil, nil
}

func (m *mockRegistry) Close() error {
	return nil
}

func (m *mockRegistry) ExecuteDeleteWithRetry(ctx context.Context, fn func(ports.Writer) error) error {
	return nil
}

// TestSyncHosts_ConditionOnlyOperation_DoesNotDeleteHost is the CRITICAL P0 bug reproduction test
//
// BUG DESCRIPTION:
// When SyncHosts() is called with ConditionOnlyOperation flag, it should ONLY update
// the conditions in k8s_metadata table and should NOT:
// - Delete the Host from database
// - Modify the hosts table
// - Create outbox entries
//
// ACTUAL BEHAVIOR (BUG):
// SyncHosts() ignores ConditionOnlyOperation flag and calls deleteHostsInScope(),
// which deletes the Host from database (at host.go:46)
//
// EXPECTED BEHAVIOR:
// SyncHosts() should check for ConditionOnlyOperation flag (like AddressGroup does at address_group.go:23-29)
// and skip deleteHostsInScope() when updating conditions only
//
// ROOT CAUSE:
// Missing check in host.go:36-50:
//
//	func (w *Writer) SyncHosts(ctx context.Context, hosts []models.Host, scope ports.Scope, options ...ports.Option) error {
//	    syncOp := models.SyncOpUpsert
//	    // ❌ NO CHECK for ConditionOnlyOperation flag!
//
//	    if !scope.IsEmpty() && syncOp != models.SyncOpDelete {
//	        // ❌ Deletes Host even when updating conditions only!
//	        if err := w.deleteHostsInScope(ctx, scope); err != nil {
func TestSyncHosts_ConditionOnlyOperation_DoesNotDeleteHost(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupHostTestDB(t, pool)

	ctx := context.Background()

	// Create mock registry
	registry := &mockRegistry{pool: pool}

	// ==========================================
	// STEP 1: Setup - Create initial Host in DB
	// ==========================================

	// Create a transaction for the Writer
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	writer := NewWriter(registry, tx, ctx)

	// Create test host
	testHost := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "default",
				Name:      "test-host-1",
			},
		},
		UUID:     uuid.New().String(),
		HostName: "agent-001",
		IsBound:  false,
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "PendingSync",
					Message: "Waiting for SGROUP sync",
				},
			},
		},
	}

	// Insert host using SyncHosts (initial creation)
	err = writer.SyncHosts(ctx, []models.Host{testHost}, ports.EmptyScope{})
	require.NoError(t, err, "Failed to create initial host")

	// Commit transaction to persist host
	err = tx.Commit(ctx)
	require.NoError(t, err, "Failed to commit initial host creation")

	// ==========================================
	// STEP 2: Verify host exists in database
	// ==========================================

	var hostCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM hosts WHERE namespace = $1 AND name = $2",
		testHost.Namespace, testHost.Name).Scan(&hostCount)
	require.NoError(t, err)
	assert.Equal(t, 1, hostCount, "Host should exist in database after creation")

	// Get current conditions from k8s_metadata
	var initialConditionsJSON []byte
	err = pool.QueryRow(ctx, `
		SELECT m.conditions
		FROM hosts h
		JOIN k8s_metadata m ON h.resource_version = m.resource_version
		WHERE h.namespace = $1 AND h.name = $2
	`, testHost.Namespace, testHost.Name).Scan(&initialConditionsJSON)
	require.NoError(t, err)

	t.Logf("Initial conditions: %s", string(initialConditionsJSON))

	// ==========================================
	// STEP 3: Create scope matching this Host
	// ==========================================

	// ResourceIdentifierScope will match the host we just created
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			{
				Namespace: testHost.Namespace,
				Name:      testHost.Name,
			},
		},
	}

	// ==========================================
	// STEP 4: Update conditions using ConditionOnlyOperation flag
	// ==========================================

	// Create updated host with new conditions (simulating Worker updating Ready status)
	updatedHost := testHost
	updatedHost.Meta.Conditions = []metav1.Condition{
		{
			Type:    "Ready",
			Status:  metav1.ConditionTrue,
			Reason:  "Synced",
			Message: "Successfully synced to SGROUP",
		},
		{
			Type:    "Validated",
			Status:  metav1.ConditionTrue,
			Reason:  "ValidationPassed",
			Message: "Host validation succeeded",
		},
	}

	// Start new transaction for condition update
	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx)

	writer2 := NewWriter(registry, tx2, ctx)

	// ⚠️ CRITICAL: Call SyncHosts with ConditionOnlyOperation flag
	// This should ONLY update conditions, NOT delete the host
	err = writer2.SyncHosts(ctx, []models.Host{updatedHost}, scope, ports.ConditionOnlyOperation{})

	// The bug causes this to fail with "host not found" because deleteHostsInScope() was called
	require.NoError(t, err, "SyncHosts with ConditionOnlyOperation should not fail")

	// Commit the condition update
	err = tx2.Commit(ctx)
	require.NoError(t, err, "Failed to commit condition update")

	// ==========================================
	// STEP 5: ASSERT - Host should STILL EXIST
	// ==========================================

	// BUG REPRODUCTION: This will FAIL because deleteHostsInScope() deleted the host
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM hosts WHERE namespace = $1 AND name = $2",
		testHost.Namespace, testHost.Name).Scan(&hostCount)
	require.NoError(t, err)

	// ❌ EXPECTED TO FAIL HERE DUE TO BUG
	assert.Equal(t, 1, hostCount,
		"❌ BUG REPRODUCED: Host was DELETED when it should only have conditions updated! "+
			"deleteHostsInScope() was called even with ConditionOnlyOperation flag")

	// Verify conditions were updated (if host still exists)
	if hostCount > 0 {
		var updatedConditionsJSON []byte
		err = pool.QueryRow(ctx, `
			SELECT m.conditions
			FROM hosts h
			JOIN k8s_metadata m ON h.resource_version = m.resource_version
			WHERE h.namespace = $1 AND h.name = $2
		`, testHost.Namespace, testHost.Name).Scan(&updatedConditionsJSON)
		require.NoError(t, err)

		var conditions []metav1.Condition
		err = json.Unmarshal(updatedConditionsJSON, &conditions)
		require.NoError(t, err)

		// Conditions should be updated
		assert.Len(t, conditions, 2, "Should have 2 updated conditions")

		// Find Ready condition
		var readyCondition *metav1.Condition
		for _, cond := range conditions {
			if cond.Type == "Ready" {
				readyCondition = &cond
				break
			}
		}

		require.NotNil(t, readyCondition, "Ready condition should exist")
		assert.Equal(t, metav1.ConditionTrue, readyCondition.Status, "Ready status should be True")
		assert.Equal(t, "Synced", readyCondition.Reason, "Ready reason should be Synced")
	}

	// ==========================================
	// STEP 6: Verify NO outbox entries were created
	// ==========================================

	// ConditionOnlyOperation should NOT create outbox entries
	var outboxCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sync_outbox
		WHERE resource_type = 'Host'
		AND resource_namespace = $1
		AND resource_name = $2
	`, testHost.Namespace, testHost.Name).Scan(&outboxCount)
	require.NoError(t, err)

	// We expect 1 outbox entry from initial creation (STEP 1)
	// ConditionOnlyOperation should NOT create additional entries
	assert.Equal(t, 1, outboxCount,
		"Should have only 1 outbox entry (from initial creation), "+
			"ConditionOnlyOperation should not create new entries")
}

// TestSyncHosts_ConditionOnlyOperation_CompareWithAddressGroup demonstrates the correct behavior
// This test shows how AddressGroup correctly handles ConditionOnlyOperation
func TestSyncHosts_ConditionOnlyOperation_CompareWithAddressGroup(t *testing.T) {
	t.Log("This test documents the CORRECT behavior from AddressGroup implementation")
	t.Log("See address_group.go:23-40 for the proper ConditionOnlyOperation handling:")
	t.Log("")
	t.Log("  func (w *Writer) SyncAddressGroups(...) error {")
	t.Log("      isConditionOnly := false")
	t.Log("      for _, opt := range opts {")
	t.Log("          if _, ok := opt.(ports.ConditionOnlyOperation); ok {")
	t.Log("              isConditionOnly = true")
	t.Log("              break")
	t.Log("          }")
	t.Log("      }")
	t.Log("")
	t.Log("      if isConditionOnly {")
	t.Log("          // CORRECT: Directly update conditions WITHOUT touching entity table")
	t.Log("          for i := range addressGroups {")
	t.Log("              if err := w.updateAddressGroupConditionsOnly(ctx, addressGroups[i]); err != nil {")
	t.Log("                  return err")
	t.Log("              }")
	t.Log("          }")
	t.Log("          return nil")
	t.Log("      }")
	t.Log("")
	t.Log("      // Normal sync path (not reached when isConditionOnly=true)")
	t.Log("      ...")
	t.Log("  }")
	t.Log("")
	t.Log("❌ HOST BUG: SyncHosts() is missing this ConditionOnlyOperation check!")
	t.Log("FIX REQUIRED: Add similar logic to host.go:36-50")
}

// TestSyncHosts_NormalSync_ShouldDeleteInScope verifies that normal sync still works
// This ensures our fix doesn't break the normal sync behavior
func TestSyncHosts_NormalSync_ShouldDeleteInScope(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()
	defer cleanupHostTestDB(t, pool)

	ctx := context.Background()
	registry := &mockRegistry{pool: pool}

	// ==========================================
	// STEP 1: Create initial hosts in DB
	// ==========================================

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	writer := NewWriter(registry, tx, ctx)

	// Create 2 hosts in same namespace
	host1 := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "default",
				Name:      "host-1",
			},
		},
		UUID:     uuid.New().String(),
		HostName: "agent-001",
		IsBound:  false,
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "PendingSync"},
			},
		},
	}

	host2 := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "default",
				Name:      "host-2",
			},
		},
		UUID:     uuid.New().String(),
		HostName: "agent-002",
		IsBound:  false,
		Meta: models.Meta{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionFalse, Reason: "PendingSync"},
			},
		},
	}

	// Create both hosts
	err = writer.SyncHosts(ctx, []models.Host{host1, host2}, ports.EmptyScope{})
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	// Verify both hosts exist
	var hostCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM hosts WHERE namespace = 'default'").Scan(&hostCount)
	require.NoError(t, err)
	assert.Equal(t, 2, hostCount, "Should have 2 hosts")

	// ==========================================
	// STEP 2: Sync with scope (normal sync, NOT ConditionOnly)
	// ==========================================

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx)

	writer2 := NewWriter(registry, tx2, ctx)

	// Create scope for host-1 only
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			{Namespace: "default", Name: "host-1"},
		},
	}

	// Sync with new host data (this is NORMAL sync, should delete existing host-1)
	updatedHost1 := host1
	updatedHost1.HostName = "agent-001-updated"

	// ✅ WITHOUT ConditionOnlyOperation flag, deleteHostsInScope() SHOULD be called
	err = writer2.SyncHosts(ctx, []models.Host{updatedHost1}, scope)
	require.NoError(t, err, "Normal sync should work")

	err = tx2.Commit(ctx)
	require.NoError(t, err)

	// ==========================================
	// STEP 3: Verify normal sync behavior
	// ==========================================

	// In normal sync with scope, the old host-1 should be deleted and new one created
	// (This is expected behavior for normal sync operations)

	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM hosts WHERE namespace = 'default'").Scan(&hostCount)
	require.NoError(t, err)

	t.Logf("Host count after normal sync: %d", hostCount)

	// Verify host-1 data was updated
	var hostname string
	err = pool.QueryRow(ctx, "SELECT host_name_sync FROM hosts WHERE namespace = 'default' AND name = 'host-1'").Scan(&hostname)
	require.NoError(t, err)
	assert.Equal(t, "agent-001-updated", hostname, "Host should be updated in normal sync")
}
