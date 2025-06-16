package validation

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group exists
func (v *AddressGroupValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	exists := false
	err := v.reader.ListAddressGroups(ctx, func(group models.AddressGroup) error {
		if group.Key() == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	if err != nil {
		return errors.Wrap(err, "failed to check address group existence")
	}

	if !exists {
		return NewEntityNotFoundError("address_group", id.Key())
	}

	return nil
}

// ValidateReferences checks if all references in an address group are valid
// Note: This is currently a no-op as we need to avoid circular dependencies
func (v *AddressGroupValidator) ValidateReferences(ctx context.Context, group models.AddressGroup) error {
	// TODO: Implement validation of service references once circular dependency is resolved
	return nil
}

// ValidateForCreation validates an address group before creation
func (v *AddressGroupValidator) ValidateForCreation(ctx context.Context, group models.AddressGroup) error {
	return v.ValidateReferences(ctx, group)
}

// CheckDependencies checks if there are dependencies before deleting an address group
func (v *AddressGroupValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check Services referencing the address group to be deleted
	hasServices := false
	err := v.reader.ListServices(ctx, func(service models.Service) error {
		for _, agRef := range service.AddressGroups {
			if agRef.Key() == id.Key() {
				hasServices = true
				break
			}
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check services")
	}

	if hasServices {
		return NewDependencyExistsError("address_group", id.Key(), "service")
	}

	// Check AddressGroupBindings referencing the address group to be deleted
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.AddressGroupRef.Key() == id.Key() {
			hasBindings = true
		}
		return nil
	}, nil)

	if err != nil {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	if hasBindings {
		return NewDependencyExistsError("address_group", id.Key(), "address_group_binding")
	}

	// For now, we're skipping the check for AddressGroupPortMappings as the model structure is unclear
	// TODO: Implement this check once we understand how AddressGroupPortMapping is associated with AddressGroup

	return nil
}
