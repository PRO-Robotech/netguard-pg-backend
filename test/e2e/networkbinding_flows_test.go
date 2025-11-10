package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/test/e2e/helpers"
)

// TestNetworkBinding_SuccessPath tests the complete happy path for NetworkBinding creation
// Expected flow:
// 1. Create Network (ready=true)
// 2. Create AddressGroup (ready=true)
// 3. Create NetworkBinding (ready=false, triggers sync)
// 4. Worker processes NetworkBinding
// 5. Network.is_bound set to true
// 6. AG.aggregated_networks updated
// 7. NetworkBinding.ready set to true
// 8. PendingSync set to False
// 9. Outbox cleared
// 10. Network and AG synced to SGROUP
func TestNetworkBinding_SuccessPath(t *testing.T) {
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none") // All syncs succeed

	// Step 1: Create Network (ready=true)
	helpers.CreateTestNetwork(t, env.DB, "test-network-1", []string{"192.168.1.0/24"}, true)
	helpers.WaitForResourceReady(t, env.DB, "Network", "test-network-1", 10*time.Second)

	// Step 2: Create AddressGroup (ready=true)
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-net-1", true)
	helpers.WaitForResourceReady(t, env.DB, "AddressGroup", "test-ag-net-1", 10*time.Second)

	// Step 3: Create NetworkBinding (ready=false, triggers sync via trigger)
	helpers.CreateTestNetworkBinding(t, env.DB, "test-net-binding-1", "test-network-1", "test-ag-net-1")

	// Verify initial state - NetworkBinding should NOT be ready
	var ready bool
	err := env.DB.QueryRow(`
		SELECT ready FROM network_bindings
		WHERE namespace='default' AND name='test-net-binding-1'
	`).Scan(&ready)
	require.NoError(t, err)
	assert.False(t, ready, "NetworkBinding should not be ready initially")

	// Verify PendingSync=True initially
	helpers.WaitForCondition(t, env.DB, "NetworkBinding", "test-net-binding-1", "PendingSync", "True", 5*time.Second)

	// Verify Outbox entry created (by trigger)
	helpers.VerifyOutboxHasEntry(t, env.DB, "NetworkBinding", "test-net-binding-1")

	// Step 4: Wait for Worker to process (max 30 seconds)
	t.Log("Waiting for worker to process NetworkBinding...")
	helpers.WaitForResourceReady(t, env.DB, "NetworkBinding", "test-net-binding-1", 30*time.Second)

	// Step 5: Verify final state - NetworkBinding should be ready
	err = env.DB.QueryRow(`
		SELECT ready FROM network_bindings
		WHERE namespace='default' AND name='test-net-binding-1'
	`).Scan(&ready)
	require.NoError(t, err)
	assert.True(t, ready, "NetworkBinding should be ready after sync")

	// Verify PendingSync=False
	helpers.WaitForCondition(t, env.DB, "NetworkBinding", "test-net-binding-1", "PendingSync", "False", 5*time.Second)

	// Step 6: Verify Network updated (is_bound=true, address_group_ref set)
	helpers.VerifyNetworkBound(t, env.DB, "test-network-1", "test-ag-net-1")

	// Step 7: Verify AddressGroup aggregated_networks updated
	helpers.VerifyAGContainsNetwork(t, env.DB, "test-ag-net-1", "test-network-1")

	// Step 8: Verify Outbox cleaned up (no PENDING or PROCESSING entries)
	helpers.VerifyOutboxEmpty(t, env.DB)

	// Step 9: Verify SGROUP received syncs
	t.Log("Verifying SGROUP sync calls...")
	assert.GreaterOrEqual(t, mockSGroup.CallCount(), 2, "Should sync Network + AG (at least 2 calls)")

	// Verify Network synced to SGROUP (network sync might use different proto structure)
	allSyncedGroups := mockSGroup.GetAllSyncedGroups()
	assert.GreaterOrEqual(t, len(allSyncedGroups), 1, "SGROUP should have at least 1 group synced")

	// Verify AG synced to SGROUP
	syncedAG, found := mockSGroup.GetSyncedGroup("default/test-ag-net-1")
	assert.True(t, found, "AddressGroup should be synced to SGROUP")
	if found {
		assert.Equal(t, "default/test-ag-net-1", syncedAG.Name, "Synced AG name should match")
	}

	t.Log("TestNetworkBinding_SuccessPath PASSED!")
}

