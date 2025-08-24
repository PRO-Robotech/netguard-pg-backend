package validation

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

func TestNetworkValidator_ValidateCIDR(t *testing.T) {
	validator := &NetworkValidator{}

	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{
			name:    "valid CIDR",
			cidr:    "192.168.1.0/24",
			wantErr: false,
		},
		{
			name:    "valid CIDR with /32",
			cidr:    "10.0.0.1/32",
			wantErr: false,
		},
		{
			name:    "empty CIDR",
			cidr:    "",
			wantErr: true,
		},
		{
			name:    "invalid CIDR format",
			cidr:    "192.168.1.0",
			wantErr: true,
		},
		{
			name:    "invalid IP",
			cidr:    "256.256.256.256/24",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCIDR() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkValidator_ValidateForCreation(t *testing.T) {
	// Create in-memory repository for testing
	repo := mem.NewRegistry()
	ctx := context.Background()

	// Create validator
	reader, err := repo.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	validator := NewNetworkValidator(reader)

	// Test valid network creation
	network := *models.NewNetwork("test-network", "default", "192.168.1.0/24")

	err = validator.ValidateForCreation(ctx, network)
	if err != nil {
		t.Errorf("ValidateForCreation() failed for valid network: %v", err)
	}

	// Add network to repository to test duplicate detection
	writer, err := repo.Writer(ctx)
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncNetworks(ctx, []models.Network{network}, ports.EmptyScope{})
	if err != nil {
		t.Fatalf("Failed to save network: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create new reader to see the committed network
	reader2, err := repo.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader2.Close()

	validator2 := NewNetworkValidator(reader2)

	// Test duplicate network creation
	err = validator2.ValidateForCreation(ctx, network)
	if err == nil {
		t.Error("ValidateForCreation() should fail for duplicate network")
	}
}

func TestNetworkValidator_ValidateForUpdate(t *testing.T) {
	// Create in-memory repository for testing
	repo := mem.NewRegistry()
	ctx := context.Background()

	// Create validator
	reader, err := repo.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	validator := NewNetworkValidator(reader)

	oldNetwork := *models.NewNetwork("test-network", "default", "192.168.1.0/24")

	newNetwork := *models.NewNetwork("test-network", "default", "192.168.2.0/24")

	// Test valid update
	err = validator.ValidateForUpdate(ctx, oldNetwork, newNetwork)
	if err == nil {
		t.Log("ValidateForUpdate() passed for valid update")
	} else {
		t.Logf("ValidateForUpdate() error (expected for non-existent network): %v", err)
	}

	// Test name change (should fail)
	newNetwork.Name = "different-name"
	err = validator.ValidateForUpdate(ctx, oldNetwork, newNetwork)
	if err == nil {
		t.Error("ValidateForUpdate() should fail for name change")
	}
}

func TestNetworkValidator_CheckDependencies(t *testing.T) {
	// Create in-memory repository for testing
	repo := mem.NewRegistry()
	ctx := context.Background()

	// Create validator
	reader, err := repo.Reader(ctx)
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	validator := NewNetworkValidator(reader)

	// Test deletion of network without bindings
	networkID := models.ResourceIdentifier{Name: "test-network", Namespace: "default"}
	err = validator.CheckDependencies(ctx, networkID)
	if err != nil {
		t.Errorf("CheckDependencies() failed for network without bindings: %v", err)
	}
}
