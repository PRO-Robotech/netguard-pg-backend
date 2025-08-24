package pg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

// TestAddressGroupPortMapping_PostgreSQL tests AddressGroupPortMapping operations with PostgreSQL backend
func TestAddressGroupPortMapping_PostgreSQL(t *testing.T) {
	// Skip if PostgreSQL not available
	if !isPostgreSQLAvailable() {
		t.Skip("PostgreSQL not available, skipping integration tests")
	}

	registry := setupTestRegistry(t)
	defer registry.Close()

	ctx := context.Background()

	t.Run("AddressGroupPortMapping_CRUD_Operations", func(t *testing.T) {
		testAddressGroupPortMappingCRUD(t, registry, ctx)
	})

	t.Run("AddressGroupPortMapping_Complex_JSONB_Handling", func(t *testing.T) {
		testAddressGroupPortMappingComplexJSONB(t, registry, ctx)
	})

	t.Run("AddressGroupPortMapping_K8s_Metadata", func(t *testing.T) {
		testAddressGroupPortMappingK8sMetadata(t, registry, ctx)
	})

	t.Run("AddressGroupPortMapping_Scoped_Operations", func(t *testing.T) {
		testAddressGroupPortMappingScopedOperations(t, registry, ctx)
	})

	t.Run("AddressGroupPortMapping_Edge_Cases", func(t *testing.T) {
		testAddressGroupPortMappingEdgeCases(t, registry, ctx)
	})
}

func testAddressGroupPortMappingCRUD(t *testing.T, registry *Registry, ctx context.Context) {
	// Create test mapping with basic AccessPorts structure
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("web-ports", models.WithNamespace("default"))),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{
			models.NewServiceRef("web-frontend", models.WithNamespace("default")): {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{
						{Start: 80, End: 80},
						{Start: 443, End: 443},
					},
					models.UDP: []models.PortRange{
						{Start: 53, End: 53},
					},
				},
			},
			models.NewServiceRef("web-backend", models.WithNamespace("default")): {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{
						{Start: 8080, End: 8090},
						{Start: 9000, End: 9000},
					},
				},
			},
		},
		Meta: models.Meta{
			Labels: map[string]string{
				"component":   "port-mapping",
				"environment": "test",
			},
			Annotations: map[string]string{
				"description": "Test port mapping for web services",
			},
		},
	}

	// Test Create
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Test Read
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, mapping.Name, retrievedMapping.Name)
	assert.Equal(t, mapping.Namespace, retrievedMapping.Namespace)
	assert.NotEmpty(t, retrievedMapping.Meta.ResourceVersion)

	// Verify AccessPorts structure
	assert.Len(t, retrievedMapping.AccessPorts, 2)

	// Verify web-frontend ports
	frontendRef := models.NewServiceRef("web-frontend", models.WithNamespace("default"))
	frontendPorts, ok := retrievedMapping.AccessPorts[frontendRef]
	require.True(t, ok, "web-frontend ports should exist")
	assert.Len(t, frontendPorts.Ports[models.TCP], 2)
	assert.Len(t, frontendPorts.Ports[models.UDP], 1)
	assert.Equal(t, models.PortRange{Start: 80, End: 80}, frontendPorts.Ports[models.TCP][0])
	assert.Equal(t, models.PortRange{Start: 443, End: 443}, frontendPorts.Ports[models.TCP][1])
	assert.Equal(t, models.PortRange{Start: 53, End: 53}, frontendPorts.Ports[models.UDP][0])

	// Verify web-backend ports
	backendRef := models.NewServiceRef("web-backend", models.WithNamespace("default"))
	backendPorts, ok := retrievedMapping.AccessPorts[backendRef]
	require.True(t, ok, "web-backend ports should exist")
	assert.Len(t, backendPorts.Ports[models.TCP], 2)
	assert.Equal(t, models.PortRange{Start: 8080, End: 8090}, backendPorts.Ports[models.TCP][0])
	assert.Equal(t, models.PortRange{Start: 9000, End: 9000}, backendPorts.Ports[models.TCP][1])

	// Test Update - add new service mapping
	writer2, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer2.Close()

	mapping.AccessPorts[models.NewServiceRef("web-api", models.WithNamespace("default"))] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{
				{Start: 3000, End: 3000},
			},
		},
	}
	mapping.Meta.Labels["version"] = "v2"

	err = writer2.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
	require.NoError(t, err)

	err = writer2.Commit()
	require.NoError(t, err)

	// Verify Update
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	updatedMapping, err := reader2.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
	require.NoError(t, err)
	assert.Equal(t, "v2", updatedMapping.Meta.Labels["version"])
	assert.Len(t, updatedMapping.AccessPorts, 3) // Now has 3 services
	assert.NotEqual(t, updatedMapping.Meta.ResourceVersion, retrievedMapping.Meta.ResourceVersion)

	// Test Delete
	writer3, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer3.Close()

	err = writer3.DeleteAddressGroupPortMappingsByIDs(ctx, []models.ResourceIdentifier{mapping.SelfRef.ResourceIdentifier})
	require.NoError(t, err)

	err = writer3.Commit()
	require.NoError(t, err)

	// Verify Delete
	reader3, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader3.Close()

	_, err = reader3.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
	assert.Equal(t, ports.ErrNotFound, err)
}

