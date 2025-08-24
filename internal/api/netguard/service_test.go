package netguard

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"netguard-pg-backend/internal/application/services"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
	// commonpb "github.com/H-BF/protos/pkg/api/common" - replaced with local types
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestSync(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Create test request
	req := &netguardpb.SyncReq{
		SyncOp: netguardpb.SyncOp_FullSync,
		Subject: &netguardpb.SyncReq_Services{
			Services: &netguardpb.SyncServices{
				Services: []*netguardpb.Service{
					{
						SelfRef: &netguardpb.ResourceIdentifier{
							Name:      "web",
							Namespace: "default",
						},
						Description: "Web service",
						IngressPorts: []*netguardpb.IngressPort{
							{
								Protocol:    netguardpb.Networks_NetIP_TCP,
								Port:        "80",
								Description: "HTTP",
							},
						},
					},
				},
			},
		},
	}

	// Call Sync method
	ctx := context.Background()
	_, err := server.Sync(ctx, req)
	if err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Check that data was saved
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	var foundServices []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		foundServices = append(foundServices, service)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list services: %v", err)
	}

	if len(foundServices) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(foundServices))
	}

	if foundServices[0].Name != "web" {
		t.Errorf("Expected name 'web', got '%s'", foundServices[0].Name)
	}
}

