package validation

import (
	"context"
	"fmt"
	"log"

	"netguard-pg-backend/internal/domain/models"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"

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

	// Create ResourceIdentifier from NamespacedObjectReference
	localServiceID := models.NewResourceIdentifier(rule.ServiceLocalRef.Name, models.WithNamespace(rule.ServiceLocalRef.Namespace))
	if err := serviceAliasValidator.ValidateExists(ctx, localServiceID); err != nil {
		return errors.Wrapf(err, "invalid service local reference in rule s2s %s", rule.Key())
	}

	// Create ResourceIdentifier from NamespacedObjectReference
	serviceID := models.NewResourceIdentifier(rule.ServiceRef.Name, models.WithNamespace(rule.ServiceRef.Namespace))
	if err := serviceAliasValidator.ValidateExists(ctx, serviceID); err != nil {
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
			existingRule.ServiceLocalRefKey() == rule.ServiceLocalRefKey() &&
			existingRule.ServiceRefKey() == rule.ServiceRefKey() {
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
	// PHASE 1: Check for duplicate entity (CRITICAL FIX for overwrite issue)
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if rs2s, ok := entity.(*models.RuleS2S); ok {
			return rs2s.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, rule.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// PHASE 2: Validate namespace rules (existing validation)
	if err := v.ValidateNamespaceRules(ctx, rule); err != nil {
		return err
	}

	// PHASE 3: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// PHASE 4: Check for business logic duplicates (existing validation)
	if err := v.ValidateNoDuplicates(ctx, rule); err != nil {
		return err
	}

	return nil
}

// ValidateForPostCommit validates a rule s2s after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *RuleS2SValidator) ValidateForPostCommit(ctx context.Context, rule models.RuleS2S) error {
	// PHASE 1: Skip duplicate entity check (entity is already committed)
	// This method is called AFTER the entity is saved to database, so existence is expected

	// PHASE 2: Validate namespace rules (existing validation)
	if err := v.ValidateNamespaceRules(ctx, rule); err != nil {
		return err
	}

	// PHASE 3: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, rule); err != nil {
		return err
	}

	// PHASE 4: Check for business logic duplicates (existing validation)
	if err := v.ValidateNoDuplicates(ctx, rule); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates a rule s2s before update
func (v *RuleS2SValidator) ValidateForUpdate(ctx context.Context, oldRule, newRule models.RuleS2S) error {
	// 🚀 PHASE 1: Ready Condition Framework - Validate spec immutability when Ready=True
	// Ported from k8s-controller rules2s_webhook.go pattern

	// Create rule spec structures for comparison
	oldSpec := struct {
		Traffic         models.Traffic
		ServiceLocalRef netguardv1beta1.NamespacedObjectReference
		ServiceRef      netguardv1beta1.NamespacedObjectReference
		Trace           bool
	}{
		Traffic:         oldRule.Traffic,
		ServiceLocalRef: oldRule.ServiceLocalRef,
		ServiceRef:      oldRule.ServiceRef,
		Trace:           oldRule.Trace,
	}

	newSpec := struct {
		Traffic         models.Traffic
		ServiceLocalRef netguardv1beta1.NamespacedObjectReference
		ServiceRef      netguardv1beta1.NamespacedObjectReference
		Trace           bool
	}{
		Traffic:         newRule.Traffic,
		ServiceLocalRef: newRule.ServiceLocalRef,
		ServiceRef:      newRule.ServiceRef,
		Trace:           newRule.Trace,
	}

	// Validate that spec hasn't changed when Ready condition is true
	if err := v.BaseValidator.ValidateSpecNotChangedWhenReady(oldRule, newRule, oldSpec, newSpec); err != nil {
		return err
	}

	// 🚀 PHASE 2: Object Reference Immutability - Validate multiple object references haven't changed when Ready=True
	referenceComparisons := []ObjectReferenceComparison{
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldRule.ServiceLocalRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newRule.ServiceLocalRef},
			FieldName: "serviceLocalRef",
		},
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldRule.ServiceRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newRule.ServiceRef},
			FieldName: "serviceRef",
		},
	}

	// Validate all object references haven't changed when Ready=True
	if err := v.BaseValidator.ValidateObjectReferencesNotChangedWhenReady(oldRule, newRule, referenceComparisons); err != nil {
		return err
	}

	// Continue with existing validation logic

	// Validate namespace rules
	if err := v.ValidateNamespaceRules(ctx, newRule); err != nil {
		return err
	}

	// Validate references
	if err := v.ValidateReferences(ctx, newRule); err != nil {
		return err
	}

	// Check that traffic direction hasn't changed (fallback validation)
	if oldRule.Traffic != newRule.Traffic {
		return fmt.Errorf("cannot change traffic direction after creation")
	}

	// Check that service local reference hasn't changed
	if oldRule.ServiceLocalRefKey() != newRule.ServiceLocalRefKey() {
		return fmt.Errorf("cannot change local service reference after creation")
	}

	// Check that service reference hasn't changed
	if oldRule.ServiceRefKey() != newRule.ServiceRefKey() {
		return fmt.Errorf("cannot change target service reference after creation")
	}

	// Check for duplicates if any of the key fields changed
	// (This is a safety check, as the above validations should prevent changes to key fields)
	if oldRule.Traffic != newRule.Traffic ||
		oldRule.ServiceLocalRefKey() != newRule.ServiceLocalRefKey() ||
		oldRule.ServiceRefKey() != newRule.ServiceRefKey() {
		if err := v.ValidateNoDuplicates(ctx, newRule); err != nil {
			return err
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting a RuleS2S
func (v *RuleS2SValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check if there are any IEAgAgRules that are generated from this RuleS2S
	hasIEAgAgRules := false
	err := v.reader.ListIEAgAgRules(ctx, func(ieagagRule models.IEAgAgRule) error {
		// IEAgAgRules are generated from RuleS2S, check if this rule contributed to any IEAgAgRule
		// Note: Since IEAgAgRules are automatically generated, they will be regenerated if needed
		// So we just log this dependency but don't block deletion
		log.Printf("CheckDependencies: Found IEAgAgRule %s that may be related to RuleS2S %s", ieagagRule.Key(), id.Key())
		hasIEAgAgRules = true
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check IEAgAg rules")
	}

	if hasIEAgAgRules {
		log.Printf("CheckDependencies: RuleS2S %s has related IEAgAgRules, but deletion is allowed (rules will be regenerated)", id.Key())
	}

	// RuleS2S can be safely deleted - IEAgAgRules are automatically regenerated
	log.Printf("CheckDependencies: RuleS2S %s can be safely deleted", id.Key())
	return nil
}
