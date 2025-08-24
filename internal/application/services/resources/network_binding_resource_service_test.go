package resources

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/services/resources/testutil"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkBindingResourceService_ListNetworkBindings(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		scope       ports.Scope
		expectedLen int
	}{
		{
			name: "list all network bindings",
			setupData: map[string]interface{}{
				"networkbinding_test-namespace/test-network-binding-1": &testutil.TestFixtures.NetworkBinding,
				"networkbinding_test-namespace/test-network-binding-2": func() *models.NetworkBinding {
					nb := testutil.TestFixtures.NetworkBinding
					nb.SelfRef.ResourceIdentifier.Name = "test-network-binding-2"
					return &nb
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "list network bindings from empty registry",
			setupData:   map[string]interface{}{},
			scope:       ports.EmptyScope{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()
			networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

			// Execute
			bindings, err := service.ListNetworkBindings(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, bindings, tt.expectedLen)

			if tt.expectedLen > 0 {
				// Verify basic structure of returned network bindings
				assert.NotEmpty(t, bindings[0].SelfRef.Name)
				assert.NotEmpty(t, bindings[0].SelfRef.Namespace)
			}
		})
	}
}

func TestNetworkBindingResourceService_GetNetworkBinding(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resourceID  models.ResourceIdentifier
		expectError bool
		expectedNB  *models.NetworkBinding
	}{
		{
			name: "get existing network binding",
			setupData: map[string]interface{}{
				"networkbinding_test-namespace/test-network-binding": &testutil.TestFixtures.NetworkBinding,
			},
			resourceID:  testutil.TestFixtures.NetworkBinding.SelfRef.ResourceIdentifier,
			expectError: false,
			expectedNB:  &testutil.TestFixtures.NetworkBinding,
		},
		{
			name:        "get non-existent network binding",
			setupData:   map[string]interface{}{},
			resourceID:  testutil.TestFixtures.NetworkBinding.SelfRef.ResourceIdentifier,
			expectError: true,
			expectedNB:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()
			networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

			// Execute
			result, err := service.GetNetworkBinding(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedNB.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedNB.SelfRef.Namespace, result.SelfRef.Namespace)
			}
		})
	}
}

func TestNetworkBindingResourceService_CreateNetworkBinding(t *testing.T) {
	tests := []struct {
		name           string
		setupData      map[string]interface{}
		networkBinding models.NetworkBinding
		expectError    bool
	}{
		{
			name: "create valid network binding",
			setupData: map[string]interface{}{
				// Add required network and address group for validation
				"network_test-namespace/test-network":            &testutil.TestFixtures.Network,
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
			},
			networkBinding: testutil.TestFixtures.NetworkBinding,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()
			networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

			// Execute
			err := service.CreateNetworkBinding(context.Background(), &tt.networkBinding)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network binding was created by attempting to retrieve it
				created, getErr := service.GetNetworkBinding(context.Background(), tt.networkBinding.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.networkBinding.SelfRef.Name, created.SelfRef.Name)
				assert.Equal(t, tt.networkBinding.SelfRef.Namespace, created.SelfRef.Namespace)
			}
		})
	}
}

func TestNetworkBindingResourceService_UpdateNetworkBinding(t *testing.T) {
	tests := []struct {
		name           string
		setupData      map[string]interface{}
		networkBinding models.NetworkBinding
		expectError    bool
	}{
		{
			name: "update existing network binding",
			setupData: map[string]interface{}{
				"networkbinding_test-namespace/test-network-binding": &testutil.TestFixtures.NetworkBinding,
				"network_test-namespace/test-network":                &testutil.TestFixtures.Network,
				"addressgroup_test-namespace/test-address-group":     &testutil.TestFixtures.AddressGroup,
			},
			networkBinding: testutil.TestFixtures.NetworkBinding,
			expectError:    false,
		},
		{
			name: "update non-existent network binding",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network":            &testutil.TestFixtures.Network,
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
			},
			networkBinding: testutil.TestFixtures.NetworkBinding,
			expectError:    true, // Update operations require existing binding
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()
			networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

			// Execute
			err := service.UpdateNetworkBinding(context.Background(), &tt.networkBinding)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network binding was updated by retrieving it
				updated, getErr := service.GetNetworkBinding(context.Background(), tt.networkBinding.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.networkBinding.SelfRef.Name, updated.SelfRef.Name)
				assert.Equal(t, tt.networkBinding.SelfRef.Namespace, updated.SelfRef.Namespace)
			}
		})
	}
}