// TestSyncWithDifferentOperations tests the Sync method with different operations and entity types
func TestSyncWithDifferentOperations(t *testing.T) {
	// Test Services with different operations
	t.Run("Services", func(t *testing.T) {
		// Create in-memory registry for tests
		registry := mem.NewRegistry()
		defer registry.Close()

		// Create service
		service := services.NewNetguardService(registry, nil)

		// Create API server
		server := NewNetguardServiceServer(service)

		ctx := context.Background()

		// Test FullSync operation
		t.Run("FullSync", func(t *testing.T) {
			// Create initial services
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web",
									Namespace: "default",
								},
								Description: "Web service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "db",
									Namespace: "default",
								},
								Description: "Database service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "5432",
										Description: "PostgreSQL",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial services: %v", err)
			}

			// Verify initial data
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundServices []models.Service
			err = reader.ListServices(ctx, func(service models.Service) error {
				foundServices = append(foundServices, service)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list services: %v", err)
			}

			if len(foundServices) != 2 {
				t.Fatalf("Expected 2 services after initial sync, got %d", len(foundServices))
			}

			// Create new services for FullSync
			fullSyncReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "api",
									Namespace: "default",
								},
								Description: "API service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "8080",
										Description: "API",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with FullSync
			_, err = server.Sync(ctx, fullSyncReq)
			if err != nil {
				t.Fatalf("Failed to sync with FullSync: %v", err)
			}

			// Verify that old data was replaced with new data
			reader, err = registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			foundServices = nil
			err = reader.ListServices(ctx, func(service models.Service) error {
				foundServices = append(foundServices, service)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list services: %v", err)
			}

			if len(foundServices) != 1 {
				t.Fatalf("Expected 1 service after FullSync, got %d", len(foundServices))
			}

			if foundServices[0].Name != "api" {
				t.Errorf("Expected service name 'api', got '%s'", foundServices[0].Name)
			}
		})

		// Test Upsert operation
		t.Run("Upsert", func(t *testing.T) {
			// Create a new registry for this test
			registry := mem.NewRegistry()
			defer registry.Close()

			// Create service
			service := services.NewNetguardService(registry, nil)

			// Create API server
			server := NewNetguardServiceServer(service)

			// Create initial services
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web",
									Namespace: "default",
								},
								Description: "Web service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "db",
									Namespace: "default",
								},
								Description: "Database service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "5432",
										Description: "PostgreSQL",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial services: %v", err)
			}

			// Create services for Upsert
			upsertReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_Upsert,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web",
									Namespace: "default",
								},
								Description: "Updated web service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "80",
										Description: "HTTP",
									},
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "443",
										Description: "HTTPS",
									},
								},
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "api",
									Namespace: "default",
								},
								Description: "API service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "8080",
										Description: "API",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with Upsert
			_, err = server.Sync(ctx, upsertReq)
			if err != nil {
				t.Fatalf("Failed to sync with Upsert: %v", err)
			}

			// Verify that data was updated and added, but not deleted
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundServices []models.Service
			err = reader.ListServices(ctx, func(service models.Service) error {
				foundServices = append(foundServices, service)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list services: %v", err)
			}

			if len(foundServices) != 3 {
				t.Fatalf("Expected 3 services after Upsert, got %d", len(foundServices))
			}

			// Find web service and check if it was updated
			var webService *models.Service
			for i, service := range foundServices {
				if service.Name == "web" {
					webService = &foundServices[i]
					break
				}
			}

			if webService == nil {
				t.Fatalf("Web service not found after Upsert")
			}

			if webService.Description != "Updated web service" {
				t.Errorf("Expected web service description 'Updated web service', got '%s'", webService.Description)
			}

			if len(webService.IngressPorts) != 2 {
				t.Errorf("Expected 2 ingress ports for web service, got %d", len(webService.IngressPorts))
			}
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			// Create a new registry for this test
			registry := mem.NewRegistry()
			defer registry.Close()

			// Create service
			service := services.NewNetguardService(registry, nil)

			// Create API server
			server := NewNetguardServiceServer(service)

			// Create initial services
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web",
									Namespace: "default",
								},
								Description: "Web service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "80",
										Description: "HTTP",
									},
								},
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "db",
									Namespace: "default",
								},
								Description: "Database service",
								IngressPorts: []*netguardpb.IngressPort{
									{
										Protocol:    netguardpb.Networks_NetIP_TCP,
										Port:        "5432",
										Description: "PostgreSQL",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial services: %v", err)
			}

			// Create services for Delete
			deleteReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_Delete,
				Subject: &netguardpb.SyncReq_Services{
					Services: &netguardpb.SyncServices{
						Services: []*netguardpb.Service{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web",
									Namespace: "default",
								},
							},
						},
					},
				},
			}

			// Call Sync method with Delete
			_, err = server.Sync(ctx, deleteReq)
			if err != nil {
				t.Fatalf("Failed to sync with Delete: %v", err)
			}

			// Verify that data was deleted
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundServices []models.Service
			err = reader.ListServices(ctx, func(service models.Service) error {
				foundServices = append(foundServices, service)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list services: %v", err)
			}

			if len(foundServices) != 1 {
				t.Fatalf("Expected 1 service after Delete, got %d", len(foundServices))
			}

			if foundServices[0].Name != "db" {
				t.Errorf("Expected service name 'db', got '%s'", foundServices[0].Name)
			}
		})
	})

	// Test AddressGroups with different operations
	t.Run("AddressGroups", func(t *testing.T) {
		// Create in-memory registry for tests
		registry := mem.NewRegistry()
		defer registry.Close()

		// Create service
		service := services.NewNetguardService(registry, nil)

		// Create API server
		server := NewNetguardServiceServer(service)

		ctx := context.Background()

		// Test FullSync operation
		t.Run("FullSync", func(t *testing.T) {
			// Create initial address groups
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "internal",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "external",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial address groups: %v", err)
			}

			// Verify initial data
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundAddressGroups []models.AddressGroup
			err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
				foundAddressGroups = append(foundAddressGroups, addressGroup)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list address groups: %v", err)
			}

			if len(foundAddressGroups) != 2 {
				t.Fatalf("Expected 2 address groups after initial sync, got %d", len(foundAddressGroups))
			}

			// Create new address groups for FullSync
			fullSyncReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "dmz",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
						},
					},
				},
			}

			// Call Sync method with FullSync
			_, err = server.Sync(ctx, fullSyncReq)
			if err != nil {
				t.Fatalf("Failed to sync with FullSync: %v", err)
			}

			// Verify that old data was replaced with new data
			reader, err = registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			foundAddressGroups = nil
			err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
				foundAddressGroups = append(foundAddressGroups, addressGroup)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list address groups: %v", err)
			}

			if len(foundAddressGroups) != 1 {
				t.Fatalf("Expected 1 address group after FullSync, got %d", len(foundAddressGroups))
			}

			if foundAddressGroups[0].Name != "dmz" {
				t.Errorf("Expected address group name 'dmz', got '%s'", foundAddressGroups[0].Name)
			}
		})

		// Test Upsert operation
		t.Run("Upsert", func(t *testing.T) {
			// Create a new registry for this test
			registry := mem.NewRegistry()
			defer registry.Close()

			// Create service
			service := services.NewNetguardService(registry, nil)

			// Create API server
			server := NewNetguardServiceServer(service)

			// Create initial address groups
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "internal",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "external",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial address groups: %v", err)
			}

			// Create address groups for Upsert
			upsertReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_Upsert,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "internal",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "dmz",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
						},
					},
				},
			}

			// Call Sync method with Upsert
			_, err = server.Sync(ctx, upsertReq)
			if err != nil {
				t.Fatalf("Failed to sync with Upsert: %v", err)
			}

			// Verify that data was updated and added, but not deleted
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundAddressGroups []models.AddressGroup
			err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
				foundAddressGroups = append(foundAddressGroups, addressGroup)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list address groups: %v", err)
			}

			if len(foundAddressGroups) != 3 {
				t.Fatalf("Expected 3 address groups after Upsert, got %d", len(foundAddressGroups))
			}

			// Find internal address group and check if it was updated
			var internalAddressGroup *models.AddressGroup
			for i, addressGroup := range foundAddressGroups {
				if addressGroup.Name == "internal" {
					internalAddressGroup = &foundAddressGroups[i]
					break
				}
			}

			if internalAddressGroup == nil {
				t.Fatalf("Internal address group not found after Upsert")
			}

			if internalAddressGroup.DefaultAction != models.ActionAccept {
				t.Errorf("Expected rule action 'ACCEPT' for internal address group, got '%s'", internalAddressGroup.DefaultAction)
			}

			if internalAddressGroup.Logs != true {
				t.Errorf("Expected logs 'true' for internal address group, got '%v'", internalAddressGroup.Logs)
			}

			if internalAddressGroup.Trace != false {
				t.Errorf("Expected trace 'false' for internal address group, got '%v'", internalAddressGroup.Trace)
			}
		})

		// Test Delete operation
		t.Run("Delete", func(t *testing.T) {
			// Create a new registry for this test
			registry := mem.NewRegistry()
			defer registry.Close()

			// Create service
			service := services.NewNetguardService(registry, nil)

			// Create API server
			server := NewNetguardServiceServer(service)

			// Create initial address groups
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "internal",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "external",
									Namespace: "default",
								},
								DefaultAction: netguardpb.RuleAction_ACCEPT,
								Logs:          true,
								Trace:         false,
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial address groups: %v", err)
			}

			// Create address groups for Delete
			deleteReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_Delete,
				Subject: &netguardpb.SyncReq_AddressGroups{
					AddressGroups: &netguardpb.SyncAddressGroups{
						AddressGroups: []*netguardpb.AddressGroup{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "internal",
									Namespace: "default",
								},
							},
						},
					},
				},
			}

			// Call Sync method with Delete
			_, err = server.Sync(ctx, deleteReq)
			if err != nil {
				t.Fatalf("Failed to sync with Delete: %v", err)
			}

			// Verify that data was deleted
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundAddressGroups []models.AddressGroup
			err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
				foundAddressGroups = append(foundAddressGroups, addressGroup)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list address groups: %v", err)
			}

			if len(foundAddressGroups) != 1 {
				t.Fatalf("Expected 1 address group after Delete, got %d", len(foundAddressGroups))
			}

			if foundAddressGroups[0].Name != "external" {
				t.Errorf("Expected address group name 'external', got '%s'", foundAddressGroups[0].Name)
			}
		})
	})

	// Test AddressGroupBindings with different operations
	t.Run("AddressGroupBindings", func(t *testing.T) {
		// Create in-memory registry for tests
		registry := mem.NewRegistry()
		defer registry.Close()

		// Create service
		service := services.NewNetguardService(registry, nil)

		// Create API server
		server := NewNetguardServiceServer(service)

		ctx := context.Background()

		// First, create services and address groups that will be referenced by bindings
		servicesReq := &netguardpb.SyncReq{
			SyncOp: netguardpb.SyncOp_FullSync,
			Subject: &netguardpb.SyncReq_Services{
				Services: &netguardpb.SyncServices{
					Services: []*netguardpb.Service{
						{
							SelfRef: &netguardpb.ResourceIdentifier{
								Name:      "web",
								Namespace: "default",
							},
							Description: "Web service",
						},
						{
							SelfRef: &netguardpb.ResourceIdentifier{
								Name:      "api",
								Namespace: "default",
							},
							Description: "API service",
						},
					},
				},
			},
		}

		// Call Sync method to create services
		_, err := server.Sync(ctx, servicesReq)
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		addressGroupsReq := &netguardpb.SyncReq{
			SyncOp: netguardpb.SyncOp_FullSync,
			Subject: &netguardpb.SyncReq_AddressGroups{
				AddressGroups: &netguardpb.SyncAddressGroups{
					AddressGroups: []*netguardpb.AddressGroup{
						{
							SelfRef: &netguardpb.ResourceIdentifier{
								Name:      "internal",
								Namespace: "default",
							},
							DefaultAction: netguardpb.RuleAction_ACCEPT,
							Logs:          true,
							Trace:         false,
						},
						{
							SelfRef: &netguardpb.ResourceIdentifier{
								Name:      "external",
								Namespace: "default",
							},
							DefaultAction: netguardpb.RuleAction_ACCEPT,
							Logs:          true,
							Trace:         false,
						},
					},
				},
			},
		}

		// Call Sync method to create address groups
		_, err = server.Sync(ctx, addressGroupsReq)
		if err != nil {
			t.Fatalf("Failed to sync address groups: %v", err)
		}

		// Test FullSync operation
		t.Run("FullSync", func(t *testing.T) {
			// Create initial bindings
			initialReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroupBindings{
					AddressGroupBindings: &netguardpb.SyncAddressGroupBindings{
						AddressGroupBindings: []*netguardpb.AddressGroupBinding{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web-internal",
									Namespace: "default",
								},
								ServiceRef: &netguardpb.ServiceRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "web",
										Namespace: "default",
									},
								},
								AddressGroupRef: &netguardpb.AddressGroupRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "internal",
										Namespace: "default",
									},
								},
							},
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "web-external",
									Namespace: "default",
								},
								ServiceRef: &netguardpb.ServiceRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "web",
										Namespace: "default",
									},
								},
								AddressGroupRef: &netguardpb.AddressGroupRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "external",
										Namespace: "default",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with initial data
			_, err := server.Sync(ctx, initialReq)
			if err != nil {
				t.Fatalf("Failed to sync initial bindings: %v", err)
			}

			// Verify initial data
			reader, err := registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			var foundBindings []models.AddressGroupBinding
			err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
				foundBindings = append(foundBindings, binding)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list bindings: %v", err)
			}

			if len(foundBindings) != 2 {
				t.Fatalf("Expected 2 bindings after initial sync, got %d", len(foundBindings))
			}

			// Create new bindings for FullSync
			fullSyncReq := &netguardpb.SyncReq{
				SyncOp: netguardpb.SyncOp_FullSync,
				Subject: &netguardpb.SyncReq_AddressGroupBindings{
					AddressGroupBindings: &netguardpb.SyncAddressGroupBindings{
						AddressGroupBindings: []*netguardpb.AddressGroupBinding{
							{
								SelfRef: &netguardpb.ResourceIdentifier{
									Name:      "api-internal",
									Namespace: "default",
								},
								ServiceRef: &netguardpb.ServiceRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "api",
										Namespace: "default",
									},
								},
								AddressGroupRef: &netguardpb.AddressGroupRef{
									Identifier: &netguardpb.ResourceIdentifier{
										Name:      "internal",
										Namespace: "default",
									},
								},
							},
						},
					},
				},
			}

			// Call Sync method with FullSync
			_, err = server.Sync(ctx, fullSyncReq)
			if err != nil {
				t.Fatalf("Failed to sync with FullSync: %v", err)
			}

			// Verify that old data was replaced with new data
			reader, err = registry.Reader(ctx)
			if err != nil {
				t.Fatalf("Failed to get reader: %v", err)
			}
			defer reader.Close()

			foundBindings = nil
			err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
				foundBindings = append(foundBindings, binding)
				return nil
			}, ports.EmptyScope{})
			if err != nil {
				t.Fatalf("Failed to list bindings: %v", err)
			}

			if len(foundBindings) != 1 {
				t.Fatalf("Expected 1 binding after FullSync, got %d", len(foundBindings))
			}

			if foundBindings[0].Name != "api-internal" {
				t.Errorf("Expected binding name 'api-internal', got '%s'", foundBindings[0].Name)
			}
		})
	})
}

