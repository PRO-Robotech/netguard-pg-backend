package pg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupBinding_PostgreSQL tests AddressGroupBinding operations with PostgreSQL backend
func TestAddressGroupBinding_PostgreSQL(t *testing.T) {
	// Skip if PostgreSQL not available
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available, skipping integration tests")
	}

	registry := setupTestRegistry(t)
	defer registry.Close()

	ctx := context.Background()

	t.Run("AddressGroupBinding_CRUD_Operations", func(t *testing.T) {
		testAddressGroupBindingCRUD(t, registry, ctx)
	})

	t.Run("AddressGroupBinding_K8s_Metadata", func(t *testing.T) {
		testAddressGroupBindingK8sMetadata(t, registry, ctx)
	})

	t.Run("AddressGroupBinding_Relationships", func(t *testing.T) {
		testAddressGroupBindingRelationships(t, registry, ctx)
	})

	t.Run("AddressGroupBinding_Scoped_Operations", func(t *testing.T) {
		testAddressGroupBindingScopedOperations(t, registry, ctx)
	})

	t.Run("AddressGroupBinding_Foreign_Key_Constraints", func(t *testing.T) {
		testAddressGroupBindingForeignKeyConstraints(t, registry, ctx)
	})
}

func testAddressGroupBindingCRUD(t *testing.T, registry *Registry, ctx context.Context) {
	// First create prerequisite Service and AddressGroup
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("default"))),
		Description: "Web service for binding test",
	}

	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("internal", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Description:   "Internal address group for binding test",
	}

	// Create prerequisites
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, []models.AddressGroup{addressGroup}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Create test binding
	binding := models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("web-internal-binding", models.WithNamespace("default"))),
		ServiceRef:      models.NewServiceRef("web", models.WithNamespace("default")),
		AddressGroupRef: models.NewAddressGroupRef("internal", models.WithNamespace("default")),
		Meta: models.Meta{
			Labels: map[string]string{
				"binding-type": "service-to-addressgroup",
				"environment":  "test",
			},
			Annotations: map[string]string{
				"description": "Test binding for web service to internal address group",
			},
		},
	}

	// Test Create
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{binding}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Test Read
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, binding.Name, retrievedBinding.Name)
	assert.Equal(t, binding.Namespace, retrievedBinding.Namespace)
	assert.Equal(t, binding.ServiceRef.Name, retrievedBinding.ServiceRef.Name)
	assert.Equal(t, binding.ServiceRef.Namespace, retrievedBinding.ServiceRef.Namespace)
	assert.Equal(t, binding.AddressGroupRef.Name, retrievedBinding.AddressGroupRef.Name)
	assert.Equal(t, binding.AddressGroupRef.Namespace, retrievedBinding.AddressGroupRef.Namespace)
	assert.Equal(t, binding.Meta.Labels["binding-type"], retrievedBinding.Meta.Labels["binding-type"])
	assert.Greater(t, retrievedBinding.Meta.ResourceVersion, int64(0))

	// Test Update
	writer2, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer2.Close()

	binding.Meta.Labels["version"] = "v2"
	binding.Meta.Annotations["updated"] = "true"

	err = writer2.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{binding}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer2.Commit()
	require.NoError(t, err)

	// Verify Update
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	updatedBinding, err := reader2.GetAddressGroupBindingByID(ctx, binding.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "v2", updatedBinding.Meta.Labels["version"])
	assert.Equal(t, "true", updatedBinding.Meta.Annotations["updated"])
	assert.Greater(t, updatedBinding.Meta.ResourceVersion, retrievedBinding.Meta.ResourceVersion)

	// Test Delete
	writer3, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer3.Close()

	err = writer3.DeleteAddressGroupBindingsByIDs(ctx, []models.ResourceIdentifier{binding.SelfRef.ResourceIdentifier})
	require.NoError(t, err)

	err = writer3.Commit()
	require.NoError(t, err)

	// Verify Delete
	reader3, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader3.Close()

	_, err = reader3.GetAddressGroupBindingByID(ctx, binding.SelfRef.ResourceIdentifier)
	assert.Equal(t, ports.ErrNotFound, err)
}

