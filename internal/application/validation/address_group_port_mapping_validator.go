package validation

import (
	"context"
	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"
	
	"github.com/pkg/errors"
)

// ValidateExists checks if an address group port mapping exists
func (v *AddressGroupPortMappingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	exists := false
	err := v.reader.ListAddressGroupPortMappings(ctx, func(mapping models.AddressGroupPortMapping) error {
		if mapping.Key() == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))
	
	if err != nil {
		return errors.Wrap(err, "failed to check address group port mapping existence")
	}
	
	if !exists {
		return NewEntityNotFoundError("address_group_port_mapping", id.Key())
	}
	
	return nil
}

// ValidateReferences checks if all references in an address group port mapping are valid
func (v *AddressGroupPortMappingValidator) ValidateReferences(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	serviceValidator := NewServiceValidator(v.reader)
	
	// Validate service references in the AccessPorts map
	for serviceRef := range mapping.AccessPorts {
		if err := serviceValidator.ValidateExists(ctx, serviceRef.ResourceIdentifier); err != nil {
			return errors.Wrapf(err, "invalid service reference in address group port mapping %s", mapping.Key())
		}
	}
	
	// Note: We're not validating any AddressGroup reference because it's not clear from the model
	// how AddressGroupPortMapping is associated with an AddressGroup
	
	return nil
}

// ValidateForCreation validates an address group port mapping before creation
func (v *AddressGroupPortMappingValidator) ValidateForCreation(ctx context.Context, mapping models.AddressGroupPortMapping) error {
	return v.ValidateReferences(ctx, mapping)
}