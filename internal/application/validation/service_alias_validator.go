package validation

import (
	"context"
	"fmt"
	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if a service alias exists
func (v *ServiceAliasValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(models.ServiceAlias).Key()
	})
}

// ValidateReferences checks if all references in a service alias are valid
func (v *ServiceAliasValidator) ValidateReferences(ctx context.Context, alias models.ServiceAlias) error {
	serviceValidator := NewServiceValidator(v.reader)

	if err := serviceValidator.ValidateExists(ctx, alias.ServiceRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid service reference in service alias %s", alias.Key())
	}

	return nil
}

// ValidateForCreation validates a service alias before creation
func (v *ServiceAliasValidator) ValidateForCreation(ctx context.Context, alias models.ServiceAlias) error {
	return v.ValidateReferences(ctx, alias)
}

// ValidateForUpdate validates a service alias before update
func (v *ServiceAliasValidator) ValidateForUpdate(ctx context.Context, oldAlias, newAlias models.ServiceAlias) error {
	// Validate references
	if err := v.ValidateReferences(ctx, newAlias); err != nil {
		return err
	}

	// Check that service reference hasn't changed
	if oldAlias.ServiceRef.Key() != newAlias.ServiceRef.Key() {
		return fmt.Errorf("cannot change service reference after creation")
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting a service alias
func (v *ServiceAliasValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check RuleS2S referencing the service alias to be deleted
	hasRules := false
	err := v.reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		if rule.ServiceLocalRef.Key() == id.Key() || rule.ServiceRef.Key() == id.Key() {
			hasRules = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check rules s2s")
	}

	if hasRules {
		return NewDependencyExistsError("service_alias", id.Key(), "rule_s2s")
	}

	return nil
}
