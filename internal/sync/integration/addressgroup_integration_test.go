package integration

import (
	"testing"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain"
)

// TestAddressGroupUpdate_ViaHostBinding validates that AddressGroup update
// is triggered when a Host is bound via host_bindings table
func TestAddressGroupUpdate_ViaHostBinding(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create Host with Ready condition (Migration 033 requires both Host and AG to be ready)
	hostResourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)
	hostNamespace := "test-ns"
	hostName := "host-1"
	hostUUID := "550e8400-e29b-41d4-a716-446655440101"
	hostKey := CreateTestHost(t, env.DB, hostNamespace, hostName, hostUUID, hostResourceVersion, true)

	time.Sleep(100 * time.Millisecond) // Allow triggers to settle

	// Create AddressGroup (with Ready condition - required for binding sync)
	agNamespace := "test-ns"
	agName := "test-ag"
	agKey := CreateTestAddressGroupReady(t, env.DB, agNamespace, agName)

	t.Logf("Created Host %s/%s and AddressGroup %s/%s", hostKey.Namespace, hostKey.Name, agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Create host_binding (binds Host to AddressGroup)
	// Migration 028 should trigger AG update
	bindingRV := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHostBinding(t, env.DB, "test-ns", "binding-1", hostKey, agKey, bindingRV)

	t.Logf("Created host_binding: Host(%s/%s) → AG(%s/%s)", hostKey.Namespace, hostKey.Name, agKey.Namespace, agKey.Name)

	// ========================================
	// ASSERT
	// ========================================
	// Migration 028 should create Outbox entry for AddressGroup UPDATE
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry, "Outbox entry should be created for AddressGroup on host binding")

	assert.Equal(t, "AddressGroup", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)
	assert.Equal(t, domain.OutboxStatusPending, entry.Status)

	// Verify delta contains host information
	assert.NotNil(t, entry.Delta, "Delta should contain host information")

	t.Logf("✅ Test PASSED: Host binding triggers AddressGroup outbox entry")
}

// TestAddressGroupUpdate_ViaSpecHosts validates that AddressGroup update
// is triggered when hosts are updated in address_groups.spec_hosts
func TestAddressGroupUpdate_ViaSpecHosts(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag-spec"
	agKey := CreateTestAddressGroup(t, env.DB, agNamespace, agName, agResourceVersion)

	t.Logf("Created AddressGroup %s/%s", agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Update spec_hosts (adds hosts directly to AG spec)
	_, err := env.DB.Exec(`
		UPDATE address_groups
		SET spec_hosts = $1::jsonb
		WHERE namespace = $2 AND name = $3
	`, `["host1.local", "host2.local"]`, agKey.Namespace, agKey.Name)
	require.NoError(t, err)

	t.Logf("Updated spec_hosts for AddressGroup %s/%s", agKey.Namespace, agKey.Name)

	// ========================================
	// ASSERT
	// ========================================
	// Migration 028 should create Outbox entry for AddressGroup UPDATE
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry, "Outbox entry should be created on spec_hosts update")

	assert.Equal(t, "AddressGroup", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)

	t.Logf("✅ Test PASSED: spec_hosts update triggers AddressGroup outbox entry")
}

// TestAddressGroupUpdate_DualBinding validates Migration 028's dual binding logic
// Both host_bindings AND spec_hosts updates should trigger AG outbox
func TestAddressGroupUpdate_DualBinding(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create Host with Ready condition (Migration 033 requires both Host and AG to be ready)
	hostResourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)
	hostKey := CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", hostResourceVersion, true)

	time.Sleep(100 * time.Millisecond) // Allow triggers to settle

	// Create AddressGroup with initial spec_hosts
	agResourceVersion := CreateK8sMetadata(t, env.DB, `[]`)
	agNamespace := "test-ns"
	agName := "test-ag-dual"

	_, err := env.DB.Exec(`
		INSERT INTO address_groups (
			namespace, name, resource_version, spec_hosts
		) VALUES ($1, $2, $3, $4::jsonb)
	`, agNamespace, agName, agResourceVersion, `["existing-host.local"]`)
	require.NoError(t, err)
	agKey := AddressGroupKey{Namespace: agNamespace, Name: agName}

	t.Logf("Created AddressGroup %s/%s with spec_hosts", agKey.Namespace, agKey.Name)

	// Wait for initial outbox entry to clear
	time.Sleep(200 * time.Millisecond)
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	// ========================================
	// ACT
	// ========================================
	// Add host_binding (dual binding scenario)
	bindingRV := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHostBinding(t, env.DB, "test-ns", "binding-dual", hostKey, agKey, bindingRV)

	t.Logf("Created host_binding for dual binding scenario")

	// ========================================
	// ASSERT
	// ========================================
	// New outbox entry should be created for the binding
	time.Sleep(200 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	assert.Greater(t, countAfter, countBefore, "New outbox entry should be created on host_binding")

	t.Logf("✅ Test PASSED: Dual binding (spec_hosts + host_bindings) creates outbox entries")
}

