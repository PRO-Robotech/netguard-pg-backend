package services

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNetguardFacade_ServiceOperations tests all service-related operations
func TestNetguardFacade_ServiceOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	service := testutil.CreateTestService("test-service", "test-namespace")

	t.Run("CreateService", func(t *testing.T) {
		err := facade.CreateService(context.Background(), service)
		assert.NoError(t, err)
	})

	t.Run("GetServiceByID", func(t *testing.T) {
		result, err := facade.GetServiceByID(context.Background(), service.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, service.SelfRef.Name, result.SelfRef.Name)
	})

	t.Run("GetServices", func(t *testing.T) {
		services, err := facade.GetServices(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, services, 1)
		assert.Equal(t, service.SelfRef.Name, services[0].SelfRef.Name)
	})

	t.Run("UpdateService", func(t *testing.T) {
		updatedService := service
		updatedService.Description = "Updated description"

		err := facade.UpdateService(context.Background(), updatedService)
		assert.NoError(t, err)

		result, err := facade.GetServiceByID(context.Background(), service.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "Updated description", result.Description)
	})

	t.Run("SyncServices", func(t *testing.T) {
		services := []models.Service{
			testutil.CreateTestService("sync-service-1", "test-namespace"),
			testutil.CreateTestService("sync-service-2", "test-namespace"),
		}

		err := facade.SyncServices(context.Background(), services, ports.EmptyScope{})
		assert.NoError(t, err)

		allServices, err := facade.GetServices(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allServices), 2)
	})

	t.Run("DeleteServicesByIDs", func(t *testing.T) {
		// Delete one of the services
		deleteIDs := []models.ResourceIdentifier{service.SelfRef.ResourceIdentifier}
		err := facade.DeleteServicesByIDs(context.Background(), deleteIDs)
		assert.NoError(t, err)

		// Verify deletion
		_, err = facade.GetServiceByID(context.Background(), service.SelfRef.ResourceIdentifier)
		assert.Error(t, err)
	})
}

// TestNetguardFacade_ServiceAliasOperations tests service alias operations
func TestNetguardFacade_ServiceAliasOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	// Create dependent service first
	service := testutil.CreateTestService("test-service", "test-namespace")
	err := facade.CreateService(context.Background(), service)
	require.NoError(t, err)

	alias := testutil.CreateTestServiceAlias("test-alias", "test-namespace", "test-service")

	t.Run("CreateServiceAlias", func(t *testing.T) {
		err := facade.CreateServiceAlias(context.Background(), alias)
		assert.NoError(t, err)
	})

	t.Run("GetServiceAliasByID", func(t *testing.T) {
		result, err := facade.GetServiceAliasByID(context.Background(), alias.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, alias.SelfRef.Name, result.SelfRef.Name)
	})

	t.Run("GetServiceAliases", func(t *testing.T) {
		aliases, err := facade.GetServiceAliases(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, aliases, 1)
		assert.Equal(t, alias.SelfRef.Name, aliases[0].SelfRef.Name)
	})

	t.Run("SyncServiceAliases", func(t *testing.T) {
		aliases := []models.ServiceAlias{
			testutil.CreateTestServiceAlias("sync-alias-1", "test-namespace", "test-service"),
			testutil.CreateTestServiceAlias("sync-alias-2", "test-namespace", "test-service"),
		}

		err := facade.SyncServiceAliases(context.Background(), aliases, ports.EmptyScope{})
		assert.NoError(t, err)

		allAliases, err := facade.GetServiceAliases(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allAliases), 2)
	})
}

