package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

func CreateNewPortMapping(addressGroupID models.ResourceIdentifier, service models.Service) *models.AddressGroupPortMapping {
	portMapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: addressGroupID,
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	servicePorts := models.ServicePorts{
		Ports: make(models.ProtocolPorts),
	}

	for _, ingressPort := range service.IngressPorts {
		portRanges, err := ParsePortRanges(ingressPort.Port)
		if err != nil {
			continue
		}

		for _, portRange := range portRanges {
			servicePorts.Ports[ingressPort.Protocol] = append(
				servicePorts.Ports[ingressPort.Protocol],
				portRange,
			)
		}
	}

	serviceRef := models.NewServiceRef(service.Name, models.WithNamespace(service.Namespace))
	portMapping.AccessPorts[serviceRef] = servicePorts

	return portMapping
}

// UpdatePortMapping updates an existing port mapping with service ports
func UpdatePortMapping(
	existingMapping models.AddressGroupPortMapping,
	serviceRef models.ServiceRef,
	service models.Service,
) *models.AddressGroupPortMapping {
	updatedMapping := existingMapping

	if updatedMapping.AccessPorts == nil {
		updatedMapping.AccessPorts = make(map[models.ServiceRef]models.ServicePorts)
	}

	servicePorts := models.ServicePorts{
		Ports: make(models.ProtocolPorts),
	}

	for _, ingressPort := range service.IngressPorts {
		portRanges, err := ParsePortRanges(ingressPort.Port)
		if err != nil {
			continue
		}

		for _, portRange := range portRanges {
			servicePorts.Ports[ingressPort.Protocol] = append(
				servicePorts.Ports[ingressPort.Protocol],
				portRange,
			)
		}
	}

	updatedMapping.AccessPorts[models.NewServiceRef(service.Name, models.WithNamespace(service.Namespace))] = servicePorts

	return &updatedMapping
}

// CheckPortOverlaps checks for port overlaps in a port mapping
func CheckPortOverlaps(service models.Service, portMapping models.AddressGroupPortMapping) error {
	// Create a map of service ports by protocol
	servicePorts := make(map[models.TransportProtocol][]models.PortRange)

	for _, ingressPort := range service.IngressPorts {
		portRanges, err := ParsePortRanges(ingressPort.Port)
		if err != nil {
			return fmt.Errorf("invalid port in service %s: %w", service.Key(), err)
		}

		if err := CheckPortRangeOverlapsOptimized(portRanges, string(ingressPort.Protocol)); err != nil {
			return fmt.Errorf("port conflict detected within port specification: %s port %s - %v",
				ingressPort.Protocol, ingressPort.Port, err)
		}

		servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRanges...)
	}

	for protocol, ranges := range servicePorts {
		if err := CheckPortRangeOverlapsOptimized(ranges, string(protocol)); err != nil {
			return fmt.Errorf("port conflict detected within service %s: %v", service.Key(), err)
		}
	}

	for existingServiceRef, existingServicePorts := range portMapping.AccessPorts {
		// Skip the current service
		if models.ServiceRefKey(existingServiceRef) == service.Key() {
			continue
		}

		for protocol, serviceRanges := range servicePorts {
			existingRanges := existingServicePorts.Ports[protocol]
			if len(existingRanges) == 0 {
				continue
			}

			allRanges := make([]models.PortRange, 0, len(serviceRanges)+len(existingRanges))
			allRanges = append(allRanges, serviceRanges...)
			allRanges = append(allRanges, existingRanges...)

			if err := CheckPortRangeOverlapsOptimized(allRanges, string(protocol)); err != nil {
				for _, serviceRange := range serviceRanges {
					for _, existingRange := range existingRanges {
						if DoPortRangesOverlap(serviceRange, existingRange) {
							return fmt.Errorf("%s port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
								protocol, serviceRange.Start, serviceRange.End, service.Key(),
								existingRange.Start, existingRange.End, models.ServiceRefKey(existingServiceRef))
						}
					}
				}
				// Fallback error if we can't identify specific overlap
				return fmt.Errorf("%s port conflict between service %s and existing service %s",
					protocol, service.Key(), models.ServiceRefKey(existingServiceRef))
			}
		}
	}

	return nil
}

