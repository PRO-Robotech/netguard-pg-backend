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
		return entity.(models.RuleS2S).Key()
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

// ValidateForCreation validates a rule s2s before creation
func (v *RuleS2SValidator) ValidateForCreation(ctx context.Context, rule models.RuleS2S) error {
	return v.ValidateReferences(ctx, rule)
}

// ValidateForUpdate validates a rule s2s before update
func (v *RuleS2SValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.RuleS2S) error {
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

	return nil
}