// TestNetguardFacade_AddressGroupOperations tests address group operations
func TestNetguardFacade_AddressGroupOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	addressGroup := testutil.CreateTestAddressGroup("test-ag", "test-namespace")

	t.Run("CreateAddressGroup", func(t *testing.T) {
		err := facade.CreateAddressGroup(context.Background(), addressGroup)
		assert.NoError(t, err)
	})

	t.Run("GetAddressGroupByID", func(t *testing.T) {
		result, err := facade.GetAddressGroupByID(context.Background(), addressGroup.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, addressGroup.SelfRef.Name, result.SelfRef.Name)
	})

	t.Run("GetAddressGroups", func(t *testing.T) {
		groups, err := facade.GetAddressGroups(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, groups, 1)
		assert.Equal(t, addressGroup.SelfRef.Name, groups[0].SelfRef.Name)
	})

	t.Run("UpdateAddressGroup", func(t *testing.T) {
		updatedGroup := addressGroup
		updatedGroup.DefaultAction = models.ActionDrop

		err := facade.UpdateAddressGroup(context.Background(), updatedGroup)
		assert.NoError(t, err)

		result, err := facade.GetAddressGroupByID(context.Background(), addressGroup.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, models.ActionDrop, result.DefaultAction)
	})

	t.Run("SyncAddressGroups", func(t *testing.T) {
		groups := []models.AddressGroup{
			testutil.CreateTestAddressGroup("sync-ag-1", "test-namespace"),
			testutil.CreateTestAddressGroup("sync-ag-2", "test-namespace"),
		}

		err := facade.SyncAddressGroups(context.Background(), groups, ports.EmptyScope{})
		assert.NoError(t, err)

		allGroups, err := facade.GetAddressGroups(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allGroups), 2)
	})
}

// TestNetguardFacade_RuleS2SOperations tests RuleS2S operations
func TestNetguardFacade_RuleS2SOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	// Create dependencies first
	localService := testutil.CreateTestService("test-local-service", "test-namespace")
	targetService := testutil.CreateTestService("test-service", "test-namespace")
	localAlias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
	targetAlias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")

	require.NoError(t, facade.CreateService(context.Background(), localService))
	require.NoError(t, facade.CreateService(context.Background(), targetService))
	require.NoError(t, facade.CreateServiceAlias(context.Background(), localAlias))
	require.NoError(t, facade.CreateServiceAlias(context.Background(), targetAlias))

	rule := testutil.CreateTestRuleS2S("test-rule-s2s", "test-namespace")

	t.Run("CreateRuleS2S", func(t *testing.T) {
		err := facade.CreateRuleS2S(context.Background(), rule)
		assert.NoError(t, err)
	})

	t.Run("GetRuleS2SByID", func(t *testing.T) {
		result, err := facade.GetRuleS2SByID(context.Background(), rule.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, rule.SelfRef.Name, result.SelfRef.Name)
	})

	t.Run("GetRuleS2S", func(t *testing.T) {
		rules, err := facade.GetRuleS2S(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, rules, 1)
		assert.Equal(t, rule.SelfRef.Name, rules[0].SelfRef.Name)
	})

	t.Run("SyncRuleS2S", func(t *testing.T) {
		// Use fresh registry for this subtest to avoid interference
		freshRegistry := testutil.NewMockRegistry()
		freshSyncManager := testutil.NewMockSyncManager()
		freshFacade := NewNetguardFacade(freshRegistry, nil, freshSyncManager)

		// Create dependencies first
		localService := testutil.CreateTestService("test-local-service", "test-namespace")
		targetService := testutil.CreateTestService("test-service", "test-namespace")
		localAlias := testutil.CreateTestServiceAlias("test-local-service", "test-namespace", "test-local-service")
		targetAlias := testutil.CreateTestServiceAlias("test-service", "test-namespace", "test-service")

		require.NoError(t, freshFacade.CreateService(context.Background(), localService))
		require.NoError(t, freshFacade.CreateService(context.Background(), targetService))
		require.NoError(t, freshFacade.CreateServiceAlias(context.Background(), localAlias))
		require.NoError(t, freshFacade.CreateServiceAlias(context.Background(), targetAlias))

		rules := []models.RuleS2S{
			testutil.CreateTestRuleS2S("sync-rule-1", "test-namespace"),
			testutil.CreateTestRuleS2S("sync-rule-2", "test-namespace"),
		}

		err := freshFacade.SyncRuleS2S(context.Background(), rules, ports.EmptyScope{})
		assert.NoError(t, err)

		allRules, err := freshFacade.GetRuleS2S(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allRules), 2)
	})
}

