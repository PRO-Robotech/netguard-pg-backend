package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_AddressGroupPortMappingValidation tests the integration of AddressGroupPortMappingValidator with the repository
func TestIntegration_AddressGroupPortMappingValidation(t *testing.T) {
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

	// Create a port mapping with a service reference
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	accessPorts[models.NewServiceRef(
		serviceID.Name,
		models.WithNamespace(serviceID.Namespace),
	)] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	mappingID := models.NewResourceIdentifier("test-mapping")
	mapping := models.AddressGroupPortMapping{
		SelfRef:     models.SelfRef{ResourceIdentifier: mappingID},
		AccessPorts: accessPorts,
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
	err = writer.SyncAddressGroupPortMappings(context.Background(), []models.AddressGroupPortMapping{mapping}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address group port mappings: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Act & Assert
	// Test ValidateExists with existing mapping
	err = mappingValidator.ValidateExists(context.Background(), mappingID)
	if err != nil {
		t.Errorf("Expected no error for existing mapping, got %v", err)
	}

	// Test ValidateExists with non-existent mapping
	nonExistentID := models.NewResourceIdentifier("non-existent-mapping")
	err = mappingValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent mapping, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_AddressGroupPortMappingReferences tests the ValidateReferences method of AddressGroupPortMappingValidator
func TestIntegration_AddressGroupPortMappingReferences(t *testing.T) {
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

	// Create a port mapping with a service reference
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	accessPorts[models.NewServiceRef(
		serviceID.Name,
		models.WithNamespace(serviceID.Namespace),
	)] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	mappingID := models.NewResourceIdentifier("test-mapping")
	mapping := models.AddressGroupPortMapping{
		SelfRef:     models.SelfRef{ResourceIdentifier: mappingID},
		AccessPorts: accessPorts,
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
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Act & Assert
	// Test ValidateReferences with valid references
	err = mappingValidator.ValidateReferences(context.Background(), mapping)
	if err != nil {
		t.Errorf("Expected no error for valid references, got %v", err)
	}

	// Test ValidateReferences with invalid service reference
	invalidAccessPorts := make(map[models.ServiceRef]models.ServicePorts)
	invalidAccessPorts[models.NewServiceRef("non-existent-service", models.WithNamespace("default"))] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	invalidMapping := models.AddressGroupPortMapping{
		SelfRef:     models.SelfRef{ResourceIdentifier: mappingID},
		AccessPorts: invalidAccessPorts,
	}

	err = mappingValidator.ValidateReferences(context.Background(), invalidMapping)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestIntegration_AddressGroupPortMappingValidateForCreation tests the ValidateForCreation method of AddressGroupPortMappingValidator
func TestIntegration_AddressGroupPortMappingValidateForCreation(t *testing.T) {
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

	// Create a port mapping with a service reference
	accessPorts := make(map[models.ServiceRef]models.ServicePorts)
	accessPorts[models.NewServiceRef(
		serviceID.Name,
		models.WithNamespace(serviceID.Namespace),
	)] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	mappingID := models.NewResourceIdentifier("test-mapping")
	mapping := models.AddressGroupPortMapping{
		SelfRef:     models.SelfRef{ResourceIdentifier: mappingID},
		AccessPorts: accessPorts,
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
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	mappingValidator := validator.GetAddressGroupPortMappingValidator()

	// Act & Assert
	// Test ValidateForCreation with valid mapping
	err = mappingValidator.ValidateForCreation(context.Background(), mapping)
	if err != nil {
		t.Errorf("Expected no error for valid mapping, got %v", err)
	}

	// Test ValidateForCreation with invalid mapping
	invalidAccessPorts := make(map[models.ServiceRef]models.ServicePorts)
	invalidAccessPorts[models.NewServiceRef("non-existent-service", models.WithNamespace("default"))] = models.ServicePorts{
		Ports: models.ProtocolPorts{
			models.TCP: []models.PortRange{{Start: 80, End: 80}},
		},
	}

	invalidMapping := models.AddressGroupPortMapping{
		SelfRef:     models.SelfRef{ResourceIdentifier: mappingID},
		AccessPorts: invalidAccessPorts,
	}

	err = mappingValidator.ValidateForCreation(context.Background(), invalidMapping)
	if err == nil {
		t.Error("Expected error for invalid mapping, got nil")
	}
}