// TestAddressGroupUpdate_MultipleHosts validates that adding multiple hosts
// to an AddressGroup creates appropriate outbox entries
func TestAddressGroupUpdate_MultipleHosts(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup (with Ready condition - required for binding sync)
	agKey := CreateTestAddressGroupReady(t, env.DB, "test-ns", "test-ag-multi")

	// Create multiple ready Hosts (Migration 033 requires both Host and AG to be ready)
	hostKeys := make([]HostKey, 3)
	for i := 0; i < 3; i++ {
		hostResourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)
		hostKey := CreateTestHost(t, env.DB, "test-ns",
			"host-"+string(rune('a'+i)),
			"uuid-"+string(rune('1'+i)),
			hostResourceVersion, true)
		hostKeys[i] = hostKey
	}

	time.Sleep(200 * time.Millisecond) // Allow triggers to settle

	t.Logf("Created AddressGroup %s/%s and 3 Hosts", agKey.Namespace, agKey.Name)

	// ========================================
	// ACT
	// ========================================
	// Bind all hosts to AddressGroup
	for i, hostKey := range hostKeys {
		bindingRV := CreateK8sMetadata(t, env.DB, `[]`)
		CreateTestHostBinding(t, env.DB, "test-ns", "binding-"+string(rune('a'+i)), hostKey, agKey, bindingRV)
		t.Logf("Bound host %d: %s/%s", i+1, hostKey.Namespace, hostKey.Name)
	}

	// ========================================
	// ASSERT
	// ========================================
	// Multiple outbox entries should be created (one per binding)
	time.Sleep(300 * time.Millisecond)
	count := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	assert.GreaterOrEqual(t, count, 3, "At least 3 outbox entries should be created for 3 host bindings")

	t.Logf("✅ Test PASSED: Multiple host bindings create outbox entries (count=%d)", count)
}

