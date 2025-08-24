package validation

import (
	"context"
	"fmt"
	"net"

	"netguard-pg-backend/internal/domain/models"
	"netguard-pg-backend/internal/domain/ports"

	"github.com/pkg/errors"
)

// NetworkValidator validates Network resources
type NetworkValidator struct {
	*BaseValidator
	reader ports.Reader
}

// NewNetworkValidator creates a new network validator
func NewNetworkValidator(reader ports.Reader) *NetworkValidator {
	return &NetworkValidator{
		BaseValidator: NewBaseValidator(reader, "Network", func(ctx context.Context, consume func(entity interface{}) error, scope ports.Scope) error {
			return reader.ListNetworks(ctx, func(network models.Network) error {
				return consume(&network)
			}, scope)
		}),
		reader: reader,
	}
}

// ValidateExists checks if a network exists
func (v *NetworkValidator) ValidateExists(ctx context.Context, id models.ResourceIdentifier) error {
	return v.BaseValidator.ValidateExists(ctx, id, func(entity interface{}) string {
		return entity.(*models.Network).Key()
	})
}

// ValidateCIDR validates the CIDR format
func (v *NetworkValidator) ValidateCIDR(cidr string) error {
	if cidr == "" {
		return errors.New("CIDR cannot be empty")
	}

	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return errors.Wrapf(err, "invalid CIDR format: %s", cidr)
	}

	return nil
}

// ValidateForCreation validates a network for creation
func (v *NetworkValidator) ValidateForCreation(ctx context.Context, network models.Network) error {
	// Validate CIDR
	if err := v.ValidateCIDR(network.CIDR); err != nil {
		return err
	}

	// Check for duplicate network
	existingNetwork, err := v.reader.GetNetworkByID(ctx, network.ResourceIdentifier)
	if err == nil && existingNetwork != nil {
		return NewValidationError(fmt.Sprintf("Network %s already exists", network.Key()))
	}
	if err != nil && err != ports.ErrNotFound {
		return errors.Wrap(err, "failed to check for existing network")
	}

	return nil
}

// ValidateForUpdate validates a network for update
func (v *NetworkValidator) ValidateForUpdate(ctx context.Context, oldNetwork, newNetwork models.Network) error {
	// Validate CIDR
	if err := v.ValidateCIDR(newNetwork.CIDR); err != nil {
		return err
	}

	// Check if network exists
	if err := v.ValidateExists(ctx, models.ResourceIdentifier{Name: oldNetwork.Name, Namespace: oldNetwork.Namespace}); err != nil {
		return err
	}

	// Check if name or namespace changed (should not be allowed)
	if oldNetwork.Name != newNetwork.Name || oldNetwork.Namespace != newNetwork.Namespace {
		return errors.New("network name and namespace cannot be changed")
	}

	return nil
}

// CheckDependencies checks if the network can be deleted
func (v *NetworkValidator) CheckDependencies(ctx context.Context, id models.ResourceIdentifier) error {
	// Check if there are any NetworkBindings that reference this network
	hasBindings := false
	err := v.reader.ListNetworkBindings(ctx, func(binding models.NetworkBinding) error {
		if binding.NetworkRef.Name == id.Name && binding.Namespace == id.Namespace {
			hasBindings = true
		}
		return nil
	}, ports.EmptyScope{})

	if err != nil {
		return errors.Wrap(err, "failed to check network bindings")
	}

	if hasBindings {
		return errors.Errorf("cannot delete network %s: it is referenced by network bindings", id.Key())
	}

	return nil
}
