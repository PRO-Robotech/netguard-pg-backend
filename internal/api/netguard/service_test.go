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
	commonpb "netguard-pg-backend/protos/pkg/api/common"
	netguardpb "netguard-pg-backend/protos/pkg/api/netguard"
)

func TestSync(t *testing.T) {
	// Create in-memory registry for tests
	registry := mem.NewRegistry()
	defer registry.Close()

	// Create service
	service := services.NewNetguardService(registry)

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
								Protocol:    commonpb.Networks_NetIP_TCP,
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
	service := services.NewNetguardService(registry)

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
	service := services.NewNetguardService(registry)

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
	service := services.NewNetguardService(registry)

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
			Description: "Internal addresses",
			Addresses:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			Services: []models.ServiceRef{
				models.NewServiceRef("web", models.WithNamespace("default")),
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"external", models.WithNamespace("default"))),
			Description: "External addresses",
			Addresses:   []string{"0.0.0.0/0"},
			Services: []models.ServiceRef{
				models.NewServiceRef("db", models.WithNamespace("default")),
			},
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
	service := services.NewNetguardService(registry)

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
	service := services.NewNetguardService(registry)

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
	service := services.NewNetguardService(registry)

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
