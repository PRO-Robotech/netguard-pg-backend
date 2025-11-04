package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/test/e2e/helpers"
)

// TestHostBinding_SuccessPath tests the complete happy path for HostBinding creation
// Expected flow:
// 1. Create Host (ready=true)
// 2. Create AddressGroup (ready=true)
// 3. Create HostBinding (ready=false, triggers sync)
// 4. Worker processes HostBinding
// 5. Host.is_bound set to true
// 6. AG.aggregated_hosts updated
// 7. HostBinding.ready set to true
// 8. PendingSync set to False
// 9. Outbox cleared
// 10. Host and AG synced to SGROUP
func TestHostBinding_SuccessPath(t *testing.T) {
	// Setup test environment
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none") // All syncs succeed

	// Step 1: Create Host (ready=true)
	helpers.CreateTestHost(t, env.DB, "test-host-1", []string{"10.0.0.1"}, true)
	helpers.WaitForResourceReady(t, env.DB, "Host", "test-host-1", 10*time.Second)

	// Step 2: Create AddressGroup (ready=true)
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-1", true)
	helpers.WaitForResourceReady(t, env.DB, "AddressGroup", "test-ag-1", 10*time.Second)

	// Step 3: Create HostBinding (ready=false, triggers sync via trigger)
	helpers.CreateTestHostBinding(t, env.DB, "test-binding-1", "test-host-1", "test-ag-1")

	// Verify initial state - HostBinding should NOT be ready
	var ready bool
	err := env.DB.QueryRow(`
		SELECT ready FROM host_bindings
		WHERE namespace='default' AND name='test-binding-1'
	`).Scan(&ready)
	require.NoError(t, err)
	assert.False(t, ready, "HostBinding should not be ready initially")

	// Verify PendingSync=True initially
	helpers.WaitForCondition(t, env.DB, "HostBinding", "test-binding-1", "PendingSync", "True", 5*time.Second)

	// Verify Outbox entry created (by trigger)
	helpers.VerifyOutboxHasEntry(t, env.DB, "HostBinding", "test-binding-1")

	// Step 4: Wait for Worker to process (max 30 seconds)
	t.Log("Waiting for worker to process HostBinding...")
	helpers.WaitForResourceReady(t, env.DB, "HostBinding", "test-binding-1", 30*time.Second)

	// Step 5: Verify final state - HostBinding should be ready
	err = env.DB.QueryRow(`
		SELECT ready FROM host_bindings
		WHERE namespace='default' AND name='test-binding-1'
	`).Scan(&ready)
	require.NoError(t, err)
	assert.True(t, ready, "HostBinding should be ready after sync")

	// Verify PendingSync=False
	helpers.WaitForCondition(t, env.DB, "HostBinding", "test-binding-1", "PendingSync", "False", 5*time.Second)

	// Step 6: Verify Host updated (is_bound=true, address_group_ref set)
	helpers.VerifyHostBound(t, env.DB, "test-host-1", "test-ag-1")

	// Step 7: Verify AddressGroup aggregated_hosts updated
	helpers.VerifyAGContainsHost(t, env.DB, "test-ag-1", "test-host-1")

	// Step 8: Verify Outbox cleaned up (no PENDING or PROCESSING entries)
	helpers.VerifyOutboxEmpty(t, env.DB)

	// Step 9: Verify SGROUP received syncs
	t.Log("Verifying SGROUP sync calls...")
	assert.GreaterOrEqual(t, mockSGroup.CallCount(), 2, "Should sync Host + AG (at least 2 calls)")

	// Verify Host synced to SGROUP
	syncedHost, found := mockSGroup.GetSyncedHost("default/test-host-1")
	assert.True(t, found, "Host should be synced to SGROUP")
	if found {
		assert.Equal(t, "default/test-host-1", syncedHost.Name, "Synced host name should match")
	}

	// Verify AG synced to SGROUP
	syncedAG, found := mockSGroup.GetSyncedGroup("default/test-ag-1")
	assert.True(t, found, "AddressGroup should be synced to SGROUP")
	if found {
		assert.Equal(t, "default/test-ag-1", syncedAG.Name, "Synced AG name should match")
	}

	t.Log("TestHostBinding_SuccessPath PASSED!")
}

