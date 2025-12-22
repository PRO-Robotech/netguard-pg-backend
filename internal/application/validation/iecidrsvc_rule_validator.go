package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// IECidrSvcRuleValidator provides methods for validating IECidrSvcRule
type IECidrSvcRuleValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewIECidrSvcRuleValidator creates a new IECidrSvcRule validator
func NewIECidrSvcRuleValidator(reader ports.Reader) *IECidrSvcRuleValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListIECidrSvcRules(ctx, func(rule models.IECidrSvcRule) error {
			return consume(&rule)
		}, scope)
	}

	return &IECidrSvcRuleValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "iecidrsvc_rule", listFunction),
	}
}

// ValidateExists checks if an IECidrSvcRule exists
func (v *IECidrSvcRuleValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.IECidrSvcRule).Key()
	})
}

// ValidateReferences checks if all references in an IECidrSvcRule are valid
func (v *IECidrSvcRuleValidator) ValidateReferences(ctx context.Context, rule models.IECidrSvcRule) error {
	serviceValidator := NewServiceValidator(v.reader)

	// Validate Service exists
	serviceID := models.NewResourceIdentifier(rule.ServiceRef.Name, models.WithNamespace(rule.ServiceRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid svc reference in IECidrSvcRule %s", rule.Key())
	}

	// Check if Service is Ready
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get service %s for Ready check", serviceID.Key())
	}
	if !isServiceReadyForRule(service) {
		return fmt.Errorf("service is not ready yet (Ready=False): %s - IECidrSvcRule can only reference Ready services", serviceID.Key())
	}

	return nil
}

// ValidatePriority checks that priority is within valid range (0-1000)
func (v *IECidrSvcRuleValidator) ValidatePriority(rule models.IECidrSvcRule) error {
	if rule.Priority < 0 || rule.Priority > 1000 {
		return fmt.Errorf("priority must be between 0 and 1000, got %d", rule.Priority)
	}
	return nil
}

// ValidateNoDuplicates checks if there are any other rules with the same unique key
func (v *IECidrSvcRuleValidator) ValidateNoDuplicates(ctx context.Context, rule models.IECidrSvcRule) error {
	var duplicateFound bool
	var duplicateKey string

	err := v.reader.ListIECidrSvcRules(ctx, func(existingRule models.IECidrSvcRule) error {
		// Skip the rule being validated (important for updates)
		if existingRule.Key() == rule.Key() {
			return nil
		}
		// Check unique key: (namespace, cidr, svc, traffic)
		if existingRule.Namespace == rule.Namespace &&
			existingRule.CIDR == rule.CIDR &&
			existingRule.ServiceKey() == rule.ServiceKey() &&
			existingRule.Traffic == rule.Traffic {
			duplicateFound = true
			duplicateKey = existingRule.Key()
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check for duplicate IECidrSvcRules")
	}

	if duplicateFound {
		return fmt.Errorf("duplicate IECidrSvcRule detected: a rule with the same (cidr, svc, traffic) already exists: %s", duplicateKey)
	}

	return nil
}

// ValidateForCreation validates an IECidrSvcRule before creation
func (v *IECidrSvcRuleValidator) ValidateForCreation(ctx context.Context, rule models.IECidrSvcRule) error {
	keyExtractor := func(entity interface{}) string {
		if r, ok := entity.(*models.IECidrSvcRule); ok {
			return r.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, rule.ResourceIdentifier, keyExtractor); err != nil {
		return err
	}

	if err := v.ValidatePriority(rule); err != nil {
		return err
	}

	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	if err := v.ValidateNoDuplicates(ctx, rule); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates an IECidrSvcRule before update
func (v *IECidrSvcRuleValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.IECidrSvcRule) error {
	if err := v.ValidatePriority(newRule); err != nil {
		return err
	}

	if err := v.ValidateReferences(ctx, newRule); err != nil {
		return err
	}

	// Check immutable fields
	if oldRule.CIDR != newRule.CIDR {
		return fmt.Errorf("cannot change cidr after creation")
	}
	if oldRule.ServiceKey() != newRule.ServiceKey() {
		return fmt.Errorf("cannot change svc reference after creation")
	}
	if oldRule.Traffic != newRule.Traffic {
		return fmt.Errorf("cannot change traffic after creation")
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an IECidrSvcRule
func (v *IECidrSvcRuleValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