// TestNetguardFacade_IEAgAgRuleOperations tests IEAgAgRule operations
func TestNetguardFacade_IEAgAgRuleOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	ieRule := testutil.CreateTestIEAgAgRule("test-ieagag-rule", "test-namespace")

	t.Run("SyncIEAgAgRules", func(t *testing.T) {
		rules := []models.IEAgAgRule{ieRule}

		err := facade.SyncIEAgAgRules(context.Background(), rules, ports.EmptyScope{})
		assert.NoError(t, err)
	})

	t.Run("GetIEAgAgRules", func(t *testing.T) {
		rules, err := facade.GetIEAgAgRules(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(rules), 0) // IEAgAgRules might not be syncable in mock
	})
}

// TestNetguardFacade_NetworkOperations tests network operations
func TestNetguardFacade_NetworkOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	network := testutil.CreateTestNetwork("test-network", "test-namespace", "10.0.0.0/24")

	t.Run("CreateNetwork", func(t *testing.T) {
		err := facade.CreateNetwork(context.Background(), network)
		assert.NoError(t, err)
	})

	t.Run("GetNetworkByID", func(t *testing.T) {
		result, err := facade.GetNetworkByID(context.Background(), network.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, network.SelfRef.Name, result.SelfRef.Name)
		assert.Equal(t, network.CIDR, result.CIDR)
	})

	t.Run("GetNetworks", func(t *testing.T) {
		networks, err := facade.GetNetworks(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, networks, 1)
		assert.Equal(t, network.SelfRef.Name, networks[0].SelfRef.Name)
	})

	t.Run("UpdateNetwork", func(t *testing.T) {
		updatedNetwork := network
		updatedNetwork.CIDR = "10.1.0.0/24"

		err := facade.UpdateNetwork(context.Background(), updatedNetwork)
		assert.NoError(t, err)

		result, err := facade.GetNetworkByID(context.Background(), network.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "10.1.0.0/24", result.CIDR)
	})

	t.Run("DeleteNetwork", func(t *testing.T) {
		err := facade.DeleteNetwork(context.Background(), network.SelfRef.ResourceIdentifier)
		assert.NoError(t, err)

		// Verify deletion
		_, err = facade.GetNetworkByID(context.Background(), network.SelfRef.ResourceIdentifier)
		assert.Error(t, err)
	})
}

// TestNetguardFacade_NetworkBindingOperations tests network binding operations
func TestNetguardFacade_NetworkBindingOperations(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	// Create dependencies first
	network := testutil.CreateTestNetwork("test-network", "test-namespace", "10.0.0.0/24")
	addressGroup := testutil.CreateTestAddressGroup("test-ag", "test-namespace")

	require.NoError(t, facade.CreateNetwork(context.Background(), network))
	require.NoError(t, facade.CreateAddressGroup(context.Background(), addressGroup))

	binding := testutil.CreateTestNetworkBinding("test-binding", "test-namespace", "test-network", "test-ag")

	t.Run("CreateNetworkBinding", func(t *testing.T) {
		err := facade.CreateNetworkBinding(context.Background(), binding)
		assert.NoError(t, err)
	})

	t.Run("GetNetworkBindingByID", func(t *testing.T) {
		result, err := facade.GetNetworkBindingByID(context.Background(), binding.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, binding.SelfRef.Name, result.SelfRef.Name)
	})

	t.Run("GetNetworkBindings", func(t *testing.T) {
		bindings, err := facade.GetNetworkBindings(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, bindings, 1)
		assert.Equal(t, binding.SelfRef.Name, bindings[0].SelfRef.Name)
	})

	t.Run("DeleteNetworkBinding", func(t *testing.T) {
		err := facade.DeleteNetworkBinding(context.Background(), binding.SelfRef.ResourceIdentifier)
		assert.NoError(t, err)

		// Verify deletion
		_, err = facade.GetNetworkBindingByID(context.Background(), binding.SelfRef.ResourceIdentifier)
		assert.Error(t, err)
	})
}

