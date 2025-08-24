package validation

import (
	"context"
	"fmt"
	"net"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group exists
func (v *AddressGroupValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.AddressGroup).Key()
	})
}

// ValidateReferences checks if all references in an address group are valid
func (v *AddressGroupValidator) ValidateReferences(ctx context.Context, group models.AddressGroup) error {
	// AddressGroup doesn't have references to other resources
	// Networks field contains only CIDR addresses, not resource references
	return nil
}

// ValidateForCreation validates an address group before creation
func (v *AddressGroupValidator) ValidateForCreation(ctx context.Context, group models.AddressGroup) error {
	if err := v.validateNetworks(group.Networks); err != nil {
		return err
	}
	return v.ValidateReferences(ctx, group)
}

// ValidateForUpdate validates an address group before update
func (v *AddressGroupValidator) ValidateForUpdate(ctx context.Context, oldGroup, newGroup models.AddressGroup) error {
	if err := v.validateNetworks(newGroup.Networks); err != nil {
		return err
	}
	// For address groups, the validation for update is the same as for creation
	// We might add specific update validation rules in the future if needed
	return v.ValidateReferences(ctx, newGroup)
}

// validateNetworks validates the Networks field of an AddressGroup
func (v *AddressGroupValidator) validateNetworks(networks []models.NetworkItem) error {
	for i, network := range networks {
		// Validate required fields
		if network.Name == "" {
			return fmt.Errorf("network item %d: name is required", i)
		}

		if network.CIDR == "" {
			return fmt.Errorf("network item %d (%s): CIDR is required", i, network.Name)
		}

		// Validate CIDR format
		if _, _, err := net.ParseCIDR(network.CIDR); err != nil {
			return fmt.Errorf("network item %d (%s): invalid CIDR format '%s': %v", i, network.Name, network.CIDR, err)
		}

		// NetworkItem should not have Kind field for AddressGroup
		// Kind field is used in other contexts, not here
		if network.Kind != "" {
			return fmt.Errorf("network item %d (%s): kind field should not be used in AddressGroup networks", i, network.Name)
		}
	}

	return nil
}

// CheckDependencies checks if there are dependencies before deleting an address group
func (v *AddressGroupValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check Services referencing the address group to be deleted
	hasServices := false
	err := v.reader.ListServices(ctx, func(service models.Service) error {
		for _, agRef := range service.AddressGroups {
			if models.AddressGroupRefKey(agRef) == id.Key() {
				hasServices = true
				break
			}
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check services")
	}

	if hasServices {
		return NewDependencyExistsError("address_group", id.Key(), "service")
	}

	// Check AddressGroupBindings referencing the address group to be deleted
	hasBindings := false
	err = v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.AddressGroupRefKey() == id.Key() {
			hasBindings = true
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check address group bindings")
	}

	if hasBindings {
		return NewDependencyExistsError("address_group", id.Key(), "address_group_binding")
	}

	return nil
}
