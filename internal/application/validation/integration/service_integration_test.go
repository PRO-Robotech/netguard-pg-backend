package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_ServiceValidation tests the integration of ServiceValidator with the repository
func TestIntegration_ServiceValidation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create a service to add to the repository
	serviceID := models.NewResourceIdentifier("test-service")
	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
	}

	// Add the service to the repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncServices(context.Background(), []models.Service{service}, nil)
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Act & Assert
	// Test ValidateExists with existing service
	err = serviceValidator.ValidateExists(context.Background(), serviceID)
	if err != nil {
		t.Errorf("Expected no error for existing service, got %v", err)
	}

	// Test ValidateExists with non-existent service
	nonExistentID := models.NewResourceIdentifier("non-existent-service")
	err = serviceValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent service, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_ServiceReferences tests the ValidateReferences method of ServiceValidator
func TestIntegration_ServiceReferences(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	addressGroupID := models.NewResourceIdentifier("test-address-group")
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: addressGroupID},
	}

	serviceID := models.NewResourceIdentifier("test-service")
	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
		AddressGroups: []models.AddressGroupRef{
			models.NewAddressGroupRef(addressGroupID.Name, models.WithNamespace(addressGroupID.Namespace)),
		},
	}

	// Add address group to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	serviceValidator := validator.GetServiceValidator()

	// Act & Assert
	// Test ValidateReferences with valid references
	err = serviceValidator.ValidateReferences(context.Background(), service)
	if err != nil {
		t.Errorf("Expected no error for valid references, got %v", err)
	}

	// Remove the address group
	writer, err = registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.DeleteAddressGroupsByIDs(context.Background(), []models.ResourceIdentifier{addressGroupID})
	if err != nil {
		t.Fatalf("Failed to delete address groups: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test ValidateReferences with invalid references
	err = serviceValidator.ValidateReferences(context.Background(), service)
	if err == nil {
		t.Error("Expected error for invalid references, got nil")
	}
}
