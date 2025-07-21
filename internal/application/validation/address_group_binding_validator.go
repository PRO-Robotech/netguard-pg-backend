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
	serviceRef := models.ServiceRef{ResourceIdentifier: service.ResourceIdentifier}
	portMapping.AccessPorts[serviceRef] = servicePorts

	klog.Infof("🔧 Final port mapping has %d service entries", len(portMapping.AccessPorts))
	for serviceRef, servicePorts := range portMapping.AccessPorts {
		klog.Infof("🔧 Service %s has %d protocols", serviceRef.Key(), len(servicePorts.Ports))
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
	updatedMapping.AccessPorts[models.ServiceRef{ResourceIdentifier: service.ResourceIdentifier}] = servicePorts

	klog.Infof("🔧 Updated port mapping has %d service entries", len(updatedMapping.AccessPorts))
	for serviceRef, servicePorts := range updatedMapping.AccessPorts {
		klog.Infof("🔧 Service %s has %d protocols", serviceRef.Key(), len(servicePorts.Ports))
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

		// Проверяем на перекрытия внутри текущего набора портов
		for i, range1 := range portRanges {
			for j, range2 := range portRanges {
				if i != j && DoPortRangesOverlap(range1, range2) {
					return fmt.Errorf("port conflict detected within port specification: %s port %s has overlapping ranges %d-%d and %d-%d",
						ingressPort.Protocol, ingressPort.Port, range1.Start, range1.End, range2.Start, range2.End)
				}
			}
		}

		servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRanges...)
	}

	// Проверяем на перекрытия внутри одного протокола для сервиса
	for protocol, ranges := range servicePorts {
		for i, range1 := range ranges {
			for j, range2 := range ranges {
				if i != j && DoPortRangesOverlap(range1, range2) {
					return fmt.Errorf("port conflict detected within service %s: %s port ranges %d-%d and %d-%d overlap",
						service.Key(), protocol, range1.Start, range1.End, range2.Start, range2.End)
				}
			}
		}
	}

	// Check for overlaps with existing services in the port mapping
	for existingServiceRef, existingServicePorts := range portMapping.AccessPorts {
		// Skip the current service
		if existingServiceRef.Key() == service.Key() {
			continue
		}

		// Check TCP ports
		for _, serviceRange := range servicePorts[models.TCP] {
			for _, tcpPortRange := range existingServicePorts.Ports[models.TCP] {
				if DoPortRangesOverlap(serviceRange, tcpPortRange) {
					return fmt.Errorf("TCP port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
						serviceRange.Start, serviceRange.End, service.Key(), tcpPortRange.Start, tcpPortRange.End, existingServiceRef.Key())
				}
			}
		}

		// Check UDP ports
		for _, serviceRange := range servicePorts[models.UDP] {
			for _, udpPortRange := range existingServicePorts.Ports[models.UDP] {
				if DoPortRangesOverlap(serviceRange, udpPortRange) {
					return fmt.Errorf("UDP port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
						serviceRange.Start, serviceRange.End, service.Key(), udpPortRange.Start, udpPortRange.End, existingServiceRef.Key())
				}
			}
		}
	}

	return nil
}

// ValidateExists checks if an address group binding exists
func (v *AddressGroupBindingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(models.AddressGroupBinding).Key()
	})
}

// ValidateReferences checks if all references in an address group binding are valid
func (v *AddressGroupBindingValidator) ValidateReferences(ctx context.Context, binding models.AddressGroupBinding) error {
	serviceValidator := NewServiceValidator(v.reader)
	addressGroupValidator := NewAddressGroupValidator(v.reader)

	if err := serviceValidator.ValidateExists(ctx, binding.ServiceRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid service reference in address group binding %s", binding.Key())
	}

	if err := addressGroupValidator.ValidateExists(ctx, binding.AddressGroupRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid address group reference in address group binding %s", binding.Key())
	}

	// Get the service to check namespace
	service, err := v.reader.GetServiceByID(ctx, binding.ServiceRef.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for namespace validation in binding %s", binding.Key())
	}

	// Ensure binding is in the same namespace as the service
	if binding.Namespace != service.Namespace {
		return fmt.Errorf("address group binding namespace '%s' must match service namespace '%s'",
			binding.Namespace, service.Namespace)
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
		if existingBinding.ServiceRef.Key() == binding.ServiceRef.Key() &&
			existingBinding.AddressGroupRef.Key() == binding.AddressGroupRef.Key() {
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
			binding.ServiceRef.Key(), binding.AddressGroupRef.Key())
	}

	return nil
}

