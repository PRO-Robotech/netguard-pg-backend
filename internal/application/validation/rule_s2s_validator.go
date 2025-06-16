package validation

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	
	"github.com/pkg/errors"
)

// ValidateExists checks if a rule s2s exists
func (v *RuleS2SValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	exists := false
	err := v.reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		if rule.Key() == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))
	
	if err != nil {
		return errors.Wrap(err, "failed to check rule s2s existence")
	}
	
	if !exists {
		return NewEntityNotFoundError("rule_s2s", id.Key())
	}
	
	return nil
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