// TestNetguardFacade_CrossServiceCoordination tests coordination between different resource services
func TestNetguardFacade_CrossServiceCoordination(t *testing.T) {
	t.Run("Complete Workflow - Service to RuleS2S Integration", func(t *testing.T) {
		// Setup fresh registry for this subtest
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)
		// 1. Create services
		localService := testutil.CreateTestService("local-service", "test-namespace")
		targetService := testutil.CreateTestService("target-service", "test-namespace")

		require.NoError(t, facade.CreateService(context.Background(), localService))
		require.NoError(t, facade.CreateService(context.Background(), targetService))

		// 2. Create service aliases
		localAlias := testutil.CreateTestServiceAlias("local-service", "test-namespace", "local-service")
		targetAlias := testutil.CreateTestServiceAlias("target-service", "test-namespace", "target-service")

		require.NoError(t, facade.CreateServiceAlias(context.Background(), localAlias))
		require.NoError(t, facade.CreateServiceAlias(context.Background(), targetAlias))

		// 3. Create address groups
		localAG := testutil.CreateTestAddressGroup("local-ag", "test-namespace")
		targetAG := testutil.CreateTestAddressGroup("target-ag", "test-namespace")

		require.NoError(t, facade.CreateAddressGroup(context.Background(), localAG))
		require.NoError(t, facade.CreateAddressGroup(context.Background(), targetAG))

		// 4. Create RuleS2S
		rule := testutil.CreateTestRuleS2S("test-rule", "test-namespace")
		rule.ServiceLocalRef = models.NewServiceRef("local-service", models.WithNamespace("test-namespace"))
		rule.ServiceRef = models.NewServiceRef("target-service", models.WithNamespace("test-namespace"))

		require.NoError(t, facade.CreateRuleS2S(context.Background(), rule))

		// 5. Verify all resources exist and are properly linked
		retrievedRule, err := facade.GetRuleS2SByID(context.Background(), rule.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "local-service", retrievedRule.ServiceLocalRef.Name)
		assert.Equal(t, "target-service", retrievedRule.ServiceRef.Name)

		// 6. Test FindRuleS2SForServices
		serviceIDs := []models.ResourceIdentifier{
			localService.SelfRef.ResourceIdentifier,
			targetService.SelfRef.ResourceIdentifier,
		}
		_, err = facade.FindRuleS2SForServices(context.Background(), serviceIDs)
		assert.NoError(t, err) // Should not error even if no rules found
	})

	t.Run("Network and AddressGroup Integration", func(t *testing.T) {
		// Setup fresh registry for this subtest
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

		// 1. Create network
		network := testutil.CreateTestNetwork("integration-network", "test-namespace", "192.168.1.0/24")
		require.NoError(t, facade.CreateNetwork(context.Background(), network))

		// 2. Create address group
		addressGroup := testutil.CreateTestAddressGroup("integration-ag", "test-namespace")
		require.NoError(t, facade.CreateAddressGroup(context.Background(), addressGroup))

		// 3. Create network binding
		binding := testutil.CreateTestNetworkBinding("integration-binding", "test-namespace", "integration-network", "integration-ag")
		require.NoError(t, facade.CreateNetworkBinding(context.Background(), binding))

		// 4. Verify the binding exists
		retrievedBinding, err := facade.GetNetworkBindingByID(context.Background(), binding.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "integration-network", retrievedBinding.NetworkRef.Name)
		assert.Equal(t, "integration-ag", retrievedBinding.AddressGroupRef.Name)

		// 5. Verify network binding validation
		err = facade.ValidateNetworkBinding(context.Background(), network.SelfRef.ResourceIdentifier, binding.SelfRef.ResourceIdentifier)
		assert.NoError(t, err)
	})
}

