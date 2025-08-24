package validation

import (
	"context"
	"fmt"
	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
	netguardv1beta1 "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1"
)

// ValidateExists checks if a service alias exists
func (v *ServiceAliasValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.ServiceAlias).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in a service alias are valid
func (v *ServiceAliasValidator) ValidateReferences(ctx context.Context, alias models.ServiceAlias) error {
	serviceValidator := NewServiceValidator(v.reader)

	// Create ResourceIdentifier from ObjectReference - service is in same namespace as alias
	serviceID := models.NewResourceIdentifier(alias.ServiceRef.Name, models.WithNamespace(alias.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid service reference in service alias %s", alias.Key())
	}

	// Получаем сервис для проверки namespace
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for namespace validation in service alias %s", alias.Key())
	}
	if service == nil {
		return fmt.Errorf("service not found or is nil for service alias %s", alias.Key())
	}

	// Проверяем соответствие namespace, если он указан в алиасе
	if alias.Namespace != "" && alias.Namespace != service.Namespace {
		return fmt.Errorf("service alias namespace '%s' must match service namespace '%s'",
			alias.Namespace, service.Namespace)
	}

	return nil
}

// ValidateForCreation validates a service alias before creation
func (v *ServiceAliasValidator) ValidateForCreation(ctx context.Context, alias *models.ServiceAlias) error {
	// Проверяем существование сервиса
	serviceValidator := NewServiceValidator(v.reader)
	// Create ResourceIdentifier from ObjectReference - service is in same namespace as alias
	serviceID := models.NewResourceIdentifier(alias.ServiceRef.Name, models.WithNamespace(alias.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid service reference in service alias %s", alias.Key())
	}

	// Получаем сервис для проверки и/или установки namespace
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for namespace validation in service alias %s", alias.Key())
	}
	if service == nil {
		return fmt.Errorf("service not found or is nil for service alias %s", alias.Key())
	}

	// Если namespace не указан, берем его из сервиса
	if alias.Namespace == "" {
		alias.Namespace = service.Namespace
	} else if alias.Namespace != service.Namespace {
		// Если namespace указан, проверяем соответствие
		return fmt.Errorf("service alias namespace '%s' must match service namespace '%s'",
			alias.Namespace, service.Namespace)
	}

	return nil
}

// ValidateForUpdate validates a service alias before update
func (v *ServiceAliasValidator) ValidateForUpdate(ctx context.Context, oldAlias, newAlias models.ServiceAlias) error {
	// 🚀 PHASE 1: Ready Condition Framework - Validate spec immutability when Ready=True
	// Ported from k8s-controller servicealias_webhook.go pattern

	// Create alias spec structures for comparison
	oldSpec := struct {
		ServiceRef netguardv1beta1.NamespacedObjectReference
	}{
		ServiceRef: oldAlias.ServiceRef,
	}

	newSpec := struct {
		ServiceRef netguardv1beta1.NamespacedObjectReference
	}{
		ServiceRef: newAlias.ServiceRef,
	}

	// Validate that spec hasn't changed when Ready condition is true
	if err := v.BaseValidator.ValidateSpecNotChangedWhenReady(oldAlias, newAlias, oldSpec, newSpec); err != nil {
		return err
	}

	// 🚀 PHASE 2: Object Reference Immutability - Validate service reference hasn't changed when Ready=True
	referenceComparisons := []ObjectReferenceComparison{
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldAlias.ServiceRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newAlias.ServiceRef},
			FieldName: "serviceRef",
		},
	}

	// Validate all object references haven't changed when Ready=True
	if err := v.BaseValidator.ValidateObjectReferencesNotChangedWhenReady(oldAlias, newAlias, referenceComparisons); err != nil {
		return err
	}

	// Fallback field-level validation for additional protection
	if err := v.BaseValidator.ValidateFieldNotChangedWhenReady("serviceRef", oldAlias, newAlias, oldAlias.ServiceRefKey(), newAlias.ServiceRefKey()); err != nil {
		return err
	}

	// Continue with existing validation logic

	// Validate references
	if err := v.ValidateReferences(ctx, newAlias); err != nil {
		return err
	}

	// Check that service reference hasn't changed (fallback validation)
	if oldAlias.ServiceRefKey() != newAlias.ServiceRefKey() {
		return fmt.Errorf("cannot change service reference after creation")
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting a service alias
func (v *ServiceAliasValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check RuleS2S referencing the service alias to be deleted
	hasRules := false
	err := v.reader.ListRuleS2S(ctx, func(rule models.RuleS2S) error {
		if rule.ServiceLocalRefKey() == id.Key() || rule.ServiceRefKey() == id.Key() {
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
