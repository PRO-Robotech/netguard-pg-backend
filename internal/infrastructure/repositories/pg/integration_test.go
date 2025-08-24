package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestServiceAddressGroupIntegration tests the integration between Services, AddressGroups, and AddressGroupBindings
func TestServiceAddressGroupIntegration(t *testing.T) {
	// Skip if PostgreSQL not available
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available, skipping integration tests")
	}

	registry := setupTestRegistry(t)
	defer registry.Close()

	ctx := context.Background()

	t.Run("Service_AddressGroup_Relationship_Via_Bindings", func(t *testing.T) {
		// Step 1: Create Service and AddressGroup
		service := models.Service{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web-app", models.WithNamespace("integration"))),
			Description: "Web application service",
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80", Description: "HTTP"},
				{Protocol: models.TCP, Port: "443", Description: "HTTPS"},
			},
		}

		addressGroups := []models.AddressGroup{
			{
				SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("frontend", models.WithNamespace("integration"))),
				DefaultAction: models.ActionAccept,
				Description:   "Frontend network zone",
			},
			{
				SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("external", models.WithNamespace("integration"))),
				DefaultAction: models.ActionDrop,
				Description:   "External access zone",
			},
		}

		// Create prerequisites
		err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
			if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
				return err
			}
			return writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
		})
		require.NoError(t, err)

		// Step 2: Create AddressGroupBindings to link Service with AddressGroups
		bindings := []models.AddressGroupBinding{
			{
				SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("web-app-frontend", models.WithNamespace("integration"))),
				ServiceRef:      models.NewServiceRef("web-app", models.WithNamespace("integration")),
				AddressGroupRef: models.NewAddressGroupRef("frontend", models.WithNamespace("integration")),
			},
			{
				SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("web-app-external", models.WithNamespace("integration"))),
				ServiceRef:      models.NewServiceRef("web-app", models.WithNamespace("integration")),
				AddressGroupRef: models.NewAddressGroupRef("external", models.WithNamespace("integration")),
			},
		}

		err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
			return writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
		})
		require.NoError(t, err)

		// Step 3: Verify that Service now loads AddressGroup relationships
		reader, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		retrievedService, err := reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		// Verify service basic properties
		assert.Equal(t, "web-app", retrievedService.Name)
		assert.Equal(t, "integration", retrievedService.Namespace)
		assert.Equal(t, "Web application service", retrievedService.Description)
		assert.Len(t, retrievedService.IngressPorts, 2)

		// Verify address group relationships are loaded
		assert.Len(t, retrievedService.AddressGroups, 2, "Service should have 2 address groups via bindings")

		// Verify the address group references
		addressGroupNames := make(map[string]bool)
		for _, agRef := range retrievedService.AddressGroups {
			assert.Equal(t, "integration", agRef.Namespace)
			addressGroupNames[agRef.Name] = true
		}

		assert.True(t, addressGroupNames["frontend"], "Service should reference frontend address group")
		assert.True(t, addressGroupNames["external"], "Service should reference external address group")

		// Step 4: Test that deleting a binding removes the relationship
		writer, err := registry.Writer(ctx)
		require.NoError(t, err)
		defer writer.Close()

		// Delete one binding
		err = writer.DeleteAddressGroupBindingsByIDs(ctx, []models.ResourceIdentifier{
			models.NewResourceIdentifier("web-app-external", models.WithNamespace("integration")),
		})
		require.NoError(t, err)

		err = writer.Commit()
		require.NoError(t, err)

		// Step 5: Verify relationship is updated
		reader2, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader2.Close()

		updatedService, err := reader2.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		assert.Len(t, updatedService.AddressGroups, 1, "Service should now have only 1 address group")
		assert.Equal(t, "frontend", updatedService.AddressGroups[0].Name)

		// Step 6: Test cascade deletion when Service is deleted
		writer2, err := registry.Writer(ctx)
		require.NoError(t, err)
		defer writer2.Close()

		err = writer2.DeleteServicesByIDs(ctx, []models.ResourceIdentifier{service.SelfRef.ResourceIdentifier})
		require.NoError(t, err)

		err = writer2.Commit()
		require.NoError(t, err)

		// Verify that remaining binding is also deleted due to CASCADE
		reader3, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader3.Close()

		var remainingBindings []models.AddressGroupBinding
		err = reader3.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
			remainingBindings = append(remainingBindings, binding)
			return nil
		}, ports.ResourceIdentifierScope{
			Identifiers: []models.ResourceIdentifier{
				models.NewResourceIdentifier("", models.WithNamespace("integration")),
			},
		})
		require.NoError(t, err)

		assert.Empty(t, remainingBindings, "All bindings should be deleted when service is deleted")
	})
}

