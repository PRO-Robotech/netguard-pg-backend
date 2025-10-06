package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

// CreateNewPortMapping creates a new port mapping for an address group and service
func CreateNewPortMapping(addressGroupID models.ResourceIdentifier, service models.Service) *models.AddressGroupPortMapping {
	klog.Infof("🔧 CreateNewPortMapping: creating port mapping for address group %s and service %s", addressGroupID.Key(), service.Key())
	klog.Infof("🔧 Service has %d ingress ports", len(service.IngressPorts))

	// Create a new port mapping with the same ID as the address group
	portMapping := &models.AddressGroupPortMapping{
		SelfRef: models.SelfRef{
			ResourceIdentifier: addressGroupID,
		},
		AccessPorts: make(map[models.ServiceRef]models.ServicePorts),
	}

	// Add the service ports to the mapping
	servicePorts := models.ServicePorts{
		Ports: make(models.ProtocolPorts),
	}

	// Convert service ingress ports to port ranges
	for i, ingressPort := range service.IngressPorts {
		klog.Infof("🔧 Processing port %d: Protocol=%s, Port=%s", i, ingressPort.Protocol, ingressPort.Port)

		portRanges, err := ParsePortRanges(ingressPort.Port)
		if err != nil {
			klog.Errorf("❌ Failed to parse port %s: %v", ingressPort.Port, err)
			continue
		}

		klog.Infof("🔧 Parsed %d port ranges for port %s", len(portRanges), ingressPort.Port)

		// Add port ranges to the appropriate protocol
		for j, portRange := range portRanges {
			klog.Infof("🔧 Adding port range %d: %d-%d for protocol %s", j, portRange.Start, portRange.End, ingressPort.Protocol)
			servicePorts.Ports[ingressPort.Protocol] = append(
				servicePorts.Ports[ingressPort.Protocol],
				portRange,
			)
		}
	}

	// Add the service ports to the mapping
	serviceRef := models.NewServiceRef(service.Name, models.WithNamespace(service.Namespace))
	portMapping.AccessPorts[serviceRef] = servicePorts

	klog.Infof("🔧 Final port mapping has %d service entries", len(portMapping.AccessPorts))
	for serviceRef, servicePorts := range portMapping.AccessPorts {
		klog.Infof("🔧 Service %s has %d protocols", models.ServiceRefKey(serviceRef), len(servicePorts.Ports))
		for protocol, ranges := range servicePorts.Ports {
			klog.Infof("🔧 Protocol %s has %d port ranges", protocol, len(ranges))
		}
	}

	return portMapping
}

