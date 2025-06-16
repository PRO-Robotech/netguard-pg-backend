package pg

//
//import (
//	"context"
//	"fmt"
//	"log"
//	"os"
//	"testing"
//	"time"
//
//	"github.com/jackc/pgx/v5/pgxpool"
//	"github.com/ory/dockertest/v3"
//	"github.com/ory/dockertest/v3/docker"
//
//	"netguard-pg-backend/internal/domain/models"
//	"netguard-pg-backend/internal/domain/ports"
//)
//
//var pgURI string
//
//// TestMain sets up the test environment with a PostgreSQL container
//func TestMain(m *testing.M) {
//	// Uses a sensible default on windows (tcp/http) and linux/osx (socket)
//	pool, err := dockertest.NewPool("")
//	if err != nil {
//		log.Fatalf("Could not connect to docker: %s", err)
//	}
//
//	// Pulls an image, creates a container based on it and runs it
//	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
//		Repository: "postgres",
//		Tag:        "14",
//		Env: []string{
//			"POSTGRES_PASSWORD=postgres",
//			"POSTGRES_USER=postgres",
//			"POSTGRES_DB=netguard_test",
//		},
//	}, func(config *docker.HostConfig) {
//		// Set AutoRemove to true so that stopped container goes away by itself
//		config.AutoRemove = true
//		config.RestartPolicy = docker.RestartPolicy{
//			Name: "no",
//		}
//	})
//	if err != nil {
//		log.Fatalf("Could not start resource: %s", err)
//	}
//
//	// Get the connection string
//	pgURI = fmt.Sprintf("postgres://postgres:postgres@localhost:%s/netguard_test?sslmode=disable", resource.GetPort("5432/tcp"))
//
//	// Exponential backoff-retry, because the application in the container might not be ready to accept connections yet
//	if err := pool.Retry(func() error {
//		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//		defer cancel()
//
//		conn, err := pgxpool.New(ctx, pgURI)
//		if err != nil {
//			return err
//		}
//		return conn.Ping(ctx)
//	}); err != nil {
//		log.Fatalf("Could not connect to database: %s", err)
//	}
//
//	// Run migrations
//	if err := RunMigrations(pgURI); err != nil {
//		log.Fatalf("Could not run migrations: %s", err)
//	}
//
//	// Run tests
//	code := m.Run()
//
//	// You can't defer this because os.Exit doesn't care for defer
//	if err := pool.Purge(resource); err != nil {
//		log.Fatalf("Could not purge resource: %s", err)
//	}
//
//	os.Exit(code)
//}
//
//func TestPgRegistry(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create test data
//	services := []models.Service{
//		{
//			Name:        "web",
//			Namespace:   "default",
//			Description: "Web service",
//			IngressPorts: []models.IngressPort{
//				{Protocol: models.TCP, Port: "80", Description: "HTTP"},
//			},
//		},
//	}
//
//	// Sync data
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved
//	var foundServices []models.Service
//	err = reader.ListServices(ctx, func(service models.Service) error {
//		foundServices = append(foundServices, service)
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list services: %v", err)
//	}
//
//	if len(foundServices) != 1 {
//		t.Fatalf("Expected 1 service, got %d", len(foundServices))
//	}
//
//	if foundServices[0].Name != "web" {
//		t.Errorf("Expected name 'web', got '%s'", foundServices[0].Name)
//	}
//}
//
//func TestPgRegistryAddressGroups(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create test data
//	addressGroups := []models.AddressGroup{
//		{
//			Name:        "internal",
//			Namespace:   "default",
//			Description: "Internal addresses",
//			Addresses:   []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
//		},
//	}
//
//	// Sync data
//	err = writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync address groups: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved
//	var foundAddressGroups []models.AddressGroup
//	err = reader.ListAddressGroups(ctx, func(addressGroup models.AddressGroup) error {
//		foundAddressGroups = append(foundAddressGroups, addressGroup)
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list address groups: %v", err)
//	}
//
//	if len(foundAddressGroups) != 1 {
//		t.Fatalf("Expected 1 address group, got %d", len(foundAddressGroups))
//	}
//
//	if foundAddressGroups[0].Name != "internal" {
//		t.Errorf("Expected name 'internal', got '%s'", foundAddressGroups[0].Name)
//	}
//
//	if len(foundAddressGroups[0].Addresses) != 3 {
//		t.Errorf("Expected 3 addresses, got %d", len(foundAddressGroups[0].Addresses))
//	}
//}
//
//func TestPgRegistryAddressGroupBindings(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create prerequisite data
//	services := []models.Service{
//		{
//			Name:        "web",
//			Namespace:   "default",
//			Description: "Web service",
//		},
//	}
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	addressGroups := []models.AddressGroup{
//		{
//			Name:        "internal",
//			Namespace:   "default",
//			Description: "Internal addresses",
//		},
//	}
//	err = writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync address groups: %v", err)
//	}
//
//	// Create test data
//	bindings := []models.AddressGroupBinding{
//		{
//			Name:      "web-internal",
//			Namespace: "default",
//			ServiceRef: models.ServiceRef{
//				Name:      "web",
//				Namespace: "default",
//			},
//			AddressGroupRef: models.AddressGroupRef{
//				Name:      "internal",
//				Namespace: "default",
//			},
//		},
//	}
//
//	// Sync data
//	err = writer.SyncAddressGroupBindings(ctx, bindings, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync address group bindings: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved
//	var foundBindings []models.AddressGroupBinding
//	err = reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
//		foundBindings = append(foundBindings, binding)
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list address group bindings: %v", err)
//	}
//
//	if len(foundBindings) != 1 {
//		t.Fatalf("Expected 1 binding, got %d", len(foundBindings))
//	}
//
//	if foundBindings[0].Name != "web-internal" {
//		t.Errorf("Expected name 'web-internal', got '%s'", foundBindings[0].Name)
//	}
//
//	if foundBindings[0].ServiceRef.Name != "web" {
//		t.Errorf("Expected service name 'web', got '%s'", foundBindings[0].ServiceRef.Name)
//	}
//
//	if foundBindings[0].AddressGroupRef.Name != "internal" {
//		t.Errorf("Expected address group name 'internal', got '%s'", foundBindings[0].AddressGroupRef.Name)
//	}
//}
//
//func TestPgRegistryAddressGroupPortMappings(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create prerequisite data
//	addressGroups := []models.AddressGroup{
//		{
//			Name:        "internal",
//			Namespace:   "default",
//			Description: "Internal addresses",
//		},
//	}
//	err = writer.SyncAddressGroups(ctx, addressGroups, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync address groups: %v", err)
//	}
//
//	// Create test data
//	mappings := []models.AddressGroupPortMapping{
//		{
//			Name:      "internal",
//			Namespace: "default",
//			AccessPorts: []models.ServicePortsRef{
//				{
//					Name:      "web",
//					Namespace: "default",
//					Ports: models.ProtocolPorts{
//						models.TCP: []models.PortRange{
//							{Start: 80, End: 80},
//							{Start: 443, End: 443},
//						},
//					},
//				},
//			},
//		},
//	}
//
//	// Sync data
//	err = writer.SyncAddressGroupPortMappings(ctx, mappings, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync address group port mappings: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved
//	var foundMappings []models.AddressGroupPortMapping
//	err = reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
//		foundMappings = append(foundMappings, mapping)
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list address group port mappings: %v", err)
//	}
//
//	if len(foundMappings) != 1 {
//		t.Fatalf("Expected 1 mapping, got %d", len(foundMappings))
//	}
//
//	if foundMappings[0].Name != "internal" {
//		t.Errorf("Expected name 'internal', got '%s'", foundMappings[0].Name)
//	}
//
//	if len(foundMappings[0].AccessPorts) != 1 {
//		t.Errorf("Expected 1 access port, got %d", len(foundMappings[0].AccessPorts))
//	}
//
//	if foundMappings[0].AccessPorts[0].Name != "web" {
//		t.Errorf("Expected service name 'web', got '%s'", foundMappings[0].AccessPorts[0].Name)
//	}
//
//	if len(foundMappings[0].AccessPorts[0].Ports[models.TCP]) != 2 {
//		t.Errorf("Expected 2 TCP port ranges, got %d", len(foundMappings[0].AccessPorts[0].Ports[models.TCP]))
//	}
//}
//
//func TestPgRegistryRuleS2S(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create prerequisite data
//	services := []models.Service{
//		{
//			Name:        "web",
//			Namespace:   "default",
//			Description: "Web service",
//		},
//		{
//			Name:        "db",
//			Namespace:   "default",
//			Description: "Database service",
//		},
//	}
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	// Create test data
//	rules := []models.RuleS2S{
//		{
//			Name:      "web-to-db",
//			Namespace: "default",
//			Traffic:   models.EGRESS,
//			ServiceLocalRef: models.ServiceRef{
//				Name:      "web",
//				Namespace: "default",
//			},
//			ServiceRef: models.ServiceRef{
//				Name:      "db",
//				Namespace: "default",
//			},
//		},
//	}
//
//	// Sync data
//	err = writer.SyncRuleS2S(ctx, rules, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync rule s2s: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved
//	var foundRules []models.RuleS2S
//	err = reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
//		foundRules = append(foundRules, rule)
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list rule s2s: %v", err)
//	}
//
//	if len(foundRules) != 1 {
//		t.Fatalf("Expected 1 rule, got %d", len(foundRules))
//	}
//
//	if foundRules[0].Name != "web-to-db" {
//		t.Errorf("Expected name 'web-to-db', got '%s'", foundRules[0].Name)
//	}
//
//	if foundRules[0].Traffic != models.EGRESS {
//		t.Errorf("Expected traffic EGRESS, got %s", foundRules[0].Traffic)
//	}
//
//	if foundRules[0].ServiceLocalRef.Name != "web" {
//		t.Errorf("Expected local service name 'web', got '%s'", foundRules[0].ServiceLocalRef.Name)
//	}
//
//	if foundRules[0].ServiceRef.Name != "db" {
//		t.Errorf("Expected service name 'db', got '%s'", foundRules[0].ServiceRef.Name)
//	}
//}
//
//func TestPgRegistrySyncStatus(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create test data
//	services := []models.Service{
//		{
//			Name:        "web",
//			Namespace:   "default",
//			Description: "Web service",
//		},
//	}
//
//	// Sync data
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check sync status
//	status, err := reader.GetSyncStatus(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get sync status: %v", err)
//	}
//
//	if status.UpdatedAt.IsZero() {
//		t.Errorf("Expected non-zero updated at time")
//	}
//
//	// Check that the updated at time is recent
//	if time.Since(status.UpdatedAt) > time.Minute {
//		t.Errorf("Expected recent updated at time, got %v", status.UpdatedAt)
//	}
//}
//
//func TestPgRegistryNameScope(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create test data
//	services := []models.Service{
//		{
//			Name:        "web",
//			Namespace:   "default",
//			Description: "Web service",
//		},
//		{
//			Name:        "db",
//			Namespace:   "default",
//			Description: "Database service",
//		},
//	}
//
//	// Sync data
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	// Commit changes
//	err = writer.Commit()
//	if err != nil {
//		t.Fatalf("Failed to commit: %v", err)
//	}
//
//	// Test Reader with name scope
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was saved with name scope
//	var foundServices []models.Service
//	err = reader.ListServices(ctx, func(service models.Service) error {
//		foundServices = append(foundServices, service)
//		return nil
//	}, ports.NewNameScope("web"))
//	if err != nil {
//		t.Fatalf("Failed to list services: %v", err)
//	}
//
//	if len(foundServices) != 1 {
//		t.Fatalf("Expected 1 service, got %d", len(foundServices))
//	}
//
//	if foundServices[0].Name != "web" {
//		t.Errorf("Expected name 'web', got '%s'", foundServices[0].Name)
//	}
//}
//
//func TestPgRegistryAbort(t *testing.T) {
//	ctx := context.Background()
//
//	// Create registry
//	registry, err := NewRegistry(ctx, pgURI)
//	if err != nil {
//		t.Fatalf("Failed to create registry: %v", err)
//	}
//	defer registry.Close()
//
//	// Test Writer
//	writer, err := registry.Writer(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get writer: %v", err)
//	}
//
//	// Create test data
//	services := []models.Service{
//		{
//			Name:        "abort-test",
//			Namespace:   "default",
//			Description: "Service to be aborted",
//		},
//	}
//
//	// Sync data
//	err = writer.SyncServices(ctx, services, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to sync services: %v", err)
//	}
//
//	// Abort changes
//	writer.Abort()
//
//	// Test Reader
//	reader, err := registry.Reader(ctx)
//	if err != nil {
//		t.Fatalf("Failed to get reader: %v", err)
//	}
//	defer reader.Close()
//
//	// Check that data was not saved
//	var foundServices []models.Service
//	err = reader.ListServices(ctx, func(service models.Service) error {
//		if service.Name == "abort-test" {
//			foundServices = append(foundServices, service)
//		}
//		return nil
//	}, ports.EmptyScope{})
//	if err != nil {
//		t.Fatalf("Failed to list services: %v", err)
//	}
//
//	if len(foundServices) != 0 {
//		t.Fatalf("Expected 0 services, got %d", len(foundServices))
//	}
//}
