package validation

import (
	"context"
	"fmt"
	"reflect"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// ValidateExists checks if a service exists
func (v *ServiceValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.Service).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in a service are valid
func (v *ServiceValidator) ValidateReferences(ctx context.Context, service models.Service) error {
	agValidator := NewAddressGroupValidator(v.reader)

	for _, agRef := range service.AddressGroups {
		if err := agValidator.ValidateExists(ctx, models.ResourceIdentifier{Name: agRef.Name, Namespace: agRef.Namespace}); err != nil {
			return errors.Wrapf(err, "invalid address group reference in service %s", service.Key())
		}
	}

	return nil
}

// ValidateNoDuplicateAddressGroups checks that Service.Spec.AddressGroups contains no duplicate AddressGroups
func (v *ServiceValidator) ValidateNoDuplicateAddressGroups(addressGroups []models.AddressGroupRef) error {
	seen := make(map[string]bool)

	for _, ag := range addressGroups {
		// Create unique key: namespace/name
		key := fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)

		if seen[key] {
			return fmt.Errorf("duplicate AddressGroup in spec.addressGroups: %s", key)
		}
		seen[key] = true
	}

	return nil
}

// ValidateNoAddressGroupConflictsWithBindings checks that AddressGroups from Service.spec.addressGroups
// do NOT conflict with AddressGroups already bound via AddressGroupBindings
func (v *ServiceValidator) ValidateNoAddressGroupConflictsWithBindings(ctx context.Context, service models.Service) error {
	// Build map of AG from spec for quick lookup
	specAGs := make(map[string]bool)
	for _, ag := range service.AddressGroups {
		key := fmt.Sprintf("%s/%s", ag.Namespace, ag.Name)
		specAGs[key] = true
	}

	if len(specAGs) == 0 {
		return nil
	}

	// Find all AddressGroupBindings for this Service
	var conflictingAGs []string
	err := v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		// Only check bindings for this service
		if binding.ServiceRefKey() != service.Key() {
			return nil
		}

		// Check if this binding's AG is also in spec
		agKey := fmt.Sprintf("%s/%s", binding.AddressGroupRef.Namespace, binding.AddressGroupRef.Name)
		if specAGs[agKey] {
			conflictingAGs = append(conflictingAGs, agKey)
		}

		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to list address group bindings for conflict check")
	}

	if len(conflictingAGs) > 0 {
		return fmt.Errorf("AddressGroup conflict: the following AddressGroups are already bound via AddressGroupBinding and cannot be added to spec.addressGroups: %v", conflictingAGs)
	}

	return nil
}

// ValidateNoDuplicatePorts проверяет отсутствие дубликатов или перекрытий портов в сервисе
func (v *ServiceValidator) ValidateNoDuplicatePorts(ingressPorts []models.IngressPort) error {
	// Создаем слайсы для хранения диапазонов портов по протоколам
	tcpRanges := []models.PortRange{}
	udpRanges := []models.PortRange{}

	for _, port := range ingressPorts {
		// Парсим строку порта в несколько PortRange
		portRanges, err := ParsePortRanges(port.Port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port.Port, err)
		}

		// Проверяем на перекрытия внутри текущего набора портов
		for i, range1 := range portRanges {
			for j, range2 := range portRanges {
				if i != j && DoPortRangesOverlap(range1, range2) {
					return fmt.Errorf("port conflict detected within port specification: %s port %s has overlapping ranges %d-%d and %d-%d",
						port.Protocol, port.Port, range1.Start, range1.End, range2.Start, range2.End)
				}
			}
		}

		// Проверяем на перекрытия с существующими портами того же протокола
		var existingRanges []models.PortRange
		if port.Protocol == models.TCP {
			existingRanges = tcpRanges
		} else if port.Protocol == models.UDP {
			existingRanges = udpRanges
		}

		for _, newRange := range portRanges {
			for _, existingRange := range existingRanges {
				if DoPortRangesOverlap(newRange, existingRange) {
					return fmt.Errorf("port conflict detected: %s port range %d-%d overlaps with existing port range %d-%d",
						port.Protocol, newRange.Start, newRange.End, existingRange.Start, existingRange.End)
				}
			}
		}

		// Добавляем эти диапазоны портов в соответствующий слайс
		if port.Protocol == models.TCP {
			tcpRanges = append(tcpRanges, portRanges...)
		} else if port.Protocol == models.UDP {
			udpRanges = append(udpRanges, portRanges...)
		}
	}

	return nil
}