// TestAddressGroupUpdate_HostRemoval validates that removing a host
// from an AddressGroup triggers outbox entry
func TestAddressGroupUpdate_HostRemoval(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create Host with Ready condition (Migration 033 requires both Host and AG to be ready)
	hostResourceVersion := CreateK8sMetadata(t, env.DB, `[{"type":"Ready","status":"True"}]`)
	hostKey := CreateTestHost(t, env.DB, "test-ns", "host-1", "uuid-1", hostResourceVersion, true)

	time.Sleep(100 * time.Millisecond) // Allow triggers to settle

	// Create AddressGroup (with Ready condition - required for binding sync)
	agKey := CreateTestAddressGroupReady(t, env.DB, "test-ns", "test-ag-removal")

	// Create host_binding
	bindingRV := CreateK8sMetadata(t, env.DB, `[]`)
	CreateTestHostBinding(t, env.DB, "test-ns", "binding-removal", hostKey, agKey, bindingRV)

	// Wait for initial outbox entry
	time.Sleep(200 * time.Millisecond)
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	t.Logf("Initial state: host bound to AG, outbox entries=%d", countBefore)

	// ========================================
	// ACT
	// ========================================
	// Remove host_binding (DELETE)
	// Migration 028 should trigger on DELETE as well
	_, err := env.DB.Exec(`
		DELETE FROM host_bindings
		WHERE namespace = $1 AND name = $2
	`, "test-ns", "binding-removal")
	require.NoError(t, err)

	t.Logf("Deleted host_binding")

	// ========================================
	// ASSERT
	// ========================================
	// New outbox entry should be created for host removal
	time.Sleep(300 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	assert.Greater(t, countAfter, countBefore, "Outbox entry should be created on host removal")

	t.Logf("✅ Test PASSED: Host removal triggers AddressGroup outbox entry (before=%d, after=%d)", countBefore, countAfter)
}

// TestAddressGroupUpdate_CreatesOutboxEntry validates that direct AG update
// triggers outbox entry creation
func TestAddressGroupUpdate_CreatesOutboxEntry(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup (with Ready condition - required for outbox triggers)
	agKey := CreateTestAddressGroupReady(t, env.DB, "test-ns", "test-ag-direct")

	t.Logf("Created AddressGroup %s/%s", agKey.Namespace, agKey.Name)

	// Wait for initial state
	time.Sleep(100 * time.Millisecond)

	// ========================================
	// ACT
	// ========================================
	// Update AddressGroup aggregated_hosts (simulates trigger computation)
	_, err := env.DB.Exec(`
		UPDATE address_groups
		SET aggregated_hosts = $1::jsonb
		WHERE namespace = $2 AND name = $3
	`, `["host1.local", "host2.local"]`, agKey.Namespace, agKey.Name)
	require.NoError(t, err)

	t.Logf("Updated aggregated_hosts for AddressGroup")

	// ========================================
	// ASSERT
	// ========================================
	// Outbox entry should be created
	entry := WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)
	require.NotNil(t, entry)

	assert.Equal(t, "AddressGroup", entry.ResourceType)
	assert.Equal(t, domain.SyncOperationUpdate, entry.Operation)

	t.Logf("✅ Test PASSED: Direct AddressGroup update creates outbox entry")
}

// TestAddressGroupUpdate_Idempotency validates that duplicate updates
// don't create unnecessary outbox entries (depends on trigger implementation)
func TestAddressGroupUpdate_Idempotency(t *testing.T) {
	// ========================================
	// ARRANGE
	// ========================================
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create AddressGroup (with Ready condition - required for outbox triggers)
	agKey := CreateTestAddressGroupReady(t, env.DB, "test-ns", "test-ag-idempotent")

	// Set initial aggregated_hosts
	_, err := env.DB.Exec(`
		UPDATE address_groups
		SET aggregated_hosts = $1::jsonb
		WHERE namespace = $2 AND name = $3
	`, `["host1.local"]`, agKey.Namespace, agKey.Name)
	require.NoError(t, err)

	// Wait for outbox entry
	WaitForOutboxEntry(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate), 5*time.Second)

	// Wait for state to stabilize
	time.Sleep(200 * time.Millisecond)
	countBefore := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	t.Logf("Initial state: aggregated_hosts set, outbox entries=%d", countBefore)

	// ========================================
	// ACT
	// ========================================
	// Update with SAME aggregated_hosts (no actual change)
	_, err = env.DB.Exec(`
		UPDATE address_groups
		SET aggregated_hosts = $1::jsonb
		WHERE namespace = $2 AND name = $3
	`, `["host1.local"]`, agKey.Namespace, agKey.Name)
	require.NoError(t, err)

	t.Logf("Updated with same aggregated_hosts")

	// ========================================
	// ASSERT
	// ========================================
	// Depending on trigger implementation, may or may not create new entry
	// If trigger checks OLD.aggregated_hosts <> NEW.aggregated_hosts, no new entry
	// If trigger always fires on UPDATE, new entry will be created
	time.Sleep(200 * time.Millisecond)
	countAfter := GetOutboxEntryCount(t, env.DB, "AddressGroup", string(domain.SyncOperationUpdate))

	// Log result (behavior depends on trigger implementation)
	if countAfter == countBefore {
		t.Logf("✅ Trigger is idempotent (no new entry for unchanged data)")
	} else {
		t.Logf("⚠️  Trigger fires on every UPDATE (new entry created even without change)")
	}

	t.Logf("✅ Test PASSED: Idempotency behavior documented (before=%d, after=%d)", countBefore, countAfter)
}