// ValidateExists checks if an address group binding exists
func (v *AddressGroupBindingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.AddressGroupBinding).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in an address group binding are valid
func (v *AddressGroupBindingValidator) ValidateReferences(ctx context.Context, binding models.AddressGroupBinding) error {
	serviceValidator := NewServiceValidator(v.reader)
	addressGroupValidator := NewAddressGroupValidator(v.reader)

	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.ServiceRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid service reference in address group binding %s", binding.Key())
	}

	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	if err := addressGroupValidator.ValidateExists(ctx, agID); err != nil {
		return errors.Wrapf(err, "invalid address group reference in address group binding %s", binding.Key())
	}

	return nil
}

// ValidateNoConflictWithServiceSpec checks that the AddressGroup being bound
// is NOT already present in Service.spec.addressGroups
func (v *AddressGroupBindingValidator) ValidateNoConflictWithServiceSpec(ctx context.Context, binding models.AddressGroupBinding) error {
	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.ServiceRef.Namespace))
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			// Service doesn't exist - reference validation will catch this
			return nil
		}
		return errors.Wrap(err, "failed to get service for AddressGroup conflict check")
	}

	agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
	for _, specAG := range service.AddressGroups {
		specAGKey := fmt.Sprintf("%s/%s", specAG.Namespace, specAG.Name)
		if agKey == specAGKey {
			return fmt.Errorf("AddressGroup conflict: AddressGroup %s is already present in Service %s spec.addressGroups and cannot be bound via AddressGroupBinding", agKey, service.Key())
		}
	}

	return nil
}

// ValidateNoDuplicateBindings проверяет, что нет существующего биндинга между тем же сервисом и той же адресной группой
func (v *AddressGroupBindingValidator) ValidateNoDuplicateBindings(ctx context.Context, binding models.AddressGroupBinding) error {
	// Создаем флаг для отслеживания наличия дубликата
	duplicateFound := false

	// Получаем все существующие биндинги
	err := v.reader.ListAddressGroupBindings(ctx, func(existingBinding models.AddressGroupBinding) error {
		// Пропускаем сравнение с самим собой (для случая обновления)
		if existingBinding.Key() == binding.Key() {
			return nil
		}

		// Проверяем, есть ли биндинг с тем же сервисом и той же адресной группой
		if existingBinding.ServiceRefKey() == binding.ServiceRefKey() &&
			existingBinding.AddressGroupRefKey() == binding.AddressGroupRefKey() {
			duplicateFound = true
			// Возвращаем ошибку для прерывания цикла
			return fmt.Errorf("duplicate found")
		}

		return nil
	}, nil)

	// Игнорируем ошибку "duplicate found", так как это не настоящая ошибка, а способ прервать цикл
	if err != nil && err.Error() != "duplicate found" {
		return errors.Wrap(err, "failed to check for duplicate bindings")
	}

	// Если найден дубликат, возвращаем ошибку
	if duplicateFound {
		return fmt.Errorf("duplicate binding found: a binding between service '%s' and address group '%s' already exists",
			binding.ServiceRefKey(), binding.AddressGroupRefKey())
	}

	return nil
}