func testAddressGroupPortMappingComplexJSONB(t *testing.T, registry *Registry, ctx context.Context) {
	// Test complex AccessPorts JSONB with multiple protocols and port ranges
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("complex-ports", models.WithNamespace("prod"))),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{
			// Service with complex TCP and UDP port ranges
			models.NewServiceRef("complex-service", models.WithNamespace("prod")): {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{
						{Start: 80, End: 80},     // HTTP
						{Start: 443, End: 443},   // HTTPS
						{Start: 8000, End: 8999}, // Development range
						{Start: 3000, End: 3010}, // API range
					},
					models.UDP: []models.PortRange{
						{Start: 53, End: 53},     // DNS
						{Start: 123, End: 123},   // NTP
						{Start: 5000, End: 5100}, // Custom range
					},
				},
			},
			// Service with only TCP
			models.NewServiceRef("tcp-only", models.WithNamespace("prod")): {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{
						{Start: 22, End: 22},      // SSH
						{Start: 1024, End: 65535}, // High ports
					},
				},
			},
			// Service with only UDP
			models.NewServiceRef("udp-only", models.WithNamespace("prod")): {
				Ports: models.ProtocolPorts{
					models.UDP: []models.PortRange{
						{Start: 67, End: 68},   // DHCP
						{Start: 161, End: 162}, // SNMP
					},
				},
			},
		},
	}

	// Create mapping
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Verify complex JSONB retrieval
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
	require.NoError(t, err)

	// Verify complex-service ports
	complexRef := models.NewServiceRef("complex-service", models.WithNamespace("prod"))
	complexPorts, ok := retrievedMapping.AccessPorts[complexRef]
	require.True(t, ok)
	assert.Len(t, complexPorts.Ports[models.TCP], 4)
	assert.Len(t, complexPorts.Ports[models.UDP], 3)

	// Verify specific port ranges
	tcpPorts := complexPorts.Ports[models.TCP]
	assert.Contains(t, tcpPorts, models.PortRange{Start: 8000, End: 8999})
	assert.Contains(t, tcpPorts, models.PortRange{Start: 3000, End: 3010})

	udpPorts := complexPorts.Ports[models.UDP]
	assert.Contains(t, udpPorts, models.PortRange{Start: 5000, End: 5100})

	// Verify TCP-only service
	tcpOnlyRef := models.NewServiceRef("tcp-only", models.WithNamespace("prod"))
	tcpOnlyPorts, ok := retrievedMapping.AccessPorts[tcpOnlyRef]
	require.True(t, ok)
	assert.Len(t, tcpOnlyPorts.Ports[models.TCP], 2)
	assert.Len(t, tcpOnlyPorts.Ports[models.UDP], 0) // No UDP ports
	assert.Contains(t, tcpOnlyPorts.Ports[models.TCP], models.PortRange{Start: 1024, End: 65535})

	// Verify UDP-only service
	udpOnlyRef := models.NewServiceRef("udp-only", models.WithNamespace("prod"))
	udpOnlyPorts, ok := retrievedMapping.AccessPorts[udpOnlyRef]
	require.True(t, ok)
	assert.Len(t, udpOnlyPorts.Ports[models.TCP], 0) // No TCP ports
	assert.Len(t, udpOnlyPorts.Ports[models.UDP], 2)
	assert.Contains(t, udpOnlyPorts.Ports[models.UDP], models.PortRange{Start: 67, End: 68})
	assert.Contains(t, udpOnlyPorts.Ports[models.UDP], models.PortRange{Start: 161, End: 162})
}

