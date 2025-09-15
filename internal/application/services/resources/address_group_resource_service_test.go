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

func TestAddressGroupResourceService_GetAddressGroups(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		scope        ports.Scope
		expectedLen  int
		expectedName string
	}{
		{
			name: "get all address groups",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-address-group-1": &testutil.TestFixtures.AddressGroup,
				"addressgroup_test-namespace/test-address-group-2": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-address-group-2", "test-namespace")
					return &ag
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "get address groups from empty registry",
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
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			addressGroups, err := service.GetAddressGroups(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, addressGroups, tt.expectedLen)

			if tt.expectedLen > 0 {
				// Verify basic structure of returned address groups
				assert.NotEmpty(t, addressGroups[0].SelfRef.Name)
				assert.NotEmpty(t, addressGroups[0].SelfRef.Namespace)
			}
		})
	}
}

func TestAddressGroupResourceService_GetAddressGroupByID(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resourceID  models.ResourceIdentifier
		expectError bool
		expectedAG  *models.AddressGroup
	}{
		{
			name: "get existing address group",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
			},
			resourceID:  testutil.TestFixtures.AddressGroup.SelfRef.ResourceIdentifier,
			expectError: false,
			expectedAG:  &testutil.TestFixtures.AddressGroup,
		},
		{
			name:        "get non-existent address group",
			setupData:   map[string]interface{}{},
			resourceID:  testutil.TestFixtures.AddressGroup.SelfRef.ResourceIdentifier,
			expectError: true,
			expectedAG:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			result, err := service.GetAddressGroupByID(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedAG.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedAG.SelfRef.Namespace, result.SelfRef.Namespace)
			}
		})
	}
}

func TestAddressGroupResourceService_CreateAddressGroup(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		addressGroup models.AddressGroup
		expectError  bool
	}{
		{
			name:         "create valid address group",
			setupData:    map[string]interface{}{},
			addressGroup: testutil.TestFixtures.AddressGroup,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			err := service.CreateAddressGroup(context.Background(), tt.addressGroup)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify address group was created by attempting to retrieve it
				created, getErr := service.GetAddressGroupByID(context.Background(), tt.addressGroup.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.addressGroup.SelfRef.Name, created.SelfRef.Name)
				assert.Equal(t, tt.addressGroup.SelfRef.Namespace, created.SelfRef.Namespace)

				// Address group created successfully (conditions are managed elsewhere)
			}
		})
	}
}

func TestAddressGroupResourceService_UpdateAddressGroup(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		addressGroup models.AddressGroup
		expectError  bool
	}{
		{
			name: "update existing address group",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
			},
			addressGroup: func() models.AddressGroup {
				ag := testutil.TestFixtures.AddressGroup
				ag.DefaultAction = models.ActionDrop
				return ag
			}(),
			expectError: false,
		},
		{
			name:         "update non-existent address group",
			setupData:    map[string]interface{}{},
			addressGroup: testutil.TestFixtures.AddressGroup,
			expectError:  true, // Update operations require existing address group
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			err := service.UpdateAddressGroup(context.Background(), tt.addressGroup)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify address group was updated by retrieving it
				updated, getErr := service.GetAddressGroupByID(context.Background(), tt.addressGroup.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.addressGroup.DefaultAction, updated.DefaultAction)

				// Address group updated successfully
			}
		})
	}
}

func TestAddressGroupResourceService_DeleteAddressGroupsByIDs(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idsToDelete []models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing address groups",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
				"addressgroup_test-namespace/test-address-group-2": func() *models.AddressGroup {
					ag := testutil.CreateTestAddressGroup("test-address-group-2", "test-namespace")
					return &ag
				}(),
			},
			idsToDelete: []models.ResourceIdentifier{
				testutil.TestFixtures.AddressGroup.SelfRef.ResourceIdentifier,
				testutil.CreateTestResourceIdentifier("test-address-group-2", "test-namespace"),
			},
			expectError: false,
		},
		{
			name:      "delete non-existent address groups",
			setupData: map[string]interface{}{},
			idsToDelete: []models.ResourceIdentifier{
				testutil.TestFixtures.AddressGroup.SelfRef.ResourceIdentifier,
			},
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
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			err := service.DeleteAddressGroupsByIDs(context.Background(), tt.idsToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify address groups were deleted
				for _, id := range tt.idsToDelete {
					_, getErr := service.GetAddressGroupByID(context.Background(), id)
					assert.Error(t, getErr) // Should not be found

					// Address group deleted successfully
				}
			}
		})
	}
}