func TestConvertSyncOp(t *testing.T) {
	tests := []struct {
		name    string
		protoOp netguardpb.SyncOp
		modelOp models.SyncOp
	}{
		{"NoOp", netguardpb.SyncOp_NoOp, models.SyncOpNoOp},
		{"FullSync", netguardpb.SyncOp_FullSync, models.SyncOpFullSync},
		{"Upsert", netguardpb.SyncOp_Upsert, models.SyncOpUpsert},
		{"Delete", netguardpb.SyncOp_Delete, models.SyncOpDelete},
		{"Invalid", netguardpb.SyncOp(99), models.SyncOpFullSync}, // По умолчанию должен быть FullSync
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test convertSyncOp (proto -> model)
			modelOp := convertSyncOp(tt.protoOp)
			if modelOp != tt.modelOp {
				t.Errorf("convertSyncOp(%v) = %v, want %v", tt.protoOp, modelOp, tt.modelOp)
			}

			// Test convertSyncOpToPB (model -> proto)
			if tt.name != "Invalid" { // Пропускаем тест для недопустимого значения
				protoOp := convertSyncOpToPB(tt.modelOp)
				if protoOp != tt.protoOp {
					t.Errorf("convertSyncOpToPB(%v) = %v, want %v", tt.modelOp, protoOp, tt.protoOp)
				}
			}
		})
	}
}