func (v *ServiceValidator) ValidateWithoutDuplicateCheck(ctx context.Context, service models.Service) error {
	if err := v.ValidateReferences(ctx, service); err != nil {
		return err
	}

	if err := v.ValidateNoDuplicatePorts(service.IngressPorts); err != nil {
		return err
	}

	if err := v.CheckPortOverlaps(ctx, service); err != nil {
		return err
	}

	return nil
}

// ValidateForPostCommit is deprecated, use ValidateWithoutDuplicateCheck instead
// Kept for backward compatibility
func (v *ServiceValidator) ValidateForPostCommit(ctx context.Context, service models.Service) error {
	return v.ValidateWithoutDuplicateCheck(ctx, service)
}

// CheckPortOverlaps проверяет перекрытие портов между сервисом и существующими сервисами в AddressGroup
func (v *ServiceValidator) CheckPortOverlaps(ctx context.Context, service models.Service) error {
	// Для каждой AddressGroup, к которой привязан сервис
	for _, agRef := range service.AddressGroups {
		// Получаем AddressGroupPortMapping
		portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, models.ResourceIdentifier{Name: agRef.Name, Namespace: agRef.Namespace})
		if err != nil {
			// Если портмаппинг не найден, пропускаем проверку для этой AddressGroup
			continue
		}

		// Проверяем, что портмаппинг не nil и AccessPorts не nil
		if portMapping == nil || portMapping.AccessPorts == nil {
			continue
		}

		// Создаем карту портов сервиса по протоколам
		servicePorts := make(map[models.TransportProtocol][]models.PortRange)
		for _, ingressPort := range service.IngressPorts {
			portRange, err := ParsePortRange(ingressPort.Port)
			if err != nil {
				return fmt.Errorf("invalid port in service %s: %w", service.Key(), err)
			}
			servicePorts[ingressPort.Protocol] = append(servicePorts[ingressPort.Protocol], portRange)
		}

		// Проверяем на перекрытия с существующими сервисами в портмаппинге
		for serviceRef, existingServicePorts := range portMapping.AccessPorts {
			// Пропускаем текущий сервис (для обновлений)
			if models.ServiceRefKey(serviceRef) == service.Key() {
				continue
			}

			// Проверяем, что existingServicePorts.Ports не nil
			if existingServicePorts.Ports == nil {
				continue
			}

			// Проверяем TCP порты
			for _, serviceRange := range servicePorts[models.TCP] {
				for _, tcpPortRange := range existingServicePorts.Ports[models.TCP] {
					if DoPortRangesOverlap(serviceRange, tcpPortRange) {
						return fmt.Errorf("TCP port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
							serviceRange.Start, serviceRange.End, service.Key(), tcpPortRange.Start, tcpPortRange.End, models.ServiceRefKey(serviceRef))
					}
				}
			}

			// Проверяем UDP порты
			for _, serviceRange := range servicePorts[models.UDP] {
				for _, udpPortRange := range existingServicePorts.Ports[models.UDP] {
					if DoPortRangesOverlap(serviceRange, udpPortRange) {
						return fmt.Errorf("UDP port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
							serviceRange.Start, serviceRange.End, service.Key(), udpPortRange.Start, udpPortRange.End, models.ServiceRefKey(serviceRef))
					}
				}
			}
		}
	}

	return nil
}