// UpdatePortMapping updates an existing port mapping with service ports
func UpdatePortMapping(
	existingMapping models.AddressGroupPortMapping,
	serviceRef models.ServiceRef,
	service models.Service,
) *models.AddressGroupPortMapping {
	klog.Infof("🔧 UpdatePortMapping: updating port mapping for address group %s and service %s", existingMapping.Key(), service.Key())
	klog.Infof("🔧 Service has %d ingress ports", len(service.IngressPorts))

	// Create a copy of the existing mapping
	updatedMapping := existingMapping

	// Create service ports if they don't exist
	if updatedMapping.AccessPorts == nil {
		updatedMapping.AccessPorts = make(map[models.ServiceRef]models.ServicePorts)
	}

	// Create or update the service ports
	servicePorts := models.ServicePorts{
		Ports: make(models.ProtocolPorts),
	}

	// Convert service ingress ports to port ranges
	for i, ingressPort := range service.IngressPorts {
		klog.Infof("🔧 Processing port %d: Protocol=%s, Port=%s", i, ingressPort.Protocol, ingressPort.Port)

		portRanges, err := ParsePortRanges(ingressPort.Port)
		if err != nil {
			klog.Errorf("❌ Failed to parse port %s: %v", ingressPort.Port, err)
			continue
		}

		klog.Infof("🔧 Parsed %d port ranges for port %s", len(portRanges), ingressPort.Port)

		// Add port ranges to the appropriate protocol
		for j, portRange := range portRanges {
			klog.Infof("🔧 Adding port range %d: %d-%d for protocol %s", j, portRange.Start, portRange.End, ingressPort.Protocol)
			servicePorts.Ports[ingressPort.Protocol] = append(
				servicePorts.Ports[ingressPort.Protocol],
				portRange,
			)
		}
	}

	// Update the service ports in the mapping
	// Use the service's ResourceIdentifier to ensure the namespace is preserved
	updatedMapping.AccessPorts[models.NewServiceRef(service.Name, models.WithNamespace(service.Namespace))] = servicePorts

	klog.Infof("🔧 Updated port mapping has %d service entries", len(updatedMapping.AccessPorts))
	for serviceRef, servicePorts := range updatedMapping.AccessPorts {
		klog.Infof("🔧 Service %s has %d protocols", models.ServiceRefKey(serviceRef), len(servicePorts.Ports))
		for protocol, ranges := range servicePorts.Ports {
			klog.Infof("🔧 Protocol %s has %d port ranges", protocol, len(ranges))
		}
	}

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

		// ✨ OPTIMIZATION: Use optimized overlap checking within current port set
		if err := CheckPortRangeOverlapsOptimized(portRanges, string(ingressPort.Protocol)); err != nil {
			return fmt.Errorf("port conflict detected within port specification: %s port %s - %v",
				ingressPort.Protocol, ingressPort.Port, err)
		}

		servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRanges...)
	}

	// ✨ OPTIMIZATION: Use optimized overlap checking within same protocol for service
	for protocol, ranges := range servicePorts {
		if err := CheckPortRangeOverlapsOptimized(ranges, string(protocol)); err != nil {
			return fmt.Errorf("port conflict detected within service %s: %v", service.Key(), err)
		}
	}

	// Check for overlaps with existing services in the port mapping
	for existingServiceRef, existingServicePorts := range portMapping.AccessPorts {
		// Skip the current service
		if models.ServiceRefKey(existingServiceRef) == service.Key() {
			continue
		}

		// ✨ OPTIMIZATION: Check each protocol using optimized algorithm
		for protocol, serviceRanges := range servicePorts {
			existingRanges := existingServicePorts.Ports[protocol]
			if len(existingRanges) == 0 {
				continue
			}
			// Combine all ranges for this protocol and check for overlaps
			allRanges := make([]models.PortRange, 0, len(serviceRanges)+len(existingRanges))
			allRanges = append(allRanges, serviceRanges...)
			allRanges = append(allRanges, existingRanges...)

			// Use optimized overlap detection - if there are overlaps, identify which services
			if err := CheckPortRangeOverlapsOptimized(allRanges, string(protocol)); err != nil {
				// Enhanced error reporting: identify which specific services have conflicts
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

	// Create ResourceIdentifier from ObjectReference
	// 🔧 CRITICAL FIX: Use ServiceRef.Namespace instead of binding.Namespace for cross-namespace support
	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.ServiceRef.Namespace))
	if err := serviceValidator.ValidateExists(ctx, serviceID); err != nil {
		return errors.Wrapf(err, "invalid service reference in address group binding %s", binding.Key())
	}

	// Create ResourceIdentifier from NamespacedObjectReference
	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	if err := addressGroupValidator.ValidateExists(ctx, agID); err != nil {
		return errors.Wrapf(err, "invalid address group reference in address group binding %s", binding.Key())
	}

	// 🔧 REMOVED: Cross-namespace binding restriction - this is now handled by AddressGroupBindingPolicy validation
	// Cross-namespace bindings are allowed when there's a proper AddressGroupBindingPolicy in place
	// The policy validation is performed later in ValidateForCreation/ValidateForUpdate methods
	// No need to fetch service just for namespace validation since we support cross-namespace bindings
	// REBUILD FORCE: 2025-08-18 08:07

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
	// PHASE 1: Check for duplicate entity (CRITICAL FIX for overwrite issue)
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if agb, ok := entity.(*models.AddressGroupBinding); ok {
			return agb.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, binding.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// PHASE 2: Basic field validation (existing validation)
	klog.Infof("🔍 BACKEND ValidateForCreation: binding %s - performing basic field validation", binding.Key())

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

	// If AddressGroup namespace is not specified, assume it's in the same namespace as the binding
	if binding.AddressGroupRef.Namespace == "" {
		klog.Infof("AddressGroupRef.Namespace not specified for binding %s, using binding namespace %s",
			binding.Key(), binding.Namespace)
	}

	// Basic duplicate binding validation that doesn't require service lookups
	// This will validate against existing bindings but won't validate if the service exists
	if err := v.ValidateNoDuplicateBindings(ctx, *binding); err != nil {
		klog.Errorf("Duplicate binding validation failed for %s: %v", binding.Key(), err)
		return err
	}

	// 🔧 FIX: For AddressGroupBinding CREATE during admission webhook,
	// we CAN and SHOULD perform port conflict validation because:
	// 1. The referenced service must already exist
	// 2. The referenced AddressGroup must already exist
	// 3. We need to prevent port conflicts BEFORE binding creation
	klog.Infof("🔧 FIX: ValidateForCreation binding %s - performing port conflict validation", binding.Key())

	// Validate that the service and AddressGroup exist and check for port conflicts
	if err := v.ValidateReferences(ctx, *binding); err != nil {
		klog.Errorf("🔧 FIX: Reference validation failed for binding %s: %v", binding.Key(), err)
		return err
	}

	// Get service and existing port mapping to check for port conflicts
	// 🔧 CRITICAL FIX: Use ServiceRef.Namespace instead of binding.Namespace for cross-namespace support
	serviceID := models.NewResourceIdentifier(binding.ServiceRef.Name, models.WithNamespace(binding.ServiceRef.Namespace))
	service, err := v.reader.GetServiceByID(ctx, serviceID)
	if err != nil {
		klog.Errorf("🔧 FIX: Failed to get service for port conflict validation in binding %s: %v", binding.Key(), err)
		return fmt.Errorf("failed to get service for port conflict validation: %v", err)
	}

	// Get AddressGroup to check namespace for cross-namespace policy validation
	agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, agID)
	if err != nil {
		klog.Errorf("🔧 FIX: Failed to get address group for namespace validation in binding %s: %v", binding.Key(), err)
		return fmt.Errorf("failed to get address group for namespace validation: %v", err)
	}
	if addressGroup == nil {
		klog.Errorf("🔧 FIX: Address group not found or is nil for binding %s", binding.Key())
		return fmt.Errorf("address group not found or is nil for binding %s", binding.Key())
	}

	// 🔧 FIX: Add cross-namespace policy validation to ValidateForCreation
	// If AddressGroup is in a different namespace than Binding/Service, check for policy
	if addressGroup.Namespace != binding.Namespace {
		klog.Infof("🔧 FIX: Cross-namespace binding detected - AddressGroup %s in namespace %s, binding %s in namespace %s",
			addressGroup.Name, addressGroup.Namespace, binding.Name, binding.Namespace)

		// Check for AddressGroupBindingPolicy in AddressGroup's namespace
		policyFound := false

		// Create scope for AddressGroup's namespace
		namespaceScope := ports.ResourceIdentifierScope{
			Identifiers: []models.ResourceIdentifier{
				{Namespace: addressGroup.Namespace},
			},
		}

		err := v.reader.ListAddressGroupBindingPolicies(ctx, func(policy models.AddressGroupBindingPolicy) error {
			// Check that policy references the required AddressGroup and Service
			if policy.AddressGroupRefKey() == binding.AddressGroupRefKey() &&
				policy.ServiceRefKey() == binding.ServiceRefKey() {
				policyFound = true
				klog.Infof("🔧 FIX: Found required policy %s in namespace %s for cross-namespace binding %s",
					policy.Name, policy.Namespace, binding.Key())
				return fmt.Errorf("policy found") // Use error to break the loop
			}
			return nil
		}, namespaceScope)

		// Ignore "policy found" error as it's not a real error
		if err != nil && err.Error() != "policy found" {
			klog.Errorf("🔧 FIX: Failed to check for binding policies: %v", err)
			return fmt.Errorf("failed to check for binding policies: %v", err)
		}

		if !policyFound {
			klog.Errorf("🔧 FIX: Cross-namespace binding blocked - no policy found")
			return fmt.Errorf("cross-namespace binding not allowed: no AddressGroupBindingPolicy found in namespace %s that references both AddressGroup %s and Service %s",
				addressGroup.Namespace, binding.AddressGroupRef.Name, binding.ServiceRef.Name)
		}

		klog.Infof("✅ Cross-namespace binding policy validation passed for binding %s", binding.Key())
	}

	// Check if there's an existing port mapping and validate port conflicts
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, agID)
	if err == nil && portMapping != nil {
		// Port mapping exists - check for port overlaps with this new service
		if err := CheckPortOverlaps(*service, *portMapping); err != nil {
			klog.Errorf("🔧 FIX: Port conflict detected for binding %s: %v", binding.Key(), err)
			return fmt.Errorf("port conflict detected: %v", err)
		}
	}

	klog.Infof("✅ ValidateForCreation completed for binding %s - all validation passed including cross-namespace policy and port conflicts", binding.Key())
	return nil
}