func testAddressGroupPortMappingK8sMetadata(t *testing.T, registry *Registry, ctx context.Context) {
	mapping := models.AddressGroupPortMapping{
		SelfRef: models.NewSelfRef(models.NewResourceIdentifier("k8s-metadata-test", models.WithNamespace("system"))),
		AccessPorts: map[models.ServiceRef]models.ServicePorts{
			models.NewServiceRef("system-service", models.WithNamespace("system")): {
				Ports: models.ProtocolPorts{
					models.TCP: []models.PortRange{
						{Start: 6443, End: 6443}, // Kubernetes API
					},
				},
			},
		},
		Meta: models.Meta{
			Labels: map[string]string{
				"k8s.io/managed-by":           "netguard",
				"app.kubernetes.io/name":      "port-mapping-system",
				"app.kubernetes.io/component": "network-policy",
				"tier":                        "system",
			},
			Annotations: map[string]string{
				"kubernetes.io/managed-by":                         "netguard-controller",
				"kubectl.kubernetes.io/last-applied-configuration": `{"kind":"AddressGroupPortMapping","apiVersion":"netguard.sgroups.io/v1beta1"}`,
				"netguard.sgroups.io/description":                  "System-level port mapping",
			},
		},
	}

	// Create mapping
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Verify K8s metadata
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
	require.NoError(t, err)

	// Check labels
	assert.Equal(t, "netguard", retrievedMapping.Meta.Labels["k8s.io/managed-by"])
	assert.Equal(t, "port-mapping-system", retrievedMapping.Meta.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "network-policy", retrievedMapping.Meta.Labels["app.kubernetes.io/component"])
	assert.Equal(t, "system", retrievedMapping.Meta.Labels["tier"])

	// Check annotations
	assert.Equal(t, "netguard-controller", retrievedMapping.Meta.Annotations["kubernetes.io/managed-by"])
	assert.Contains(t, retrievedMapping.Meta.Annotations, "kubectl.kubernetes.io/last-applied-configuration")
	assert.Equal(t, "System-level port mapping", retrievedMapping.Meta.Annotations["netguard.sgroups.io/description"])

	// Check timestamps and resource version would be here in extended model
	assert.NotEmpty(t, retrievedMapping.Meta.ResourceVersion)
}

func testAddressGroupPortMappingScopedOperations(t *testing.T, registry *Registry, ctx context.Context) {
	// Create mappings in different namespaces
	mappings := []models.AddressGroupPortMapping{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("prod-mapping-1", models.WithNamespace("prod"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("prod-service-1", models.WithNamespace("prod")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{{Start: 80, End: 80}},
					},
				},
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("prod-mapping-2", models.WithNamespace("prod"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("prod-service-2", models.WithNamespace("prod")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{{Start: 443, End: 443}},
					},
				},
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("dev-mapping-1", models.WithNamespace("dev"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("dev-service-1", models.WithNamespace("dev")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{{Start: 3000, End: 3000}},
					},
				},
			},
		},
	}

	// Create all mappings
	err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
		return writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{})
	})
	require.NoError(t, err)

	// Test namespace-scoped listing
	reader, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader.Close()

	// List mappings in production namespace
	scope := ports.ResourceIdentifierScope{
		Identifiers: []models.ResourceIdentifier{
			models.NewResourceIdentifier("", models.WithNamespace("prod")),
		},
	}

	var prodMappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		prodMappings = append(prodMappings, mapping)
		return nil
	}, scope)
	require.NoError(t, err)

	assert.Len(t, prodMappings, 2)
	for _, mapping := range prodMappings {
		assert.Equal(t, "prod", mapping.Namespace)
	}

	// Test scoped deletion - delete all mappings in prod namespace
	writer, err := registry.Writer(ctx)
	require.NoError(t, err)
	defer writer.Close()

	err = writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{}, scope)
	require.NoError(t, err)

	err = writer.Commit()
	require.NoError(t, err)

	// Verify deletion
	reader2, err := registry.Reader(ctx)
	require.NoError(t, err)
	defer reader2.Close()

	var remainingMappings []models.AddressGroupPortMapping
	err = reader2.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		remainingMappings = append(remainingMappings, mapping)
		return nil
	}, ports.EmptyScope{})
	require.NoError(t, err)

	assert.Len(t, remainingMappings, 1)
	assert.Equal(t, "dev", remainingMappings[0].Namespace)
}