func (v *ServiceValidator) ValidateForCreation(ctx context.Context, service models.Service) error {
	keyExtractor := func(entity interface{}) string {
		if svc, ok := entity.(*models.Service); ok {
			return svc.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, service.ResourceIdentifier, keyExtractor); err != nil {
		return err
	}

	if err := v.ValidateNoDuplicateAddressGroups(service.AddressGroups); err != nil {
		return err
	}

	if err := v.ValidateNoAddressGroupConflictsWithBindings(ctx, service); err != nil {
		return err
	}

	if err := v.ValidateReferences(ctx, service); err != nil {
		return err
	}

	if err := v.ValidateNoDuplicatePorts(service.IngressPorts); err != nil {
		return err
	}

	if err := v.CheckPortOverlaps(ctx, service); err != nil {
		return err
	}

	return nil
}

// CheckBindingsPortOverlaps проверяет перекрытие портов во всех AddressGroupPortMappings,
// которые ссылаются на сервис через AddressGroupBindings
func (v *ServiceValidator) CheckBindingsPortOverlaps(ctx context.Context, service models.Service) error {
	// Находим все AddressGroupBindings, которые ссылаются на этот сервис
	var addressGroupIDs []models.ResourceIdentifier
	err := v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.ServiceRefKey() == service.Key() {
			// Create ResourceIdentifier from NamespacedObjectReference
			agID := models.NewResourceIdentifier(binding.AddressGroupRef.Name, models.WithNamespace(binding.AddressGroupRef.Namespace))
			addressGroupIDs = append(addressGroupIDs, agID)
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to list address group bindings")
	}

	// Проверяем перекрытие портов в каждом AddressGroupPortMapping
	for _, agID := range addressGroupIDs {
		portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, agID)
		if err != nil || portMapping == nil {
			// Если портмаппинг не найден или nil, пропускаем проверку для этой AddressGroup
			continue
		}

		// Создаем временную копию портмаппинга для проверки
		tempMapping := *portMapping

		// Удаляем текущий сервис из временной копии
		//if tempMapping.AccessPorts != nil {
		//	delete(tempMapping.AccessPorts, models.ServiceRef{ResourceIdentifier: service.ResourceIdentifier})
		//}

		// Проверяем перекрытие портов
		if err := CheckPortOverlaps(service, tempMapping); err != nil {
			return err
		}
	}

	return nil
}

// ValidateForUpdate валидирует сервис перед обновлением
func (v *ServiceValidator) ValidateForUpdate(ctx context.Context, oldService, newService models.Service) error {
	// Validate no duplicate AddressGroups in updated spec
	if err := v.ValidateNoDuplicateAddressGroups(newService.AddressGroups); err != nil {
		return err
	}

	// Validate no conflicts between spec.addressGroups and existing bindings
	if err := v.ValidateNoAddressGroupConflictsWithBindings(ctx, newService); err != nil {
		return err
	}

	// Проверяем ссылки
	if err := v.ValidateReferences(ctx, newService); err != nil {
		return err
	}

	// Проверяем на дубликаты портов внутри сервиса
	if err := v.ValidateNoDuplicatePorts(newService.IngressPorts); err != nil {
		return err
	}

	// Проверяем, изменились ли порты или AddressGroups
	portsChanged := !reflect.DeepEqual(oldService.IngressPorts, newService.IngressPorts)
	addressGroupsChanged := !reflect.DeepEqual(oldService.AddressGroups, newService.AddressGroups)

	if portsChanged || addressGroupsChanged {
		// Проверяем перекрытие портов в AddressGroups, к которым привязан сервис
		if err := v.CheckPortOverlaps(ctx, newService); err != nil {
			return err
		}

		// Дополнительно проверяем все AddressGroupBindings, которые ссылаются на этот сервис
		if portsChanged {
			if err := v.CheckBindingsPortOverlaps(ctx, newService); err != nil {
				return err
			}
		}
	}

	return nil
}

func (v *ServiceValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	hasAliases := false
	err := v.reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRefKey() == id.Key() {
			hasAliases = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check service aliases")
	}

	if hasAliases {
		return NewDependencyExistsError("service", id.Key(), "service_alias")
	}

	service, err := v.reader.GetServiceByID(ctx, id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil
		}
		return errors.Wrap(err, "failed to get service for dependency check")
	}

	if len(service.AggregatedAddressGroups) > 0 {
		return NewDependencyExistsError("service", id.Key(), "address_groups")
	}

	return nil
}
