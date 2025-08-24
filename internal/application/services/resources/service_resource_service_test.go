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

func TestServiceResourceService_GetServices(t *testing.T) {
	tests := []struct {
		name         string
		setupData    map[string]interface{}
		scope        ports.Scope
		expectedLen  int
		expectedName string
	}{
		{
			name: "get all services",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service-1": &testutil.TestFixtures.Service,
				"service_test-namespace/test-service-2": func() *models.Service {
					svc := testutil.CreateTestService("test-service-2", "test-namespace")
					return &svc
				}(),
			},
			scope:       ports.EmptyScope{},
			expectedLen: 2,
		},
		{
			name:        "get services from empty registry",
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
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			services, err := service.GetServices(context.Background(), tt.scope)

			// Assert
			require.NoError(t, err)
			assert.Len(t, services, tt.expectedLen)

			if tt.expectedLen > 0 {
				// Verify basic structure of returned services
				assert.NotEmpty(t, services[0].SelfRef.Name)
				assert.NotEmpty(t, services[0].SelfRef.Namespace)
			}
		})
	}
}

func TestServiceResourceService_GetServiceByID(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		resourceID  models.ResourceIdentifier
		expectError bool
		expectedSvc *models.Service
	}{
		{
			name: "get existing service",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": &testutil.TestFixtures.Service,
			},
			resourceID:  testutil.TestFixtures.Service.SelfRef.ResourceIdentifier,
			expectError: false,
			expectedSvc: &testutil.TestFixtures.Service,
		},
		{
			name:        "get non-existent service",
			setupData:   map[string]interface{}{},
			resourceID:  testutil.TestFixtures.Service.SelfRef.ResourceIdentifier,
			expectError: true,
			expectedSvc: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)

			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			result, err := service.GetServiceByID(context.Background(), tt.resourceID)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedSvc.SelfRef.Name, result.SelfRef.Name)
				assert.Equal(t, tt.expectedSvc.SelfRef.Namespace, result.SelfRef.Namespace)
			}
		})
	}
}

func TestServiceResourceService_CreateService(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		service     models.Service
		expectError bool
	}{
		{
			name:        "create valid service",
			setupData:   map[string]interface{}{},
			service:     testutil.TestFixtures.Service,
			expectError: false,
		},
		{
			name: "create service with ports",
			setupData: map[string]interface{}{
				"addressgroup_test-namespace/test-address-group": &testutil.TestFixtures.AddressGroup,
			},
			service:     testutil.TestFixtures.ServiceWithPorts,
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
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.CreateService(context.Background(), tt.service)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify service was created by attempting to retrieve it
				created, getErr := service.GetServiceByID(context.Background(), tt.service.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.service.SelfRef.Name, created.SelfRef.Name)
				assert.Equal(t, tt.service.SelfRef.Namespace, created.SelfRef.Namespace)

				// Service created successfully (conditions are managed elsewhere)
			}
		})
	}
}

func TestServiceResourceService_UpdateService(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		service     models.Service
		expectError bool
	}{
		{
			name: "update existing service",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": &testutil.TestFixtures.Service,
			},
			service: func() models.Service {
				svc := testutil.TestFixtures.Service
				svc.Description = "Updated description"
				return svc
			}(),
			expectError: false,
		},
		{
			name:        "update non-existent service",
			setupData:   map[string]interface{}{},
			service:     testutil.TestFixtures.Service,
			expectError: true, // Update operations require existing service
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockRegistry.SetupTestData(tt.setupData)
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.UpdateService(context.Background(), tt.service)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify service was updated by retrieving it
				updated, getErr := service.GetServiceByID(context.Background(), tt.service.SelfRef.ResourceIdentifier)
				require.NoError(t, getErr)
				assert.Equal(t, tt.service.Description, updated.Description)

				// Service updated successfully
			}
		})
	}
}

func TestServiceResourceService_DeleteServicesByIDs(t *testing.T) {
	tests := []struct {
		name        string
		setupData   map[string]interface{}
		idsToDelete []models.ResourceIdentifier
		expectError bool
	}{
		{
			name: "delete existing services",
			setupData: map[string]interface{}{
				"service_test-namespace/test-service": &testutil.TestFixtures.Service,
				"service_test-namespace/test-service-2": func() *models.Service {
					svc := testutil.CreateTestService("test-service-2", "test-namespace")
					return &svc
				}(),
			},
			idsToDelete: []models.ResourceIdentifier{
				testutil.TestFixtures.Service.SelfRef.ResourceIdentifier,
				testutil.CreateTestResourceIdentifier("test-service-2", "test-namespace"),
			},
			expectError: false,
		},
		{
			name:      "delete non-existent services",
			setupData: map[string]interface{}{},
			idsToDelete: []models.ResourceIdentifier{
				testutil.TestFixtures.Service.SelfRef.ResourceIdentifier,
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
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.DeleteServicesByIDs(context.Background(), tt.idsToDelete)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify services were deleted
				for _, id := range tt.idsToDelete {
					_, getErr := service.GetServiceByID(context.Background(), id)
					assert.Error(t, getErr) // Should not be found

					// Service deleted successfully
				}
			}
		})
	}
}

