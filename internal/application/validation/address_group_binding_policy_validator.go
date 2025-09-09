package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group binding policy exists
func (v *AddressGroupBindingPolicyValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.AddressGroupBindingPolicy).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in an address group binding policy are valid
func (v *AddressGroupBindingPolicyValidator) ValidateReferences(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	serviceValidator := NewServiceValidator(v.reader)
	addressGroupValidator := NewAddressGroupValidator(v.reader)

	// Create ResourceIdentifier from NamespacedObjectReference
	serviceID := models.NewResourceIdentifier(policy.ServiceRef.Name, models.WithNamespace(policy.ServiceRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid service reference in policy %s", policy.Key())
	}

	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(policy.AddressGroupRef.Name, models.WithNamespace(policy.AddressGroupRef.Namespace))
	if err := addressGroupValidator.ValidateExists(ctx, agID); err != nil {
		return errors.Wrapf(err, "invalid address group reference in policy %s", policy.Key())
	}

	// Проверяем, что политика находится в том же namespace, что и AddressGroup
	if policy.Namespace != policy.AddressGroupRef.Namespace {
		return fmt.Errorf("policy namespace '%s' must match address group namespace '%s'",
			policy.Namespace, policy.AddressGroupRef.Namespace)
	}

	return nil
}

// ValidateForCreation validates an address group binding policy before creation
func (v *AddressGroupBindingPolicyValidator) ValidateForCreation(ctx context.Context, policy *models.AddressGroupBindingPolicy) error {
	// PHASE 1: Check for duplicate entity (CRITICAL FIX for overwrite issue)
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if agbp, ok := entity.(*models.AddressGroupBindingPolicy); ok {
			return agbp.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, policy.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, *policy); err != nil {
		return err
	}

	// PHASE 3: Check for duplicate policies with same service/address group combination (business logic validation)
	var duplicateFound bool
	err := v.reader.ListAddressGroupBindingPolicies(ctx, func(existingPolicy models.AddressGroupBindingPolicy) error {
		// Пропускаем текущую политику
		if existingPolicy.Name == policy.Name && existingPolicy.Namespace == policy.Namespace {
			return nil
		}

		// Проверяем, совпадают ли ключевые поля
		// Сравниваем namespace/name для обоих references
		if existingPolicy.ServiceRef.Namespace == policy.ServiceRef.Namespace &&
			existingPolicy.ServiceRef.Name == policy.ServiceRef.Name &&
			existingPolicy.AddressGroupRef.Namespace == policy.AddressGroupRef.Namespace &&
			existingPolicy.AddressGroupRef.Name == policy.AddressGroupRef.Name {
			duplicateFound = true
			return fmt.Errorf("duplicate policy found")
		}

		return nil
	}, nil)

	if err != nil && !duplicateFound {
		return errors.Wrap(err, "failed to check for duplicate policies")
	}

	if duplicateFound {
		return fmt.Errorf("duplicate policy found: a policy with the same service and address group already exists")
	}

	return nil
}

// ValidateForPostCommit validates an address group binding policy after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *AddressGroupBindingPolicyValidator) ValidateForPostCommit(ctx context.Context, policy models.AddressGroupBindingPolicy) error {
	// PHASE 1: Skip duplicate entity check (entity is already committed)
	// This method is called AFTER the entity is saved to database, so existence is expected

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, policy); err != nil {
		return err
	}

	// PHASE 3: Check for business logic duplicates (same service/address group combination)
	var duplicateFound bool
	err := v.reader.ListAddressGroupBindingPolicies(ctx, func(existingPolicy models.AddressGroupBindingPolicy) error {
		// Skip the current policy (it's the one we just committed)
		if existingPolicy.Name == policy.Name && existingPolicy.Namespace == policy.Namespace {
			return nil
		}

		// Check if key fields match with other policies
		if existingPolicy.ServiceRef.Namespace == policy.ServiceRef.Namespace &&
			existingPolicy.ServiceRef.Name == policy.ServiceRef.Name &&
			existingPolicy.AddressGroupRef.Namespace == policy.AddressGroupRef.Namespace &&
			existingPolicy.AddressGroupRef.Name == policy.AddressGroupRef.Name {
			duplicateFound = true
			return fmt.Errorf("duplicate policy found")
		}

		return nil
	}, nil)

	if err != nil && !duplicateFound {
		return errors.Wrap(err, "failed to check for duplicate policies")
	}

	if duplicateFound {
		return fmt.Errorf("duplicate policy found: a policy with the same service and address group already exists")
	}

	return nil
}

// ValidateForUpdate validates an address group binding policy before update
func (v *AddressGroupBindingPolicyValidator) ValidateForUpdate(ctx context.Context, oldPolicy models.AddressGroupBindingPolicy, newPolicy *models.AddressGroupBindingPolicy) error {
	// Проверяем ссылки (включая проверку namespace)
	if err := v.ValidateReferences(ctx, *newPolicy); err != nil {
		return err
	}

	// Проверяем, что ссылка на сервис не изменилась
	if oldPolicy.ServiceRef.Namespace != newPolicy.ServiceRef.Namespace ||
		oldPolicy.ServiceRef.Name != newPolicy.ServiceRef.Name {
		return fmt.Errorf("cannot change service reference after creation")
	}

	// Проверяем, что ссылка на address group не изменилась
	if oldPolicy.AddressGroupRef.Namespace != newPolicy.AddressGroupRef.Namespace ||
		oldPolicy.AddressGroupRef.Name != newPolicy.AddressGroupRef.Name {
		return fmt.Errorf("cannot change address group reference after creation")
	}

	return nil
}

// CheckDependencies проверяет зависимости перед удалением политики привязки группы адресов
func (v *AddressGroupBindingPolicyValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Получаем политику по ID
	policy, err := v.reader.GetAddressGroupBindingPolicyByID(ctx, id)
	if err != nil {
		return errors.Wrap(err, "failed to get policy")
	}
	if policy == nil {
		return fmt.Errorf("policy not found")
	}

	// Проверяем, есть ли активные привязки, связанные с этой политикой
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Проверяем, ссылается ли привязка на тот же сервис и группу адресов, что и политика
		if binding.ServiceRefKey() == policy.ServiceRefKey() &&
			binding.AddressGroupRefKey() == policy.AddressGroupRefKey() {
			hasBindings = true
			return fmt.Errorf("binding found") // Используем ошибку для прерывания цикла
		}
		return nil
	}, nil)

	// Если ошибка не связана с найденной привязкой, возвращаем её
	if err != nil && !hasBindings {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	// Если найдены привязки, возвращаем ошибку
	if hasBindings {
		return NewDependencyExistsError("address_group_binding_policy", id.Key(), "address_group_binding")
	}

	return nil
}