func TestNetworkBindingResourceService_DeleteNetworkBinding(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idToDelete  models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing network binding",
			setupData: map[string]interface{}{
				"networkbinding_test-namespace/test-network-binding": &testutil.TestFixtures.NetworkBinding,
				"network_test-namespace/test-network":                &testutil.TestFixtures.Network,
				"addressgroup_test-namespace/test-address-group":     &testutil.TestFixtures.AddressGroup,
			},
			idToDelete:  testutil.TestFixtures.NetworkBinding.SelfRef.ResourceIdentifier,
			expectError: false,
		},
		{
			name:        "delete non-existent network binding",
			setupData:   map[string]interface{}{},
			idToDelete:  testutil.TestFixtures.NetworkBinding.SelfRef.ResourceIdentifier,
			expectError: false, // Delete operations are typically idempotent
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()
			mockConditionManager := testutil.NewMockConditionManager()
			networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

			// Execute
			err := service.DeleteNetworkBinding(context.Background(), tt.idToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network binding was deleted
				_, getErr := service.GetNetworkBinding(context.Background(), tt.idToDelete)
				assert.Error(t, getErr) // Should not be found
			}
		})
	}
}

func TestNetworkBindingResourceService_NetworkBindingLifecycle(t *testing.T) {
	t.Run("complete network binding lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.SetupTestData(map[string]interface{}{
			// Add required dependencies
			"network_test-namespace/test-network":            &testutil.TestFixtures.Network,
			"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
		})
		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)
		testNetworkBinding := testutil.TestFixtures.NetworkBinding

		// Create network binding
		err := service.CreateNetworkBinding(context.Background(), &testNetworkBinding)
		require.NoError(t, err)

		// Verify network binding exists
		created, err := service.GetNetworkBinding(context.Background(), testNetworkBinding.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testNetworkBinding.SelfRef.Name, created.SelfRef.Name)

		// Update network binding (network binding updates are limited, but we can test the flow)
		err = service.UpdateNetworkBinding(context.Background(), &testNetworkBinding)
		require.NoError(t, err)

		// Verify update
		updated, err := service.GetNetworkBinding(context.Background(), testNetworkBinding.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testNetworkBinding.SelfRef.Name, updated.SelfRef.Name)

		// Delete network binding
		err = service.DeleteNetworkBinding(context.Background(), testNetworkBinding.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetNetworkBinding(context.Background(), testNetworkBinding.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}

func TestNetworkBindingResourceService_Concurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		// Use a fresh mock registry to avoid interference from other tests
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

		// Create multiple address groups for concurrent testing (one per binding to avoid conflicts)
		// Use smaller number to avoid registry lock contention in complex operations
		numBindings := 3
		setupData := map[string]interface{}{}

		// Add unique networks and address groups for each binding
		for i := 0; i < numBindings; i++ {
			networkName := "test-network-" + string(rune(i+48))
			addressGroupName := "test-address-group-" + string(rune(i+48))

			network := testutil.CreateTestNetwork(networkName, "test-namespace", "10."+string(rune(i+48))+".0.0/24")
			addressGroup := testutil.CreateTestAddressGroup(addressGroupName, "test-namespace")

			setupData["network_test-namespace/"+networkName] = &network
			setupData["addressgroup_test-namespace/"+addressGroupName] = &addressGroup
		}

		mockRegistry.SetupTestData(setupData)

		// Create multiple network bindings concurrently, each with different networks and address groups
		errChan := make(chan error, numBindings)

		for i := 0; i < numBindings; i++ {
			go func(index int) {
				// Create unique binding for each goroutine
				testNB := testutil.CreateTestNetworkBinding(
					"test-network-binding-"+string(rune(index+48)),
					"test-namespace",
					"test-network-"+string(rune(index+48)),
					"test-address-group-"+string(rune(index+48)),
				)
				errChan <- service.CreateNetworkBinding(context.Background(), &testNB)
			}(i)
		}

		// Wait for all operations to complete
		for i := 0; i < numBindings; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}

		// Verify all network bindings were created
		bindings, err := service.ListNetworkBindings(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, bindings, numBindings)
	})
}

func TestNetworkBindingResourceService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		networkService := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		service := NewNetworkBindingResourceService(mockRegistry, networkService, mockSyncManager, mockConditionManager)

		// Test that errors are properly handled
		_, err := service.ListNetworkBindings(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = service.CreateNetworkBinding(context.Background(), &testutil.TestFixtures.NetworkBinding)
		assert.Error(t, err)
	})
}