func testAddressGroupPortMappingEdgeCases(t *testing.T, registry *Registry, ctx context.Context) {
	t.Run("Empty_AccessPorts_Map", func(t *testing.T) {
		// Test mapping with empty AccessPorts
		mapping := models.AddressGroupPortMapping{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("empty-ports", models.WithNamespace("edge"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{}, // Empty map
		}

		err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
			return writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
		})
		require.NoError(t, err)

		// Verify retrieval
		reader, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
		require.NoError(t, err)
		assert.Len(t, retrievedMapping.AccessPorts, 0)
	})

	t.Run("Large_Port_Ranges", func(t *testing.T) {
		// Test with very large port ranges
		mapping := models.AddressGroupPortMapping{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("large-ranges", models.WithNamespace("edge"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("wide-range-service", models.WithNamespace("edge")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{
							{Start: 1, End: 65535}, // Full port range
						},
						models.UDP: []models.PortRange{
							{Start: 1024, End: 32767},
							{Start: 40000, End: 60000},
						},
					},
				},
			},
		}

		err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
			return writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
		})
		require.NoError(t, err)

		// Verify large ranges are stored and retrieved correctly
		reader, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		serviceRef := models.NewServiceRef("wide-range-service", models.WithNamespace("edge"))
		ports := retrievedMapping.AccessPorts[serviceRef]

		assert.Equal(t, models.PortRange{Start: 1, End: 65535}, ports.Ports[models.TCP][0])
		assert.Len(t, ports.Ports[models.UDP], 2)
		assert.Equal(t, models.PortRange{Start: 1024, End: 32767}, ports.Ports[models.UDP][0])
		assert.Equal(t, models.PortRange{Start: 40000, End: 60000}, ports.Ports[models.UDP][1])
	})

	t.Run("Cross_Namespace_Service_References", func(t *testing.T) {
		// Test mapping that references services in different namespaces
		mapping := models.AddressGroupPortMapping{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("cross-ns", models.WithNamespace("edge"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("service-a", models.WithNamespace("ns-a")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{{Start: 8000, End: 8000}},
					},
				},
				models.NewServiceRef("service-b", models.WithNamespace("ns-b")): {
					Ports: models.ProtocolPorts{
						models.UDP: []models.PortRange{{Start: 9000, End: 9000}},
					},
				},
				models.NewServiceRef("service-c", models.WithNamespace("edge")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{{Start: 7000, End: 7000}},
					},
				},
			},
		}

		err := registry.WithTransaction(ctx, func(writer ports.Writer, reader ports.Reader) error {
			return writer.SyncAddressGroupPortMappings(ctx, []models.AddressGroupPortMapping{mapping}, ports.EmptyScope{})
		})
		require.NoError(t, err)

		// Verify cross-namespace references are preserved
		reader, err := registry.Reader(ctx)
		require.NoError(t, err)
		defer reader.Close()

		retrievedMapping, err := reader.GetAddressGroupPortMappingByID(ctx, mapping.SelfRef.ResourceIdentifier)
		require.NoError(t, err)

		assert.Len(t, retrievedMapping.AccessPorts, 3)

		// Verify each service reference maintains correct namespace
		refA := models.NewServiceRef("service-a", models.WithNamespace("ns-a"))
		refB := models.NewServiceRef("service-b", models.WithNamespace("ns-b"))
		refC := models.NewServiceRef("service-c", models.WithNamespace("edge"))

		portsA, okA := retrievedMapping.AccessPorts[refA]
		portsB, okB := retrievedMapping.AccessPorts[refB]
		portsC, okC := retrievedMapping.AccessPorts[refC]

		assert.True(t, okA, "service-a in ns-a should exist")
		assert.True(t, okB, "service-b in ns-b should exist")
		assert.True(t, okC, "service-c in edge should exist")

		assert.Len(t, portsA.Ports[models.TCP], 1)
		assert.Len(t, portsB.Ports[models.UDP], 1)
		assert.Len(t, portsC.Ports[models.TCP], 1)
	})
}