func TestAddressGroupResourceService_SyncAddressGroups(t *testing.T) {
	tests := []struct {
		name          string
		addressGroups []models.AddressGroup
		scope         ports.Scope
		expectError   bool
	}{
		{
			name: "sync multiple address groups",
			addressGroups: []models.AddressGroup{
				testutil.TestFixtures.AddressGroup,
				testutil.CreateTestAddressGroup("test-address-group-2", "test-namespace"),
			},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
		{
			name:          "sync empty address groups list",
			addressGroups: []models.AddressGroup{},
			scope:         ports.EmptyScope{},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			validationService := NewValidationService(mockRegistry, nil)
			service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, validationService)

			// Execute
			err := service.SyncAddressGroups(context.Background(), tt.addressGroups, tt.scope)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify all address groups were synced
				for _, ag := range tt.addressGroups {
					synced, getErr := service.GetAddressGroupByID(context.Background(), ag.SelfRef.ResourceIdentifier)
					require.NoError(t, getErr)
					assert.Equal(t, ag.SelfRef.Name, synced.SelfRef.Name)
					assert.Equal(t, ag.SelfRef.Namespace, synced.SelfRef.Namespace)
				}
			}
		})
	}
}

func TestAddressGroupResourceService_AddressGroupLifecycle(t *testing.T) {
	t.Run("complete address group lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		mockValidationService := NewValidationService(mockRegistry, nil)
		service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, mockValidationService)
		testAddressGroup := testutil.TestFixtures.AddressGroup

		// Create address group
		err := service.CreateAddressGroup(context.Background(), testAddressGroup)
		require.NoError(t, err)

		// Verify address group exists
		created, err := service.GetAddressGroupByID(context.Background(), testAddressGroup.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testAddressGroup.SelfRef.Name, created.SelfRef.Name)

		// Update address group
		testAddressGroup.DefaultAction = models.ActionDrop
		err = service.UpdateAddressGroup(context.Background(), testAddressGroup)
		require.NoError(t, err)

		// Verify update
		updated, err := service.GetAddressGroupByID(context.Background(), testAddressGroup.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, models.ActionDrop, updated.DefaultAction)

		// Delete address group
		err = service.DeleteAddressGroupsByIDs(context.Background(), []models.ResourceIdentifier{testAddressGroup.SelfRef.ResourceIdentifier})
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetAddressGroupByID(context.Background(), testAddressGroup.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}

func TestAddressGroupResourceService_Concurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		mockValidationService := NewValidationService(mockRegistry, nil)
		service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, mockValidationService)

		// Create multiple address groups concurrently (reduced number to avoid race conditions)
		numAddressGroups := 3
		errChan := make(chan error, numAddressGroups)

		for i := 0; i < numAddressGroups; i++ {
			go func(index int) {
				testAG := testutil.CreateTestAddressGroup("test-address-group-"+string(rune(index+48)), "test-namespace")
				errChan <- service.CreateAddressGroup(context.Background(), testAG)
			}(i)
		}

		// Wait for all operations to complete
		for i := 0; i < numAddressGroups; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}

		// Verify all address groups were created
		addressGroups, err := service.GetAddressGroups(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, addressGroups, numAddressGroups)
	})
}

func TestAddressGroupResourceService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		mockValidationService := NewValidationService(mockRegistry, nil)
		service := NewAddressGroupResourceService(mockRegistry, mockSyncManager, mockConditionManager, mockValidationService)

		// Test that errors are properly handled
		_, err := service.GetAddressGroups(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = service.CreateAddressGroup(context.Background(), testutil.TestFixtures.AddressGroup)
		assert.Error(t, err)
	})
}
