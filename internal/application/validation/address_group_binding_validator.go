package validation

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// ValidateExists checks if an address group binding exists
func (v *AddressGroupBindingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	exists := false
	err := v.reader.ListAddressGroupBindings(ctx, func(binding models.AddressGroupBinding) error {
		if binding.Key() == id.Key() {
			exists = true
		}
		return nil
	}, ports.NewResourceIdentifierScope(id))

	if err != nil {
		return errors.Wrap(err, "failed to check address group binding existence")
	}

	if !exists {
		return NewEntityNotFoundError("address_group_binding", id.Key())
	}

	return nil
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

	return nil
}

// ValidateForCreation validates an address group binding before creation
func (v *AddressGroupBindingValidator) ValidateForCreation(ctx context.Context, binding models.AddressGroupBinding) error {
	return v.ValidateReferences(ctx, binding)
}
