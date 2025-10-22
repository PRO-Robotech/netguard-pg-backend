package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

	"github.com/pkg/errors"
)

// SvcSvcRuleValidator provides methods for validating SvcSvcRule
type SvcSvcRuleValidator struct {
	reader        ports.Reader
	BaseValidator *BaseValidator
}

// NewSvcSvcRuleValidator creates a new SvcSvcRule validator
func NewSvcSvcRuleValidator(reader ports.Reader) *SvcSvcRuleValidator {
	listFunction := func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
		return reader.ListSvcSvcRules(ctx, func(rule models.SvcSvcRule) error {
			return consume(&rule) // Pass pointer instead of value
		}, scope)
	}

	return &SvcSvcRuleValidator{
		reader:        reader,
		BaseValidator: NewBaseValidator(reader, "svcsvc_rule", listFunction),
	}
}

// ValidateExists checks if a SvcSvcRule exists
func (v *SvcSvcRuleValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.SvcSvcRule).Key()
	})
}

// ValidateReferences checks if all references in a SvcSvcRule are valid
func (v *SvcSvcRuleValidator) ValidateReferences(ctx context.Context, rule models.SvcSvcRule) error {
	serviceValidator := NewServiceValidator(v.reader)

	// Validate ServiceFrom exists
	serviceFromID := models.NewResourceIdentifier(rule.ServiceFromRef.Name, models.WithNamespace(rule.ServiceFromRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceFromID); err != nil {
		return errors.Wrapf(err, "invalid serviceFrom reference in SvcSvcRule %s", rule.Key())
	}

	// Check if ServiceFrom is Ready
	serviceFrom, err := v.reader.GetServiceByID(ctx, serviceFromID)
	if err != nil {
		return errors.Wrapf(err, "failed to get serviceFrom %s for Ready check", serviceFromID.Key())
	}
	if !isServiceReadyForRule(serviceFrom) {
		return fmt.Errorf("serviceFrom is not ready yet (Ready=False): %s - SvcSvcRule can only reference Ready services", serviceFromID.Key())
	}

	// Validate ServiceTo exists
	serviceToID := models.NewResourceIdentifier(rule.ServiceToRef.Name, models.WithNamespace(rule.ServiceToRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceToID); err != nil {
		return errors.Wrapf(err, "invalid serviceTo reference in SvcSvcRule %s", rule.Key())
	}

	// Check if ServiceTo is Ready
	serviceTo, err := v.reader.GetServiceByID(ctx, serviceToID)
	if err != nil {
		return errors.Wrapf(err, "failed to get serviceTo %s for Ready check", serviceToID.Key())
	}
	if !isServiceReadyForRule(serviceTo) {
		return fmt.Errorf("serviceTo is not ready yet (Ready=False): %s - SvcSvcRule can only reference Ready services", serviceToID.Key())
	}

	return nil
}

// ValidateNoDuplicates checks if there are any other rules with the same service pair
func (v *SvcSvcRuleValidator) ValidateNoDuplicates(ctx context.Context, rule models.SvcSvcRule) error {
	var duplicateFound bool
	var duplicateKey string

	err := v.reader.ListSvcSvcRules(ctx, func(existingRule models.SvcSvcRule) error {
		// Skip the rule being validated (important for updates)
		if existingRule.Key() == rule.Key() {
			return nil
		}

		// Check if key fields match (namespace + serviceFrom + serviceTo)
		if existingRule.ServiceFromRefKey() == rule.ServiceFromRefKey() &&
			existingRule.ServiceToRefKey() == rule.ServiceToRefKey() {
			duplicateFound = true
			duplicateKey = existingRule.Key()
			// We found a duplicate, no need to continue
			return nil
		}
		return nil
	}, nil) // Use nil scope to check all rules

	if err != nil {
		return errors.Wrap(err, "failed to check for duplicate SvcSvcRules")
	}

	if duplicateFound {
		return fmt.Errorf("duplicate SvcSvcRule detected: a rule with the same service pair already exists: %s", duplicateKey)
	}

	return nil
}

// ValidatePriority checks that priority is within valid range (0-1000)
func (v *SvcSvcRuleValidator) ValidatePriority(rule models.SvcSvcRule) error {
	if rule.Priority < 0 || rule.Priority > 1000 {
		return fmt.Errorf("priority must be between 0 and 1000, got %d", rule.Priority)
	}
	return nil
}

// ValidateForCreation validates a SvcSvcRule before creation
func (v *SvcSvcRuleValidator) ValidateForCreation(ctx context.Context, rule models.SvcSvcRule) error {
	keyExtractor := func(entity interface{}) string {
		if svcsvcRule, ok := entity.(*models.SvcSvcRule); ok {
			return svcsvcRule.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, rule.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
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

// ValidateForUpdate validates a SvcSvcRule before update
func (v *SvcSvcRuleValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.SvcSvcRule) error {
	oldSpec := struct {
		ServiceFromRef netguardv1beta1.NamespacedObjectReference
		ServiceToRef   netguardv1beta1.NamespacedObjectReference
	}{
		ServiceFromRef: oldRule.ServiceFromRef,
		ServiceToRef:   oldRule.ServiceToRef,
	}

	newSpec := struct {
		ServiceFromRef netguardv1beta1.NamespacedObjectReference
		ServiceToRef   netguardv1beta1.NamespacedObjectReference
	}{
		ServiceFromRef: newRule.ServiceFromRef,
		ServiceToRef:   newRule.ServiceToRef,
	}

	// Validate that spec hasn't changed when Ready condition is true
	if err := v.BaseValidator.ValidateSpecNotChangedWhenReady(oldRule, newRule, oldSpec, newSpec); err != nil {
		return err
	}

	referenceComparisons := []ObjectReferenceComparison{
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldRule.ServiceFromRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newRule.ServiceFromRef},
			FieldName: "serviceFrom",
		},
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldRule.ServiceToRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newRule.ServiceToRef},
			FieldName: "serviceTo",
		},
	}

	// Validate all object references haven't changed when Ready=True
	if err := v.BaseValidator.ValidateObjectReferencesNotChangedWhenReady(oldRule, newRule, referenceComparisons); err != nil {
		return err
	}

	// Validate priority range
	if err := v.ValidatePriority(newRule); err != nil {
		return err
	}

	// Validate references
	if err := v.ValidateReferences(ctx, newRule); err != nil {
		return err
	}

	// Check that serviceFrom reference hasn't changed (fallback validation)
	if oldRule.ServiceFromRefKey() != newRule.ServiceFromRefKey() {
		return fmt.Errorf("cannot change serviceFrom reference after creation")
	}

	// Check that serviceTo reference hasn't changed
	if oldRule.ServiceToRefKey() != newRule.ServiceToRefKey() {
		return fmt.Errorf("cannot change serviceTo reference after creation")
	}

	if oldRule.ServiceFromRefKey() != newRule.ServiceFromRefKey() ||
		oldRule.ServiceToRefKey() != newRule.ServiceToRefKey() {
		if err := v.ValidateNoDuplicates(ctx, newRule); err != nil {
			return err
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting a SvcSvcRule
func (v *SvcSvcRuleValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}

// isServiceReadyForRule checks if a Service is ready to be referenced by SvcSvcRule
func isServiceReadyForRule(service *models.Service) bool {
	return service.Meta.IsReady()
}
