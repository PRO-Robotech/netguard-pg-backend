package integration

import (
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestNetworkReadyTransition_CreatesOutboxEntry validates that Network ready transition
// triggers Migration 029 → Migration 027 → sync_outbox entry creation
func TestNetworkReadyTransition_CreatesOutboxEntry(t *testing.T) {
	// ========================================
	// ARRANGE - Setup test data
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions empty (ready=false)
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)

	// Create Network with ready=FALSE
	namespace := "test-ns"
	name := "network-1"

	networkID := CreateTestNetwork(t, env.DB, namespace, name, resourceVersion, false)

	t.Logf("Test setup complete: Network %s/%s (id=%s, ready=false)", namespace, name, networkID)

	// ========================================
	// ACT - Perform action
	// ========================================
	// Trigger ready transition (FALSE → TRUE)
	// This updates k8s_metadata.conditions → triggers Migration 029
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// ========================================
	// ASSERT - Verify results
	// ========================================
	// Migration 029 should update networks.ready = TRUE
	WaitForNetworkReady(t, env.DB, namespace, name, true, 5*time.Second)
	AssertNetworkReady(t, env.DB, namespace, name, true)

	// Migration 027 should create Outbox entry for Network
	entry := WaitForOutboxEntry(t, env.DB, "Network", string(domain.SyncOperationUpdate), 5*time.Second)

	require.NotNil(t, entry, "Outbox entry should be created for Network")
	assert.Equal(t, "Network", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)
	assert.Equal(t, domain.TargetSystemSGROUP, entry.TargetSystem)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)
	assert.Equal(t, 0, entry.Attempts)

	t.Logf("✅ Test PASSED: Network ready transition → Outbox entry created")
}

// TestNetworkReadyTransition_NoChangeWhenAlreadyReady validates that
// updating conditions when network is already ready does NOT create duplicate entries
func TestNetworkReadyTransition_NoChangeWhenAlreadyReady(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)

	// Create Network with ready=TRUE (already ready)
	namespace := "test-ns"
	name := "network-2"

	networkID := CreateTestNetwork(t, env.DB, namespace, name, resourceVersion, true)
	t.Logf("Test setup complete: Network %s/%s (id=%s, ready=true)", namespace, name, networkID)

	// Wait for initial state to stabilize
	time.Sleep(100 * time.Millisecond)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))
	t.Logf("Outbox entries before: %d", countBefore)

	// ========================================
	// ACT
	// ========================================
	// Update conditions again (ready=true → ready=true, no change)
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// Wait for any potential triggers
	time.Sleep(200 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// No new outbox entries should be created
	countAfter := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))
	t.Logf("Outbox entries after: %d", countAfter)

	assert.Equal(t, countBefore, countAfter, "No new outbox entries should be created when ready status unchanged")

	t.Logf("✅ Test PASSED: No duplicate entries when already ready")
}

// TestNetworkReadyTransition_FalseToTrue validates explicit FALSE → TRUE transition
func TestNetworkReadyTransition_FalseToTrue(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions empty (ready=false)
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)

	namespace := "test-ns"
	name := "network-3"

	CreateTestNetwork(t, env.DB, namespace, name, resourceVersion, false)

	// Verify initial state
	AssertNetworkReady(t, env.DB, namespace, name, false)

	// ========================================
	// ACT
	// ========================================
	// Transition FALSE → TRUE
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// ========================================
	// ASSERT
	// ========================================
	// Network ready should be TRUE
	WaitForNetworkReady(t, env.DB, namespace, name, true, 5*time.Second)

	// Outbox entry should be created
	entry := WaitForOutboxEntry(t, env.DB, "Network", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("✅ Test PASSED: FALSE → TRUE transition creates outbox entry")
}

// TestNetworkReadyTransition_TrueToFalse validates TRUE → FALSE transition
// Should update ready but may not create outbox (depends on trigger logic)
func TestNetworkReadyTransition_TrueToFalse(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)

	namespace := "test-ns"
	name := "network-4"

	CreateTestNetwork(t, env.DB, namespace, name, resourceVersion, true)

	// Verify initial state
	AssertNetworkReady(t, env.DB, namespace, name, true)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))

	// ========================================
	// ACT
	// ========================================
	// Transition TRUE → FALSE
	TriggerReadyTransition(t, env.DB, resourceVersion, false)

	// ========================================
	// ASSERT
	// ========================================
	// Network ready should be FALSE
	WaitForNetworkReady(t, env.DB, namespace, name, false, 5*time.Second)

	// Check if outbox entry created (trigger may only fire on FALSE→TRUE)
	time.Sleep(200 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))

	// Migration 027 may only trigger on FALSE→TRUE for ready
	assert.Equal(t, countBefore, countAfter, "TRUE→FALSE should not create outbox entry if trigger only on FALSE→TRUE")

	t.Logf("✅ Test PASSED: TRUE → FALSE updates ready status")
}