func testAddressGroupBindingK8sMetadata(t *testing.T, registry *Registry, ctx context.Context) {
	// Create prerequisites first
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("api", models.WithNamespace("production"))),
		Description: "API service",
	}

	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("external", models.WithNamespace("production"))),
		DefaultAction: models.ActionDrop,
	}

	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, []models.AddressGroup{addressGroup}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	binding := models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("api-external-k8s", models.WithNamespace("production"))),
		ServiceRef:      models.NewServiceRef("api", models.WithNamespace("production")),
		AddressGroupRef: models.NewAddressGroupRef("external", models.WithNamespace("production")),
		Meta: models.Meta{
			Labels: map[string]string{
				"k8s.io/managed-by":           "netguard",
				"app.kubernetes.io/name":      "api-external-binding",
				"app.kubernetes.io/component": "network-policy",
			},
			Annotations: map[string]string{
				"kubernetes.io/managed-by":                         "netguard-controller",
				"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"AddressGroupBinding"}`,
			},
			Finalizers: []string{
				"netguard.sgroups.io/address-group-binding-finalizer",
			},
		},
	}

	// Create binding
	err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{binding}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Verify K8s metadata
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedBinding, err := reader.GetAddressGroupBindingByID(ctx, binding.SelfRef.ResourceIdentifier)
	require.NoError(t, err)

	// Check labels
	assert.Equal(t, "netguard", retrievedBinding.Meta.Labels["k8s.io/managed-by"])
	assert.Equal(t, "api-external-binding", retrievedBinding.Meta.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "network-policy", retrievedBinding.Meta.Labels["app.kubernetes.io/component"])

	// Check annotations
	assert.Equal(t, "netguard-controller", retrievedBinding.Meta.Annotations["kubernetes.io/managed-by"])
	assert.Contains(t, retrievedBinding.Meta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")

	// Check finalizers
	assert.Contains(t, retrievedBinding.Meta.Finalizers, "netguard.sgroups.io/address-group-binding-finalizer")

	// Check timestamps
	assert.False(t, retrievedBinding.Meta.CreatedAt.IsZero())
	assert.False(t, retrievedBinding.Meta.UpdatedAt.IsZero())

	// Check resource version
	assert.Greater(t, retrievedBinding.Meta.ResourceVersion, int64(0))
}

func testAddressGroupBindingRelationships(t *testing.T, registry *Registry, ctx context.Context) {
	// Create multiple services and address groups
	services := []models.Service{
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("frontend", models.WithNamespace("app"))),
			Description: "Frontend service",
		},
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("backend", models.WithNamespace("app"))),
			Description: "Backend service",
		},
	}

	addressGroups := []models.AddressGroup{
		{
			SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("dmz", models.WithNamespace("app"))),
			DefaultAction: models.ActionAccept,
		},
		{
			SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("secure", models.WithNamespace("app"))),
			DefaultAction: models.ActionDrop,
		},
	}

	// Create prerequisites
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, services, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Create multiple bindings
	bindings := []models.AddressGroupBinding{
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("frontend-dmz", models.WithNamespace("app"))),
			ServiceRef:      models.NewServiceRef("frontend", models.WithNamespace("app")),
			AddressGroupRef: models.NewAddressGroupRef("dmz", models.WithNamespace("app")),
		},
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("backend-secure", models.WithNamespace("app"))),
			ServiceRef:      models.NewServiceRef("backend", models.WithNamespace("app")),
			AddressGroupRef: models.NewAddressGroupRef("secure", models.WithNamespace("app")),
		},
	}

	err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Verify relationships
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	var foundBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		foundBindings = append(foundBindings, binding)
		return nil
	}, ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			models.NewResourceIdentifier("", models.WithNamespace("app")),
		},
	})
	require.NoError(t, err)

	assert.Len(t, foundBindings, 2)

	// Check that references are correctly set
	for _, binding := range foundBindings {
		assert.NotEmpty(t, binding.ServiceRef.Name)
		assert.Equal(t, "app", binding.ServiceRef.Namespace)
		assert.NotEmpty(t, binding.AddressGroupRef.Name)
		assert.Equal(t, "app", binding.AddressGroupRef.Namespace)
	}
}

