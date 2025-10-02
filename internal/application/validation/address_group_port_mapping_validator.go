package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group port mapping exists
func (v *AddressGroupPortMappingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.AddressGroupPortMapping).Key() // Используем указатель вместо значения
	})
}

// ValidateReferences checks if all references in an address group port mapping are valid
func (v *AddressGroupPortMappingValidator) ValidateReferences(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	serviceValidator := NewServiceValidator(v.reader)

	// Validate service references in the AccessPorts map
	for serviceRef := range mapping.AccessPorts {
		if err := serviceValidator.ValidateExists(ctx, models.ResourceIdentifier{Name: serviceRef.Name, Namespace: serviceRef.Namespace}); err != nil {
			return errors.Wrapf(err, "invalid service reference in address group port mapping %s", mapping.Key())
		}
	}

	// Note: We're not validating any AddressGroup reference because it's not clear from the model
	// how AddressGroupPortMapping is associated with an AddressGroup

	return nil
}

// CheckInternalPortOverlaps проверяет перекрытия портов между сервисами в маппинге портов
func (v *AddressGroupPortMappingValidator) CheckInternalPortOverlaps(mapping models.AddressGroupPortMapping) error {
	// Создаем карты для хранения диапазонов портов по протоколу
	tcpRanges := make(map[string][]models.PortRange)
	udpRanges := make(map[string][]models.PortRange)

	// Проверяем каждый сервис в маппинге
	for serviceRef, servicePorts := range mapping.AccessPorts {
		serviceName := models.ServiceRefKey(serviceRef)

		// Проверяем TCP порты
		for _, portRange := range servicePorts.Ports[models.TCP] {
			// Проверяем перекрытия с TCP портами других сервисов
			for otherService, ranges := range tcpRanges {
				if otherService == serviceName {
					continue // Пропускаем проверку с тем же сервисом
				}

				for _, existingRange := range ranges {
					if DoPortRangesOverlap(portRange, existingRange) {
						return fmt.Errorf("TCP port range %d-%d for service %s overlaps with existing port range %d-%d for service %s",
							portRange.Start, portRange.End, serviceName, existingRange.Start, existingRange.End, otherService)
					}
				}
			}

			// Добавляем диапазон портов в карту
			tcpRanges[serviceName] = append(tcpRanges[serviceName], portRange)
		}

		// Проверяем UDP порты
		for _, portRange := range servicePorts.Ports[models.UDP] {
			// Проверяем перекрытия с UDP портами других сервисов
			for otherService, ranges := range udpRanges {
				if otherService == serviceName {
					continue // Пропускаем проверку с тем же сервисом
				}

				for _, existingRange := range ranges {
					if DoPortRangesOverlap(portRange, existingRange) {
						return fmt.Errorf("UDP port range %d-%d for service %s overlaps with existing port range %d-%d for service %s",
							portRange.Start, portRange.End, serviceName, existingRange.Start, existingRange.End, otherService)
					}
				}
			}

			// Добавляем диапазон портов в карту
			udpRanges[serviceName] = append(udpRanges[serviceName], portRange)
		}
	}

	return nil
}

// ValidateForCreation validates an address group port mapping before creation
func (v *AddressGroupPortMappingValidator) ValidateForCreation(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	// PHASE 1: Check for duplicate entity (CRITICAL FIX for overwrite issue)
	// This prevents creation of entities with the same namespace/name combination
	keyExtractor := func(entity interface{}) string {
		if agpm, ok := entity.(*models.AddressGroupPortMapping); ok {
			return agpm.Key()
		}
		return ""
	}

	if err := v.BaseValidator.ValidateEntityDoesNotExistForCreation(ctx, mapping.ResourceIdentifier, keyExtractor); err != nil {
		return err // Return the detailed EntityAlreadyExistsError with logging and context
	}

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, mapping); err != nil {
		return err
	}

	// PHASE 3: Check internal port overlaps (existing validation)
	if err := v.CheckInternalPortOverlaps(mapping); err != nil {
		return err
	}

	return nil
}

// ValidateForPostCommit validates an address group port mapping after it has been committed to database
// This skips duplicate checking since the entity already exists in the database
func (v *AddressGroupPortMappingValidator) ValidateForPostCommit(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	// PHASE 1: Skip duplicate entity check (entity is already committed)
	// This method is called AFTER the entity is saved to database, so existence is expected

	// PHASE 2: Validate references (existing validation)
	if err := v.ValidateReferences(ctx, mapping); err != nil {
		return err
	}

	// PHASE 3: Check internal port overlaps (existing validation)
	if err := v.CheckInternalPortOverlaps(mapping); err != nil {
		return err
	}

	return nil
}

// ValidateForUpdate validates an address group port mapping before update
func (v *AddressGroupPortMappingValidator) ValidateForUpdate(ctx context.Context, oldMapping, newMapping models.AddressGroupPortMapping) error {
	// Проверяем ссылки
	if err := v.ValidateReferences(ctx, newMapping); err != nil {
		return err
	}

	// Проверяем внутренние перекрытия портов
	if err := v.CheckInternalPortOverlaps(newMapping); err != nil {
		return err
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an address group port mapping
func (v *AddressGroupPortMappingValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// AddressGroupPortMapping is a derived resource created by AddressGroupBinding
	// It can be safely deleted as nothing should depend on it directly

	// Log the dependency check for consistency with other validators
	return nil
}