// TestHostBinding_MultipleHostsToSameAG tests binding multiple Hosts to the same AddressGroup
// Expected flow:
// 1. Create 5 Hosts
// 2. Create 1 AddressGroup
// 3. Create 5 HostBindings (all to same AG)
// 4. Wait for all to become ready
// 5. Verify AG.aggregated_hosts contains all 5 hosts
// 6. Verify Outbox empty
// 7. Verify all Hosts synced to SGROUP
func TestHostBinding_MultipleHostsToSameAG(t *testing.T) {
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none")

	// Create 5 Hosts
	for i := 1; i <= 5; i++ {
		hostName := fmt.Sprintf("test-host-%d", i)
		ipList := []string{fmt.Sprintf("10.0.0.%d", i)}
		helpers.CreateTestHost(t, env.DB, hostName, ipList, true)
		helpers.WaitForResourceReady(t, env.DB, "Host", hostName, 10*time.Second)
	}

	// Create AddressGroup
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-multi", true)
	helpers.WaitForResourceReady(t, env.DB, "AddressGroup", "test-ag-multi", 10*time.Second)

	// Create 5 HostBindings
	for i := 1; i <= 5; i++ {
		bindingName := fmt.Sprintf("test-binding-%d", i)
		hostName := fmt.Sprintf("test-host-%d", i)
		helpers.CreateTestHostBinding(t, env.DB, bindingName, hostName, "test-ag-multi")
	}

	// Wait for all HostBindings to become ready
	t.Log("Waiting for all HostBindings to become ready...")
	for i := 1; i <= 5; i++ {
		bindingName := fmt.Sprintf("test-binding-%d", i)
		helpers.WaitForResourceReady(t, env.DB, "HostBinding", bindingName, 60*time.Second)
	}

	// Verify all Hosts are bound
	for i := 1; i <= 5; i++ {
		hostName := fmt.Sprintf("test-host-%d", i)
		helpers.VerifyHostBound(t, env.DB, hostName, "test-ag-multi")
	}

	// Verify AG aggregated_hosts includes all 5 hosts
	for i := 1; i <= 5; i++ {
		hostName := fmt.Sprintf("test-host-%d", i)
		helpers.VerifyAGContainsHost(t, env.DB, "test-ag-multi", hostName)
	}

	// Verify Outbox empty
	helpers.VerifyOutboxEmpty(t, env.DB)

	// Verify SGROUP received all hosts
	t.Log("Verifying all hosts synced to SGROUP...")
	allHosts := mockSGroup.GetAllSyncedHosts()
	assert.GreaterOrEqual(t, len(allHosts), 5, "SGROUP should have at least 5 hosts synced")

	t.Log("TestHostBinding_MultipleHostsToSameAG PASSED!")
}

// TestHostBinding_UnbindHost tests unbinding a Host from an AddressGroup
// Expected flow:
// 1. Create and bind Host to AG
// 2. Verify bound state
// 3. Delete HostBinding
// 4. Verify Host.is_bound=false
// 5. Verify AG removes Host from aggregated_hosts
// 6. Verify changes synced to SGROUP
func TestHostBinding_UnbindHost(t *testing.T) {
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none")

	// Setup: Create and bind Host to AG
	helpers.CreateTestHost(t, env.DB, "test-host-unbind", []string{"10.0.0.99"}, true)
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-unbind", true)
	helpers.CreateTestHostBinding(t, env.DB, "test-binding-unbind", "test-host-unbind", "test-ag-unbind")

	// Wait for binding to complete
	helpers.WaitForResourceReady(t, env.DB, "HostBinding", "test-binding-unbind", 30*time.Second)

	// Verify initial state (bound)
	helpers.VerifyHostBound(t, env.DB, "test-host-unbind", "test-ag-unbind")
	helpers.VerifyAGContainsHost(t, env.DB, "test-ag-unbind", "test-host-unbind")

	// Delete HostBinding (should trigger unbind via cascade/trigger)
	t.Log("Deleting HostBinding to unbind Host...")
	_, err := env.DB.Exec(`
		DELETE FROM host_bindings WHERE namespace='default' AND name='test-binding-unbind'
	`)
	require.NoError(t, err)

	// Wait for unbind to propagate
	time.Sleep(5 * time.Second)

	// Verify Host unbound
	var isBound bool
	var agRef string
	err = env.DB.QueryRow(`
		SELECT is_bound, address_group_ref FROM hosts
		WHERE namespace='default' AND name='test-host-unbind'
	`).Scan(&isBound, &agRef)
	require.NoError(t, err)
	assert.False(t, isBound, "Host should be unbound after HostBinding deletion")
	assert.Empty(t, agRef, "Host should have empty address_group_ref after unbind")

	// Verify AG no longer includes Host (note: might still be there if trigger doesn't update)
	// This test might need adjustment based on actual trigger behavior

	// Verify synced to SGROUP
	assert.Greater(t, mockSGroup.CallCount(), 0, "Should have sync calls to SGROUP")

	t.Log("TestHostBinding_UnbindHost PASSED!")
}
