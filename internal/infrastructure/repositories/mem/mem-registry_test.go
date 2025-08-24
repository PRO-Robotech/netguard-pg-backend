package mem

import (
	"context"
	"testing"
	"time"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

func TestMemRegistry(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	services := []models.Service{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"web", models.WithNamespace("default"))),
			Description: "Web service",
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80", Description: "HTTP"},
			},
		},
	}

	// Sync data
	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
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

func TestMemRegistryAddressGroups(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	addressGroups := []models.AddressGroup{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"internal", models.WithNamespace("default"))),
			DefaultAction: models.ActionAccept,
			Logs:          true,
			Trace:         false,
		},
	}

	// Sync data
	err = writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
	var foundAddressGroups []models.AddressGroup
	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
		foundAddressGroups = append(foundAddressGroups, addressGroup)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list address groups: %v", err)
	}

	if len(foundAddressGroups) != 1 {
		t.Fatalf("Expected 1 address group, got %d", len(foundAddressGroups))
	}

	if foundAddressGroups[0].Name != "internal" {
		t.Errorf("Expected name 'internal', got '%s'", foundAddressGroups[0].Name)
	}

	if foundAddressGroups[0].DefaultAction != models.ActionAccept {
		t.Errorf("Expected DefaultAction ACCEPT, got %s", foundAddressGroups[0].DefaultAction)
	}
}

func TestMemRegistryAddressGroupBindings(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	bindings := []models.AddressGroupBinding{
		{
			SelfRef:         models.NewSelfRef(models.NewResourceIdentifier("web-internal", models.WithNamespace("default"))),
			ServiceRef:      models.NewServiceRef("web", models.WithNamespace("default")),
			AddressGroupRef: models.NewAddressGroupRef("internal", models.WithNamespace("default")),
		},
	}

	// Sync data
	err = writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address group bindings: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
	var foundBindings []models.AddressGroupBinding
	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		foundBindings = append(foundBindings, binding)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list address group bindings: %v", err)
	}

	if len(foundBindings) != 1 {
		t.Fatalf("Expected 1 binding, got %d", len(foundBindings))
	}

	if foundBindings[0].Name != "web-internal" {
		t.Errorf("Expected name 'web-internal', got '%s'", foundBindings[0].Name)
	}

	if foundBindings[0].ServiceRef.Name != "web" {
		t.Errorf("Expected service name 'web', got '%s'", foundBindings[0].ServiceRef.Name)
	}

	if foundBindings[0].AddressGroupRef.Name != "internal" {
		t.Errorf("Expected address group name 'internal', got '%s'", foundBindings[0].AddressGroupRef.Name)
	}
}

func TestMemRegistryAddressGroupPortMappings(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
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
	}

	// Sync data
	err = writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync address group port mappings: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
	var foundMappings []models.AddressGroupPortMapping
	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		foundMappings = append(foundMappings, mapping)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list address group port mappings: %v", err)
	}

	if len(foundMappings) != 1 {
		t.Fatalf("Expected 1 mapping, got %d", len(foundMappings))
	}

	if foundMappings[0].Name != "internal-ports" {
		t.Errorf("Expected name 'internal-ports', got '%s'", foundMappings[0].Name)
	}

	if len(foundMappings[0].AccessPorts) != 1 {
		t.Errorf("Expected 1 access port, got %d", len(foundMappings[0].AccessPorts))
	}
}

func TestMemRegistryRuleS2S(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	rules := []models.RuleS2S{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("web-to-db", models.WithNamespace("default"))),
			Traffic: models.EGRESS,
			ServiceLocalRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "web",
				},
				Namespace: "default",
			},
			ServiceRef: v1beta1.NamespacedObjectReference{
				ObjectReference: v1beta1.ObjectReference{
					APIVersion: "netguard.sgroups.io/v1beta1",
					Kind:       "ServiceAlias",
					Name:       "db",
				},
				Namespace: "default",
			},
		},
	}

	// Sync data
	err = writer.SyncRuleS2S(ctx, rules, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync rule s2s: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
	var foundRules []models.RuleS2S
	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		foundRules = append(foundRules, rule)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list rule s2s: %v", err)
	}

	if len(foundRules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(foundRules))
	}

	if foundRules[0].Name != "web-to-db" {
		t.Errorf("Expected name 'web-to-db', got '%s'", foundRules[0].Name)
	}

	if foundRules[0].Traffic != models.EGRESS {
		t.Errorf("Expected traffic EGRESS, got %s", foundRules[0].Traffic)
	}

	if foundRules[0].ServiceLocalRef.Name != "web" {
		t.Errorf("Expected local service name 'web', got '%s'", foundRules[0].ServiceLocalRef.Name)
	}

	if foundRules[0].ServiceRef.Name != "db" {
		t.Errorf("Expected service name 'db', got '%s'", foundRules[0].ServiceRef.Name)
	}
}

func TestMemRegistrySyncStatus(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	services := []models.Service{
		{
			SelfRef:     models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("default"))),
			Description: "Web service",
		},
	}

	// Sync data
	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check sync status
	status, err := reader.GetSyncStatus(ctx)
	if err != nil {
		t.Fatalf("Failed to get sync status: %v", err)
	}

	if status.UpdatedAt.IsZero() {
		t.Errorf("Expected non-zero updated at time")
	}

	// Check that the updated at time is recent
	if time.Since(status.UpdatedAt) > time.Minute {
		t.Errorf("Expected recent updated at time, got %v", status.UpdatedAt)
	}
}

func TestMemRegistryServiceAliases(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	aliases := []models.ServiceAlias{
		{
			SelfRef:    models.NewSelfRef(models.NewResourceIdentifier("web-alias", models.WithNamespace("default"))),
			ServiceRef: models.NewServiceRef("web", models.WithNamespace("default")),
		},
	}

	// Sync data
	err = writer.SyncServiceAliases(ctx, aliases, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync service aliases: %v", err)
	}

	// Commit changes
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was saved
	var foundAliases []models.ServiceAlias
	err = reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		foundAliases = append(foundAliases, alias)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list service aliases: %v", err)
	}

	if len(foundAliases) != 1 {
		t.Fatalf("Expected 1 service alias, got %d", len(foundAliases))
	}

	if foundAliases[0].Name != "web-alias" {
		t.Errorf("Expected name 'web-alias', got '%s'", foundAliases[0].Name)
	}

	if foundAliases[0].ServiceRef.Name != "web" {
		t.Errorf("Expected service name 'web', got '%s'", foundAliases[0].ServiceRef.Name)
	}

	if foundAliases[0].ServiceRef.Namespace != "default" {
		t.Errorf("Expected service namespace 'default', got '%s'", foundAliases[0].ServiceRef.Namespace)
	}
}

func TestMemRegistryAbort(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Test Writer
	writer, err := registry.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}

	// Create test data
	services := []models.Service{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier("web", models.WithNamespace("default"))),
		},
	}

	// Sync data
	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}

	// Abort changes
	writer.Abort()

	// Test Reader
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Check that data was not saved
	var foundServices []models.Service
	err = reader.ListServices(ctx, func(service models.Service) error {
		foundServices = append(foundServices, service)
		return nil
	}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to list services: %v", err)
	}

	if len(foundServices) != 0 {
		t.Fatalf("Expected 0 services, got %d", len(foundServices))
	}
}