func testAddressGroupBindingScopedOperations(t *testing.T, registry *Registry, ctx context.Context) {
	// Create prerequisites in different namespaces
	services := []models.Service{
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("svc1", models.WithNamespace("prod"))),
			Description: "Production service 1",
		},
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("svc2", models.WithNamespace("prod"))),
			Description: "Production service 2",
		},
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("svc1", models.WithNamespace("dev"))),
			Description: "Development service 1",
		},
	}

	addressGroups := []models.AddressGroup{
		{
			SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("ag1", models.WithNamespace("prod"))),
			DefaultAction: models.ActionAccept,
		},
		{
			SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("ag2", models.WithNamespace("prod"))),
			DefaultAction: models.ActionAccept,
		},
		{
			SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("ag1", models.WithNamespace("dev"))),
			DefaultAction: models.ActionDrop,
		},
	}

	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, services, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Create bindings in different namespaces
	bindings := []models.AddressGroupBinding{
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("binding1", models.WithNamespace("prod"))),
			ServiceRef:      models.NewServiceRef("svc1", models.WithNamespace("prod")),
			AddressGroupRef: models.NewAddressGroupRef("ag1", models.WithNamespace("prod")),
		},
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("binding2", models.WithNamespace("prod"))),
			ServiceRef:      models.NewServiceRef("svc2", models.WithNamespace("prod")),
			AddressGroupRef: models.NewAddressGroupRef("ag2", models.WithNamespace("prod")),
		},
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("binding1", models.WithNamespace("dev"))),
			ServiceRef:      models.NewServiceRef("svc1", models.WithNamespace("dev")),
			AddressGroupRef: models.NewAddressGroupRef("ag1", models.WithNamespace("dev")),
		},
	}

	err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Test namespace-scoped listing
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	// List bindings in production namespace
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			models.NewResourceIdentifier("", models.WithNamespace("prod")),
		},
	}

	var prodBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		prodBindings = append(prodBindings, binding)
		return nil
	}, scope)
	require.NoError(t, err)

	assert.Len(t, prodBindings, 2)
	for _, binding := range prodBindings {
		assert.Equal(t, "prod", binding.Namespace)
	}

	// Test scoped deletion
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	// Delete all bindings in prod namespace
	err = writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{}, scope)
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Verify deletion
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	var remainingBindings []models.AddressGroupBinding
	err = reader2.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		remainingBindings = append(remainingBindings, binding)
		return nil
	}, ports.EmptyScope{})
	require.NoError(t, err)

	assert.Len(t, remainingBindings, 1)
	assert.Equal(t, "dev", remainingBindings[0].Namespace)
}

func testAddressGroupBindingForeignKeyConstraints(t *testing.T, registry *Registry, ctx context.Context) {
	// Test that foreign key constraints work correctly

	// First create a valid service and address group
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("test-service", models.WithNamespace("constraint-test"))),
		Description: "Service for constraint test",
	}

	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("test-ag", models.WithNamespace("constraint-test"))),
		DefaultAction: models.ActionAccept,
	}

	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, []models.AddressGroup{addressGroup}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Create a valid binding
	validBinding := models.AddressGroupBinding{
		SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("valid-binding", models.WithNamespace("constraint-test"))),
		ServiceRef:      models.NewServiceRef("test-service", models.WithNamespace("constraint-test")),
		AddressGroupRef: models.NewAddressGroupRef("test-ag", models.WithNamespace("constraint-test")),
	}

	err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupBindings(ctx, []models.AddressGroupBinding{validBinding}, ports.EmptyScope{})
	})
	require.NoError(t, err, "Valid binding should be created successfully")

	// Test CASCADE delete - when we delete the service, the binding should also be deleted
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.DeleteServicesByIDs(ctx, []models.ResourceIdentifier{service.SelfRef.ResourceIdentifier})
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Verify that the binding was also deleted due to CASCADE
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	_, err = reader.GetAddressGroupBindingByID(ctx, validBinding.SelfRef.ResourceIdentifier)
	assert.Equal(t, ports.ErrNotFound, err, "Binding should be deleted due to CASCADE when service is deleted")
}
