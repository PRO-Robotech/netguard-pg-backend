package validation_test

import (
	"context"
	"testing"

	"netguard-pg-backend/internal/application/validation"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/infrastructure/repositories/mem"
)

// TestIntegration_RuleS2SValidation tests the integration of RuleS2SValidator with the repository
func TestIntegration_RuleS2SValidation(t *testing.T) {
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

	aliasID1 := models.NewResourceIdentifier("test-alias-1")
	alias1 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID1},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	aliasID2 := models.NewResourceIdentifier("test-alias-2")
	alias2 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID2},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID1},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID2},
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
	err = writer.SyncServiceAliases(context.Background(), []models.ServiceAlias{alias1, alias2}, nil)
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
	ruleValidator := validator.GetRuleS2SValidator()

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

// TestIntegration_RuleS2SReferences tests the ValidateReferences method of RuleS2SValidator
func TestIntegration_RuleS2SReferences(t *testing.T) {
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

	aliasID1 := models.NewResourceIdentifier("test-alias-1")
	alias1 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID1},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	aliasID2 := models.NewResourceIdentifier("test-alias-2")
	alias2 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID2},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID1},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID2},
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
	err = writer.SyncServiceAliases(context.Background(), []models.ServiceAlias{alias1, alias2}, nil)
	if err != nil {
		t.Fatalf("Failed to sync service aliases: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Act & Assert
	// Test ValidateReferences with valid references
	err = ruleValidator.ValidateReferences(context.Background(), rule)
	if err != nil {
		t.Errorf("Expected no error for valid references, got %v", err)
	}

	// Test ValidateReferences with invalid service local reference
	invalidLocalRule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-alias")},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID2},
	}
	err = ruleValidator.ValidateReferences(context.Background(), invalidLocalRule)
	if err == nil {
		t.Error("Expected error for invalid service local reference, got nil")
	}

	// Test ValidateReferences with invalid service reference
	invalidServiceRule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID1},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-alias")},
	}
	err = ruleValidator.ValidateReferences(context.Background(), invalidServiceRule)
	if err == nil {
		t.Error("Expected error for invalid service reference, got nil")
	}
}

// TestIntegration_RuleS2SValidateForCreation tests the ValidateForCreation method of RuleS2SValidator
func TestIntegration_RuleS2SValidateForCreation(t *testing.T) {
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

	aliasID1 := models.NewResourceIdentifier("test-alias-1")
	alias1 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID1},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	aliasID2 := models.NewResourceIdentifier("test-alias-2")
	alias2 := models.ServiceAlias{
		SelfRef:    models.SelfRef{ResourceIdentifier: aliasID2},
		ServiceRef: models.ServiceRef{ResourceIdentifier: serviceID},
	}

	ruleID := models.NewResourceIdentifier("test-rule")
	rule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: aliasID1},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID2},
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
	err = writer.SyncServiceAliases(context.Background(), []models.ServiceAlias{alias1, alias2}, nil)
	if err != nil {
		t.Fatalf("Failed to sync service aliases: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Create validator
	validator := validation.NewDependencyValidator(reader)
	ruleValidator := validator.GetRuleS2SValidator()

	// Act & Assert
	// Test ValidateForCreation with valid rule
	err = ruleValidator.ValidateForCreation(context.Background(), rule)
	if err != nil {
		t.Errorf("Expected no error for valid rule, got %v", err)
	}

	// Test ValidateForCreation with invalid rule
	invalidRule := models.RuleS2S{
		SelfRef:         models.SelfRef{ResourceIdentifier: ruleID},
		ServiceLocalRef: models.ServiceAliasRef{ResourceIdentifier: models.NewResourceIdentifier("non-existent-alias")},
		ServiceRef:      models.ServiceAliasRef{ResourceIdentifier: aliasID2},
	}
	err = ruleValidator.ValidateForCreation(context.Background(), invalidRule)
	if err == nil {
		t.Error("Expected error for invalid rule, got nil")
	}
}