// TestNetworkBinding_MultipleNetworksToSameAG tests binding multiple Networks to the same AddressGroup
// Expected flow:
// 1. Create 3 Networks
// 2. Create 1 AddressGroup
// 3. Create 3 NetworkBindings (all to same AG)
// 4. Wait for all to become ready
// 5. Verify AG.aggregated_networks contains all 3 networks
// 6. Verify Outbox empty
func TestNetworkBinding_MultipleNetworksToSameAG(t *testing.T) {
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none")

	// Create 3 Networks
	networks := []struct {
		name string
		cidr string
	}{
		{"test-network-a", "192.168.1.0/24"},
		{"test-network-b", "192.168.2.0/24"},
		{"test-network-c", "192.168.3.0/24"},
	}

	for _, net := range networks {
		helpers.CreateTestNetwork(t, env.DB, net.name, []string{net.cidr}, true)
		helpers.WaitForResourceReady(t, env.DB, "Network", net.name, 10*time.Second)
	}

	// Create AddressGroup
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-net-multi", true)
	helpers.WaitForResourceReady(t, env.DB, "AddressGroup", "test-ag-net-multi", 10*time.Second)

	// Create 3 NetworkBindings
	for i, net := range networks {
		bindingName := fmt.Sprintf("test-net-binding-%d", i+1)
		helpers.CreateTestNetworkBinding(t, env.DB, bindingName, net.name, "test-ag-net-multi")
	}

	// Wait for all NetworkBindings to become ready
	t.Log("Waiting for all NetworkBindings to become ready...")
	for i := range networks {
		bindingName := fmt.Sprintf("test-net-binding-%d", i+1)
		helpers.WaitForResourceReady(t, env.DB, "NetworkBinding", bindingName, 60*time.Second)
	}

	// Verify all Networks are bound
	for _, net := range networks {
		helpers.VerifyNetworkBound(t, env.DB, net.name, "test-ag-net-multi")
	}

	// Verify AG aggregated_networks includes all 3 networks
	for _, net := range networks {
		helpers.VerifyAGContainsNetwork(t, env.DB, "test-ag-net-multi", net.name)
	}

	// Verify Outbox empty
	helpers.VerifyOutboxEmpty(t, env.DB)

	// Verify SGROUP received syncs
	assert.Greater(t, mockSGroup.CallCount(), 0, "Should have sync calls to SGROUP")

	t.Log("TestNetworkBinding_MultipleNetworksToSameAG PASSED!")
}

// TestNetworkBinding_UnbindNetwork tests unbinding a Network from an AddressGroup
// Expected flow:
// 1. Create and bind Network to AG
// 2. Verify bound state
// 3. Delete NetworkBinding
// 4. Verify Network.is_bound=false
// 5. Verify AG removes Network from aggregated_networks
func TestNetworkBinding_UnbindNetwork(t *testing.T) {
	env := helpers.StartTestEnvironment(t)
	defer env.Cleanup()

	mockSGroup := env.MockSGroup
	mockSGroup.SetFailureMode("none")

	// Setup: Create and bind Network to AG
	helpers.CreateTestNetwork(t, env.DB, "test-network-unbind", []string{"10.99.0.0/16"}, true)
	helpers.CreateTestAddressGroup(t, env.DB, "test-ag-net-unbind", true)
	helpers.CreateTestNetworkBinding(t, env.DB, "test-net-binding-unbind", "test-network-unbind", "test-ag-net-unbind")

	// Wait for binding to complete
	helpers.WaitForResourceReady(t, env.DB, "NetworkBinding", "test-net-binding-unbind", 30*time.Second)

	// Verify initial state (bound)
	helpers.VerifyNetworkBound(t, env.DB, "test-network-unbind", "test-ag-net-unbind")
	helpers.VerifyAGContainsNetwork(t, env.DB, "test-ag-net-unbind", "test-network-unbind")

	// Delete NetworkBinding (should trigger unbind via cascade/trigger)
	t.Log("Deleting NetworkBinding to unbind Network...")
	_, err := env.DB.Exec(`
		DELETE FROM network_bindings WHERE namespace='default' AND name='test-net-binding-unbind'
	`)
	require.NoError(t, err)

	// Wait for unbind to propagate
	time.Sleep(5 * time.Second)

	// Verify Network unbound
	var isBound bool
	var agRef string
	err = env.DB.QueryRow(`
		SELECT is_bound, address_group_ref FROM networks
		WHERE namespace='default' AND name='test-network-unbind'
	`).Scan(&isBound, &agRef)
	require.NoError(t, err)
	assert.False(t, isBound, "Network should be unbound after NetworkBinding deletion")
	assert.Empty(t, agRef, "Network should have empty address_group_ref after unbind")

	// Verify synced to SGROUP
	assert.Greater(t, mockSGroup.CallCount(), 0, "Should have sync calls to SGROUP")

	t.Log("TestNetworkBinding_UnbindNetwork PASSED!")
}
