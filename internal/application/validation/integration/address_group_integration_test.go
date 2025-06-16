package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_AddressGroupValidation tests the integration of AddressGroupValidator with the repository
func TestIntegration_AddressGroupValidation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create an address group to add to the repository
	addressGroupID := models.NewResourceIdentifier("test-address-group")
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: addressGroupID},
	}

	// Add the address group to the repository
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
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Act & Assert
	// Test ValidateExists with existing address group
	err = addressGroupValidator.ValidateExists(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error for existing address group, got %v", err)
	}

	// Test ValidateExists with non-existent address group
	nonExistentID := models.NewResourceIdentifier("non-existent-address-group")
	err = addressGroupValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent address group, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_AddressGroupDependencies tests the CheckDependencies method of AddressGroupValidator
func TestIntegration_AddressGroupDependencies(t *testing.T) {
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
			{ResourceIdentifier: addressGroupID},
		},
	}

	// Add data to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
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
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Act & Assert
	// Test CheckDependencies with dependencies
	err = addressGroupValidator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for address group with dependencies, got nil")
	}
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Remove the dependency
	writer, err = registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.DeleteServicesByIDs(context.Background(), []models.ResourceIdentifier{serviceID})
	if err != nil {
		t.Fatalf("Failed to delete services: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test CheckDependencies without dependencies
	err = addressGroupValidator.CheckDependencies(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error for address group without dependencies, got %v", err)
	}
}

// TestIntegration_AddressGroupBindingDependencies tests the CheckDependencies method with AddressGroupBinding dependencies
func TestIntegration_AddressGroupBindingDependencies(t *testing.T) {
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
	}

	bindingID := models.NewResourceIdentifier("test-binding")
	binding := models.AddressGroupBinding{
		SelfRef:         models.SelfRef{ResourceIdentifier: bindingID},
		ServiceRef:      models.ServiceRef{ResourceIdentifier: serviceID},
		AddressGroupRef: models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}

	// Add data to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}
	err = writer.SyncServices(context.Background(), []models.Service{service}, nil)
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}
	err = writer.SyncAddressGroupBindings(context.Background(), []models.AddressGroupBinding{binding}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address group bindings: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	addressGroupValidator := validator.GetAddressGroupValidator()

	// Act & Assert
	// Test CheckDependencies with binding dependencies
	err = addressGroupValidator.CheckDependencies(context.Background(), addressGroupID)
	if err == nil {
		t.Error("Expected error for address group with binding dependencies, got nil")
	}
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Remove the dependency
	writer, err = registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.DeleteAddressGroupBindingsByIDs(context.Background(), []models.ResourceIdentifier{bindingID})
	if err != nil {
		t.Fatalf("Failed to delete address group bindings: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test CheckDependencies without dependencies
	err = addressGroupValidator.CheckDependencies(context.Background(), addressGroupID)
	if err != nil {
		t.Errorf("Expected no error for address group without dependencies, got %v", err)
	}
}
