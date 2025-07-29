package validation

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

func TestNetworkValidator_Integration(t *testing.T) {
	// Create in-memory registry for testing
	registry := mem.NewRegistry()
	defer registry.Close()

	ctx := context.Background()

	// Create validator
	reader, err := registry.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	validator := NewNetworkValidator(reader)

	// Test 1: Validate CIDR
	t.Run("ValidateCIDR", func(t *testing.T) {
		// Test valid CIDR
		err := validator.ValidateCIDR("192.168.1.0/24")
		if err != nil {
			t.Errorf("Expected no error for valid CIDR, got: %v", err)
		}

		// Test invalid CIDR
		err = validator.ValidateCIDR("invalid-cidr")
		if err == nil {
			t.Error("Expected error for invalid CIDR, got nil")
		}
	})

	// Test 2: Validate for creation
	t.Run("ValidateForCreation", func(t *testing.T) {
		network := *models.NewNetwork("test-network", "default", "192.168.1.0/24")

		// Should pass for new network
		err := validator.ValidateForCreation(ctx, network)
		if err != nil {
			t.Errorf("Expected no error for new network, got: %v", err)
		}

		// Add network to repository
		writer, err := registry.Writer(ctx)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		networks := []models.Network{network}
		err = writer.SyncNetworks(ctx, networks, nil)
		if err != nil {
			t.Fatalf("Failed to sync networks: %v", err)
		}

		err = writer.Commit()
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Should fail for duplicate network
		err = validator.ValidateForCreation(ctx, network)
		if err == nil {
			t.Error("Expected error for duplicate network, got nil")
		}
	})

	// Test 3: Check dependencies
	t.Run("CheckDependencies", func(t *testing.T) {
		networkID := models.ResourceIdentifier{Name: "test-network", Namespace: "default"}

		// Should pass for network without bindings
		err := validator.CheckDependencies(ctx, networkID)
		if err != nil {
			t.Errorf("Expected no error for network without bindings, got: %v", err)
		}
	})
}
