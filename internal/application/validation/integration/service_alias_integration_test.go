package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_ServiceAliasValidation tests the integration of ServiceAliasValidator with the repository
func TestIntegration_ServiceAliasValidation(t *testing.T) {
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

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
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
	err = writer.SyncServiceAliases(context.Background(), []models.ServiceAlias{alias}, nil)
	if err != nil {
		t.Fatalf("Failed to sync service aliases: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Act & Assert
	// Test ValidateExists with existing alias
	err = aliasValidator.ValidateExists(context.Background(), aliasID)
	if err != nil {
		t.Errorf("Expected no error for existing alias, got %v", err)
	}

	// Test ValidateExists with non-existent alias
	nonExistentID := models.NewResourceIdentifier("non-existent-alias")
	err = aliasValidator.ValidateExists(context.Background(), nonExistentID)
	if err == nil {
		t.Error("Expected error for non-existent alias, got nil")
	}

	// Check if it's the right type of error
	if _, ok := err.(*validation.EntityNotFoundError); !ok {
		t.Errorf("Expected EntityNotFoundError, got %T", err)
	}
}

// TestIntegration_ServiceAliasReferences tests the ValidateReferences method of ServiceAliasValidator
func TestIntegration_ServiceAliasReferences(t *testing.T) {
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

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
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
	aliasValidator := validator.GetServiceAliasValidator()

	// Act & Assert
	// Test ValidateReferences with valid references
	err = aliasValidator.ValidateReferences(context.Background(), alias)
	if err != nil {
		t.Errorf("Expected no error for valid references, got %v", err)
	}

	// Test ValidateReferences with invalid service reference
	invalidAlias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-service")},
	}
	err = aliasValidator.ValidateReferences(context.Background(), invalidAlias)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestIntegration_ServiceAliasValidateForCreation tests the ValidateForCreation method of ServiceAliasValidator
func TestIntegration_ServiceAliasValidateForCreation(t *testing.T) {
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

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
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
	aliasValidator := validator.GetServiceAliasValidator()

	// Act & Assert
	// Test ValidateForCreation with valid alias
	err = aliasValidator.ValidateForCreation(context.Background(), alias)
	if err != nil {
		t.Errorf("Expected no error for valid alias, got %v", err)
	}

	// Test ValidateForCreation with invalid alias
	invalidAlias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-service")},
	}
	err = aliasValidator.ValidateForCreation(context.Background(), invalidAlias)
	if err == nil {
		t.Error("Expected error for invalid alias, got nil")
	}
}

// TestIntegration_ServiceAliasDependencies tests the CheckDependencies method of ServiceAliasValidator
func TestIntegration_ServiceAliasDependencies(t *testing.T) {
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

	aliasID := models.NewResourceIdentifier("test-alias")
	alias := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID},
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
	err = writer.SyncServiceAliases(context.Background(), []models.ServiceAlias{alias}, nil)
	if err != nil {
		t.Fatalf("Failed to sync service aliases: %v", err)
	}
	err = writer.SyncRuleS2S(context.Background(), []models.RuleS2S{rule}, nil)
	if err != nil {
		t.Fatalf("Failed to sync rules: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	aliasValidator := validator.GetServiceAliasValidator()

	// Act & Assert
	// Test CheckDependencies with dependencies
	err = aliasValidator.CheckDependencies(context.Background(), aliasID)
	if err == nil {
		t.Error("Expected error for alias with dependencies, got nil")
	}
	if _, ok := err.(*validation.DependencyExistsError); !ok {
		t.Errorf("Expected DependencyExistsError, got %T", err)
	}

	// Remove the dependency
	writer, err = registry.Writer(context.Background())
	if err != nil {
		t.Fatalf("Failed to get writer: %v", err)
	}
	err = writer.DeleteRuleS2SByIDs(context.Background(), []models.ResourceIdentifier{ruleID})
	if err != nil {
		t.Fatalf("Failed to delete rules: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test CheckDependencies without dependencies
	err = aliasValidator.CheckDependencies(context.Background(), aliasID)
	if err != nil {
		t.Errorf("Expected no error for alias without dependencies, got %v", err)
	}
}
