package pg

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestService_PostgreSQL tests Service operations with PostgreSQL backend
func TestService_PostgreSQL(t *testing.T) {
	// Skip if PostgreSQL not available
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available, skipping integration tests")
	}

	registry := setupTestRegistry(t)
	defer registry.Close()

	ctx := context.Background()

	t.Run("Service_CRUD_Operations", func(t *testing.T) {
		testServiceCRUD(t, registry, ctx)
	})

	t.Run("Service_K8s_Metadata", func(t *testing.T) {
		testServiceK8sMetadata(t, registry, ctx)
	})

	t.Run("Service_Relationships", func(t *testing.T) {
		testServiceRelationships(t, registry, ctx)
	})

	t.Run("Service_Scoped_Operations", func(t *testing.T) {
		testServiceScopedOperations(t, registry, ctx)
	})

	t.Run("Service_Bulk_Sync", func(t *testing.T) {
		testServiceBulkSync(t, registry, ctx)
	})

	t.Run("Service_Transaction_Rollback", func(t *testing.T) {
		testServiceTransactionRollback(t, registry, ctx)
	})
}

func testServiceCRUD(t *testing.T, registry *Registry, ctx context.Context) {
	// Create test service
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web-service", models.WithNamespace("default"))),
		Description: "Test web service",
		IngressPorts: []models.IngressPort{
			{Protocol: models.TCP, Port: "80", Description: "HTTP"},
			{Protocol: models.TCP, Port: "443", Description: "HTTPS"},
		},
		Meta: models.Meta{
			Labels: map[string]string{
				"app":  "web",
				"tier": "frontend",
			},
			Annotations: map[string]string{
				"description": "Main web service",
			},
		},
	}

	// Test Create
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Test Read
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedService, err := reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, service.Name, retrievedService.Name)
	assert.Equal(t, service.Namespace, retrievedService.Namespace)
	assert.Equal(t, service.Description, retrievedService.Description)
	assert.Equal(t, len(service.IngressPorts), len(retrievedService.IngressPorts))
	assert.Equal(t, service.Meta.Labels["app"], retrievedService.Meta.Labels["app"])
	assert.Greater(t, retrievedService.Meta.ResourceVersion, int64(0))

	// Test Update
	writer2, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer2.Close()

	service.Description = "Updated web service"
	service.Meta.Labels["version"] = "v2"

	err = writer2.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer2.Commit()
	require.NoError(t, err)

	// Verify Update
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	updatedService, err := reader2.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "Updated web service", updatedService.Description)
	assert.Equal(t, "v2", updatedService.Meta.Labels["version"])
	assert.Greater(t, updatedService.Meta.ResourceVersion, retrievedService.Meta.ResourceVersion)

	// Test Delete
	writer3, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer3.Close()

	err = writer3.DeleteServicesByIDs(ctx, []models.ResourceIdentifier{service.SelfRef.ResourceIdentifier})
	require.NoError(t, err)

	err = writer3.Commit()
	require.NoError(t, err)

	// Verify Delete
	reader3, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader3.Close()

	_, err = reader3.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	assert.Equal(t, ports.ErrNotFound, err)
}

func testServiceK8sMetadata(t *testing.T, registry *Registry, ctx context.Context) {
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("k8s-service", models.WithNamespace("kube-system"))),
		Description: "Service with K8s metadata",
		Meta: models.Meta{
			Labels: map[string]string{
				"k8s.io/component": "apiserver",
				"k8s.io/instance":  "master",
			},
			Annotations: map[string]string{
				"kubernetes.io/managed-by":                         "netguard",
				"kubectl.kubernetes.io/last-applied-configuration": "{}",
			},
			Finalizers: []string{
				"netguard.sgroups.io/finalizer",
			},
		},
	}

	// Create service
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Verify K8s metadata
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedService, err := reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	require.NoError(t, err)

	// Check labels
	assert.Equal(t, "apiserver", retrievedService.Meta.Labels["k8s.io/component"])
	assert.Equal(t, "master", retrievedService.Meta.Labels["k8s.io/instance"])

	// Check annotations
	assert.Equal(t, "netguard", retrievedService.Meta.Annotations["kubernetes.io/managed-by"])
	assert.Contains(t, retrievedService.Meta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")

	// Check finalizers
	assert.Contains(t, retrievedService.Meta.Finalizers, "netguard.sgroups.io/finalizer")

	// Check timestamps
	assert.False(t, retrievedService.Meta.CreatedAt.IsZero())
	assert.False(t, retrievedService.Meta.UpdatedAt.IsZero())

	// Check resource version
	assert.Greater(t, retrievedService.Meta.ResourceVersion, int64(0))
}

func testServiceRelationships(t *testing.T, registry *Registry, ctx context.Context) {
	// Create service and address group first
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("default"))),
		Description: "Web service with relationships",
	}

	addressGroup := models.AddressGroup{
		SelfRef:       models.NewSelfRef(models.NewResourceIdentifier("internal", models.WithNamespace("default"))),
		DefaultAction: models.ActionAccept,
		Logs:          true,
	}

	// Create both resources
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		if err := writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{}); err != nil {
			return err
		}
		return writer.SyncAddressGroups(ctx, []models.AddressGroup{addressGroup}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Create binding (this would be implemented in Phase 2, but we can test the relationship loading)
	// For now, we'll test that the service loads with empty address groups initially
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedService, err := reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	require.NoError(t, err)

	// Initially no relationships
	assert.Empty(t, retrievedService.AddressGroups)

	// TODO: Add binding creation and verification when Phase 2 is implemented
}