func TestServiceResourceService_SyncServices(t *testing.T) {
	tests := []struct {
		name        string
		services    []models.Service
		scope       ports.Scope
		expectError bool
	}{
		{
			name: "sync multiple services",
			services: []models.Service{
				testutil.TestFixtures.Service,
				testutil.CreateTestService("test-service-2", "test-namespace"),
			},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
		{
			name:        "sync empty services list",
			services:    []models.Service{},
			scope:       ports.EmptyScope{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			err := service.SyncServices(context.Background(), tt.services, tt.scope)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				// Verify all services were synced
				for _, svc := range tt.services {
					synced, getErr := service.GetServiceByID(context.Background(), svc.SelfRef.ResourceIdentifier)
					require.NoError(t, getErr)
					assert.Equal(t, svc.SelfRef.Name, synced.SelfRef.Name)
					assert.Equal(t, svc.SelfRef.Namespace, synced.SelfRef.Namespace)
				}
			}
		})
	}
}

func TestServiceResourceService_GetServiceAliases(t *testing.T) {
	tests := []struct {
		name        string
		scope       ports.Scope
		expectError bool
	}{
		{
			name:        "get service aliases - basic functionality",
			scope:       ports.EmptyScope{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRegistry := testutil.NewMockRegistry()
			mockSyncManager := testutil.NewMockSyncManager()

			mockConditionManager := testutil.NewMockConditionManager()
			service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

			// Execute
			aliases, err := service.GetServiceAliases(context.Background(), tt.scope)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, aliases) // Should return empty list, not nil
			}
		})
	}
}

func TestServiceResourceService_ServiceLifecycle(t *testing.T) {
	t.Run("complete service lifecycle", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)
		testService := testutil.TestFixtures.Service

		// Create service
		err := service.CreateService(context.Background(), testService)
		require.NoError(t, err)

		// Verify service exists
		created, err := service.GetServiceByID(context.Background(), testService.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, testService.SelfRef.Name, created.SelfRef.Name)

		// Update service
		testService.Description = "Updated service"
		err = service.UpdateService(context.Background(), testService)
		require.NoError(t, err)

		// Verify update
		updated, err := service.GetServiceByID(context.Background(), testService.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "Updated service", updated.Description)

		// Delete service
		err = service.DeleteServicesByIDs(context.Background(), []models.ResourceIdentifier{testService.SelfRef.ResourceIdentifier})
		require.NoError(t, err)

		// Verify deletion
		_, err = service.GetServiceByID(context.Background(), testService.SelfRef.ResourceIdentifier)
		assert.Error(t, err) // Should not be found
	})
}

func TestServiceResourceService_Concurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		// Setup
		mockRegistry := testutil.NewMockRegistry()
		mockSyncManager := testutil.NewMockSyncManager()

		mockConditionManager := testutil.NewMockConditionManager()
		service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		// Create multiple services concurrently
		numServices := 10
		errChan := make(chan error, numServices)

		for i := 0; i < numServices; i++ {
			go func(index int) {
				testSvc := testutil.CreateTestService("test-service-"+string(rune(index+48)), "test-namespace")
				errChan <- service.CreateService(context.Background(), testSvc)
			}(i)
		}

		// Wait for all operations to complete
		for i := 0; i < numServices; i++ {
			err := <-errChan
			assert.NoError(t, err)
		}

		// Verify all services were created
		services, err := service.GetServices(context.Background(), ports.EmptyScope{})
		require.NoError(t, err)
		assert.Len(t, services, numServices)
	})
}

func TestServiceResourceService_ErrorHandling(t *testing.T) {
	t.Run("registry error handling", func(t *testing.T) {
		// Setup with closed registry to simulate error conditions
		mockRegistry := testutil.NewMockRegistry()
		mockRegistry.Close() // Close registry to force errors

		mockSyncManager := testutil.NewMockSyncManager()
		mockConditionManager := testutil.NewMockConditionManager()
		service := NewServiceResourceService(mockRegistry, mockSyncManager, mockConditionManager)

		// Test that errors are properly handled
		_, err := service.GetServices(context.Background(), ports.EmptyScope{})
		assert.Error(t, err)

		err = service.CreateService(context.Background(), testutil.TestFixtures.Service)
		assert.Error(t, err)
	})
}
