package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"

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

	// Check if there's an existing port mapping for this address group
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, binding.AddressGroupRef.ResourceIdentifier)

	if err == nil {
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

	// Get the service to access its ports
	service, err := v.reader.GetServiceByID(ctx, newBinding.ServiceRef.ResourceIdentifier)
	if err != nil {
		return errors.Wrapf(err, "failed to get service for port mapping in binding %s", newBinding.Key())
	}

	// Check if there's an existing port mapping for this address group
	portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, newBinding.AddressGroupRef.ResourceIdentifier)
	if err == nil {
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
