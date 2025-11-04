package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/pg/writers"
)

// TestHostReadyBug_RealScenario_SqlErrNoRows reproduces the ACTUAL P0 CRITICAL bug:
//
// ROOT CAUSE: Writer.upsertHost() does NOT properly handle sql.ErrNoRows for new resources.
//
// When creating a NEW Host:
// 1. QueryRow().Scan() returns sql.ErrNoRows (expected - resource doesn't exist yet)
// 2. Code does NOT check for sql.ErrNoRows
// 3. existingResourceVersion remains uninitialized/invalid
// 4. Check `if !existingResourceVersion.Valid` FAILS (undefined behavior)
// 5. forcePendingSyncCondition() is NEVER called
// 6. Result: Ready=True stored in database (WRONG!)
//
// This test verifies the REAL problem and will FAIL until fixed.
func TestHostReadyBug_RealScenario_SqlErrNoRows(t *testing.T) {
	t.Log("🧪 INTEGRATION TEST: Reproduce REAL bug - sql.ErrNoRows handling")

	// Setup real PostgreSQL container
	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()

	// Create pgxpool
	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err, "failed to create pgxpool")
	defer pool.Close()

	// ===== STEP 1: Create NEW Host using Writer (simulates kubectl apply) =====
	t.Log("  📝 STEP 1: Creating NEW Host via Writer (this is where bug occurs)")

	// Begin transaction
	tx, err := pool.Begin(ctx)
	require.NoError(t, err, "failed to begin transaction")

	// Create Writer with MockRegistry
	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	// Create NEW Host (ResourceVersion empty = new resource)
	newHost := models.Host{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-host-sql-err-no-rows",
			},
		},
		UUID: uuid.New().String(),
		IpList: []models.IPItem{
			{IP: "192.168.100.1/32"},
		},
	}
	newHost.Meta.TouchOnCreate()

	// Call Writer.SyncHosts - THIS IS WHERE THE BUG HAPPENS
	// Expected: forcePendingSyncCondition() should be called
	// Actual: sql.ErrNoRows NOT handled, forcePendingSyncCondition() NEVER called
	err = writer.SyncHosts(ctx, []models.Host{newHost}, ports.EmptyScope{}, nil)
	require.NoError(t, err, "Writer.SyncHosts failed")

	// Commit transaction (triggers fire here)
	err = tx.Commit(ctx)
	require.NoError(t, err, "failed to commit transaction")

	// ===== STEP 2: Read conditions from database =====
	t.Log("  🔍 STEP 2: Reading conditions from database AFTER Writer commit")

	var conditionsJSON string
	err = pool.QueryRow(ctx, `
		SELECT m.conditions
		FROM hosts h
		JOIN k8s_metadata m ON h.resource_version = m.resource_version
		WHERE h.namespace = $1 AND h.name = $2
	`, newHost.Namespace, newHost.Name).Scan(&conditionsJSON)
	require.NoError(t, err, "failed to query conditions from database")

	t.Logf("    Conditions in DB: %s", conditionsJSON)

	// Parse conditions
	var conditions []metav1.Condition
	err = json.Unmarshal([]byte(conditionsJSON), &conditions)
	require.NoError(t, err, "failed to unmarshal conditions")

	// Find Ready condition
	var readyCondition *metav1.Condition
	for i, cond := range conditions {
		if cond.Type == "Ready" {
			readyCondition = &conditions[i]
			break
		}
	}
	require.NotNil(t, readyCondition, "Ready condition should exist")

	t.Logf("    Ready condition: Status=%s, Reason=%s, Message=%s",
		readyCondition.Status, readyCondition.Reason, readyCondition.Message)

	// ===== VERIFICATION: This is the BUG =====
	// Current WRONG behavior (before fix):
	// - Ready.Status = True (WRONG!)
	// - Ready.Reason = "Ready" or empty (WRONG!)
	//
	// Expected CORRECT behavior (after fix):
	// - Ready.Status = False
	// - Ready.Reason = "PendingSGROUPSync"

	if readyCondition.Status == metav1.ConditionTrue {
		t.Errorf("❌ BUG REPRODUCED: Ready=True for NEW Host (should be False!)")
		t.Errorf("   This proves sql.ErrNoRows is NOT handled correctly")
		t.Errorf("   forcePendingSyncCondition() was NEVER called")
		t.Logf("   🐛 ACTUAL: Ready=%s, Reason=%s", readyCondition.Status, readyCondition.Reason)
		t.Logf("   ✅ EXPECTED: Ready=False, Reason=PendingSGROUPSync")
	} else {
		t.Logf("   ✅ CORRECT: forcePendingSyncCondition() was called")
		t.Logf("   ✅ Ready=False with PendingSGROUPSync")
	}

	// Final assertions - WILL FAIL if bug exists
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status,
		"CRITICAL BUG: New Host must have Ready=False (PendingSGROUPSync) until SGROUP sync")
	assert.Equal(t, "PendingSGROUPSync", readyCondition.Reason,
		"CRITICAL BUG: Reason must be PendingSGROUPSync for new Host")
	assert.Contains(t, readyCondition.Message, "Awaiting synchronization",
		"CRITICAL BUG: Message should indicate waiting for SGROUP sync")

	t.Log("🏁 TEST COMPLETE")
}