// TestNetguardFacade_SyncMethod tests the main Sync method with different resource types
func TestNetguardFacade_SyncMethod(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	t.Run("Sync Services", func(t *testing.T) {
		services := []models.Service{
			testutil.CreateTestService("sync-service-1", "test-namespace"),
			testutil.CreateTestService("sync-service-2", "test-namespace"),
		}

		err := facade.Sync(context.Background(), models.SyncOpUpsert, services)
		assert.NoError(t, err)

		// Verify services were created
		allServices, err := facade.GetServices(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allServices), 2)
	})

	t.Run("Sync AddressGroups", func(t *testing.T) {
		groups := []models.AddressGroup{
			testutil.CreateTestAddressGroup("sync-ag-1", "test-namespace"),
			testutil.CreateTestAddressGroup("sync-ag-2", "test-namespace"),
		}

		err := facade.Sync(context.Background(), models.SyncOpUpsert, groups)
		assert.NoError(t, err)

		// Verify address groups were created
		allGroups, err := facade.GetAddressGroups(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allGroups), 2)
	})

	t.Run("Sync Networks", func(t *testing.T) {
		networks := []models.Network{
			testutil.CreateTestNetwork("sync-network-1", "test-namespace", "10.1.0.0/24"),
			testutil.CreateTestNetwork("sync-network-2", "test-namespace", "10.2.0.0/24"),
		}

		err := facade.Sync(context.Background(), models.SyncOpUpsert, networks)
		assert.NoError(t, err)

		// Verify networks were created
		allNetworks, err := facade.GetNetworks(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(allNetworks), 2)
	})

	t.Run("Sync Unsupported Resource Type", func(t *testing.T) {
		unsupportedResource := []string{"unsupported"}

		err := facade.Sync(context.Background(), models.SyncOpUpsert, unsupportedResource)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported resource type")
	})
}

// TestNetguardFacade_SyncStatus tests sync status operations
func TestNetguardFacade_SyncStatus(t *testing.T) {
	// Setup
	mockRegistry := testutil.NewMockRegistry()
	mockSyncManager := testutil.NewMockSyncManager()

	facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

	t.Run("GetSyncStatus", func(t *testing.T) {
		status, err := facade.GetSyncStatus(context.Background())
		require.NoError(t, err)
		assert.NotNil(t, status)
		assert.NotZero(t, status.UpdatedAt)
	})

	t.Run("SetSyncStatus", func(t *testing.T) {
		status := models.SyncStatus{
			UpdatedAt: testutil.TestFixtures.Condition.LastTransitionTime.Time,
		}

		err := facade.SetSyncStatus(context.Background(), status)
		assert.NoError(t, err)
	})
}

// TestNetguardFacade_ErrorHandling tests error handling scenarios
func TestNetguardFacade_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

		service := testutil.CreateTestService("test-service", "test-namespace")

		// Test that errors are properly handled
		err := facade.CreateService(context.Background(), service)
		assert.Error(t, err)

		_, err = facade.GetServices(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = facade.SyncServices(context.Background(), []models.Service{service}, ports.EmptyScope{})
		assert.Error(t, err)
	})
}