// ValidateForCreation validates an address group binding before creation
// This method is used during CREATE operations via webhook and should avoid backend service lookups
// to prevent circular dependency issues during resource creation
func (v *AddressGroupBindingValidator) ValidateForCreation(ctx context.Context, binding *models.AddressGroupBinding) error {
	keyExtractor := func(entity interface{}) string {
		if agb, ok := entity.(*models.AddressGroupBinding); ok {
			return agb.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, binding.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	if binding.ServiceRef.Name == "" {
		return fmt.Errorf("serviceRef.name is required in binding %s", binding.Key())
	}

	if binding.AddressGroupRef.Name == "" {
		return fmt.Errorf("addressGroupRef.name is required in binding %s", binding.Key())
	}

	// Ensure namespace consistency within the binding itself
	if binding.Namespace == "" {
		return fmt.Errorf("namespace is required in binding %s", binding.Key())
	}

	if err := v.ValidateNoDuplicateBindings(ctx, *binding); err != nil {
		return err
	}

	if err := v.ValidateNoConflictWithServiceSpec(ctx, *binding); err != nil {
		return err
	}

	if err := v.ValidateReferences(ctx, *binding); err != nil {
		return err
	}

	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.ServiceRef.Namespace))
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		return fmt.Errorf("failed to get service for port conflict validation: %v", err)
	}

	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, agID)
	if err != nil {
		return fmt.Errorf("failed to get address group for namespace validation: %v", err)
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found or is nil for binding %s", binding.Key())
	}

	if addressGroup.Namespace != binding.Namespace {
		policyFound := false

		namespaceScope := ports.ResourceIdentifierScope{
			Identifiers: []models.ResourceIdentifier{
				{Namespace: addressGroup.Namespace},
			},
		}

		err := v.reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
			if policy.AddressGroupRefKey() == binding.AddressGroupRefKey() &&
				policy.ServiceRefKey() == binding.ServiceRefKey() {
				policyFound = true
				return fmt.Errorf("policy found")
			}
			return nil
		}, namespaceScope)

		if err != nil && err.Error() != "policy found" {
			return fmt.Errorf("failed to check for binding policies: %v", err)
		}

		if !policyFound {
			return fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace %s that references both AddressGroup %s and Service %s",
				addressGroup.Namespace, binding.AddressGroupRef.Name, binding.ServiceRef.Name)
		}
	}

	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == nil && portMapping != nil {
		if err := CheckPortOverlaps(*service, *portMapping); err != nil {
			return fmt.Errorf("port conflict detected: %v", err)
		}
	}

	return nil
}

// ValidateForPostCommit validates an address group binding after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *AddressGroupBindingValidator) ValidateForPostCommit(ctx context.Context, binding *models.AddressGroupBinding) error {
	if err := v.ValidateReferences(ctx, *binding); err != nil {
		return err
	}

	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.AddressGroupRef.Namespace}
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get address group for cross-namespace validation")
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}

	if addressGroup.Namespace != binding.ServiceRef.Namespace {
		policyFound := false
		namespaceScope := ports.ResourceIdentifierScope{
			Identifiers: []models.ResourceIdentifier{
				{Namespace: addressGroup.Namespace},
			},
		}
		err := v.reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
			if policy.AddressGroupRefKey() == binding.AddressGroupRefKey() &&
				policy.ServiceRefKey() == binding.ServiceRefKey() {
				policyFound = true
				return fmt.Errorf("policy found")
			}
			return nil
		}, namespaceScope)

		if err != nil && err.Error() != "policy found" {
			return fmt.Errorf("failed to check for binding policies: %v", err)
		}

		if !policyFound {
			return fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found")
		}
	}

	if err := v.ValidateNoDuplicateBindings(ctx, *binding); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates an address group binding before update