// TestMultipleNetworks_ReadyTransition validates that multiple networks
// can transition ready status independently
func TestMultipleNetworks_ReadyTransition(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create 3 networks with different ready states
	networks := []struct {
		namespace string
		name      string
		ready     bool
	}{
		{"ns1", "network-a", false},
		{"ns1", "network-b", false},
		{"ns2", "network-c", false},
	}

	resourceVersions := make([]int64, len(networks))

	for i, net := range networks {
		rv := CreateK8sMetadata(t, env.DB, `[]`)
		resourceVersions[i] = rv
		CreateTestNetwork(t, env.DB, net.namespace, net.name, rv, net.ready)
	}

	t.Logf("Created %d test networks", len(networks))

	// ========================================
	// ACT
	// ========================================
	// Trigger ready transition for network-a and network-c (skip network-b)
	TriggerReadyTransition(t, env.DB, resourceVersions[0], true) // network-a
	TriggerReadyTransition(t, env.DB, resourceVersions[2], true) // network-c

	// ========================================
	// ASSERT
	// ========================================
	// network-a should be ready
	WaitForNetworkReady(t, env.DB, networks[0].namespace, networks[0].name, true, 5*time.Second)

	// network-b should still be not ready
	AssertNetworkReady(t, env.DB, networks[1].namespace, networks[1].name, false)

	// network-c should be ready
	WaitForNetworkReady(t, env.DB, networks[2].namespace, networks[2].name, true, 5*time.Second)

	t.Logf("✅ Test PASSED: Multiple networks transition independently")
}

// TestNetworkReadyTransition_WithInvalidConditions validates behavior
// with conditions that don't contain Ready=True
func TestNetworkReadyTransition_WithInvalidConditions(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create k8s_metadata with conditions that don't contain Ready=True
	resourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"OtherCondition","status":"True"}]`)

	namespace := "test-ns"
	name := "network-5"

	CreateTestNetwork(t, env.DB, namespace, name, resourceVersion, false)

	// Verify initial state
	AssertNetworkReady(t, env.DB, namespace, name, false)

	// Count outbox entries before
	countBefore := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))

	// ========================================
	// ACT
	// ========================================
	// Update conditions (still no Ready=True)
	// This should NOT change ready status (should remain FALSE)
	_, err := env.DB.Exec(`
		UPDATE k8s_metadata
		SET conditions = $1::jsonb
		WHERE resource_version = $2
	`, `[{"type":"AnotherCondition","status":"True"}]`, resourceVersion)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// ========================================
	// ASSERT
	// ========================================
	// Network ready should still be FALSE
	AssertNetworkReady(t, env.DB, namespace, name, false)

	// No outbox entry created
	countAfter := GetOutboxEntryCount(t, env.DB, "Network", string(domain.SyncOperationUpdate))
	assert.Equal(t, countBefore, countAfter, "No outbox entry when Ready=True not present")

	t.Logf("✅ Test PASSED: Invalid conditions do not trigger ready transition")
}

// TestNetworkReadyTransition_WithAddressGroupRef validates network with address_group references
// This tests Migration 027 trigger which considers address_group_ref in networks
func TestNetworkReadyTransition_WithAddressGroupRef(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup first
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag"
	agID := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	// Create Network with address_group_ref pointing to AG
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	namespace := "test-ns"
	name := "network-with-ag-ref"

	// Create network with address_group_ref
	var networkID string
	err := env.DB.QueryRow(`
		INSERT INTO networks (
			namespace, name, resource_version, ready,
			address_group_ref_namespace, address_group_ref_name
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, namespace, name, resourceVersion, false, agNamespace, agName).Scan(&networkID)
	require.NoError(t, err)

	t.Logf("Created Network with AG ref: %s/%s → AG %s/%s (ag_id=%s)", namespace, name, agNamespace, agName, agID)

	// ========================================
	// ACT
	// ========================================
	// Trigger network ready transition
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// ========================================
	// ASSERT
	// ========================================
	// Network should become ready
	WaitForNetworkReady(t, env.DB, namespace, name, true, 5*time.Second)

	// Outbox entry should be created for Network
	entry := WaitForOutboxEntry(t, env.DB, "Network", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)
	assert.Equal(t, "Network", entry.ResourceType)

	t.Logf("✅ Test PASSED: Network with AG ref creates outbox entry on ready transition")
}

// TestNetworkReadyTransition_WaitsForAddressGroupReady validates that network
// ready transition considers referenced AddressGroup ready status
// NOTE: This depends on actual trigger logic - may need adjustment based on implementation
func TestNetworkReadyTransition_WaitsForAddressGroupReady(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup (not ready yet - no hosts)
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag"
	agID := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	// Create Network with address_group_ref
	resourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	namespace := "test-ns"
	name := "network-waiting-ag"

	var networkID string
	err := env.DB.QueryRow(`
		INSERT INTO networks (
			namespace, name, resource_version, ready,
			address_group_ref_namespace, address_group_ref_name
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, namespace, name, resourceVersion, false, agNamespace, agName).Scan(&networkID)
	require.NoError(t, err)

	t.Logf("Created Network %s with AG ref to %s (ag_id=%s)", networkID, agName, agID)

	// ========================================
	// ACT
	// ========================================
	// Trigger network ready transition
	TriggerReadyTransition(t, env.DB, resourceVersion, true)

	// Wait for network ready
	WaitForNetworkReady(t, env.DB, namespace, name, true, 5*time.Second)

	// ========================================
	// ASSERT
	// ========================================
	// Network should become ready (even if AG has no hosts yet)
	// The dependency check happens at sync time, not at ready transition time
	AssertNetworkReady(t, env.DB, namespace, name, true)

	// Outbox entry should still be created
	entry := WaitForOutboxEntry(t, env.DB, "Network", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	t.Logf("✅ Test PASSED: Network ready transition creates outbox entry (dependency check happens during sync)")
}
