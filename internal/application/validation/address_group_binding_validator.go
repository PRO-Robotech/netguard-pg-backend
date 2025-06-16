package validation

import (
	"context"
	"fmt"

	"netguard-pg-backend/internal/domain/models"

	"github.com/pkg/errors"
)

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

	return nil
}

// ValidateForCreation validates an address group binding before creation
func (v *AddressGroupBindingValidator) ValidateForCreation(ctx context.Context, binding models.AddressGroupBinding) error {
	return v.ValidateReferences(ctx, binding)
}

// ValidateForUpdate validates an address group binding before update
func (v *AddressGroupBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding, newBinding models.AddressGroupBinding) error {
	// Validate references
	if err := v.ValidateReferences(ctx, newBinding); err != nil {
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

	return nil
}