func TestSyncStatus(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	services := []models.Service{
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("default"))),
			Description: "Web service",
		},
	}

	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call SyncStatus method
	resp, err := server.SyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("Failed to get sync status: %v", err)
	}

	if resp.UpdatedAt == nil {
		t.Fatalf("Expected non-nil updated_at")
	}

	// Check that the updated at time is recent
	updatedAt := resp.UpdatedAt.AsTime()
	if time.Since(updatedAt) > time.Minute {
		t.Errorf("Expected recent updated at time, got %v", updatedAt)
	}
}

func TestListServices(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	services := []models.Service{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"web", models.WithNamespace("default"))),
			Description: "Web service",
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80", Description: "HTTP"},
			},
			AddressGroups: []models.AddressGroupRef{
				models.NewAddressGroupRef("internal", models.WithNamespace("default")),
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"web", models.WithNamespace("not default"))),
			Description: "Database service",
			AddressGroups: []models.AddressGroupRef{
				models.NewAddressGroupRef("external", models.WithNamespace("default")),
			},
		},
	}

	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call ListServices method
	req := &netguardpb.ListServicesReq{}
	resp, err := server.ListServices(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list services: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(resp.Items))
	}

	// Test filtering by name
	req = &netguardpb.ListServicesReq{
		Identifiers: []*netguardpb.ResourceIdentifier{
			{Name: "web", Namespace: "default"},
		},
	}
	resp, err = server.ListServices(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list services: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(resp.Items))
	}

	if resp.Items[0].SelfRef.Name != "web" {
		t.Errorf("Expected name 'web', got '%s'", resp.Items[0].SelfRef.GetName())
	}
}