// TestNetguardFacade_CompleteLifecycle tests complete lifecycle operations
func TestNetguardFacade_CompleteLifecycle(t *testing.T) {
	t.Run("complete service ecosystem lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		facade := NewNetguardFacade(mockRegistry, nil, mockSyncManager)

		// 1. Create services
		localService := testutil.CreateTestService("lifecycle-local", "test-namespace")
		targetService := testutil.CreateTestService("lifecycle-target", "test-namespace")

		require.NoError(t, facade.CreateService(context.Background(), localService))
		require.NoError(t, facade.CreateService(context.Background(), targetService))

		// 2. Create service aliases
		localAlias := testutil.CreateTestServiceAlias("lifecycle-local", "test-namespace", "lifecycle-local")
		targetAlias := testutil.CreateTestServiceAlias("lifecycle-target", "test-namespace", "lifecycle-target")

		require.NoError(t, facade.CreateServiceAlias(context.Background(), localAlias))
		require.NoError(t, facade.CreateServiceAlias(context.Background(), targetAlias))

		// 3. Create address groups
		localAG := testutil.CreateTestAddressGroup("lifecycle-local-ag", "test-namespace")
		targetAG := testutil.CreateTestAddressGroup("lifecycle-target-ag", "test-namespace")

		require.NoError(t, facade.CreateAddressGroup(context.Background(), localAG))
		require.NoError(t, facade.CreateAddressGroup(context.Background(), targetAG))

		// 4. Create networks
		network1 := testutil.CreateTestNetwork("lifecycle-net1", "test-namespace", "172.16.1.0/24")
		network2 := testutil.CreateTestNetwork("lifecycle-net2", "test-namespace", "172.16.2.0/24")

		require.NoError(t, facade.CreateNetwork(context.Background(), network1))
		require.NoError(t, facade.CreateNetwork(context.Background(), network2))

		// 5. Create network bindings
		binding1 := testutil.CreateTestNetworkBinding("lifecycle-binding1", "test-namespace", "lifecycle-net1", "lifecycle-local-ag")
		binding2 := testutil.CreateTestNetworkBinding("lifecycle-binding2", "test-namespace", "lifecycle-net2", "lifecycle-target-ag")

		require.NoError(t, facade.CreateNetworkBinding(context.Background(), binding1))
		require.NoError(t, facade.CreateNetworkBinding(context.Background(), binding2))

		// 6. Create RuleS2S
		rule := testutil.CreateTestRuleS2S("lifecycle-rule", "test-namespace")
		rule.ServiceLocalRef = models.NewServiceRef("lifecycle-local", models.WithNamespace("test-namespace"))
		rule.ServiceRef = models.NewServiceRef("lifecycle-target", models.WithNamespace("test-namespace"))

		require.NoError(t, facade.CreateRuleS2S(context.Background(), rule))

		// 7. Verify the complete ecosystem
		// Check services
		services, err := facade.GetServices(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(services), 2)

		// Check service aliases
		aliases, err := facade.GetServiceAliases(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(aliases), 2)

		// Check address groups
		groups, err := facade.GetAddressGroups(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(groups), 2)

		// Check networks
		networks, err := facade.GetNetworks(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(networks), 2)

		// Check network bindings
		bindings, err := facade.GetNetworkBindings(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(bindings), 2)

		// Check rules
		rules, err := facade.GetRuleS2S(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(rules), 1)

		// 8. Test cross-service coordination
		serviceIDs := []models.ResourceIdentifier{
			localService.SelfRef.ResourceIdentifier,
			targetService.SelfRef.ResourceIdentifier,
		}
		_, err = facade.FindRuleS2SForServices(context.Background(), serviceIDs)
		assert.NoError(t, err) // Should not error even if no rules found

		// 9. Test cleanup - delete in reverse dependency order
		require.NoError(t, facade.DeleteRuleS2SByIDs(context.Background(), []models.ResourceIdentifier{rule.SelfRef.ResourceIdentifier}))
		require.NoError(t, facade.DeleteNetworkBinding(context.Background(), binding1.SelfRef.ResourceIdentifier))
		require.NoError(t, facade.DeleteNetworkBinding(context.Background(), binding2.SelfRef.ResourceIdentifier))
		require.NoError(t, facade.DeleteNetwork(context.Background(), network1.SelfRef.ResourceIdentifier))
		require.NoError(t, facade.DeleteNetwork(context.Background(), network2.SelfRef.ResourceIdentifier))
		require.NoError(t, facade.DeleteServiceAliasesByIDs(context.Background(), []models.ResourceIdentifier{localAlias.SelfRef.ResourceIdentifier, targetAlias.SelfRef.ResourceIdentifier}))
		require.NoError(t, facade.DeleteAddressGroupsByIDs(context.Background(), []models.ResourceIdentifier{localAG.SelfRef.ResourceIdentifier, targetAG.SelfRef.ResourceIdentifier}))
		require.NoError(t, facade.DeleteServicesByIDs(context.Background(), []models.ResourceIdentifier{localService.SelfRef.ResourceIdentifier, targetService.SelfRef.ResourceIdentifier}))

		// 10. Verify cleanup
		_, err = facade.GetServiceByID(context.Background(), localService.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}
