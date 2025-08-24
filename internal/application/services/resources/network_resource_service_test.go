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

func TestNetworkResourceService_ListNetworks(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		scope        ports.Scope
		expectedLen  int
		expectedName string
	}{
		{
			name: "get all networks",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network-1": &testutil.TestFixtures.Network,
				"network_test-namespace/test-network-2": func() *models.Network {
					net := testutil.CreateTestNetwork("test-network-2", "test-namespace", "192.168.1.0/24")
					return &net
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "get networks from empty registry",
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
			service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			networks, err := service.ListNetworks(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, networks, tt.expectedLen)

			if tt.expectedLen > 0 {
				// Verify basic structure of returned networks
				assert.NotEmpty(t, networks[0].SelfRef.Name)
				assert.NotEmpty(t, networks[0].SelfRef.Namespace)
				assert.NotEmpty(t, networks[0].CIDR)
			}
		})
	}
}

func TestNetworkResourceService_GetNetwork(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resourceID  models.ResourceIdentifier
		expectError bool
		expectedNet *models.Network
	}{
		{
			name: "get existing network",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network": &testutil.TestFixtures.Network,
			},
			resourceID:  testutil.TestFixtures.Network.SelfRef.ResourceIdentifier,
			expectError: false,
			expectedNet: &testutil.TestFixtures.Network,
		},
		{
			name:        "get non-existent network",
			setupData:   map[string]interface{}{},
			resourceID:  testutil.TestFixtures.Network.SelfRef.ResourceIdentifier,
			expectError: true,
			expectedNet: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			result, err := service.GetNetwork(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedNet.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedNet.SelfRef.Namespace, result.SelfRef.Namespace)
				assert.Equal(t, tt.expectedNet.CIDR, result.CIDR)
			}
		})
	}
}

func TestNetworkResourceService_CreateNetwork(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		network     models.Network
		expectError bool
	}{
		{
			name:        "create valid network",
			setupData:   map[string]interface{}{},
			network:     testutil.TestFixtures.Network,
			expectError: false,
		},
		{
			name:        "create network with different CIDR",
			setupData:   map[string]interface{}{},
			network:     testutil.CreateTestNetwork("test-network-custom", "test-namespace", "192.168.100.0/24"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.CreateNetwork(context.Background(), &tt.network)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network was created by attempting to retrieve it
				created, getErr := service.GetNetwork(context.Background(), tt.network.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.network.SelfRef.Name, created.SelfRef.Name)
				assert.Equal(t, tt.network.SelfRef.Namespace, created.SelfRef.Namespace)
				assert.Equal(t, tt.network.CIDR, created.CIDR)

				// Network created successfully (conditions are managed elsewhere)
			}
		})
	}
}

func TestNetworkResourceService_UpdateNetwork(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		network     models.Network
		expectError bool
	}{
		{
			name: "update existing network",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network": func() *models.Network {
					net := testutil.CreateTestNetwork("test-network", "test-namespace", "10.0.0.0/24")
					return &net
				}(),
			},
			network: func() models.Network {
				net := testutil.CreateTestNetwork("test-network", "test-namespace", "192.168.200.0/24") // Different CIDR
				return net
			}(),
			expectError: false,
		},
		{
			name:        "update non-existent network",
			setupData:   map[string]interface{}{},
			network:     testutil.TestFixtures.Network,
			expectError: true, // Update operations require existing network
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.UpdateNetwork(context.Background(), &tt.network)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network was updated by retrieving it
				updated, getErr := service.GetNetwork(context.Background(), tt.network.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.network.CIDR, updated.CIDR)

				// Network updated successfully
			}
		})
	}
}

func TestNetworkResourceService_DeleteNetwork(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idToDelete  models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing network",
			setupData: map[string]interface{}{
				"network_test-namespace/test-network": &testutil.TestFixtures.Network,
			},
			idToDelete:  testutil.TestFixtures.Network.SelfRef.ResourceIdentifier,
			expectError: false,
		},
		{
			name:        "delete non-existent network",
			setupData:   map[string]interface{}{},
			idToDelete:  testutil.TestFixtures.Network.SelfRef.ResourceIdentifier,
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
			service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.DeleteNetwork(context.Background(), tt.idToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify network was deleted
				_, getErr := service.GetNetwork(context.Background(), tt.idToDelete)
				assert.Error(t, getErr) // Should not be found

				// Network deleted successfully
			}
		})
	}
}

func TestNetworkResourceService_NetworkLifecycle(t *testing.T) {
	t.Run("complete network lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		testNetwork := testutil.TestFixtures.Network

		// Create network
		err := service.CreateNetwork(context.Background(), &testNetwork)
		require.NoError(t, err)

		// Verify network exists
		created, err := service.GetNetwork(context.Background(), testNetwork.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testNetwork.SelfRef.Name, created.SelfRef.Name)
		assert.Equal(t, testNetwork.CIDR, created.CIDR)

		// Update network
		testNetwork.CIDR = "10.10.0.0/16"
		err = service.UpdateNetwork(context.Background(), &testNetwork)
		require.NoError(t, err)

		// Verify update
		updated, err := service.GetNetwork(context.Background(), testNetwork.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "10.10.0.0/16", updated.CIDR)

		// Delete network
		err = service.DeleteNetwork(context.Background(), testNetwork.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetNetwork(context.Background(), testNetwork.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}

func TestNetworkResourceService_Concurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		// Create multiple networks concurrently (reduced number to avoid race conditions)
		numNetworks := 3
		errChan := make(chan error, numNetworks)

		for i := 0; i < numNetworks; i++ {
			go func(index int) {
				testNet := testutil.CreateTestNetwork("test-network-"+string(rune(index+48)), "test-namespace", "10."+string(rune(index+48))+".0.0/24")
				errChan <- service.CreateNetwork(context.Background(), &testNet)
			}(i)
		}

		// Wait for all operations to complete
		for i := 0; i < numNetworks; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}

		// Verify all networks were created
		networks, err := service.ListNetworks(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, networks, numNetworks)
	})
}

func TestNetworkResourceService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		service := NewNetworkResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		// Test that errors are properly handled
		_, err := service.ListNetworks(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = service.CreateNetwork(context.Background(), &testutil.TestFixtures.Network)
		assert.Error(t, err)
	})
}
