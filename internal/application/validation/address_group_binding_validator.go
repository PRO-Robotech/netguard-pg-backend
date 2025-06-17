package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// CreateNewPortMapping creates a new port mapping for an address group and service
func CreateNewPortMapping(addressGroupID models.ResourceIdentifier, service models.Service) *models.AddressGroupPortMapping {
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
	for _, ingressPort := range service.IngressPorts {
		portRange, err := ParsePortRange(ingressPort.Port)
		if err != nil {
			// Skip invalid ports
			continue
		}

		// Add port range to the appropriate protocol
		servicePorts.Ports[ingressPort.Protocol] = append(
			servicePorts.Ports[ingressPort.Protocol],
			portRange,
		)
	}

	// Add the service ports to the mapping
	portMapping.AccessPorts[models.ServiceRef{ResourceIdentifier: service.ResourceIdentifier}] = servicePorts

	return portMapping
}

// UpdatePortMapping updates an existing port mapping with service ports
func UpdatePortMapping(
	existingMapping models.AddressGroupPortMapping,
	serviceRef models.ServiceRef,
	service models.Service,
) *models.AddressGroupPortMapping {
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
	for _, ingressPort := range service.IngressPorts {
		portRange, err := ParsePortRange(ingressPort.Port)
		if err != nil {
			// Skip invalid ports
			continue
		}

		// Add port range to the appropriate protocol
		servicePorts.Ports[ingressPort.Protocol] = append(
			servicePorts.Ports[ingressPort.Protocol],
			portRange,
		)
	}

	// Update the service ports in the mapping
	updatedMapping.AccessPorts[serviceRef] = servicePorts

	return &updatedMapping
}

// CheckPortOverlaps checks for port overlaps in a port mapping
func CheckPortOverlaps(service models.Service, portMapping models.AddressGroupPortMapping) error {
	// Create a map of service ports by protocol
	servicePorts := make(map[models.TransportProtocol][]models.PortRange)
	for _, ingressPort := range service.IngressPorts {
		portRange, err := ParsePortRange(ingressPort.Port)
		if err != nil {
			return fmt.Errorf("invalid port in service %s: %w", service.Key(), err)
		}
		servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRange)
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