func TestListAddressGroups(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	addressGroups := []models.AddressGroup{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"internal", models.WithNamespace("default"))),
			DefaultAction: models.ActionAccept,
			Logs:          true,
			Trace:         false,
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"external", models.WithNamespace("default"))),
			DefaultAction: models.ActionAccept,
			Logs:          true,
			Trace:         false,
		},
	}

	err = writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call ListAddressGroups method
	req := &netguardpb.ListAddressGroupsReq{}
	resp, err := server.ListAddressGroups(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address groups: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("Expected 2 address groups, got %d", len(resp.Items))
	}

	// Test filtering by name
	req = &netguardpb.ListAddressGroupsReq{
		Identifiers: []*netguardpb.ResourceIdentifier{
			{Name: "internal", Namespace: "default"},
		},
	}
	resp, err = server.ListAddressGroups(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address groups: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("Expected 1 address group, got %d", len(resp.Items))
	}

	if resp.Items[0].SelfRef.GetName() != "internal" {
		t.Errorf("Expected name 'internal', got '%s'", resp.Items[0].SelfRef.GetName())
	}
}

func TestListAddressGroupBindings(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	bindings := []models.AddressGroupBinding{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("web-internal", models.WithNamespace("default"))),
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier("web", models.WithNamespace("default")),
			},
			AddressGroupRef: models.AddressGroupRef{
				ResourceIdentifier: models.NewResourceIdentifier("internal", models.WithNamespace("default")),
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("db-internal", models.WithNamespace("default"))),
			ServiceRef: models.ServiceRef{
				ResourceIdentifier: models.NewResourceIdentifier("db", models.WithNamespace("default")),
			},
			AddressGroupRef: models.AddressGroupRef{
				ResourceIdentifier: models.NewResourceIdentifier("internal", models.WithNamespace("default")),
			},
		},
	}

	err = writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address group bindings: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call ListAddressGroupBindings method
	req := &netguardpb.ListAddressGroupBindingsReq{}
	resp, err := server.ListAddressGroupBindings(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address group bindings: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("Expected 2 address group bindings, got %d", len(resp.Items))
	}

	// Test filtering by name
	req = &netguardpb.ListAddressGroupBindingsReq{
		Identifiers: []*netguardpb.ResourceIdentifier{
			{
				Name:      "web-internal",
				Namespace: "default",
			},
		},
	}
	resp, err = server.ListAddressGroupBindings(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address group bindings: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("Expected 1 address group binding, got %d", len(resp.Items))
	}

	if resp.Items[0].SelfRef.GetName() != "web-internal" {
		t.Errorf("Expected name 'web-internal', got '%s'", resp.Items[0].SelfRef.GetName())
	}
}

