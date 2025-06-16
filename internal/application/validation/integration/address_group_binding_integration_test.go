package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_AddressGroupBindingValidation tests the integration of AddressGroupBindingValidator with the repository
func TestIntegration_AddressGroupBindingValidation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	serviceID := models.NewResourceIdentifier("test-service")
	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
	}

	addressGroupID := models.NewResourceIdentifier("test-address-group")
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: addressGroupID},
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
	err = writer.SyncServices(context.Background(), []models.Service{service}, nil)
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
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
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Act & Assert
	// Test ValidateExists with existing binding
	err = bindingValidator.ValidateExists(context.Background(), bindingID)
	if err != nil {
		t.Errorf("Expected no error for existing binding, got %v", err)
	}

	// Test ValidateExists with non-existent binding
	nonExistentID := models.NewResourceIdentifier("non-existent-binding")
	err = bindingValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent binding, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_AddressGroupBindingReferences tests the ValidateReferences method of AddressGroupBindingValidator
func TestIntegration_AddressGroupBindingReferences(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	serviceID := models.NewResourceIdentifier("test-service", models.WithNamespace("test-ns"))
	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
	}

	addressGroupID := models.NewResourceIdentifier("test-address-group")
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: addressGroupID},
	}

	bindingID := models.NewResourceIdentifier("test-binding", models.WithNamespace("test-ns"))
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
	err = writer.SyncServices(context.Background(), []models.Service{service}, nil)
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
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
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Act & Assert
	// Test ValidateReferences with valid references and matching namespace
	err = bindingValidator.ValidateReferences(context.Background(), binding)
	if err != nil {
		t.Errorf("Expected no error for valid references and matching namespace, got %v", err)
	}

	// Test ValidateReferences with mismatched namespace
	bindingWithMismatchedNS := models.AddressGroupBinding{
		SelfRef:         models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-binding", models.WithNamespace("other-ns"))},
		ServiceRef:      models.ServiceRef{ResourceIdentifier: serviceID},
		AddressGroupRef: models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}
	err = bindingValidator.ValidateReferences(context.Background(), bindingWithMismatchedNS)
	if err == nil {
		t.Error("Expected error for mismatched namespaces, got nil")
	}

	// Test ValidateReferences with invalid service reference
	invalidServiceBinding := models.AddressGroupBinding{
		SelfRef:         models.SelfRef{ResourceIdentifier: bindingID},
		ServiceRef:      models.ServiceRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-service")},
		AddressGroupRef: models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}
	err = bindingValidator.ValidateReferences(context.Background(), invalidServiceBinding)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}

	// Test ValidateReferences with invalid address group reference
	invalidAddressGroupBinding := models.AddressGroupBinding{
		SelfRef:         models.SelfRef{ResourceIdentifier: bindingID},
		ServiceRef:      models.ServiceRef{ResourceIdentifier: serviceID},
		AddressGroupRef: models.AddressGroupRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-address-group")},
	}
	err = bindingValidator.ValidateReferences(context.Background(), invalidAddressGroupBinding)
	if err == nil {
		t.Error("Expected error for invalid address group reference, got nil")
	}
}

// TestIntegration_AddressGroupBindingValidateForCreation tests the ValidateForCreation method of AddressGroupBindingValidator
func TestIntegration_AddressGroupBindingValidateForCreation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	serviceID := models.NewResourceIdentifier("test-service")
	service := models.Service{
		SelfRef: models.SelfRef{ResourceIdentifier: serviceID},
	}

	addressGroupID := models.NewResourceIdentifier("test-address-group")
	addressGroup := models.AddressGroup{
		SelfRef: models.SelfRef{ResourceIdentifier: addressGroupID},
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
	err = writer.SyncServices(context.Background(), []models.Service{service}, nil)
	if err != nil {
		t.Fatalf("Failed to sync services: %v", err)
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
	bindingValidator := validator.GetAddressGroupBindingValidator()

	// Act & Assert
	// Test ValidateForCreation with valid binding
	err = bindingValidator.ValidateForCreation(context.Background(), &binding)
	if err != nil {
		t.Errorf("Expected no error for valid binding, got %v", err)
	}

	// Test ValidateForCreation with invalid binding
	invalidBinding := models.AddressGroupBinding{
		SelfRef:         models.SelfRef{ResourceIdentifier: bindingID},
		ServiceRef:      models.ServiceRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-service")},
		AddressGroupRef: models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}
	err = bindingValidator.ValidateForCreation(context.Background(), &invalidBinding)
	if err == nil {
		t.Error("Expected error for invalid binding, got nil")
	}
}
