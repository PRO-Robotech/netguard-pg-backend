package mem

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
)

func TestSyncServicesWithDifferentOperations(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Create initial data
	initialServices := []models.Service{
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"web", models.WithNamespace("default"))),
			Description: "Web service",
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "80", Description: "HTTP"},
			},
		},
		{
			SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
				"db", models.WithNamespace("default"))),
			Description: "Database service",
			IngressPorts: []models.IngressPort{
				{Protocol: models.TCP, Port: "5432", Description: "PostgreSQL"},
			},
		},
	}

	// Test FullSync operation
	t.Run("FullSync", func(t *testing.T) {
		// Get writer
		writer, err := registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync initial data
		err = writer.SyncServices(ctx, initialServices, ports.EmptyScope{})
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
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
			t.Fatalf("Expected 2 services, got %d", len(foundServices))
		}

		// Create new data for FullSync
		newServices := []models.Service{
			{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
					"api", models.WithNamespace("default"))),
				Description: "API service",
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "8080", Description: "API"},
				},
			},
		}

		// Get new writer
		writer, err = registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync new data with FullSync operation
		err = writer.SyncServices(ctx, newServices, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpFullSync))
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
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
		// Reset registry
		registry = NewRegistry()
		defer registry.Close()

		// Get writer
		writer, err := registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync initial data
		err = writer.SyncServices(ctx, initialServices, ports.EmptyScope{})
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Create data for Upsert
		upsertServices := []models.Service{
			{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
					"web", models.WithNamespace("default"))),
				Description: "Updated web service",
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "80", Description: "HTTP"},
					{Protocol: models.TCP, Port: "443", Description: "HTTPS"},
				},
			},
			{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
					"api", models.WithNamespace("default"))),
				Description: "API service",
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "8080", Description: "API"},
				},
			},
		}

		// Get new writer
		writer, err = registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync data with Upsert operation
		err = writer.SyncServices(ctx, upsertServices, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpUpsert))
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
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
		// Reset registry
		registry = NewRegistry()
		defer registry.Close()

		// Get writer
		writer, err := registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync initial data
		err = writer.SyncServices(ctx, initialServices, ports.EmptyScope{})
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Create data for Delete
		deleteServices := []models.Service{
			{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
					"web", models.WithNamespace("default"))),
			},
		}

		// Get new writer
		writer, err = registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync data with Delete operation
		err = writer.SyncServices(ctx, deleteServices, ports.EmptyScope{}, ports.WithSyncOp(models.SyncOpDelete))
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
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

	// Test FullSync with ResourceIdentifierScope
	t.Run("FullSyncWithScope", func(t *testing.T) {
		// Reset registry
		registry = NewRegistry()
		defer registry.Close()

		// Get writer
		writer, err := registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync initial data
		err = writer.SyncServices(ctx, initialServices, ports.EmptyScope{})
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Create scope for web service
		webScope := ports.NewResourceIdentifierScope(
			models.NewResourceIdentifier("web", models.WithNamespace("default")),
		)

		// Scope for FullSync with scope test

		// Create new data for FullSync with scope
		newServices := []models.Service{
			{
				SelfRef: models.NewSelfRef(models.NewResourceIdentifier(
					"web", models.WithNamespace("default"))),
				Description: "Updated web service",
				IngressPorts: []models.IngressPort{
					{Protocol: models.TCP, Port: "80", Description: "HTTP"},
					{Protocol: models.TCP, Port: "443", Description: "HTTPS"},
				},
			},
		}

		// Get new writer
		writer, err = registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		// Sync new data with FullSync operation and scope
		err = writer.SyncServices(ctx, newServices, webScope, ports.WithSyncOp(models.SyncOpFullSync))
		if err != nil {
			t.Fatalf("Failed to sync services: %v", err)
		}

		// Commit changes
		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify that only web service was updated, db service remains unchanged
		reader, err := registry.Reader(ctx)
		if err != nil {
			t.Fatalf("Failed to get reader: %v", err)
		}
		defer reader.Close()

		// Get all services from the registry
		_ = registry.db.GetServices()

		var foundServices []models.Service
		err = reader.ListServices(ctx, func(service models.Service) error {
			foundServices = append(foundServices, service)
			return nil
		}, ports.EmptyScope{})
		if err != nil {
			t.Fatalf("Failed to list services: %v", err)
		}

		if len(foundServices) != 2 {
			t.Fatalf("Expected 2 services after FullSync with scope, got %d", len(foundServices))
		}

		// Find web service and check if it was updated
		var webService *models.Service
		var dbService *models.Service
		for i, service := range foundServices {
			if service.Name == "web" {
				webService = &foundServices[i]
			} else if service.Name == "db" {
				dbService = &foundServices[i]
			}
		}

		if webService == nil {
			t.Fatalf("Web service not found after FullSync with scope")
		}

		if dbService == nil {
			t.Fatalf("DB service not found after FullSync with scope")
		}

		if webService.Description != "Updated web service" {
			t.Errorf("Expected web service description 'Updated web service', got '%s'", webService.Description)
		}

		if len(webService.IngressPorts) != 2 {
			t.Errorf("Expected 2 ingress ports for web service, got %d", len(webService.IngressPorts))
		}

		if dbService.Description != "Database service" {
			t.Errorf("Expected db service description 'Database service', got '%s'", dbService.Description)
		}
	})
}