func TestListAddressGroupPortMappings(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	mappings := []models.AddressGroupPortMapping{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("internal-ports", models.WithNamespace("default"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("web", models.WithNamespace("default")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{
							{Start: 80, End: 80},
							{Start: 443, End: 443},
						},
					},
				},
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("external-ports", models.WithNamespace("default"))),
			AccessPorts: map[models.ServiceRef]models.ServicePorts{
				models.NewServiceRef("db", models.WithNamespace("default")): {
					Ports: models.ProtocolPorts{
						models.TCP: []models.PortRange{
							{Start: 5432, End: 5432},
						},
					},
				},
			},
		},
	}

	err = writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address group port mappings: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call ListAddressGroupPortMappings method
	req := &netguardpb.ListAddressGroupPortMappingsReq{}
	resp, err := server.ListAddressGroupPortMappings(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address group port mappings: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("Expected 2 address group port mappings, got %d", len(resp.Items))
	}

	// Test filtering by name
	req = &netguardpb.ListAddressGroupPortMappingsReq{
		Identifiers: []*netguardpb.ResourceIdentifier{
			{Name: "internal-ports", Namespace: "default"},
		},
	}
	resp, err = server.ListAddressGroupPortMappings(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list address group port mappings: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("Expected 1 address group port mapping, got %d", len(resp.Items))
	}

	if resp.Items[0].SelfRef.GetName() != "internal-ports" {
		t.Errorf("Expected name 'internal-ports', got '%s'", resp.Items[0].SelfRef.GetName())
	}
}

func TestListRuleS2S(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry, nil)

	// Create API server
	server := NewNetguardServiceServer(service)

	// Prepare data
	ctx := context.Background()
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	rules := []models.RuleS2S{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("web-to-db", models.WithNamespace("default"))),
			Traffic: models.EGRESS,
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("web", models.WithNamespace("default")),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("db", models.WithNamespace("default")),
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("db-to-web", models.WithNamespace("default"))),
			Traffic: models.INGRESS,
			ServiceLocalRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("db", models.WithNamespace("default")),
			},
			ServiceRef: models.ServiceAliasRef{
				ResourceIdentifier: models.NewResourceIdentifier("web", models.WithNamespace("default")),
			},
		},
	}

	err = writer.SyncRuleS2S(ctx, rules, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync rule s2s: %v", err)
	}

	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call ListRuleS2S method
	req := &netguardpb.ListRuleS2SReq{}
	resp, err := server.ListRuleS2S(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list rule s2s: %v", err)
	}

	if len(resp.Items) != 2 {
		t.Fatalf("Expected 2 rule s2s, got %d", len(resp.Items))
	}

	// Test filtering by name
	req = &netguardpb.ListRuleS2SReq{
		Identifiers: []*netguardpb.ResourceIdentifier{
			{Name: "web-to-db", Namespace: "default"},
		},
	}
	resp, err = server.ListRuleS2S(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list rule s2s: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("Expected 1 rule s2s, got %d", len(resp.Items))
	}

	if resp.Items[0].SelfRef.GetName() != "web-to-db" {
		t.Errorf("Expected name 'web-to-db', got '%s'", resp.Items[0].SelfRef.GetName())
	}
}