// ValidateForPostCommit validates an address group binding after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *AddressGroupBindingValidator) ValidateForPostCommit(ctx context.Context, binding *models.AddressGroupBinding) error {
	// PHASE 1: Skip duplicate entity check (entity is already committed)
	// This method is called AFTER the entity is saved to database, so existence is expected

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, *binding); err != nil {
		return err
	}

	// PHASE 3: Check for cross-namespace policy requirement (existing validation)
	// Get address group to determine namespace
	addressGroupRef := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.AddressGroupRef.Namespace}
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, addressGroupRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get address group for cross-namespace validation")
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found: %s", addressGroupRef.Key())
	}

	// Check if cross-namespace validation is needed
	if addressGroup.Namespace != binding.ServiceRef.Namespace {
		klog.Infof("🔧 Cross-namespace binding detected in post-commit validation: AddressGroup=%s, Service=%s",
			addressGroup.Namespace, binding.ServiceRef.Namespace)

		// Check for required policy
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

	// PHASE 4: Check for business logic duplicates
	if err := v.ValidateNoDuplicateBindings(ctx, *binding); err != nil {
		return err
	}

	klog.Infof("✅ ValidateForPostCommit completed for binding %s", binding.Key())
	return nil
}

// ValidateForUpdate validates an address group binding before update
func (v *AddressGroupBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding models.AddressGroupBinding, newBinding *models.AddressGroupBinding) error {
	// 🚀 PHASE 1 & 2: Ready Condition Framework + Object Reference Immutability
	// Ported from k8s-controller addressgroupbinding_webhook.go pattern

	// Advanced object reference validation - validate multiple references at once
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
	// AddressGroupBinding is a relationship entity - nothing should depend on it directly
	// It can be safely deleted as it doesn't have dependents

	// Log the dependency check for consistency with other validators
	return nil
}