// ValidateForCreation validates an address group binding before creation
func (v *AddressGroupBindingValidator) ValidateForCreation(ctx context.Context, binding *models.AddressGroupBinding) error {
	// Проверяем существование сервиса
	serviceValidator := NewServiceValidator(v.reader)
	if err := serviceValidator.ValidateExists(ctx, binding.ServiceRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid service reference in binding %s", binding.Key())
	}

	// Получаем сервис для проверки и/или установки namespace
	service, err := v.reader.GetServiceByID(ctx, binding.ServiceRef.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for namespace validation in binding %s", binding.Key())
	}

	// Проверяем наличие дубликатов биндингов
	if err := v.ValidateNoDuplicateBindings(ctx, *binding); err != nil {
		return err
	}

	// Если namespace не указан, берем его из сервиса
	if binding.Namespace == "" {
		binding.Namespace = service.Namespace
	} else if binding.Namespace != service.Namespace {
		// Если namespace указан, проверяем соответствие
		return fmt.Errorf("binding namespace '%s' must match service namespace '%s'",
			binding.Namespace, service.Namespace)
	}

	// Проверяем существование address group
	addressGroupValidator := NewAddressGroupValidator(v.reader)
	if err := addressGroupValidator.ValidateExists(ctx, binding.AddressGroupRef.ResourceIdentifier); err != nil {
		return errors.Wrapf(err, "invalid address group reference in binding %s", binding.Key())
	}

	// Получаем address group для проверки namespace
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, binding.AddressGroupRef.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to get address group for namespace validation in binding %s", binding.Key())
	}
	if addressGroup == nil {
		return fmt.Errorf("address group not found or is nil for binding %s", binding.Key())
	}

	// Если AddressGroup находится в другом namespace, чем Binding/Service
	if addressGroup.Namespace != binding.Namespace {
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
			if policy.AddressGroupRef.Key() == binding.AddressGroupRef.Key() &&
				policy.ServiceRef.Key() == binding.ServiceRef.Key() {
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
				addressGroup.Namespace, binding.AddressGroupRef.Name, binding.ServiceRef.Name)
		}
	}

	// Check if there's an existing port mapping for this address group
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, binding.AddressGroupRef.ResourceIdentifier)

	if err == nil && portMapping != nil {
		// Port mapping exists - check for port overlaps
		// Create a temporary updated mapping to check for overlaps
		updatedMapping := UpdatePortMapping(*portMapping, binding.ServiceRef, *service)

		// Check for port overlaps
		if err := CheckPortOverlaps(*service, *updatedMapping); err != nil {
			return err
		}
	}

	return nil
}

// ValidateForUpdate validates an address group binding before update
func (v *AddressGroupBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding models.AddressGroupBinding, newBinding *models.AddressGroupBinding) error {
	// Validate references (including namespace check)
	if err := v.ValidateReferences(ctx, *newBinding); err != nil {
		return err
	}

	// Check that service reference hasn't changed
	if oldBinding.ServiceRef.Key() != newBinding.ServiceRef.Key() {
		return fmt.Errorf("cannot change service reference after creation")
	}

	// Check that address group reference hasn't changed
	if oldBinding.AddressGroupRef.Key() != newBinding.AddressGroupRef.Key() {
		return fmt.Errorf("cannot change address group reference after creation")
	}

	// Получаем address group для проверки namespace
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, newBinding.AddressGroupRef.ResourceIdentifier)
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
			if policy.AddressGroupRef.Key() == newBinding.AddressGroupRef.Key() &&
				policy.ServiceRef.Key() == newBinding.ServiceRef.Key() {
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
	service, err := v.reader.GetServiceByID(ctx, newBinding.ServiceRef.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for port mapping in binding %s", newBinding.Key())
	}
	if service == nil {
		return fmt.Errorf("service not found or is nil for binding %s", newBinding.Key())
	}

	// Check if there's an existing port mapping for this address group
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, newBinding.AddressGroupRef.ResourceIdentifier)
	if err == nil && portMapping != nil {
		// Port mapping exists - check for port overlaps
		// Create a temporary updated mapping to check for overlaps
		updatedMapping := UpdatePortMapping(*portMapping, newBinding.ServiceRef, *service)

		// Check for port overlaps
		if err := CheckPortOverlaps(*service, *updatedMapping); err != nil {
			return err
		}
	}

	return nil
}
