package validation

import (
	"context"
	"fmt"
	"reflect"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if a service exists
func (v *ServiceValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(models.Service).Key()
	})
}

// ValidateReferences checks if all references in a service are valid
func (v *ServiceValidator) ValidateReferences(ctx context.Context, service models.Service) error {
	agValidator := NewAddressGroupValidator(v.reader)

	for _, agRef := range service.AddressGroups {
		if err := agValidator.ValidateExists(ctx, agRef.ResourceIdentifier); err != nil {
			return errors.Wrapf(err, "invalid address group reference in service %s", service.Key())
		}
	}

	return nil
}

// ValidateNoDuplicatePorts проверяет отсутствие дубликатов или перекрытий портов в сервисе
func (v *ServiceValidator) ValidateNoDuplicatePorts(ingressPorts []models.IngressPort) error {
	// Создаем слайсы для хранения диапазонов портов по протоколам
	tcpRanges := []models.PortRange{}
	udpRanges := []models.PortRange{}

	for _, port := range ingressPorts {
		// Парсим строку порта в PortRange
		portRange, err := ParsePortRange(port.Port)
		if err != nil {
			return fmt.Errorf("invalid port %s: %w", port.Port, err)
		}

		// Проверяем на перекрытия с существующими портами того же протокола
		var existingRanges []models.PortRange
		if port.Protocol == models.TCP {
			existingRanges = tcpRanges
		} else if port.Protocol == models.UDP {
			existingRanges = udpRanges
		}

		for _, existingRange := range existingRanges {
			if DoPortRangesOverlap(portRange, existingRange) {
				return fmt.Errorf("port conflict detected: %s port range %s overlaps with existing port range %d-%d",
					port.Protocol, port.Port, existingRange.Start, existingRange.End)
			}
		}

		// Добавляем этот диапазон портов в соответствующий слайс
		if port.Protocol == models.TCP {
			tcpRanges = append(tcpRanges, portRange)
		} else if port.Protocol == models.UDP {
			udpRanges = append(udpRanges, portRange)
		}
	}

	return nil
}

// CheckPortOverlaps проверяет перекрытие портов между сервисом и существующими сервисами в AddressGroup
func (v *ServiceValidator) CheckPortOverlaps(ctx context.Context, service models.Service) error {
	// Для каждой AddressGroup, к которой привязан сервис
	for _, agRef := range service.AddressGroups {
		// Получаем AddressGroupPortMapping
		portMapping, err := v.reader.GetAddressGroupPortMappingByID(ctx, agRef.ResourceIdentifier)
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
			if serviceRef.Key() == service.Key() {
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
							serviceRange.Start, serviceRange.End, service.Key(), tcpPortRange.Start, tcpPortRange.End, serviceRef.Key())
					}
				}
			}

			// Проверяем UDP порты
			for _, serviceRange := range servicePorts[models.UDP] {
				for _, udpPortRange := range existingServicePorts.Ports[models.UDP] {
					if DoPortRangesOverlap(serviceRange, udpPortRange) {
						return fmt.Errorf("UDP port range %d-%d in service %s overlaps with existing port range %d-%d in service %s",
							serviceRange.Start, serviceRange.End, service.Key(), udpPortRange.Start, udpPortRange.End, serviceRef.Key())
					}
				}
			}
		}
	}

	return nil
}

// ValidateForCreation validates a service before creation
func (v *ServiceValidator) ValidateForCreation(ctx context.Context, service models.Service) error {
	// Проверяем ссылки
	if err := v.ValidateReferences(ctx, service); err != nil {
		return err
	}

	// Проверяем на дубликаты портов внутри сервиса
	if err := v.ValidateNoDuplicatePorts(service.IngressPorts); err != nil {
		return err
	}

	// Проверяем на перекрытие портов с другими сервисами
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
		if binding.ServiceRef.Key() == service.Key() {
			addressGroupIDs = append(addressGroupIDs, binding.AddressGroupRef.ResourceIdentifier)
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

// CheckDependencies checks if there are dependencies before deleting a service
func (v *ServiceValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check ServiceAliases referencing the service to be deleted
	hasAliases := false
	err := v.reader.ListServiceAliases(ctx, func(alias models.ServiceAlias) error {
		if alias.ServiceRef.Key() == id.Key() {
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

	// Check AddressGroupBindings referencing the service to be deleted
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.ServiceRef.Key() == id.Key() {
			hasBindings = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	if hasBindings {
		return NewDependencyExistsError("service", id.Key(), "address_group_binding")
	}

	return nil
}