func (v *AddressGroupBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding models.AddressGroupBinding, newBinding *models.AddressGroupBinding) error {
	referenceComparisons := []ObjectReferenceComparison{
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldBinding.ServiceRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newBinding.ServiceRef},
			FieldName: "serviceRef",
		},
		{
			OldRef:    &NamespacedObjectReferenceAdapter{Ref: oldBinding.AddressGroupRef},
			NewRef:    &NamespacedObjectReferenceAdapter{Ref: newBinding.AddressGroupRef},
			FieldName: "addressGroupRef",
		},
	}

	// Validate all object references haven't changed when Ready=True
	if err := v.BaseValidator.ValidateObjectReferencesNotChangedWhenReady(oldBinding, *newBinding, referenceComparisons); err != nil {
		return err
	}

	// Fallback field-level validation for additional protection
	if err := v.BaseValidator.ValidateFieldNotChangedWhenReady("serviceRef", oldBinding, *newBinding, oldBinding.ServiceRefKey(), newBinding.ServiceRefKey()); err != nil {
		return err
	}

	if err := v.BaseValidator.ValidateFieldNotChangedWhenReady("addressGroupRef", oldBinding, *newBinding, oldBinding.AddressGroupRefKey(), newBinding.AddressGroupRefKey()); err != nil {
		return err
	}

	// Create binding spec structures for comparison
	oldSpec := struct {
		ServiceRef      models.ServiceRef
		AddressGroupRef models.AddressGroupRef
	}{
		ServiceRef:      oldBinding.ServiceRef,
		AddressGroupRef: oldBinding.AddressGroupRef,
	}

	newSpec := struct {
		ServiceRef      models.ServiceRef
		AddressGroupRef models.AddressGroupRef
	}{
		ServiceRef:      newBinding.ServiceRef,
		AddressGroupRef: newBinding.AddressGroupRef,
	}

	// Validate that spec hasn't changed when Ready condition is true
	if err := v.BaseValidator.ValidateSpecNotChangedWhenReady(oldBinding, *newBinding, oldSpec, newSpec); err != nil {
		return err
	}

	// Continue with existing validation logic

	// Validate references (including namespace check)
	if err := v.ValidateReferences(ctx, *newBinding); err != nil {
		return err
	}

	// Check that service reference hasn't changed (fallback validation)
	if oldBinding.ServiceRefKey() != newBinding.ServiceRefKey() {
		return fmt.Errorf("cannot change service reference after creation")
	}

	// Check that address group reference hasn't changed (fallback validation)
	if oldBinding.AddressGroupRefKey() != newBinding.AddressGroupRefKey() {
		return fmt.Errorf("cannot change address group reference after creation")
	}

	// Получаем address group для проверки namespace
	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(newBinding.AddressGroupRef.Name, models.WithNamespace(newBinding.AddressGroupRef.Namespace))
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, agID)
	if err != nil {
		return errors.Wrapf(err, "failed to get address group for namespace validation in binding %s", newBinding.Key())
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found or is nil for binding %s", newBinding.Key())
	}

	// Если AddressGroup находится в другом namespace, чем Binding/Service
	if addressGroup.Namespace != newBinding.Namespace {
		// Проверяем наличие политики в namespace AddressGroup
		policyFound := false

		// Создаем скоуп для namespace адресной группы
		namespaceScope := ports.ResourceIdentifierScope{
			Identifiers: []models.ResourceIdentifier{
				{Namespace: addressGroup.Namespace},
			},
		}

		err := v.reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
			// Проверяем, что политика ссылается на нужные AddressGroup и Service
			if policy.AddressGroupRefKey() == newBinding.AddressGroupRefKey() &&
				policy.ServiceRefKey() == newBinding.ServiceRefKey() {
				policyFound = true
				return fmt.Errorf("policy found") // Используем ошибку для прерывания цикла
			}
			return nil
		}, namespaceScope)

		// Игнорируем ошибку "policy found", так как это не настоящая ошибка
		if err != nil && err.Error() != "policy found" {
			return errors.Wrap(err, "failed to check for binding policies")
		}

		if !policyFound {
			return fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace %s that references both AddressGroup %s and Service %s",
				addressGroup.Namespace, newBinding.AddressGroupRef.Name, newBinding.ServiceRef.Name)
		}
	}

	// Get the service to access its ports
	// Create ResourceIdentifier from ObjectReference
	serviceID := models.NewResourceIdentifier(newBinding.ServiceRef.Name, models.WithNamespace(newBinding.Namespace))
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for port mapping in binding %s", newBinding.Key())
	}
	if service == nil {
		return fmt.Errorf("service not found or is nil for binding %s", newBinding.Key())
	}

	// Check if there's an existing port mapping for this address group
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == nil && portMapping != nil {
		// Port mapping exists - check for port overlaps
		// Create a temporary updated mapping to check for overlaps
		// Convert ObjectReference to ServiceRef
		serviceRef := models.NewServiceRef(newBinding.ServiceRef.Name, models.WithNamespace(newBinding.Namespace))
		updatedMapping := UpdatePortMapping(*portMapping, serviceRef, *service)

		// Check for port overlaps
		if err := CheckPortOverlaps(*service, *updatedMapping); err != nil {
			return err
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an address group binding
func (v *AddressGroupBindingValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	return nil
}
