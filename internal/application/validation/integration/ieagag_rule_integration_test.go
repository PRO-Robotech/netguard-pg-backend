package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_IEAgAgRuleValidation tests the integration of IEAgAgRuleValidator with the repository
func TestIntegration_IEAgAgRuleValidation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: ruleID},
		Transport:         models.TCP,
		Traffic:           models.INGRESS,
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: addressGroupLocalID},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: addressGroupID},
		Ports: []models.PortSpec{
			{
				Destination: "80",
			},
		},
		Action:   models.ActionAccept,
		Logs:     true,
		Priority: 100,
	}

	// Add data to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroupLocal, addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}
	err = writer.SyncIEAgAgRules(context.Background(), []models.IEAgAgRule{rule}, nil)
	if err != nil {
		t.Fatalf("Failed to sync rules: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Act & Assert
	// Test ValidateExists with existing rule
	err = ruleValidator.ValidateExists(context.Background(), ruleID)
	if err != nil {
		t.Errorf("Expected no error for existing rule, got %v", err)
	}

	// Test ValidateExists with non-existent rule
	nonExistentID := models.NewResourceIdentifier("non-existent-rule")
	err = ruleValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent rule, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_IEAgAgRuleReferences tests the ValidateReferences method of IEAgAgRuleValidator
func TestIntegration_IEAgAgRuleReferences(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	// Add data to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroupLocal, addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Act & Assert
	// Test ValidateReferences with valid references
	rule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule")},
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: addressGroupLocalID},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}

	err = ruleValidator.ValidateReferences(context.Background(), rule)
	if err != nil {
		t.Errorf("Expected no error for valid references, got %v", err)
	}

	// Test ValidateReferences with invalid local address group reference
	invalidLocalRule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule")},
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-ag-local")},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: addressGroupID},
	}

	err = ruleValidator.ValidateReferences(context.Background(), invalidLocalRule)
	if err == nil {
		t.Error("Expected error for invalid local address group reference, got nil")
	}

	// Test ValidateReferences with invalid address group reference
	invalidRule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule")},
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: addressGroupLocalID},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-ag")},
	}

	err = ruleValidator.ValidateReferences(context.Background(), invalidRule)
	if err == nil {
		t.Error("Expected error for invalid address group reference, got nil")
	}
}

// TestIntegration_IEAgAgRuleValidateForCreation tests the ValidateForCreation method of IEAgAgRuleValidator
func TestIntegration_IEAgAgRuleValidateForCreation(t *testing.T) {
	// Arrange
	registry := mem.NewRegistry()
	reader, err := registry.Reader(context.Background())
	if err != nil {
		t.Fatalf("Failed to get reader: %v", err)
	}
	defer reader.Close()

	// Create test data
	addressGroupLocalID := models.NewResourceIdentifier("test-ag-local")
	addressGroupLocal := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupLocalID},
		Addresses: []string{"192.168.1.1/32"},
	}

	addressGroupID := models.NewResourceIdentifier("test-ag")
	addressGroup := models.AddressGroup{
		SelfRef:   models.SelfRef{ResourceIdentifier: addressGroupID},
		Addresses: []string{"10.0.0.1/32"},
	}

	// Add data to repository
	writer, err := registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.SyncAddressGroups(context.Background(), []models.AddressGroup{addressGroupLocal, addressGroup}, nil)
	if err != nil {
		t.Fatalf("Failed to sync address groups: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetIEAgAgRuleValidator()

	// Act & Assert
	// Test ValidateForCreation with valid rule
	validRule := models.IEAgAgRule{
		SelfRef:           models.SelfRef{ResourceIdentifier: models.NewResourceIdentifier("test-rule")},
		Transport:         models.TCP,
		Traffic:           models.INGRESS,
		AddressGroupLocal: models.AddressGroupRef{ResourceIdentifier: addressGroupLocalID},
		AddressGroup:      models.AddressGroupRef{ResourceIdentifier: addressGroupID},
		Ports: []models.PortSpec{
			{
				Destination: "80",
			},
		},
		Action:   models.ActionAccept,
		Logs:     true,
		Priority: 100,
	}

	err = ruleValidator.ValidateForCreation(context.Background(), validRule)
	if err != nil {
		t.Errorf("Expected no error for valid rule, got %v", err)
	}

	// Test ValidateForCreation with invalid port spec
	invalidPortRule := validRule
	invalidPortRule.Ports = []models.PortSpec{
		{
			Destination: "abc",
		},
	}

	err = ruleValidator.ValidateForCreation(context.Background(), invalidPortRule)
	if err == nil {
		t.Error("Expected error for invalid port spec, got nil")
	}

	// Test ValidateForCreation with invalid references
	invalidRefRule := validRule
	invalidRefRule.AddressGroupLocal = models.AddressGroupRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-ag-local")}

	err = ruleValidator.ValidateForCreation(context.Background(), invalidRefRule)
	if err == nil {
		t.Error("Expected error for invalid references, got nil")
	}
}