// TestPhase2Complete verifies that Phase 2 implementation is complete and working
func TestPhase2Complete(t *testing.T) {
	// Skip if PostgreSQL not available
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available, skipping integration tests")
	}

	registry := setupTestRegistry(t)
	defer registry.Close()

	ctx := context.Background()

	// Test that all Phase 1 + Phase 2 resources work together

	// Phase 1 resources: Service, AddressGroup, SyncStatus
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("phase2-test", models.WithNamespace("complete"))),
		Description: "Phase 2 completion test service",
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "8080", Description: "HTTP API"},
		},
		Meta: models.Meta{
			Labels: map[string]string{
				"phase": "2",
				"test":  "completion",
			},
		},
	}

	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("phase2-ag", models.WithNamespace("complete"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
		Trace:         false,
		Meta: models.Meta{
			Labels: map[string]string{
				"phase": "2",
				"type":  "address-group",
			},
		},
	}

	// Phase 2 resource: AddressGroupBinding
	binding := models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("phase2-binding", models.WithNamespace("complete"))),
		ServiceRef:      models.NewServiceRef("phase2-test", models.WithNamespace("complete")),
		AddressGroupRef: models.NewAddressGroupRef("phase2-ag", models.WithNamespace("complete")),
		Meta: models.Meta{
			Labels: map[string]string{
				"phase": "2",
				"type":  "binding",
			},
			Finalizers: []string{
				"netguard.sgroups.io/binding-finalizer",
			},
		},
	}

	// Create all resources in a single transaction
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		// Phase 1 resources
		if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
			return err
		}
		if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{addressGroup}, ports.EmptyScope{}); err != nil {
			return err
		}
		// Phase 2 resource
		return writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{binding}, ports.EmptyScope{})
	})
	require.NoError(t, err, "All Phase 1 + Phase 2 resources should be created successfully")

	// Verify all resources exist and relationships work
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	// Check Service with relationships
	retrievedService, err := reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "phase2-test", retrievedService.Name)
	assert.Len(t, retrievedService.AddressGroups, 1, "Service should have address group relationship")
	assert.Equal(t, "phase2-ag", retrievedService.AddressGroups[0].Name)

	// Check AddressGroup
	retrievedAG, err := reader.GetAddressGroupByID(ctx, addressGroup.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "phase2-ag", retrievedAG.Name)
	assert.Equal(t, models.ActionAccept, retrievedAG.DefaultAction)

	// Check AddressGroupBinding
	retrievedBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "phase2-binding", retrievedBinding.Name)
	assert.Equal(t, "phase2-test", retrievedBinding.ServiceRef.Name)
	assert.Equal(t, "phase2-ag", retrievedBinding.AddressGroupRef.Name)
	assert.Contains(t, retrievedBinding.Meta.Finalizers, "netguard.sgroups.io/binding-finalizer")

	// Check SyncStatus
	syncStatus, err := reader.GetSyncStatus(ctx)
	require.NoError(t, err)
	assert.False(t, syncStatus.UpdatedAt.IsZero(), "Sync status should be updated")

	t.Logf("✅ Phase 2 Complete! All resources working correctly:")
	t.Logf("  - Service: %s (with %d address groups)", retrievedService.Name, len(retrievedService.AddressGroups))
	t.Logf("  - AddressGroup: %s (action: %s)", retrievedAG.Name, retrievedAG.DefaultAction)
	t.Logf("  - AddressGroupBinding: %s", retrievedBinding.Name)
	t.Logf("  - SyncStatus: updated at %s", syncStatus.UpdatedAt.Format("2006-01-02 15:04:05"))
}
