package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if a rule s2s exists
func (v *RuleS2SValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.RuleS2S).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in a rule s2s are valid
func (v *RuleS2SValidator) ValidateReferences(ctx context.Context, rule models.RuleS2S) error {
	serviceAliasValidator := NewServiceAliasValidator(v.reader)

	if err := serviceAliasValidator.ValidateExists(ctx, rule.ServiceLocalRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid service local reference in rule s2s %s", rule.Key())
	}

	if err := serviceAliasValidator.ValidateExists(ctx, rule.ServiceRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid service reference in rule s2s %s", rule.Key())
	}

	return nil
}

// ValidateNoDuplicates checks if there are any other rules with the same Traffic, ServiceLocalRef, and ServiceRef
func (v *RuleS2SValidator) ValidateNoDuplicates(ctx context.Context, rule models.RuleS2S) error {
	var duplicateFound bool
	var duplicateKey string

	err := v.reader.ListRuleS2S(ctx, func(existingRule models.RuleS2S) error {
		// Skip the rule being validated (important for updates)
		if existingRule.Key() == rule.Key() {
			return nil
		}

		// Check if key fields match
		if existingRule.Traffic == rule.Traffic &&
			existingRule.ServiceLocalRef.Key() == rule.ServiceLocalRef.Key() &&
			existingRule.ServiceRef.Key() == rule.ServiceRef.Key() {
			duplicateFound = true
			duplicateKey = existingRule.Key()
			// We found a duplicate, no need to continue
			return nil
		}
		return nil
	}, nil) // Use nil scope to check all rules

	if err != nil {
		return errors.Wrap(err, "failed to check for duplicate rules")
	}

	if duplicateFound {
		return fmt.Errorf("duplicate RuleS2S detected: a rule with the same specification already exists: %s", duplicateKey)
	}

	return nil
}

// ValidateNamespaceRules checks namespace rules for RuleS2S
func (v *RuleS2SValidator) ValidateNamespaceRules(ctx context.Context, rule models.RuleS2S) error {
	// 1. Check that ServiceLocalRef is in the same namespace as the rule
	if rule.Namespace != "" && rule.ServiceLocalRef.Namespace != "" &&
		rule.Namespace != rule.ServiceLocalRef.Namespace {
		return fmt.Errorf("serviceLocalRef must be in the same namespace as the rule: rule namespace=%s, serviceLocalRef namespace=%s",
			rule.Namespace, rule.ServiceLocalRef.Namespace)
	}

	// 2. Check that ServiceRef has a correct namespace
	// If ServiceRef namespace is not specified, it should be the same as the rule's namespace
	if rule.ServiceRef.Namespace == "" && rule.Namespace != "" {
		// Create a copy of ServiceRef with updated namespace for validation
		serviceRefWithNamespace := models.ServiceAliasRef{
			ResourceIdentifier: models.ResourceIdentifier{
				Name:      rule.ServiceRef.Name,
				Namespace: rule.Namespace,
			},
		}

		// Check if ServiceRef exists with the rule's namespace
		serviceAliasValidator := NewServiceAliasValidator(v.reader)
		if err := serviceAliasValidator.ValidateExists(ctx, serviceRefWithNamespace.ResourceIdentifier); err != nil {
			return errors.Wrapf(err, "invalid service reference in rule s2s %s: service must exist in rule's namespace", rule.Key())
		}
	}

	return nil
}

// ValidateForCreation validates a rule s2s before creation
func (v *RuleS2SValidator) ValidateForCreation(ctx context.Context, rule models.RuleS2S) error {
	// Validate namespace rules
	if err := v.ValidateNamespaceRules(ctx, rule); err != nil {
		return err
	}

	// Validate references
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// Check for duplicates
	if err := v.ValidateNoDuplicates(ctx, rule); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates a rule s2s before update
func (v *RuleS2SValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.RuleS2S) error {
	// Validate namespace rules
	if err := v.ValidateNamespaceRules(ctx, newRule); err != nil {
		return err
	}

	// Validate references
	if err := v.ValidateReferences(ctx, newRule); err != nil {
		return err
	}

	// Check that traffic direction hasn't changed
	if oldRule.Traffic != newRule.Traffic {
		return fmt.Errorf("cannot change traffic direction after creation")
	}

	// Check that service local reference hasn't changed
	if oldRule.ServiceLocalRef.Key() != newRule.ServiceLocalRef.Key() {
		return fmt.Errorf("cannot change local service reference after creation")
	}

	// Check that service reference hasn't changed
	if oldRule.ServiceRef.Key() != newRule.ServiceRef.Key() {
		return fmt.Errorf("cannot change target service reference after creation")
	}

	// Check for duplicates if any of the key fields changed
	// (This is a safety check, as the above validations should prevent changes to key fields)
	if oldRule.Traffic != newRule.Traffic ||
		oldRule.ServiceLocalRef.Key() != newRule.ServiceLocalRef.Key() ||
		oldRule.ServiceRef.Key() != newRule.ServiceRef.Key() {
		if err := v.ValidateNoDuplicates(ctx, newRule); err != nil {
			return err
		}
	}

	return nil
}
