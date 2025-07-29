package validation

import (
	"context"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// NetworkBindingValidator validates NetworkBinding resources
type NetworkBindingValidator struct {
	*BaseValidator
}

// NewNetworkBindingValidator creates a new network binding validator
func NewNetworkBindingValidator(reader ports.Reader) *NetworkBindingValidator {
	return &NetworkBindingValidator{
		BaseValidator: NewBaseValidator(reader, "NetworkBinding", func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
			return reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
				return consume(&binding)
			}, scope)
		}),
	}
}

// ValidateExists checks if a network binding exists
func (v *NetworkBindingValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.NetworkBinding).Key()
	})
}

// ValidateReferences validates that referenced Network and AddressGroup exist
func (v *NetworkBindingValidator) ValidateReferences(ctx context.Context, binding models.NetworkBinding) error {
	// Validate Network reference
	networkID := models.ResourceIdentifier{Name: binding.NetworkRef.Name, Namespace: binding.Namespace}
	network, err := v.reader.GetNetworkByID(ctx, networkID)
	if err != nil {
		return errors.Wrapf(err, "invalid network reference in binding %s", binding.Key())
	}
	if network == nil {
		return errors.Errorf("network %s not found", networkID.Key())
	}

	// Validate AddressGroup reference
	addressGroupID := models.ResourceIdentifier{Name: binding.AddressGroupRef.Name, Namespace: binding.Namespace}
	addressGroup, err := v.reader.GetAddressGroupByID(ctx, addressGroupID)
	if err != nil {
		return errors.Wrapf(err, "invalid address group reference in binding %s", binding.Key())
	}
	if addressGroup == nil {
		return errors.Errorf("address group %s not found", addressGroupID.Key())
	}

	return nil
}

// ValidateForCreation validates a network binding for creation
func (v *NetworkBindingValidator) ValidateForCreation(ctx context.Context, binding models.NetworkBinding) error {
	// Validate references
	if err := v.ValidateReferences(ctx, binding); err != nil {
		return err
	}

	// No additional validation needed for creation
	return nil
}

// ValidateForUpdate validates a network binding for update
func (v *NetworkBindingValidator) ValidateForUpdate(ctx context.Context, oldBinding, newBinding models.NetworkBinding) error {
	// Validate references
	if err := v.ValidateReferences(ctx, newBinding); err != nil {
		return err
	}

	// Check if binding exists
	if err := v.ValidateExists(ctx, models.ResourceIdentifier{Name: oldBinding.Name, Namespace: oldBinding.Namespace}); err != nil {
		return err
	}

	// Check if name or namespace changed (should not be allowed)
	if oldBinding.Name != newBinding.Name || oldBinding.Namespace != newBinding.Namespace {
		return errors.New("network binding name and namespace cannot be changed")
	}

	// If network reference changed, validate that the new network is not already bound
	if oldBinding.NetworkRef.Name != newBinding.NetworkRef.Name {
		networkID := models.ResourceIdentifier{Name: newBinding.NetworkRef.Name, Namespace: newBinding.Namespace}
		network, err := v.reader.GetNetworkByID(ctx, networkID)
		if err != nil {
			return errors.Wrap(err, "failed to get network")
		}

		if network != nil && network.IsBound {
			return errors.Errorf("network %s is already bound to another address group", networkID.Key())
		}
	}

	return nil
}

// CheckDependencies checks if the network binding can be deleted
func (v *NetworkBindingValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Network bindings don't have dependencies that would prevent deletion
	// The cleanup of related resources (like updating Network.IsBound status)
	// should be handled by the application service
	return nil
}