// TestNetworkReadyBug_RealScenario verifies same bug exists for Network
func TestNetworkReadyBug_RealScenario(t *testing.T) {
	t.Log("🧪 INTEGRATION TEST: Test Network Ready bug (same sql.ErrNoRows issue)")

	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	// Create NEW Network
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	newNetwork := models.Network{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-network-sql-err",
			},
		},
		CIDR: "10.100.0.0/16",
	}
	newNetwork.Meta.TouchOnCreate()

	err = writer.SyncNetworks(ctx, []models.Network{newNetwork}, ports.EmptyScope{}, nil)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	// Check Ready condition
	var conditionsJSON string
	err = pool.QueryRow(ctx, `
		SELECT m.conditions
		FROM networks n
		JOIN k8s_metadata m ON n.resource_version = m.resource_version
		WHERE n.namespace = $1 AND n.name = $2
	`, newNetwork.Namespace, newNetwork.Name).Scan(&conditionsJSON)
	require.NoError(t, err)

	var conditions []metav1.Condition
	err = json.Unmarshal([]byte(conditionsJSON), &conditions)
	require.NoError(t, err)

	var readyCondition *metav1.Condition
	for i, cond := range conditions {
		if cond.Type == "Ready" {
			readyCondition = &conditions[i]
			break
		}
	}
	require.NotNil(t, readyCondition)

	t.Logf("Network Ready: Status=%s, Reason=%s", readyCondition.Status, readyCondition.Reason)

	// Same bug should exist for Network
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status,
		"Network should have Ready=False for new resource")
	assert.Equal(t, "PendingSGROUPSync", readyCondition.Reason,
		"Network should have PendingSGROUPSync reason")
}

// TestAddressGroupReadyBug_RealScenario verifies same bug exists for AddressGroup
func TestAddressGroupReadyBug_RealScenario(t *testing.T) {
	t.Log("🧪 INTEGRATION TEST: Test AddressGroup Ready bug (same sql.ErrNoRows issue)")

	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	// Create NEW AddressGroup
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	newAG := models.AddressGroup{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-ag-sql-err",
			},
		},
		DefaultAction: models.ActionAccept,
		Logs:          false,
		Trace:         false,
		Networks:      []models.NetworkItem{},
	}
	newAG.Meta.TouchOnCreate()

	err = writer.SyncAddressGroups(ctx, []models.AddressGroup{newAG}, ports.EmptyScope{}, nil)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	// Check Ready condition
	var conditionsJSON string
	err = pool.QueryRow(ctx, `
		SELECT m.conditions
		FROM address_groups ag
		JOIN k8s_metadata m ON ag.resource_version = m.resource_version
		WHERE ag.namespace = $1 AND ag.name = $2
	`, newAG.Namespace, newAG.Name).Scan(&conditionsJSON)
	require.NoError(t, err)

	var conditions []metav1.Condition
	err = json.Unmarshal([]byte(conditionsJSON), &conditions)
	require.NoError(t, err)

	var readyCondition *metav1.Condition
	for i, cond := range conditions {
		if cond.Type == "Ready" {
			readyCondition = &conditions[i]
			break
		}
	}
	require.NotNil(t, readyCondition)

	t.Logf("AddressGroup Ready: Status=%s, Reason=%s", readyCondition.Status, readyCondition.Reason)

	// Same bug should exist for AddressGroup
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status,
		"AddressGroup should have Ready=False for new resource")
	assert.Equal(t, "PendingSGROUPSync", readyCondition.Reason,
		"AddressGroup should have PendingSGROUPSync reason")
}

// TestServiceReadyBug_RealScenario verifies same bug exists for Service
func TestServiceReadyBug_RealScenario(t *testing.T) {
	t.Log("🧪 INTEGRATION TEST: Test Service Ready bug (same sql.ErrNoRows issue)")

	tc := SetupTestEnvironment(t)
	defer tc.Cleanup()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, tc.ConnectionString)
	require.NoError(t, err)
	defer pool.Close()

	// Create NEW Service
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	registry := NewMockRegistry()
	writer := writers.NewWriter(registry, tx, ctx)

	newService := models.Service{
		SelfRef: models.SelfRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Namespace: "test-namespace",
				Name:      "test-service-sql-err",
			},
		},
		Description: "Test Service for Ready bug verification",
		IngressPorts: []models.IngressPort{
			{
				Protocol:    models.TCP,
				Port:        "80",
				Description: "HTTP port",
			},
			{
				Protocol:    models.TCP,
				Port:        "443",
				Description: "HTTPS port",
			},
		},
		AddressGroups: []models.AddressGroupRef{},
	}
	newService.Meta.TouchOnCreate()

	err = writer.SyncServices(ctx, []models.Service{newService}, ports.EmptyScope{}, nil)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	// Check Ready condition
	var conditionsJSON string
	err = pool.QueryRow(ctx, `
		SELECT m.conditions
		FROM services s
		JOIN k8s_metadata m ON s.resource_version = m.resource_version
		WHERE s.namespace = $1 AND s.name = $2
	`, newService.Namespace, newService.Name).Scan(&conditionsJSON)
	require.NoError(t, err)

	t.Logf("Conditions in DB: %s", conditionsJSON)

	var conditions []metav1.Condition
	err = json.Unmarshal([]byte(conditionsJSON), &conditions)
	require.NoError(t, err)

	var readyCondition *metav1.Condition
	for i, cond := range conditions {
		if cond.Type == "Ready" {
			readyCondition = &conditions[i]
			break
		}
	}
	require.NotNil(t, readyCondition, "Ready condition should exist")

	t.Logf("Service Ready: Status=%s, Reason=%s, Message=%s",
		readyCondition.Status, readyCondition.Reason, readyCondition.Message)

	// Same bug should exist for Service
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status,
		"Service should have Ready=False for new resource")
	assert.Equal(t, "PendingSGROUPSync", readyCondition.Reason,
		"Service should have PendingSGROUPSync reason")
	assert.Contains(t, readyCondition.Message, "Awaiting synchronization",
		"Service should have message indicating waiting for SGROUP sync")

	t.Log("🏁 TEST COMPLETE")
}