func testServiceScopedOperations(t *testing.T, registry *Registry, ctx context.Context) {
	// Create services in different namespaces
	services := []models.Service{
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("production"))),
			Description: "Production web service",
		},
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("api", models.WithNamespace("production"))),
			Description: "Production API service",
		},
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("staging"))),
			Description: "Staging web service",
		},
	}

	// Create all services
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncServices(ctx, services, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Test namespace-scoped listing
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	// List services in production namespace
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			models.NewResourceIdentifier("", models.WithNamespace("production")),
		},
	}

	var productionServices []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		productionServices = append(productionServices, service)
		return nil
	}, scope)
	require.NoError(t, err)

	assert.Len(t, productionServices, 2)
	for _, svc := range productionServices {
		assert.Equal(t, "production", svc.Namespace)
	}

	// Test scoped deletion
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	// Delete all services in production namespace
	err = writer.SyncServices(ctx, []models.Service{}, scope) // Empty slice with scope = delete all in scope
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Verify deletion
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	var remainingServices []models.Service
	err = reader2.ListServices(ctx, func(service models.Service) error {
		remainingServices = append(remainingServices, service)
		return nil
	}, ports.EmptyScope{})
	require.NoError(t, err)

	assert.Len(t, remainingServices, 1)
	assert.Equal(t, "staging", remainingServices[0].Namespace)
}

func testServiceBulkSync(t *testing.T, registry *Registry, ctx context.Context) {
	// Create a large number of services for bulk operations
	services := make([]models.Service, 100)
	for i := 0; i < 100; i++ {
		services[i] = models.Service{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				fmt.Sprintf("service-%03d", i),
				models.WithNamespace("bulk-test"),
			)),
			Description: fmt.Sprintf("Bulk test service %d", i),
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: fmt.Sprintf("%d", 8000+i), Description: "HTTP"},
			},
		}
	}

	// Measure bulk sync performance
	start := time.Now()

	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncServices(ctx, services, ports.EmptyScope{})
	})
	require.NoError(t, err)

	duration := time.Since(start)
	t.Logf("Bulk sync of 100 services took: %v", duration)

	// Verify all services were created
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			models.NewResourceIdentifier("", models.WithNamespace("bulk-test")),
		},
	}

	var createdServices []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		createdServices = append(createdServices, service)
		return nil
	}, scope)
	require.NoError(t, err)

	assert.Len(t, createdServices, 100)

	// Test bulk update
	for i := range services {
		services[i].Description = fmt.Sprintf("Updated bulk service %d", i)
		services[i].Meta.Labels = map[string]string{"batch": "updated"}
	}

	start = time.Now()
	err = registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncServices(ctx, services, ports.EmptyScope{})
	})
	require.NoError(t, err)

	updateDuration := time.Since(start)
	t.Logf("Bulk update of 100 services took: %v", updateDuration)

	// Verify updates
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	var updatedServices []models.Service
	err = reader2.ListServices(ctx, func(service models.Service) error {
		updatedServices = append(updatedServices, service)
		return nil
	}, scope)
	require.NoError(t, err)

	for _, svc := range updatedServices {
		assert.Contains(t, svc.Description, "Updated bulk service")
		assert.Equal(t, "updated", svc.Meta.Labels["batch"])
	}
}

func testServiceTransactionRollback(t *testing.T, registry *Registry, ctx context.Context) {
	service := models.Service{
		SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("rollback-test", models.WithNamespace("default"))),
		Description: "Service for rollback test",
	}

	// Test transaction rollback
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.SyncServices(ctx, []models.Service{service}, ports.EmptyScope{})
	require.NoError(t, err)

	// Rollback the transaction
	err = writer.Abort()
	require.NoError(t, err)

	// Verify service was not created
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	_, err = reader.GetServiceByID(ctx, service.SelfRef.ResourceIdentifier)
	assert.Equal(t, ports.ErrNotFound, err)
}

// Helper functions for testing
func isPostgreSQLAvailable() bool {
	// Check if PostgreSQL connection URI is available
	uri := os.Getenv("TEST_PG_URI")
	if uri == "" {
		uri = "postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable"
	}

	config := ConnectionConfig{
		URI:           uri,
		MaxConns:      5,
		MinConns:      1,
		HealthTimeout: 5 * time.Second,
	}

	connManager := NewConnectionManager(config)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := connManager.Connect(ctx)
	if err != nil {
		return false
	}

	connManager.Close()
	return true
}

func setupTestRegistry(t *testing.T) *Registry {
	uri := os.Getenv("TEST_PG_URI")
	if uri == "" {
		uri = "postgres://postgres:postgres@localhost:5432/netguard_test?sslmode=disable"
	}

	config := ConnectionConfig{
		URI:             uri,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		HealthTimeout:   30 * time.Second,
	}

	connManager := NewConnectionManager(config)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := connManager.Connect(ctx)
	require.NoError(t, err, "Failed to connect to test database")

	// Run migrations
	err = connManager.RunMigrations(ctx, "../../../../../migrations")
	require.NoError(t, err, "Failed to run migrations")

	registry := NewRegistry(connManager)

	// Clean up test data
	cleanupTestData(t, registry)

	return registry
}

func cleanupTestData(t *testing.T, registry *Registry) {
	ctx := context.Background()

	// Clean up all test data
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		// Delete services in test namespaces
		testNamespaces := []string{"default", "production", "staging", "bulk-test", "kube-system"}
		for _, ns := range testNamespaces {
			scope := ports.ResourceIdentifierScope{
				Identifiers: []models.ResourceIdentifier{
					models.NewResourceIdentifier("", models.WithNamespace(ns)),
				},
			}
			if err := writer.SyncServices(ctx, []models.Service{}, scope); err != nil {
				return err
			}
			if err := writer.SyncAddressGroups(ctx, []models.AddressGroup{}, scope); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err, "Failed to cleanup test data")
}
