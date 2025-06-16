package validation

import (
	"context"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group port mapping exists
func (v *AddressGroupPortMappingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(models.AddressGroupPortMapping).Key()
	})
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

// ValidateForUpdate validates an address group port mapping before update
func (v *AddressGroupPortMappingValidator) ValidateForUpdate(ctx context.Context, oldMapping, newMapping models.AddressGroupPortMapping) error {
	// For address group port mappings, the validation for update is the same as for creation
	// We might add specific update validation rules in the future if needed
	return v.ValidateReferences(ctx, newMapping)
}
